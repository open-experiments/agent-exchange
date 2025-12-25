# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /build

# Copy internal modules first
COPY internal/events internal/events
COPY internal/httpclient internal/httpclient
COPY internal/testutil internal/testutil

# Copy service files
COPY aex-bid-evaluator aex-bid-evaluator

# Build the binary
WORKDIR /build/aex-bid-evaluator
RUN go mod download && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o aex-bid-evaluator ./src

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/aex-bid-evaluator/aex-bid-evaluator .

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser && \
    chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

CMD ["./aex-bid-evaluator"]

