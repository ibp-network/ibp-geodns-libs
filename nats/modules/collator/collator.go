package collator

import (
	"strings"

	"github.com/ibp-network/ibp-geodns-libs/nats/router"
	"github.com/ibp-network/ibp-geodns-libs/nats/subjects"
	"github.com/nats-io/nats.go"
)

type Dependencies struct {
	Subjects        SubjectProvider
	CacheProposal   func(*nats.Msg)
	HandleFinalize  func(*nats.Msg)
	HandleStatsData func(*nats.Msg)
	HandleUsageData func(*nats.Msg)
}

type SubjectProvider interface {
	Subjects() (propose, vote, finalize string)
}

func Register(reg *router.Registry, deps Dependencies) {
	reg.Register("IBPCollator", module{deps: deps})
}

type module struct {
	deps Dependencies
}

func (module) Name() string { return "collator-core" }

func (m module) Handle(msg *nats.Msg) bool {
	subj := msg.Subject

	switch subj {
	case subjects.MonitorStatsData:
		if m.deps.HandleStatsData != nil {
			m.deps.HandleStatsData(msg)
			return true
		}
	case subjects.DnsUsageData:
		if m.deps.HandleUsageData != nil {
			m.deps.HandleUsageData(msg)
			return true
		}
	}

	if strings.Contains(subj, "downtimeReply") && m.deps.HandleStatsData != nil {
		m.deps.HandleStatsData(msg)
		return true
	}
	if strings.Contains(subj, "usageReply") && m.deps.HandleUsageData != nil {
		m.deps.HandleUsageData(msg)
		return true
	}

	propose, _, finalize := m.subjectStrings()
	switch subj {
	case propose:
		if m.deps.CacheProposal != nil {
			m.deps.CacheProposal(msg)
			return true
		}
	case finalize:
		if m.deps.HandleFinalize != nil {
			m.deps.HandleFinalize(msg)
			return true
		}
	}

	return false
}

func (m module) subjectStrings() (string, string, string) {
	if m.deps.Subjects == nil {
		return "", "", ""
	}
	return m.deps.Subjects.Subjects()
}
