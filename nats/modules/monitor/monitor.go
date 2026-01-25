package monitor

import (
	"strings"

	"github.com/ibp-network/ibp-geodns-libs/nats/router"
	"github.com/ibp-network/ibp-geodns-libs/nats/subjects"
	"github.com/nats-io/nats.go"
)

// SubjectProvider returns the current consensus subjects (proposal/vote/finalize).
type SubjectProvider interface {
	Subjects() (propose, vote, finalize string)
}

// Dependencies enumerates the callbacks the monitor module needs from the parent nats package.
type Dependencies struct {
	Subjects        SubjectProvider
	HandleProposal  func(*nats.Msg)
	HandleVote      func(*nats.Msg)
	HandleFinalize  func(*nats.Msg)
	HandleStatsReq  func(*nats.Msg)
	HandleStatsData func(*nats.Msg)
}

// Register wires the monitor module into the provided registry.
func Register(reg *router.Registry, deps Dependencies) {
	reg.Register("IBPMonitor", module{deps: deps})
}

type module struct {
	deps Dependencies
}

func (module) Name() string { return "monitor-core" }

func (m module) Handle(msg *nats.Msg) bool {
	subj := msg.Subject

	switch subj {
	case subjects.MonitorStatsRequest:
		if m.deps.HandleStatsReq != nil {
			m.deps.HandleStatsReq(msg)
			return true
		}
	}

	if strings.Contains(subj, "downtimeReply") && m.deps.HandleStatsData != nil {
		m.deps.HandleStatsData(msg)
		return true
	}

	propose, vote, finalize := m.subjectStrings()
	switch subj {
	case propose:
		if m.deps.HandleProposal != nil {
			m.deps.HandleProposal(msg)
			return true
		}
	case vote:
		if m.deps.HandleVote != nil {
			m.deps.HandleVote(msg)
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
