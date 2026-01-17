package nats

import "github.com/nats-io/nats.go"

// NC exposes the active NATS connection for legacy consumers (e.g. ibp-geodns
// repos that reference nats.NC directly). Prefer GetConnection for new code.
var NC *nats.Conn

// NatsMsg is a compatibility alias so downstream repos can keep referring to
// nats.NatsMsg without any changes.
type NatsMsg = nats.Msg
