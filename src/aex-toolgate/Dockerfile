# Build stage
FROM golang:1.26.7-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Internal modules the service depends on
COPY internal/events internal/events
COPY internal/nats internal/nats
COPY internal/toolgate internal/toolgate

# Service files
COPY aex-toolgate aex-toolgate

WORKDIR /build/aex-toolgate
RUN go mod download && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o aex-toolgate ./src

# Runtime stage
FROM alpine:latest

RUN apk --no-cache upgrade && apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /build/aex-toolgate/aex-toolgate .

# No policy is baked in on purpose. POLICY_FILE must point at the provider's
# own policy (a mounted ConfigMap or secret); the gate refuses to start
# without it rather than fall back to a fixture.

RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser && \
    chown -R appuser:appuser /app

USER appuser

EXPOSE 8090

CMD ["./aex-toolgate"]
