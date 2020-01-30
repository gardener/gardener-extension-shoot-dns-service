############# builder
FROM golang:1.13.4 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-dns-service
COPY . .
RUN make install-requirements && make VERIFY=true all

############# gardener-extension-shoot-dns-service
FROM alpine:3.11.3 AS gardener-extension-shoot-dns-service

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-shoot-dns-service /gardener-extension-shoot-dns-service
ENTRYPOINT ["/gardener-extension-shoot-dns-service"]
