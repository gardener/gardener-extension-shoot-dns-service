############# builder
FROM golang:1.27.4@sha256:5f1b98fa02fe396ebce2a0227a0e08309e11792915b9dd2e1f71053d7e7e6666 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-dns-service

# Copy go mod and sum files
COPY go.mod go.sum ./
# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

COPY . .

ARG EFFECTIVE_VERSION
RUN make install EFFECTIVE_VERSION=$EFFECTIVE_VERSION

############# base
FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5 AS base

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
