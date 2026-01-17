package nats

import (
	"time"

	modusage "github.com/ibp-network/ibp-geodns-libs/nats/modules/usage"
	"github.com/ibp-network/ibp-geodns-libs/nats/subjects"

	"github.com/nats-io/nats.go"
)

var usageDeps = modusage.Dependencies{
	State:               &State,
	Publish:             Publish,
	PublishMsgWithReply: PublishMsgWithReply,
	Subscribe:           Subscribe,
	CountActiveDns:      countActiveDns,
	MarkNodeHeard:       markNodeHeard,
	UsageDataSubject:    subjects.DnsUsageData,
}

func handleDnsUsageRequest(m *nats.Msg) {
	modusage.HandleRequest(usageDeps, m.Reply, m.Data)
}

func handleDnsUsageData(m *nats.Msg) {
	modusage.HandleData(usageDeps, m.Data)
}

func RequestAllDnsUsage(req UsageRequest, timeout time.Duration) ([]UsageRecord, error) {
	return modusage.RequestAll(usageDeps, req, timeout, subjects.DnsUsageRequest)
}
