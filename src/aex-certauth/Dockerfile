# Build stage
FROM golang:1.26.7-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /build

# Copy internal modules first
COPY internal/events internal/events
COPY internal/httpclient internal/httpclient
COPY internal/nats internal/nats
COPY internal/telemetry internal/telemetry

# Copy service files
COPY aex-certauth aex-certauth

# Build the binary
WORKDIR /build/aex-certauth
RUN go mod download && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o aex-certauth ./src

# Runtime stage
FROM alpine:latest

RUN apk --no-cache upgrade && apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/aex-certauth/aex-certauth .

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser && \
    chown -R appuser:appuser /app

USER appuser

EXPOSE 8091

CMD ["./aex-certauth"]
