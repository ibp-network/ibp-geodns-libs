package data

import (
	"testing"

	cfg "github.com/ibp-network/ibp-geodns-libs/config"
)

func currentOfficialSnapshot() Snapshot {
	muOfficial.RLock()
	defer muOfficial.RUnlock()

	return Snapshot{
		SiteResults:     cloneSiteResults(official.SiteResults),
		DomainResults:   cloneDomainResults(official.DomainResults),
		EndpointResults: cloneEndpointResults(official.EndpointResults),
	}
}

func sampleOfficialSnapshot() Snapshot {
	return Snapshot{
		SiteResults: []SiteResult{
			{
				Check: cfg.Check{
					Name:         "ping",
					ExtraOptions: map[string]interface{}{"mode": "strict"},
				},
				IsIPv6: true,
				Results: []Result{
					{
						Member: cfg.Member{
							Details: cfg.MemberDetails{Name: "provider1"},
							ServiceAssignments: map[string][]string{
								"rpc": {"rpc.example.com"},
							},
						},
						Status: true,
						Data: map[string]interface{}{
							"meta": map[string]interface{}{
								"source": "probe-a",
							},
						},
						IsIPv6: true,
					},
				},
			},
		},
	}
}

func TestSetOfficialSnapshotClonesInput(t *testing.T) {
	original := currentOfficialSnapshot()
	t.Cleanup(func() { SetOfficialSnapshot(original) })

	snap := sampleOfficialSnapshot()
	SetOfficialSnapshot(snap)

	snap.SiteResults[0].Check.Name = "changed"
	snap.SiteResults[0].Check.ExtraOptions["mode"] = "relaxed"
	snap.SiteResults[0].Results[0].Member.ServiceAssignments["rpc"][0] = "mutated"

	sites, _, _ := GetOfficialResults()
	if got := sites[0].Check.Name; got != "ping" {
		t.Fatalf("expected stored check name to remain unchanged, got %q", got)
	}
	if got := sites[0].Check.ExtraOptions["mode"]; got != "strict" {
		t.Fatalf("expected stored extra options to remain unchanged, got %#v", got)
	}
	if got := sites[0].Results[0].Member.ServiceAssignments["rpc"][0]; got != "rpc.example.com" {
		t.Fatalf("expected stored service assignment to remain unchanged, got %q", got)
	}
}

func TestGetOfficialResultsReturnsDeepCopies(t *testing.T) {
	original := currentOfficialSnapshot()
	t.Cleanup(func() { SetOfficialSnapshot(original) })

	SetOfficialSnapshot(sampleOfficialSnapshot())

	sites, _, _ := GetOfficialResults()
	sites[0].Check.Name = "changed"
	sites[0].Check.ExtraOptions["mode"] = "relaxed"
	sites[0].Results[0].Member.ServiceAssignments["rpc"][0] = "mutated"
	meta := sites[0].Results[0].Data["meta"].(map[string]interface{})
	meta["source"] = "probe-b"

	again, _, _ := GetOfficialResults()
	if got := again[0].Check.Name; got != "ping" {
		t.Fatalf("expected fresh read to preserve check name, got %q", got)
	}
	if got := again[0].Check.ExtraOptions["mode"]; got != "strict" {
		t.Fatalf("expected fresh read to preserve extra options, got %#v", got)
	}
	if got := again[0].Results[0].Member.ServiceAssignments["rpc"][0]; got != "rpc.example.com" {
		t.Fatalf("expected fresh read to preserve service assignment, got %q", got)
	}
	if got := again[0].Results[0].Data["meta"].(map[string]interface{})["source"]; got != "probe-a" {
		t.Fatalf("expected nested data map to be cloned, got %#v", got)
	}
}
