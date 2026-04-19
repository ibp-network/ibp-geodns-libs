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
			PendingVotes:    make(map[core.ProposalID]map[string]core.Vote),
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

func TestProposeCheckStatusRepublishesUnresolvedLocalProposalAfterInterval(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	publishedProposals := 0
	deps.Publish = func(subject string, data []byte) error {
		if subject == deps.State.SubjectPropose {
			publishedProposals++
		}
		return nil
	}

	ProposeCheckStatus(
		deps,
		"endpoint",
		"noop",
		"provider1",
		"rpc.example.com",
		"wss://rpc.example.com/ws",
		true,
		"",
		nil,
		false,
	)

	deps.State.Mu.Lock()
	if got := len(deps.State.Proposals); got != 1 {
		deps.State.Mu.Unlock()
		t.Fatalf("expected one tracked proposal, got %d", got)
	}
	var pt *core.ProposalTracking
	for _, candidate := range deps.State.Proposals {
		pt = candidate
		break
	}
	if pt == nil {
		deps.State.Mu.Unlock()
		t.Fatal("expected tracked proposal to exist")
	}
	pt.LastBroadcastAt = time.Now().Add(-proposalRepublishInterval - time.Second)
	deps.State.Mu.Unlock()

	ProposeCheckStatus(
		deps,
		"endpoint",
		"noop",
		"provider1",
		"rpc.example.com",
		"wss://rpc.example.com/ws",
		true,
		"",
		nil,
		false,
	)

	if publishedProposals != 2 {
		t.Fatalf("expected unresolved local proposal to be republished, got %d publishes", publishedProposals)
	}

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if got := len(deps.State.Proposals); got != 1 {
		t.Fatalf("expected exactly one tracked proposal after republish, got %d", got)
	}
}

func TestProposeCheckStatusPublishesDistinctProposals(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	published := 0
	deps.Publish = func(subject string, data []byte) error {
		if subject == deps.State.SubjectPropose {
			published++
		}
		return nil
	}

	const proposalCount = 64
	for i := 0; i < proposalCount; i++ {
		memberName := "member-" + string(rune('A'+(i%26))) + string(rune('a'+((i/26)%26)))
		domainName := "domain-" + string(rune('A'+(i%26))) + ".example.com"
		endpoint := "wss://" + domainName + "/rpc/" + string(rune('0'+(i%10)))
		ProposeCheckStatus(
			deps,
			"endpoint",
			"wss",
			memberName,
			domainName,
			endpoint,
			i%2 == 0,
			"",
			nil,
			false,
		)
	}

	if published != proposalCount {
		t.Fatalf("expected %d distinct proposals to publish, got %d", proposalCount, published)
	}

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if got := len(deps.State.Proposals); got != proposalCount {
		t.Fatalf("expected %d tracked proposals, got %d", proposalCount, got)
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

func TestHandleVoteBuffersUntilProposalArrives(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	prevLocal := dat.Local
	resetLocalResults()
	defer func() {
		dat.Local = prevLocal
	}()

	incomingVote := core.Vote{
		ProposalID:   core.ProposalID("remote-proposal-id"),
		SenderNodeID: "monitor-b",
		NodeID:       "monitor-b",
		Agree:        true,
		Timestamp:    time.Now().UTC(),
	}

	votePayload, err := json.Marshal(incomingVote)
	if err != nil {
		t.Fatalf("failed to marshal vote: %v", err)
	}

	HandleVote(deps, &nats.Msg{Data: votePayload})

	deps.State.Mu.RLock()
	if _, ok := deps.State.PendingVotes[incomingVote.ProposalID]; !ok {
		deps.State.Mu.RUnlock()
		t.Fatalf("expected vote to be buffered until proposal arrives")
	}
	deps.State.Mu.RUnlock()

	check := cfg.Check{Name: "http"}
	member := cfg.Member{Details: cfg.MemberDetails{Name: "provider1"}}
	dat.UpdateLocalDomainResult(check, member, cfg.Service{}, "rpc.example.com", true, "", nil, false)

	votePublished := make(chan struct{}, 1)
	deps.Publish = func(subject string, data []byte) error {
		if subject == deps.State.SubjectVote {
			select {
			case votePublished <- struct{}{}:
			default:
			}
		}
		return nil
	}

	proposal := core.Proposal{
		ID:             incomingVote.ProposalID,
		SenderNodeID:   "monitor-b",
		CheckType:      "domain",
		CheckName:      "http",
		MemberName:     "provider1",
		DomainName:     "rpc.example.com",
		Endpoint:       "",
		ProposedStatus: true,
		ErrorText:      "",
		IsIPv6:         false,
		Timestamp:      time.Now().UTC(),
	}
	proposalPayload, err := json.Marshal(proposal)
	if err != nil {
		t.Fatalf("failed to marshal proposal: %v", err)
	}

	HandleProposal(deps, &nats.Msg{Data: proposalPayload})

	select {
	case <-votePublished:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected proposal arrival to trigger local vote processing")
	}

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if pt, ok := deps.State.Proposals[incomingVote.ProposalID]; ok {
		if got, ok := pt.Votes[incomingVote.NodeID]; !ok || !got {
			t.Fatalf("expected buffered vote to be applied after proposal arrival")
		}
	}
	if _, ok := deps.State.PendingVotes[incomingVote.ProposalID]; ok {
		t.Fatalf("expected buffered vote entry to be cleared after application")
	}
}

func TestHandleVoteCountsMonitorByConsensusTrafficEvenWithoutMonitorNamedNodeID(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	deps.CountActiveMonitors = func() int { return 2 }
	deps.State.ClusterNodes[deps.State.NodeID] = core.NodeInfo{
		NodeID:    deps.State.NodeID,
		NodeRole:  "IBPMonitor",
		LastHeard: time.Now().UTC(),
	}

	proposalID := core.ProposalID("proposal-with-rotko-vote")
	deps.State.Proposals[proposalID] = &core.ProposalTracking{
		Proposal: core.Proposal{
			ID:           proposalID,
			SenderNodeID: "ROTKO",
		},
		Votes: map[string]bool{
			deps.State.NodeID: true,
		},
	}

	vote := core.Vote{
		ProposalID:   proposalID,
		SenderNodeID: "ROTKO",
		NodeID:       "ROTKO",
		Agree:        true,
		Timestamp:    time.Now().UTC(),
	}

	payload, err := json.Marshal(vote)
	if err != nil {
		t.Fatalf("failed to marshal vote: %v", err)
	}

	HandleVote(deps, &nats.Msg{Data: payload})

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	pt := deps.State.Proposals[proposalID]
	if !pt.Finalized || !pt.Passed {
		t.Fatalf("expected vote from arbitrary monitor node id to count toward quorum")
	}
	if got := deps.State.ClusterNodes["ROTKO"].NodeRole; got != "IBPMonitor" {
		t.Fatalf("expected ROTKO to be classified as IBPMonitor, got %q", got)
	}
}

func TestVoteOnProposalAppliesLocalVoteWithoutEcho(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	deps.CountActiveMonitors = func() int { return 1 }
	deps.State.ClusterNodes[deps.State.NodeID] = core.NodeInfo{
		NodeID:    deps.State.NodeID,
		NodeRole:  "IBPMonitor",
		LastHeard: time.Now().UTC(),
	}

	prevLocal := dat.Local
	resetLocalResults()
	defer func() {
		dat.Local = prevLocal
	}()

	check := cfg.Check{Name: "wss"}
	member := cfg.Member{Details: cfg.MemberDetails{Name: "provider1"}}
	dat.UpdateLocalEndpointResult(check, member, cfg.Service{}, "rpc.example.com", "wss://rpc.example.com/ws", false, "timeout", nil, true)

	proposal := core.Proposal{
		ID:             core.ProposalID("self-vote-no-echo"),
		SenderNodeID:   deps.State.NodeID,
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
	deps.State.Proposals[proposal.ID] = &core.ProposalTracking{
		Proposal: proposal,
		Votes:    make(map[string]bool),
	}

	publishedVotes := 0
	deps.Publish = func(subject string, data []byte) error {
		if subject == deps.State.SubjectVote {
			publishedVotes++
		}
		return nil
	}

	voteOnProposal(deps, proposal)

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	pt := deps.State.Proposals[proposal.ID]
	if pt == nil {
		t.Fatalf("expected proposal to remain tracked")
	}
	if got, ok := pt.Votes[deps.State.NodeID]; !ok || !got {
		t.Fatalf("expected local vote to be recorded immediately")
	}
	if pt.Finalized {
		t.Fatalf("expected single active monitor to keep proposal pending until quorum is met")
	}
	if publishedVotes != 1 {
		t.Fatalf("expected vote to still be published to NATS, got %d publishes", publishedVotes)
	}
}

func TestVoteOnProposalWithLockingCountFunctionDoesNotDeadlock(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	deps.CountActiveMonitors = func() int {
		deps.State.Mu.RLock()
		defer deps.State.Mu.RUnlock()
		count := 0
		for _, node := range deps.State.ClusterNodes {
			if node.NodeRole == "IBPMonitor" && deps.IsNodeActive(node) {
				count++
			}
		}
		return count
	}
	deps.State.ClusterNodes[deps.State.NodeID] = core.NodeInfo{
		NodeID:    deps.State.NodeID,
		NodeRole:  "IBPMonitor",
		LastHeard: time.Now().UTC(),
	}

	prevLocal := dat.Local
	resetLocalResults()
	defer func() {
		dat.Local = prevLocal
	}()

	check := cfg.Check{Name: "wss"}
	member := cfg.Member{Details: cfg.MemberDetails{Name: "provider1"}}
	dat.UpdateLocalEndpointResult(check, member, cfg.Service{}, "rpc.example.com", "wss://rpc.example.com/ws", false, "timeout", nil, true)

	proposal := core.Proposal{
		ID:             core.ProposalID("locking-count-local-vote"),
		SenderNodeID:   deps.State.NodeID,
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
	deps.State.Proposals[proposal.ID] = &core.ProposalTracking{
		Proposal: proposal,
		Votes:    make(map[string]bool),
	}

	votePublished := make(chan struct{}, 1)
	deps.Publish = func(subject string, data []byte) error {
		if subject == deps.State.SubjectVote {
			select {
			case votePublished <- struct{}{}:
			default:
			}
		}
		return nil
	}

	done := make(chan struct{})
	go func() {
		voteOnProposal(deps, proposal)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected voteOnProposal to complete without deadlocking")
	}

	select {
	case <-votePublished:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected voteOnProposal to publish a vote")
	}
}

func TestHandleVoteWithLockingCountFunctionDoesNotDeadlock(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	deps.CountActiveMonitors = func() int {
		deps.State.Mu.RLock()
		defer deps.State.Mu.RUnlock()
		count := 0
		for _, node := range deps.State.ClusterNodes {
			if node.NodeRole == "IBPMonitor" && deps.IsNodeActive(node) {
				count++
			}
		}
		return count
	}

	deps.State.ClusterNodes[deps.State.NodeID] = core.NodeInfo{
		NodeID:    deps.State.NodeID,
		NodeRole:  "IBPMonitor",
		LastHeard: time.Now().UTC(),
	}
	deps.State.ClusterNodes["monitor-b"] = core.NodeInfo{
		NodeID:    "monitor-b",
		NodeRole:  "IBPMonitor",
		LastHeard: time.Now().UTC(),
	}

	proposalID := core.ProposalID("locking-count-remote-vote")
	deps.State.Proposals[proposalID] = &core.ProposalTracking{
		Proposal: core.Proposal{
			ID:           proposalID,
			SenderNodeID: deps.State.NodeID,
		},
		Votes: map[string]bool{
			deps.State.NodeID: true,
		},
	}

	payload, err := json.Marshal(core.Vote{
		ProposalID:   proposalID,
		SenderNodeID: "monitor-b",
		NodeID:       "monitor-b",
		Agree:        true,
		Timestamp:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to marshal vote: %v", err)
	}

	done := make(chan struct{})
	go func() {
		HandleVote(deps, &nats.Msg{Data: payload})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected HandleVote to complete without deadlocking")
	}

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if !deps.State.Proposals[proposalID].Finalized {
		t.Fatal("expected remote vote to finalize proposal once quorum is reached")
	}
}

func TestFinalizeAppliesLocallyWithoutEcho(t *testing.T) {
	deps := newTestDependencies()
	defer stopProposalTimers(deps.State)

	applied := make(chan core.FinalizeMessage, 1)
	deps.OnFinalize = func(msg core.FinalizeMessage) {
		select {
		case applied <- msg:
		default:
		}
	}

	publishedFinalize := 0
	deps.Publish = func(subject string, data []byte) error {
		if subject == deps.State.SubjectFinalize {
			publishedFinalize++
		}
		return nil
	}

	pt := &core.ProposalTracking{
		Proposal: core.Proposal{
			ID: core.ProposalID("finalize-local"),
		},
		Passed: true,
	}
	deps.State.Proposals[pt.Proposal.ID] = pt

	finalize(deps, pt)

	select {
	case msg := <-applied:
		if msg.Proposal.ID != pt.Proposal.ID || !msg.Passed {
			t.Fatalf("expected local finalize callback for %s, got %+v", pt.Proposal.ID, msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected finalize to apply locally without waiting for echo")
	}

	if publishedFinalize != 1 {
		t.Fatalf("expected finalize to still be published once, got %d", publishedFinalize)
	}

	deps.State.Mu.RLock()
	defer deps.State.Mu.RUnlock()
	if _, ok := deps.State.Proposals[pt.Proposal.ID]; ok {
		t.Fatalf("expected finalized proposal to be removed from in-memory state")
	}
}
