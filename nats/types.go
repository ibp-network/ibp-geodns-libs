package nats

import (
	"github.com/ibp-network/ibp-geodns-libs/nats/core"
)

type UsageRequest = core.UsageRequest

type NodeState = core.NodeState
type NodeInfo = core.NodeInfo
type ProposalID = core.ProposalID
type Proposal = core.Proposal
type ProposalTracking = core.ProposalTracking
type Vote = core.Vote
type FinalizeMessage = core.FinalizeMessage
type UsageRecord = core.UsageRecord
type UsageResponse = core.UsageResponse
type DowntimeRequest = core.DowntimeRequest
type DowntimeEvent = core.DowntimeEvent
type DowntimeResponse = core.DowntimeResponse
type ClusterMessage = core.ClusterMessage

var State NodeState
