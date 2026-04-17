package data2

import "testing"

func snapshotProposalStore() map[string]Proposal {
	memMu.RLock()
	defer memMu.RUnlock()

	cp := make(map[string]Proposal, len(memStore))
	for id, proposal := range memStore {
		proposalCopy := proposal
		if proposal.VoteData != nil {
			proposalCopy.VoteData = make(map[string]bool, len(proposal.VoteData))
			for nodeID, agree := range proposal.VoteData {
				proposalCopy.VoteData[nodeID] = agree
			}
		}
		cp[id] = proposalCopy
	}
	return cp
}

func restoreProposalStore(state map[string]Proposal) {
	memMu.Lock()
	defer memMu.Unlock()

	memStore = make(map[string]Proposal, len(state))
	for id, proposal := range state {
		proposalCopy := proposal
		if proposal.VoteData != nil {
			proposalCopy.VoteData = make(map[string]bool, len(proposal.VoteData))
			for nodeID, agree := range proposal.VoteData {
				proposalCopy.VoteData[nodeID] = agree
			}
		}
		memStore[id] = proposalCopy
	}
}

func TestRecordProposalVoteCreatesStubAndMergesIntoProposal(t *testing.T) {
	previous := snapshotProposalStore()
	t.Cleanup(func() { restoreProposalStore(previous) })

	restoreProposalStore(nil)

	if got := RecordProposalVote("proposal-1", "ROTKO", true); got != 1 {
		t.Fatalf("expected first vote count to be 1, got %d", got)
	}

	CacheProposal(Proposal{
		ID:           "proposal-1",
		SenderNodeID: "STAKEPLUS",
		MemberName:   "provider1",
	})

	proposal, ok := PopProposal("proposal-1")
	if !ok {
		t.Fatalf("expected proposal to exist after caching")
	}
	if proposal.SenderNodeID != "STAKEPLUS" {
		t.Fatalf("expected proposal fields to be preserved, got sender %q", proposal.SenderNodeID)
	}
	if got, ok := proposal.VoteData["ROTKO"]; !ok || !got {
		t.Fatalf("expected buffered vote data to merge into cached proposal")
	}
}

func TestRecordProposalVoteUpdatesVoteMap(t *testing.T) {
	previous := snapshotProposalStore()
	t.Cleanup(func() { restoreProposalStore(previous) })

	restoreProposalStore(nil)

	CacheProposal(Proposal{
		ID:       "proposal-2",
		VoteData: map[string]bool{"AMFORC": true},
	})

	if got := RecordProposalVote("proposal-2", "ROTKO", false); got != 2 {
		t.Fatalf("expected vote count to become 2, got %d", got)
	}

	proposal, ok := PopProposal("proposal-2")
	if !ok {
		t.Fatalf("expected proposal to exist after vote update")
	}
	if proposal.VoteData["AMFORC"] != true {
		t.Fatalf("expected existing vote to be preserved")
	}
	if proposal.VoteData["ROTKO"] != false {
		t.Fatalf("expected new vote to be stored")
	}
}
