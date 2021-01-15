############# builder
FROM eu.gcr.io/gardener-project/3rd/golang:1.15.5 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-dns-service
COPY . .
RUN make install

############# gardener-extension-shoot-dns-service
FROM eu.gcr.io/gardener-project/3rd/alpine:3.12.3 AS gardener-extension-shoot-dns-service

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-shoot-dns-service /gardener-extension-shoot-dns-service
ENTRYPOINT ["/gardener-extension-shoot-dns-service"]
