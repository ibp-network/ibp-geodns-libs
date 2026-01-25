package consensus

import (
	"encoding/json"
	"time"

	dat "github.com/ibp-network/ibp-geodns-libs/data"
	log "github.com/ibp-network/ibp-geodns-libs/logging"
	"github.com/ibp-network/ibp-geodns-libs/nats/core"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

const minConsensusVotes = 1

type Dependencies struct {
	State               *core.NodeState
	Publish             func(subject string, data []byte) error
	CountActiveMonitors func() int
	IsNodeActive        func(core.NodeInfo) bool
	MarkNodeHeard       func(string)
	OnFinalize          func(core.FinalizeMessage)
}

func ProposeCheckStatus(
	deps Dependencies,
	checkType, checkName, memberName,
	domainName, endpoint string,
	status bool,
	errorText string,
	dataMap map[string]interface{},
	isIPv6 bool,
) {
	state := deps.State
	state.Mu.RLock()
	for _, pt := range state.Proposals {
		if !pt.Finalized &&
			pt.Proposal.CheckType == checkType &&
			pt.Proposal.CheckName == checkName &&
			pt.Proposal.MemberName == memberName &&
			pt.Proposal.DomainName == domainName &&
			pt.Proposal.Endpoint == endpoint &&
			pt.Proposal.ProposedStatus == status &&
			pt.Proposal.IsIPv6 == isIPv6 {
			state.Mu.RUnlock()
			return
		}
	}
	state.Mu.RUnlock()

	propose(deps, checkType, checkName, memberName, domainName, endpoint,
		status, errorText, dataMap, isIPv6)
}

func propose(
	deps Dependencies,
	checkType, checkName, memberName, domainName, endpoint string,
	status bool,
	errorText string,
	data map[string]interface{},
	isIPv6 bool,
) {
	state := deps.State
	pid := core.ProposalID(uuid.New().String())

	prop := core.Proposal{
		ID:             pid,
		SenderNodeID:   state.NodeID,
		CheckType:      checkType,
		CheckName:      checkName,
		MemberName:     memberName,
		DomainName:     domainName,
		Endpoint:       endpoint,
		ProposedStatus: status,
		ErrorText:      errorText,
		Data:           data,
		IsIPv6:         isIPv6,
		Timestamp:      time.Now().UTC(),
	}

	log.Log(log.Debug,
		"[CONSENSUS] → PROPOSAL created id=%s type=%s member=%s status=%v v6=%v",
		prop.ID, prop.CheckType, prop.MemberName, prop.ProposedStatus, prop.IsIPv6)
	log.Log(log.Debug, "[CONSENSUS]     details=%+v", prop)

	pt := &core.ProposalTracking{
		Proposal: prop,
		Votes:    make(map[string]bool),
	}

	state.Mu.Lock()
	if state.Proposals == nil {
		state.Proposals = make(map[core.ProposalID]*core.ProposalTracking)
	}
	state.Proposals[pid] = pt
	pt.Timer = time.AfterFunc(state.ProposalTimeout, func() { forceFinalize(deps, pid) })
	state.Mu.Unlock()

	if dataBytes, _ := json.Marshal(prop); deps.Publish(state.SubjectPropose, dataBytes) != nil {
		log.Log(log.Error, "[NATS] failed to publish proposal %s", pid)
	}

	go voteOnProposal(deps, prop)
}

func HandleProposal(deps Dependencies, m *nats.Msg) {
	state := deps.State
	var prop core.Proposal
	if err := json.Unmarshal(m.Data, &prop); err != nil {
		log.Log(log.Error, "[NATS] handleProposal: unmarshal error: %v", err)
		return
	}
	log.Log(log.Debug,
		"[CONSENSUS] ← PROPOSAL received id=%s type=%s member=%s status=%v v6=%v",
		prop.ID, prop.CheckType, prop.MemberName, prop.ProposedStatus, prop.IsIPv6)
	deps.MarkNodeHeard(prop.SenderNodeID)

	state.Mu.Lock()
	if _, exists := state.Proposals[prop.ID]; !exists {
		state.Proposals[prop.ID] = &core.ProposalTracking{
			Proposal: prop,
			Votes:    make(map[string]bool),
		}
		state.Proposals[prop.ID].Timer = time.AfterFunc(state.ProposalTimeout,
			func() { forceFinalize(deps, prop.ID) })
		state.Mu.Unlock()
		go voteOnProposal(deps, prop)
		return
	}
	state.Mu.Unlock()
}

func voteOnProposal(deps Dependencies, prop core.Proposal) {
	state := deps.State
	time.Sleep(5 * time.Millisecond)

	found, localStatus := checkLocalStatus(
		prop.CheckType, prop.CheckName, prop.MemberName,
		prop.DomainName, prop.Endpoint, prop.IsIPv6)
	if !found {
		return
	}

	v := core.Vote{
		ProposalID:   prop.ID,
		SenderNodeID: state.NodeID,
		NodeID:       state.NodeID,
		Agree:        localStatus == prop.ProposedStatus,
		Timestamp:    time.Now().UTC(),
	}

	log.Log(log.Debug,
		"[CONSENSUS]    vote id=%s agree=%v (local=%v proposed=%v)",
		prop.ID, v.Agree, localStatus, prop.ProposedStatus)

	if data, _ := json.Marshal(v); deps.Publish(state.SubjectVote, data) != nil {
		log.Log(log.Error, "[NATS] failed to publish vote for %s", prop.ID)
	}
}

func HandleVote(deps Dependencies, m *nats.Msg) {
	state := deps.State
	var v core.Vote
	if err := json.Unmarshal(m.Data, &v); err != nil {
		log.Log(log.Error, "[NATS] handleVote: unmarshal error: %v", err)
		return
	}
	log.Log(log.Debug, "[CONSENSUS] ← vote id=%s from=%s agree=%v", v.ProposalID, v.NodeID, v.Agree)
	deps.MarkNodeHeard(v.SenderNodeID)

	state.Mu.Lock()
	pt, ok := state.Proposals[v.ProposalID]
	if !ok || pt.Finalized {
		state.Mu.Unlock()
		return
	}
	pt.Votes[v.NodeID] = v.Agree
	decideLocked(deps, pt)
	state.Mu.Unlock()
}

func decideLocked(deps Dependencies, pt *core.ProposalTracking) {
	state := deps.State
	total := deps.CountActiveMonitors()
	if total == 0 {
		return
	}
	maj := (total / 2) + 1

	yes, no := 0, 0
	for nid, agree := range pt.Votes {
		if node, ok := state.ClusterNodes[nid]; ok && node.NodeRole == "IBPMonitor" && deps.IsNodeActive(node) {
			if agree {
				yes++
			} else {
				no++
			}
		}
	}

	switch {
	case yes >= maj && yes >= minConsensusVotes:
		pt.Finalized, pt.Passed = true, true
	case no >= maj && no >= minConsensusVotes:
		pt.Finalized, pt.Passed = true, false
	}

	if pt.Finalized {
		log.Log(log.Info,
			"[CONSENSUS] ⇢ finalize id=%s PASS=%v yes=%d no=%d (%d active monitors)",
			pt.Proposal.ID, pt.Passed, yes, no, total)

		if pt.Timer != nil {
			pt.Timer.Stop()
		}
		go finalize(deps, pt)
	}
}

func forceFinalize(deps Dependencies, pid core.ProposalID) {
	state := deps.State
	state.Mu.Lock()
	pt, ok := state.Proposals[pid]
	if !ok || pt.Finalized {
		state.Mu.Unlock()
		return
	}
	decideLocked(deps, pt)
	if !pt.Finalized {
		// No decision yet (e.g., zero monitors). Reschedule another finalize attempt.
		pt.Timer = time.AfterFunc(state.ProposalTimeout, func() { forceFinalize(deps, pid) })
	}
	state.Mu.Unlock()
}

func HandleFinalize(deps Dependencies, m *nats.Msg) {
	var fm core.FinalizeMessage
	if err := json.Unmarshal(m.Data, &fm); err != nil {
		log.Log(log.Error, "[NATS] handleFinalize: unmarshal error: %v", err)
		return
	}
	log.Log(log.Debug,
		"[CONSENSUS] ← FINALIZE id=%s PASS=%v", fm.Proposal.ID, fm.Passed)
	deps.MarkNodeHeard(fm.Proposal.SenderNodeID)

	if deps.OnFinalize != nil {
		deps.OnFinalize(fm)
	}
}

func checkLocalStatus(checkType, checkName, memberName, domainName, endpoint string, isIPv6 bool) (bool, bool) {
	switch checkType {
	case "site":
		return dat.GetLocalSiteStatusIPv4v6(checkName, memberName, isIPv6)
	case "domain":
		return dat.GetLocalDomainStatusIPv4v6(checkName, memberName, domainName, isIPv6)
	case "endpoint":
		return dat.GetLocalEndpointStatusIPv4v6(checkName, memberName, domainName, endpoint, isIPv6)
	default:
		return false, false
	}
}

func finalize(deps Dependencies, pt *core.ProposalTracking) {
	state := deps.State
	msg := core.FinalizeMessage{
		Proposal:  pt.Proposal,
		Passed:    pt.Passed,
		DecidedAt: time.Now().UTC(),
	}
	if data, _ := json.Marshal(msg); deps.Publish(state.SubjectFinalize, data) != nil {
		log.Log(log.Error, "[NATS] failed to publish finalize for %s", pt.Proposal.ID)
	}

	if deps.OnFinalize != nil {
		deps.OnFinalize(msg)
	}

	state.Mu.Lock()
	delete(state.Proposals, pt.Proposal.ID)
	state.Mu.Unlock()
}
