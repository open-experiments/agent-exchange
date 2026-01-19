FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy internal dependencies first
COPY internal/ ./internal/

# Copy service module
COPY aex-credentials-provider/ ./aex-credentials-provider/

WORKDIR /app/aex-credentials-provider

# Download dependencies
RUN go mod download

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /credentials-provider ./src/main.go

# Final stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /credentials-provider .

EXPOSE 8111

CMD ["./credentials-provider"]
