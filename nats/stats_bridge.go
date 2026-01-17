package nats

import (
	"time"

	modstats "github.com/ibp-network/ibp-geodns-libs/nats/modules/stats"
	"github.com/ibp-network/ibp-geodns-libs/nats/subjects"

	"github.com/nats-io/nats.go"
)

var statsDeps = modstats.Dependencies{
	State:               &State,
	Publish:             Publish,
	PublishMsgWithReply: PublishMsgWithReply,
	Subscribe:           Subscribe,
	CountActiveMonitors: countActiveMonitors,
	MarkNodeHeard:       markNodeHeard,
	StatsDataSubject:    subjects.MonitorStatsData,
}

func handleMonitorStatsRequest(m *nats.Msg) {
	modstats.HandleRequest(statsDeps, m.Reply, m.Data)
}

func handleMonitorStatsData(m *nats.Msg) {
	modstats.HandleData(statsDeps, m.Data)
}

func RequestAllMonitorsDowntime(req DowntimeRequest, timeout time.Duration) ([]DowntimeEvent, error) {
	return modstats.RequestAll(statsDeps, req, timeout, subjects.MonitorStatsRequest)
}
