package nats

import (
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsio "github.com/nats-io/nats.go"
)

func runRoleTestServer(t *testing.T) *natsserver.Server {
	t.Helper()

	srv, err := natsserver.NewServer(&natsserver.Options{
		Host:   "127.0.0.1",
		Port:   -1,
		NoLog:  true,
		NoSigs: true,
	})
	if err != nil {
		t.Fatalf("new NATS server: %v", err)
	}

	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		srv.Shutdown()
		t.Fatal("test NATS server did not become ready")
	}

	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})

	return srv
}

func collectClusterMessages(ch <-chan *natsio.Msg, window time.Duration) []ClusterMessage {
	timer := time.NewTimer(window)
	defer timer.Stop()

	var out []ClusterMessage
	for {
		select {
		case msg := <-ch:
			if msg == nil {
				continue
			}
			var clusterMsg ClusterMessage
			if err := json.Unmarshal(msg.Data, &clusterMsg); err == nil {
				out = append(out, clusterMsg)
			}
		case <-timer.C:
			return out
		}
	}
}

func countJoinMessages(msgs []ClusterMessage, senderID string) int {
	count := 0
	for _, msg := range msgs {
		if msg.Type == "join" && msg.Sender.NodeID == senderID {
			count++
		}
	}
	return count
}

func TestEnableRoleBootstrapsClusterVisibility(t *testing.T) {
	srv := runRoleTestServer(t)

	libConn, err := natsio.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect library client: %v", err)
	}
	connectionMu.Lock()
	nc = libConn
	NC = libConn
	connectionMu.Unlock()
	t.Cleanup(func() {
		Disconnect()
		State = NodeState{}
		atomic.StoreInt64(&lastJoin, 0)
	})

	probeConn, err := natsio.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect probe client: %v", err)
	}
	t.Cleanup(func() {
		probeConn.Close()
	})

	clusterMsgs := make(chan *natsio.Msg, 32)
	sub, err := probeConn.ChanSubscribe("consensus.cluster", clusterMsgs)
	if err != nil {
		t.Fatalf("subscribe probe client: %v", err)
	}
	t.Cleanup(func() {
		_ = sub.Unsubscribe()
	})
	if err := probeConn.Flush(); err != nil {
		t.Fatalf("flush probe subscription: %v", err)
	}

	State = NodeState{}
	atomic.StoreInt64(&lastJoin, 0)
	State.NodeID = "node-a"
	State.ThisNode = NodeInfo{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1",
		ListenPort:    "1234",
		NodeRole:      "IBPDns",
	}

	if err := EnableDnsRole(); err != nil {
		t.Fatalf("enable dns role: %v", err)
	}

	initial := collectClusterMessages(clusterMsgs, 1500*time.Millisecond)
	if got := countJoinMessages(initial, "node-a"); got < broadcastJoinRetryCount {
		t.Fatalf("expected at least %d startup JOINs from node-a, got %d (%+v)", broadcastJoinRetryCount, got, initial)
	}

	payload, err := json.Marshal(ClusterMessage{
		Type: "join",
		Sender: NodeInfo{
			NodeID:        "node-b",
			ListenAddress: "127.0.0.1",
			ListenPort:    "4321",
			NodeRole:      "IBPDns",
		},
	})
	if err != nil {
		t.Fatalf("marshal peer join: %v", err)
	}
	if err := probeConn.Publish("consensus.cluster", payload); err != nil {
		t.Fatalf("publish peer join: %v", err)
	}
	if err := probeConn.Flush(); err != nil {
		t.Fatalf("flush peer join: %v", err)
	}

	response := collectClusterMessages(clusterMsgs, 750*time.Millisecond)
	if got := countJoinMessages(response, "node-a"); got == 0 {
		t.Fatalf("expected node-a to answer a new peer JOIN with its own JOIN, got %+v", response)
	}
}
