FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

WORKDIR /build

# Install make and git for Makefile
RUN apk add --no-cache make git

# Copy dependencies and Makefile
COPY go.mod go.sum Makefile ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build using Makefile (centralized ldflags)
RUN CGO_ENABLED=0 make build \
    VERSION=${VERSION} \
    COMMIT=${COMMIT} \
    DATE=${DATE} && \
    mv kube-cluster-binpacking-exporter /binpacking-exporter

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /binpacking-exporter /binpacking-exporter

ENTRYPOINT ["/binpacking-exporter"]
