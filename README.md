# mDNS NAT64 reflector

Make IPv4-only mDNS services available to IPv6-only devices, by mapping IPv4
addresses through a [NAT64 gateway](https://datatracker.ietf.org/doc/html/rfc6146)

This works by listening for IPv6 mDNS requests, then making an IPv4 sub-request
on the client's behalf, mapping the IPv4 address to a AAAA recording using the
NAT64 prefix, and replying with the modified response now containing the
synthesised AAAA record.

Currently this assumes the well-known NAT64 prefix of 64:ff9b::/96.

Buyer beware- this is riddled with bugs and barely just about works enough for
my use-case, which is a couple of older Google Chromecast devices.

## Running

Simply invoke the binary with the interface name being the only argument.

```sh
./mdns64 eth0
```

The log-level can also be overridden from the default of `info`:

```sh
./mdns64 -l debug eth0
```

It can also be run from a container

```sh
docker run -d --rm --net=host registry.spritsail.io/frebib/mdns64 eth0
```
Image mirrors are also available at ghcr.io/frebib/mdns64 and
docker.io/frebib/mdns64
