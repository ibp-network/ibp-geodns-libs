package data2

import (
	"sync"
	"time"
)

var (
	memMu      sync.RWMutex
	memStore   = make(map[string]Proposal)
	expiryTime = 10 * time.Minute
)

func CacheProposal(p Proposal) {
	memMu.Lock()
	if existing, ok := memStore[p.ID]; ok {
		if p.VoteData == nil && existing.VoteData != nil {
			p.VoteData = make(map[string]bool, len(existing.VoteData))
			for nodeID, agree := range existing.VoteData {
				p.VoteData[nodeID] = agree
			}
		}
		if p.CreatedAt.IsZero() {
			p.CreatedAt = existing.CreatedAt
		}
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.VoteData == nil {
		p.VoteData = make(map[string]bool)
	}
	memStore[p.ID] = p
	memMu.Unlock()
}

func PopProposal(id string) (Proposal, bool) {
	memMu.Lock()
	defer memMu.Unlock()
	p, ok := memStore[id]
	if ok {
		delete(memStore, id)
	}
	return p, ok
}

func ExpireStaleProposals() {
	cut := time.Now().UTC().Add(-expiryTime)
	memMu.Lock()
	for id, p := range memStore {
		if p.CreatedAt.Before(cut) {
			delete(memStore, id)
		}
	}
	memMu.Unlock()
}

func RecordProposalVote(id, nodeID string, agree bool) int {
	memMu.Lock()
	defer memMu.Unlock()

	p, ok := memStore[id]
	if !ok {
		p = Proposal{
			ID:        id,
			CreatedAt: time.Now().UTC(),
			VoteData:  make(map[string]bool),
		}
	}
	if p.VoteData == nil {
		p.VoteData = make(map[string]bool)
	}
	p.VoteData[nodeID] = agree
	memStore[id] = p
	return len(p.VoteData)
}

func StoreProposal(p Proposal) error { CacheProposal(p); return nil }

func MarkProposalFinal(id string, yes, total int) error { _, _ = PopProposal(id); return nil }
