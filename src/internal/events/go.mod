module github.com/parlakisik/agent-exchange/internal/events

go 1.25.0

toolchain go1.26.7

require github.com/parlakisik/agent-exchange/internal/nats v0.0.0

require (
	github.com/klauspost/compress v1.18.7 // indirect
	github.com/nats-io/nats.go v1.37.0 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace github.com/parlakisik/agent-exchange/internal/nats => ../nats
