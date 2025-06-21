package main

import (
	"errors"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/miekg/dns"
)

var (
	// MulticastIPv4 represents the mDNS IPv4 multicast udp IP+port 224.0.0.251:5353
	MulticastIPv4 = net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	// MulticastIPv6 represents the mDNS IPv6 multicast udp IP+port [ff02:fb]:5353
	MulticastIPv6 = net.UDPAddr{IP: net.IP{0xff, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xfb}, Port: 5353}
)

func main() {
	loglevel := pflag.StringP("log-level", "l", "info", "log level; one of debug, info, warn or error")
	pflag.Parse()

	args := pflag.Args()
	if len(args) == 0 {
		slog.Error("Usage: mdns64 [-l,--log-level level] interface")
		os.Exit(1)
	}

	switch strings.ToLower(*loglevel) {
	case "debug":
		slog.SetLogLoggerLevel(slog.LevelDebug)
	case "info":
		slog.SetLogLoggerLevel(slog.LevelInfo)
	case "warn":
		slog.SetLogLoggerLevel(slog.LevelWarn)
	case "error":
		slog.SetLogLoggerLevel(slog.LevelError)
	default:
		panic("invalid log level " + *loglevel)
	}

	iface, err := net.InterfaceByName(args[0])
	if err != nil {
		panic(err)
	}

	mconn4, err := net.ListenMulticastUDP("udp4", iface, &MulticastIPv4)
	if err != nil {
		panic(err)
	}
	mconn6, err := net.ListenMulticastUDP("udp6", iface, &MulticastIPv6)
	if err != nil {
		panic(err)
	}

	slog.Info("Listening for mDNS queries on " + MulticastIPv6.String() + " interface " + iface.Name)
	for {
		var buf = make([]byte, 4096)
		count, src, err := mconn6.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			slog.With("src", src, "error", err).Error("failed to read packet")
			continue
		}
		go handle(mconn4, mconn6, src, buf[:count])
	}
}

func handle(mconn4 *net.UDPConn, mconn6 *net.UDPConn, src *net.UDPAddr, data []byte) {
	var req dns.Msg
	err := req.Unpack(data)
	if err != nil {
		slog.With("src", src, "error", err).Error("failed to parse dns message")
		return
	}

	if req.Response || len(req.Question) == 0 {
		// Not much point in relaying responses only (or maybe there is?)
		return
	}
	slog.With("question", req.Question, "from", src).Debug("received question")

	// Ask for a multicast response (QM) instead of unicast (QU)
	req.Question[0].Qclass |= 1 << 15

	packed, err := req.Pack()
	if err != nil {
		slog.With("error", err).Error("failed to pack request packet")
		return
	}
	err = mconn4.SetWriteDeadline(time.Now().Add(time.Second))
	if err != nil {
		slog.With("error", err).Error("failed to set write deadline")
		return
	}
	_, err = mconn4.WriteToUDP(packed, &MulticastIPv4)
	if err != nil {
		slog.With("error", err).Error("failed to send packet")
		return
	}

	var n int
	var buf = make([]byte, 4096)
	var resp dns.Msg
	var respip *net.UDPAddr
	for {
		err = mconn4.SetReadDeadline(time.Now().Add(time.Second * 3))
		if err != nil {
			slog.With("error", err).Error("failed to set read deadline")
			return
		}
		n, respip, err = mconn4.ReadFromUDP(buf)
		if err != nil {
			slog.With("error", err).Error("failed to read response packet")
			return
		}
		slog.With("src", respip, "req", req).Debug("received v4 response")

		err = resp.Unpack(buf[:n])
		if err != nil {
			slog.With("error", err).Error("failed to parse response packet")
			return
		}
		if resp.Response {
			break
		}
		slog.With("req", req).Warn("received a request when we wanted a response")
	}

	var arecords []*dns.A
	hasAAAA := false
	for _, answer := range append(resp.Answer, resp.Extra...) {
		switch rr := answer.(type) {
		case *dns.A:
			arecords = append(arecords, rr)
		case *dns.AAAA:
			hasAAAA = true
		}
	}
	// Nothing for us to do here
	if hasAAAA || len(arecords) == 0 {
		return
	}

	for _, arecord := range arecords {
		translated := net.IP{0, 0x64, 0xff, 0x9b, 0, 0, 0, 0, 0, 0, 0, 0, arecord.A[0], arecord.A[1],
			arecord.A[2], arecord.A[3]}
		hdr := arecord.Hdr
		hdr.Rrtype = dns.TypeAAAA
		aaaa := dns.AAAA{
			Hdr:  hdr,
			AAAA: translated,
		}
		slog.With("aaaa", aaaa, "to", src).Info("responding with synthesised nat64 address")
		resp.Extra = append(resp.Extra, &aaaa)
	}
	resp.Id = req.Id

	packed, err = resp.Pack()
	if err != nil {
		slog.With("error", err).Error("failed to pack response packet")
		return
	}

	_, err = mconn6.WriteToUDP(packed, &MulticastIPv6)
	if err != nil {
		slog.With("error", err, "to", src).Error("failed to write response")
		return
	}
}
