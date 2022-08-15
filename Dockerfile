############# builder
FROM golang:1.18.5 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-dns-service
COPY . .
RUN make install

############# base
FROM gcr.io/distroless/static-debian11:nonroot AS base

############# gardener-extension-shoot-dns-service
FROM base AS gardener-extension-shoot-dns-service
WORKDIR /

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-shoot-dns-service /gardener-extension-shoot-dns-service
ENTRYPOINT ["/gardener-extension-shoot-dns-service"]

############# gardener-extension-admission-shoot-dns-service
FROM base AS gardener-extension-admission-shoot-dns-service
WORKDIR /

COPY --from=builder /go/bin/gardener-extension-admission-shoot-dns-service /gardener-extension-admission-shoot-dns-service
ENTRYPOINT ["/gardener-extension-admission-shoot-dns-service"]
