package nats

import (
	"encoding/json"
	"testing"

	natsio "github.com/nats-io/nats.go"
)

func TestBuildUsageRecordPreservesIPv6(t *testing.T) {
	record, err := buildUsageRecord("dns-node-1", UsageRecord{
		Date:        "2026-04-15",
		Domain:      "rpc.example.com",
		MemberName:  "provider1",
		CountryCode: "US",
		Asn:         "AS64500",
		NetworkName: "ExampleNet",
		CountryName: "United States",
		Hits:        42,
		IsIPv6:      true,
	})
	if err != nil {
		t.Fatalf("buildUsageRecord returned error: %v", err)
	}

	if record.NodeID != "dns-node-1" {
		t.Fatalf("expected node id to be preserved, got %q", record.NodeID)
	}
	if !record.IsIPv6 {
		t.Fatalf("expected IPv6 flag to be preserved")
	}
	if got := record.Date.Format("2006-01-02"); got != "2026-04-15" {
		t.Fatalf("expected parsed date to match input, got %q", got)
	}
}

func TestBuildUsageRecordRejectsInvalidDate(t *testing.T) {
	_, err := buildUsageRecord("dns-node-1", UsageRecord{Date: "not-a-date"})
	if err == nil {
		t.Fatalf("expected invalid date to return an error")
	}
}

func TestHandleUsageDataMarksNodeHeardBeforeReturningOnEmptyRecords(t *testing.T) {
	State.Mu.Lock()
	originalNodes := State.ClusterNodes
	State.ClusterNodes = make(map[string]NodeInfo)
	State.Mu.Unlock()
	t.Cleanup(func() {
		State.Mu.Lock()
		State.ClusterNodes = originalNodes
		State.Mu.Unlock()
	})

	payload, err := json.Marshal(UsageResponse{
		NodeID:       "dns-node-test",
		UsageRecords: []UsageRecord{},
	})
	if err != nil {
		t.Fatalf("marshal usage response: %v", err)
	}

	handleUsageData(&natsio.Msg{Data: payload})

	State.Mu.RLock()
	node, ok := State.ClusterNodes["dns-node-test"]
	State.Mu.RUnlock()
	if !ok {
		t.Fatalf("expected usage handler to mark node heard")
	}
	if node.LastHeard.IsZero() {
		t.Fatalf("expected marked node to have LastHeard set")
	}
}
