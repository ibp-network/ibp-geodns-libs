package nats

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	log "github.com/ibp-network/ibp-geodns-libs/logging"

	"github.com/nats-io/nats.go"
)

const (
	activeNodeWindow        = 10 * time.Minute
	broadcastJoinRetryCount = 3
	broadcastJoinDelay      = 500 * time.Millisecond
	joinThrottleWindow      = 5 * time.Second
)

var (
	reMonitor = regexp.MustCompile(`(?i)monitor`)
	reDns     = regexp.MustCompile(`(?i)dns`)
)

var lastJoin int64 // unix‑nano timestamp of our last JOIN

func EnableMonitorRole() error  { return enableRoleInternal("IBPMonitor") }
func EnableDnsRole() error      { return enableRoleInternal("IBPDns") }
func EnableCollatorRole() error { return enableRoleInternal("IBPCollator") }

func enableRoleInternal(role string) error {
	State.SubjectPropose = "consensus.propose"
	State.SubjectVote = "consensus.vote"
	State.SubjectFinalize = "consensus.finalize"
	State.SubjectCluster = "consensus.cluster"
	State.ProposalTimeout = 30 * time.Second

	if strings.TrimSpace(State.NodeID) == "" {
		return fmt.Errorf("NodeID is empty; cannot enable role %s", role)
	}

	if State.Proposals == nil {
		State.Proposals = make(map[ProposalID]*ProposalTracking)
	}
	if State.PendingVotes == nil {
		State.PendingVotes = make(map[ProposalID]map[string]Vote)
	}
	if State.ClusterNodes == nil {
		State.ClusterNodes = make(map[string]NodeInfo)
	}

	State.ThisNode.NodeRole = role
	State.ThisNode.LastHeard = time.Now().UTC()

	State.Mu.Lock()
	State.ClusterNodes[State.NodeID] = State.ThisNode
	State.Mu.Unlock()

	// Be more resilient to transient NATS unavailability.
	var err error
	for i := 0; i < 5; i++ {
		if _, err = Subscribe(">", handleAllMessages); err == nil {
			break
		}
		log.Log(log.Warn, "[NATS] subscribe failed (attempt %d/5): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return err
	}

	if role == "IBPMonitor" || role == "IBPCollator" {
		StartGarbageCollection()
	}
	startHeartbeat()

	log.Log(log.Info, "[NATS] %s role enabled for node=%s", role, State.NodeID)

	go func() {
		for i := 0; i < broadcastJoinRetryCount; i++ {
			broadcastClusterJoin(true)
			time.Sleep(broadcastJoinDelay)
		}
	}()

	return nil
}

func startHeartbeat() {
	go func() {
		time.Sleep(2 * time.Second)
		t := time.NewTicker(90 * time.Second)
		defer t.Stop()
		for range t.C {
			broadcastClusterJoin(false)
		}
	}()
}

func broadcastClusterJoin(force bool) {
	now := time.Now().UTC()
	nowUnix := now.UnixNano()
	if !force {
		if last := atomic.LoadInt64(&lastJoin); last != 0 && nowUnix-last < int64(joinThrottleWindow) {
			return
		}
	}
	atomic.StoreInt64(&lastJoin, nowUnix)

	State.Mu.Lock()
	if State.ThisNode.NodeID == "" {
		State.Mu.Unlock()
		log.Log(log.Error, "[NATS] JOIN suppressed – NodeID is empty; role not fully active (refuse to proceed)")
		return
	}
	State.ThisNode.LastHeard = now
	State.ClusterNodes[State.NodeID] = State.ThisNode
	sender := State.ThisNode
	State.Mu.Unlock()

	msg := ClusterMessage{
		Type:   "join",
		Sender: sender,
	}
	data, _ := json.Marshal(msg)
	if err := Publish(State.SubjectCluster, data); err != nil {
		log.Log(log.Error, "[NATS] Failed to publish JOIN: %v", err)
	}
}

func handleAllMessages(m *nats.Msg) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Log(log.Error, "[NATS] message handler panic for %s: %v", m.Subject, r)
			}
		}()

		subj := m.Subject
		if subj == State.SubjectCluster {
			handleClusterMessage(m)
			return
		}

		if !messageRouter.Dispatch(State.ThisNode.NodeRole, m) && strings.HasPrefix(subj, "consensus.") {
			log.Log(log.Debug, "[NATS] unhandled consensus subject %s for role=%s", subj, State.ThisNode.NodeRole)
		}
	}()
}

func handleClusterMessage(m *nats.Msg) {
	var msg ClusterMessage
	if err := json.Unmarshal(m.Data, &msg); err != nil {
		log.Log(log.Error, "[NATS] handleClusterMessage: unmarshal error: %v", err)
		return
	}
	if msg.Sender.NodeID == "" {
		return
	}

	wasNew := markNodeHeardWithState(msg.Sender.NodeID)

	if msg.Type == "join" {
		updated := addNode(msg.Sender)
		if msg.Sender.NodeID != State.NodeID && (wasNew || updated) {
			go broadcastClusterJoin(true)
		}
	}
}

func addNode(n NodeInfo) bool {
	State.Mu.Lock()
	defer State.Mu.Unlock()

	if n.NodeID == "" {
		return false
	}
	cur, exists := State.ClusterNodes[n.NodeID]
	if !exists {
		State.ClusterNodes[n.NodeID] = n
		return true
	}

	updated := false
	if cur.NodeRole == "" && n.NodeRole != "" {
		cur.NodeRole = n.NodeRole
		updated = true
	}
	if cur.PublicAddress == "" && n.PublicAddress != "" {
		cur.PublicAddress = n.PublicAddress
		updated = true
	}
	if cur.ListenAddress == "" && n.ListenAddress != "" {
		cur.ListenAddress = n.ListenAddress
		updated = true
	}
	if cur.ListenPort == "" && n.ListenPort != "" {
		cur.ListenPort = n.ListenPort
		updated = true
	}
	if updated {
		State.ClusterNodes[n.NodeID] = cur
	}
	return updated
}

func markNodeHeard(id string) {
	_ = markNodeHeardWithState(id)
}

func markNodeHeardWithState(id string) bool {
	if id == "" {
		return false
	}
	State.Mu.Lock()
	defer State.Mu.Unlock()

	n, exists := State.ClusterNodes[id]
	if !exists {
		n = NodeInfo{NodeID: id}
	}
	if n.NodeRole == "" {
		n.NodeRole = guessRoleFromID(id)
	}
	n.LastHeard = time.Now().UTC()
	State.ClusterNodes[id] = n
	return !exists
}

func guessRoleFromID(id string) string {
	switch {
	case reMonitor.MatchString(id):
		return "IBPMonitor"
	case reDns.MatchString(id):
		return "IBPDns"
	default:
		return ""
	}
}

func IsNodeActive(n NodeInfo) bool {
	return n.NodeID != "" && !n.LastHeard.IsZero() && time.Since(n.LastHeard) < activeNodeWindow
}

func CountActiveMonitors() int {
	State.Mu.RLock()
	defer State.Mu.RUnlock()
	n := 0
	for _, node := range State.ClusterNodes {
		if node.NodeRole == "IBPMonitor" && IsNodeActive(node) {
			n++
		}
	}
	return n
}

func CountActiveDns() int {
	State.Mu.RLock()
	defer State.Mu.RUnlock()
	n := 0
	for _, node := range State.ClusterNodes {
		if node.NodeRole == "IBPDns" && IsNodeActive(node) {
			n++
		}
	}
	return n
}

func StartGarbageCollection() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			cleanOldProposals()
			cleanStaleNodes()
		}
	}()
}

func cleanOldProposals() {
	State.Mu.Lock()
	defer State.Mu.Unlock()

	now := time.Now().UTC()
	for id, pt := range State.Proposals {
		if now.Sub(pt.Proposal.Timestamp) > 10*time.Minute {
			if pt.Timer != nil {
				pt.Timer.Stop()
			}
			delete(State.Proposals, id)
		}
	}
}

func cleanStaleNodes() {
	now := time.Now().UTC()
	State.Mu.Lock()
	defer State.Mu.Unlock()

	for id, node := range State.ClusterNodes {
		if id == State.NodeID {
			continue
		}
		if !node.LastHeard.IsZero() && now.Sub(node.LastHeard) > 15*time.Minute {
			delete(State.ClusterNodes, id)
		}
	}
}

var (
	countActiveMonitors = CountActiveMonitors
	countActiveDns      = CountActiveDns
	isNodeActive        = IsNodeActive
)
