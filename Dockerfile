FROM golang:alpine AS build

WORKDIR /tmp/build
COPY go.mod go.sum ./
RUN go mod download -x

ARG CGO_ENABLED=0
ADD . .
RUN go build -v -trimpath

# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

FROM registry.spritsail.io/spritsail/alpine:3.22

LABEL org.opencontainers.image.authors="Joe Groocock <mdns64@frebib.net>" \
      org.opencontainers.image.title="mDNS NAT64 repeator/reflector" \
      org.opencontainers.image.url="https://github.com/frebib/mdns64" \
      org.opencontainers.image.source="https://github.com/frebib/mdns64" \
      org.opencontainers.image.description="Repeat mDNS queries on IPv6 multicast for IPv4-only services"

COPY --from=build /tmp/build/mdns64 /mdns64

ENTRYPOINT ["/mdns64"]
