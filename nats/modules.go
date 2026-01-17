package nats

import (
	modCollator "github.com/ibp-network/ibp-geodns-libs/nats/modules/collator"
	modDns "github.com/ibp-network/ibp-geodns-libs/nats/modules/dns"
	modMonitor "github.com/ibp-network/ibp-geodns-libs/nats/modules/monitor"
	"github.com/ibp-network/ibp-geodns-libs/nats/router"
	"github.com/nats-io/nats.go"
)

var messageRouter = router.New()

func init() {
	registerModules()
}

func registerModules() {
	subjects := stateSubjectProvider{}

	modMonitor.Register(messageRouter, modMonitor.Dependencies{
		Subjects:        subjects,
		HandleProposal:  handleProposal,
		HandleVote:      handleVote,
		HandleFinalize:  handleFinalize,
		HandleStatsReq:  handleMonitorStatsRequest,
		HandleStatsData: handleMonitorStatsData,
	})

	modDns.Register(messageRouter, modDns.Dependencies{
		HandleUsageRequest: handleDnsUsageRequest,
		HandleUsageData:    handleDnsUsageData,
	})

	modCollator.Register(messageRouter, modCollator.Dependencies{
		Subjects:        subjects,
		CacheProposal:   cacheCollatorProposal,
		HandleFinalize:  handleFinalize,
		HandleStatsData: handleMonitorStatsData,
		HandleUsageData: handleDnsUsageData,
	})
}

type stateSubjectProvider struct{}

func (stateSubjectProvider) Subjects() (string, string, string) {
	State.Mu.RLock()
	defer State.Mu.RUnlock()
	return State.SubjectPropose, State.SubjectVote, State.SubjectFinalize
}

// expose helper for tests or future modules
func dispatchMessage(role string, msg *nats.Msg) bool {
	return messageRouter.Dispatch(role, msg)
}
