############# builder
FROM golang:1.15.3 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-dns-service
COPY . .
RUN make install

############# gardener-extension-shoot-dns-service
FROM alpine:3.11.6 AS gardener-extension-shoot-dns-service

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-shoot-dns-service /gardener-extension-shoot-dns-service
ENTRYPOINT ["/gardener-extension-shoot-dns-service"]
