package consensus

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/ibp-network/ibp-geodns-libs/nats/core"

	"github.com/nats-io/nats.go"
)

func newTestDependencies() Dependencies {
	return Dependencies{
		State: &core.NodeState{
			NodeID:          "monitor-a",
			Proposals:       make(map[core.ProposalID]*core.ProposalTracking),
			ClusterNodes:    make(map[string]core.NodeInfo),
			ProposalTimeout: time.Minute,
			SubjectPropose:  "consensus.propose",
			SubjectVote:     "consensus.vote",
			SubjectFinalize: "consensus.finalize",
		},
		Publish:             func(string, []byte) error { return nil },
		CountActiveMonitors: func() int { return 1 },
		IsNodeActive:        func(core.NodeInfo) bool { return true },
		MarkNodeHeard:       func(string) {},
	}
}

func stopProposalTimers(state *core.NodeState) {
	state.Mu.Lock()
	defer state.Mu.Unlock()

	for _, pt := range state.Proposals {
		if pt.Timer != nil {
			pt.Timer.Stop()
		}
	}
}

func TestProposeCheckStatusDeduplicatesConcurrentMatches(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ProposeCheckStatus(
				deps,
				"endpoint",
				"wss",
				"provider1",
				"rpc.example.com",
				"wss://rpc.example.com/ws",
				false,
				"timeout",
				nil,
				true,
			)
		}()
	}
	wg.Wait()

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if got := len(deps.State.Proposals); got != 1 {
		t.Fatalf("expected exactly one proposal, got %d", got)
	}
}

func TestHandleProposalIgnoresDuplicateProposalContent(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	ProposeCheckStatus(
		deps,
		"domain",
		"http",
		"provider1",
		"rpc.example.com",
		"",
		false,
		"timeout",
		nil,
		false,
	)

	duplicate := core.Proposal{
		ID:             core.ProposalID("duplicate-id"),
		SenderNodeID:   "monitor-b",
		CheckType:      "domain",
		CheckName:      "http",
		MemberName:     "provider1",
		DomainName:     "rpc.example.com",
		Endpoint:       "",
		ProposedStatus: false,
		ErrorText:      "timeout",
		IsIPv6:         false,
		Timestamp:      time.Now().UTC(),
	}
	payload, err := json.Marshal(duplicate)
	if err != nil {
		t.Fatalf("failed to marshal proposal: %v", err)
	}

	HandleProposal(deps, &nats.Msg{Data: payload})

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if got := len(deps.State.Proposals); got != 1 {
		t.Fatalf("expected duplicate proposal to be ignored, got %d proposals", got)
	}
}
