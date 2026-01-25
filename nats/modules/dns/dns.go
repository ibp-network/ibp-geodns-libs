package dns

import (
	"strings"

	"github.com/ibp-network/ibp-geodns-libs/nats/router"
	"github.com/ibp-network/ibp-geodns-libs/nats/subjects"
	"github.com/nats-io/nats.go"
)

type Dependencies struct {
	HandleUsageRequest func(*nats.Msg)
	HandleUsageData    func(*nats.Msg)
}

func Register(reg *router.Registry, deps Dependencies) {
	reg.Register("IBPDns", module{deps: deps})
}

type module struct {
	deps Dependencies
}

func (module) Name() string { return "dns-usage" }

func (m module) Handle(msg *nats.Msg) bool {
	switch msg.Subject {
	case subjects.DnsUsageRequest:
		if m.deps.HandleUsageRequest != nil {
			m.deps.HandleUsageRequest(msg)
			return true
		}
	default:
		if strings.Contains(msg.Subject, "usageReply") && m.deps.HandleUsageData != nil {
			m.deps.HandleUsageData(msg)
			return true
		}
	}
	return false
}
