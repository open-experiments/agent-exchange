module github.com/parlakisik/agent-exchange/hack/toolgate-harness

go 1.25.0

toolchain go1.26.7

require (
	github.com/parlakisik/agent-exchange/internal/events v0.0.0
	github.com/parlakisik/agent-exchange/internal/nats v0.0.0
	github.com/parlakisik/agent-exchange/internal/toolgate v0.0.0
)

require (
	github.com/klauspost/compress v1.18.7 // indirect
	github.com/nats-io/nats.go v1.37.0 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace github.com/parlakisik/agent-exchange/internal/events => ../../src/internal/events

replace github.com/parlakisik/agent-exchange/internal/nats => ../../src/internal/nats

replace github.com/parlakisik/agent-exchange/internal/toolgate => ../../src/internal/toolgate
