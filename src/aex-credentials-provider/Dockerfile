FROM golang:1.26.7-alpine AS builder

WORKDIR /app

# Copy internal modules first
COPY internal/ap2 ./internal/ap2
COPY internal/telemetry ./internal/telemetry

# Copy service module
COPY aex-credentials-provider/ ./aex-credentials-provider/

WORKDIR /app/aex-credentials-provider

# Download dependencies
RUN go mod download

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /credentials-provider ./src/main.go

# Final stage
FROM alpine:3.19

RUN apk --no-cache upgrade && apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /credentials-provider .

EXPOSE 8111

CMD ["./credentials-provider"]
