package nats

import (
	cfg "github.com/ibp-network/ibp-geodns-libs/config"
	dat "github.com/ibp-network/ibp-geodns-libs/data"
	data2 "github.com/ibp-network/ibp-geodns-libs/data2"
	log "github.com/ibp-network/ibp-geodns-libs/logging"
	"github.com/ibp-network/ibp-geodns-libs/nats/core"
	modconsensus "github.com/ibp-network/ibp-geodns-libs/nats/modules/consensus"

	"github.com/nats-io/nats.go"
)

var consensusDeps = modconsensus.Dependencies{
	State:               &State,
	Publish:             Publish,
	CountActiveMonitors: countActiveMonitors,
	IsNodeActive:        isNodeActive,
	MarkNodeHeard:       markNodeHeard,
	OnFinalize:          onConsensusFinalize,
}

func ProposeCheckStatus(
	checkType, checkName, memberName,
	domainName, endpoint string,
	status bool,
	errorText string,
	dataMap map[string]interface{},
	isIPv6 bool,
) {
	modconsensus.ProposeCheckStatus(consensusDeps, checkType, checkName, memberName, domainName, endpoint, status, errorText, dataMap, isIPv6)
}

func handleProposal(m *nats.Msg) {
	modconsensus.HandleProposal(consensusDeps, m)
}

func handleVote(m *nats.Msg) {
	modconsensus.HandleVote(consensusDeps, m)
}

func handleFinalize(m *nats.Msg) {
	modconsensus.HandleFinalize(consensusDeps, m)
}

func onConsensusFinalize(fm core.FinalizeMessage) {
	if !fm.Passed {
		return
	}

	switch State.ThisNode.NodeRole {
	case "IBPMonitor":
		applyOfficialChanges(fm.Proposal)
	case "IBPCollator":
		handleCollatorFinalize(fm)
	}
}

func handleCollatorFinalize(fm core.FinalizeMessage) {
	ct := checkTypeToInt(fm.Proposal.CheckType)
	url := deriveCheckURL(fm.Proposal)

	rec := data2.NetStatusRecord{
		CheckType: ct,
		CheckName: fm.Proposal.CheckName,
		CheckURL:  url,
		Domain:    fm.Proposal.DomainName,
		Member:    fm.Proposal.MemberName,
		IsIPv6:    fm.Proposal.IsIPv6,
	}

	if !fm.Proposal.ProposedStatus {
		rec.Status = false
		rec.StartTime = fm.DecidedAt.UTC()
		rec.Error = fm.Proposal.ErrorText
		rec.VoteData = nil
		rec.Extra = fm.Proposal.Data

		if err := data2.InsertNetStatus(rec); err != nil {
			log.Log(log.Error, "[NATS] handleFinalize: InsertNetStatus: %v", err)
		}
	} else {
		if err := data2.CloseOpenEvent(rec); err != nil {
			log.Log(log.Error, "[NATS] handleFinalize: CloseOpenEvent: %v", err)
		}
	}
}

func applyOfficialChanges(prop core.Proposal) {
	log.Log(log.Debug,
		"[CONSENSUS] ⇢ apply official change id=%s type=%s member=%s status=%v v6=%v",
		prop.ID, prop.CheckType, prop.MemberName, prop.ProposedStatus, prop.IsIPv6)

	chk, okChk := findCheckByName(prop.CheckName, prop.CheckType)
	if !okChk {
		log.Log(log.Warn, "[NATS] applyOfficialChanges: check %s/%s not found", prop.CheckType, prop.CheckName)
		return
	}
	mem, okMem := findMemberByName(prop.MemberName)
	if !okMem {
		log.Log(log.Warn, "[NATS] applyOfficialChanges: member %s not found", prop.MemberName)
		return
	}

	var svc cfg.Service
	if prop.CheckType == "domain" || prop.CheckType == "endpoint" {
		if s, ok := findServiceForDomain(prop.DomainName); ok {
			svc = s
		}
	}

	switch prop.CheckType {
	case "site":
		dat.UpdateOfficialSiteResult(chk, mem, prop.ProposedStatus, prop.ErrorText, prop.Data, prop.IsIPv6)
	case "domain":
		dat.UpdateOfficialDomainResult(chk, mem, svc, prop.DomainName, prop.ProposedStatus, prop.ErrorText, prop.Data, prop.IsIPv6)
	case "endpoint":
		dat.UpdateOfficialEndpointResult(chk, mem, svc, prop.DomainName, prop.Endpoint, prop.ProposedStatus, prop.ErrorText, prop.Data, prop.IsIPv6)
	}
}

func checkTypeToInt(t string) int {
	switch t {
	case "site":
		return 1
	case "domain":
		return 2
	case "endpoint":
		return 3
	default:
		return 0
	}
}

func deriveCheckURL(p core.Proposal) string {
	switch p.CheckType {
	case "endpoint":
		return p.Endpoint
	case "domain":
		return p.DomainName
	default:
		return ""
	}
}
