FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /binpacking-exporter .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /binpacking-exporter /binpacking-exporter

ENTRYPOINT ["/binpacking-exporter"]
