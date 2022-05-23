############# builder
FROM golang:1.18.2 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-dns-service
COPY . .
RUN make install

############# base
FROM alpine:3.15.4 AS base

############# gardener-extension-shoot-dns-service
FROM base AS gardener-extension-shoot-dns-service

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-shoot-dns-service /gardener-extension-shoot-dns-service
ENTRYPOINT ["/gardener-extension-shoot-dns-service"]

############# gardener-extension-admission-shoot-dns-service
FROM base AS gardener-extension-admission-shoot-dns-service

COPY --from=builder /go/bin/gardener-extension-admission-shoot-dns-service /gardener-extension-admission-shoot-dns-service
ENTRYPOINT ["/gardener-extension-admission-shoot-dns-service"]
