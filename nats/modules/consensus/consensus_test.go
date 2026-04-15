package consensus

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	cfg "github.com/ibp-network/ibp-geodns-libs/config"
	dat "github.com/ibp-network/ibp-geodns-libs/data"
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

func resetLocalResults() {
	dat.Local = dat.LocalResults{
		SiteResults:     make([]dat.SiteResult, 0),
		DomainResults:   make([]dat.DomainResult, 0),
		EndpointResults: make([]dat.EndpointResult, 0),
	}
}

func TestProposeCheckStatusDeduplicatesConcurrentMatches(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	prevLocal := dat.Local
	resetLocalResults()
	defer func() {
		dat.Local = prevLocal
	}()

	check := cfg.Check{Name: "wss"}
	member := cfg.Member{Details: cfg.MemberDetails{Name: "provider1"}}
	dat.UpdateLocalEndpointResult(check, member, cfg.Service{}, "rpc.example.com", "wss://rpc.example.com/ws", false, "timeout", nil, true)

	votePublished := make(chan struct{}, 8)
	deps.Publish = func(subject string, data []byte) error {
		if subject == deps.State.SubjectVote {
			select {
			case votePublished <- struct{}{}:
			default:
			}
		}
		return nil
	}

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

	receivedVotes := 0
	timeout := time.After(500 * time.Millisecond)
	for receivedVotes < 8 {
		select {
		case <-votePublished:
			receivedVotes++
		case <-timeout:
			t.Fatalf("expected 8 vote attempts from deduplicated proposals, got %d", receivedVotes)
		}
	}

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if got := len(deps.State.Proposals); got != 1 {
		t.Fatalf("expected exactly one proposal, got %d", got)
	}
}

func TestHandleProposalIgnoresDuplicateProposalID(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	prop := core.Proposal{
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
	deps.State.Proposals[prop.ID] = &core.ProposalTracking{
		Proposal: prop,
		Votes:    make(map[string]bool),
	}

	payload, err := json.Marshal(prop)
	if err != nil {
		t.Fatalf("failed to marshal proposal: %v", err)
	}

	HandleProposal(deps, &nats.Msg{Data: payload})

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if got := len(deps.State.Proposals); got != 1 {
		t.Fatalf("expected duplicate proposal id to be ignored, got %d proposals", got)
	}
}

func TestHandleProposalVotesOnMatchingProposalWithDifferentID(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	prevLocal := dat.Local
	resetLocalResults()
	defer func() {
		dat.Local = prevLocal
	}()

	check := cfg.Check{Name: "http"}
	member := cfg.Member{Details: cfg.MemberDetails{Name: "provider1"}}
	dat.UpdateLocalDomainResult(check, member, cfg.Service{}, "rpc.example.com", false, "timeout", nil, false)

	votePublished := make(chan struct{}, 4)
	deps.Publish = func(subject string, data []byte) error {
		if subject == deps.State.SubjectVote {
			select {
			case votePublished <- struct{}{}:
			default:
			}
		}
		return nil
	}

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

	incoming := core.Proposal{
		ID:             core.ProposalID("remote-proposal-id"),
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

	payload, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("failed to marshal proposal: %v", err)
	}

	HandleProposal(deps, &nats.Msg{Data: payload})

	receivedVotes := 0
	timeout := time.After(500 * time.Millisecond)
	for receivedVotes < 2 {
		select {
		case <-votePublished:
			receivedVotes++
		case <-timeout:
			t.Fatalf("expected both local and remote proposals to trigger votes, got %d", receivedVotes)
		}
	}

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if got := len(deps.State.Proposals); got != 2 {
		t.Fatalf("expected both local and remote proposals to be tracked, got %d proposals", got)
	}
}

func TestProposeCheckStatusVotesOnExistingMatchingProposal(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	prevLocal := dat.Local
	resetLocalResults()
	defer func() {
		dat.Local = prevLocal
	}()

	incoming := core.Proposal{
		ID:             core.ProposalID("remote-proposal-id"),
		SenderNodeID:   "monitor-b",
		CheckType:      "endpoint",
		CheckName:      "wss",
		MemberName:     "provider1",
		DomainName:     "rpc.example.com",
		Endpoint:       "wss://rpc.example.com/ws",
		ProposedStatus: false,
		ErrorText:      "timeout",
		IsIPv6:         true,
		Timestamp:      time.Now().UTC(),
	}
	deps.State.Proposals[incoming.ID] = &core.ProposalTracking{
		Proposal: incoming,
		Votes:    make(map[string]bool),
	}

	votePublished := make(chan core.Vote, 1)
	deps.Publish = func(subject string, data []byte) error {
		if subject == deps.State.SubjectVote {
			var vote core.Vote
			if err := json.Unmarshal(data, &vote); err == nil {
				select {
				case votePublished <- vote:
				default:
				}
			}
		}
		return nil
	}

	check := cfg.Check{Name: "wss"}
	member := cfg.Member{Details: cfg.MemberDetails{Name: "provider1"}}
	dat.UpdateLocalEndpointResult(check, member, cfg.Service{}, "rpc.example.com", "wss://rpc.example.com/ws", false, "timeout", nil, true)

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

	select {
	case vote := <-votePublished:
		if vote.ProposalID != incoming.ID {
			t.Fatalf("expected vote for existing proposal id %s, got %s", incoming.ID, vote.ProposalID)
		}
		if vote.NodeID != deps.State.NodeID {
			t.Fatalf("expected vote node %s, got %s", deps.State.NodeID, vote.NodeID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected local catch-up propose to vote on existing matching proposal")
	}

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if got := len(deps.State.Proposals); got != 1 {
		t.Fatalf("expected existing proposal to be reused, got %d proposals", got)
	}
}
