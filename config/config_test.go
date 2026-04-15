package config

import (
	"sync"
	"testing"
)

func seedTestConfig() Config {
	return Config{
		Local: LocalConfig{
			DnsApi: ApiConfig{
				AuthKeys: map[string]string{
					"primary": "secret",
				},
			},
			Checks: []Check{
				{
					Name: "wss",
					ExtraOptions: map[string]interface{}{
						"headers": map[string]interface{}{
							"User-Agent": "ibp-monitor",
						},
					},
				},
			},
		},
		StaticDNS: []DNSRecord{
			{QName: "rpc.example.com", Content: "192.0.2.10"},
		},
		Members: map[string]Member{
			"provider1": {
				Details: MemberDetails{Name: "provider1"},
				ServiceAssignments: map[string][]string{
					"rpc": {"rpc.example.com"},
				},
			},
		},
		Services: map[string]Service{
			"rpc": {
				Providers: map[string]ServiceProvider{
					"provider1": {
						RpcUrls: []string{"https://rpc.example.com:8443"},
					},
				},
			},
		},
		Pricing: map[string]IaasPricing{
			"provider1": {Cores: 1.0},
		},
		ServiceRequests: ServiceRequests{
			Requests: map[string]map[string]MonthlyData{
				"rpc": {
					"2026-04": {
						DNS: RequestStats{Requests: 10},
					},
				},
			},
		},
		Alerts: AlertsConfig{
			Matrix: struct {
				Room         string              `json:"room"`
				InternalRoom string              `json:"internal_room"`
				Members      map[string][]string `json:"members"`
			}{
				Members: map[string][]string{
					"provider1": {"@ops:example.org"},
				},
			},
		},
	}
}

func withTestConfig(t *testing.T, data Config) {
	t.Helper()

	prev := cfg
	cfg = &ConfigInit{data: data}
	t.Cleanup(func() {
		cfg = prev
	})
}

func TestGetConfigReturnsDeepCopy(t *testing.T) {
	withTestConfig(t, seedTestConfig())

	got := GetConfig()

	got.Local.DnsApi.AuthKeys["primary"] = "changed"
	got.Local.Checks[0].ExtraOptions["headers"].(map[string]interface{})["User-Agent"] = "mutated"
	got.StaticDNS[0].Content = "198.51.100.15"

	member := got.Members["provider1"]
	member.ServiceAssignments["rpc"][0] = "changed.example.com"

	service := got.Services["rpc"]
	service.Providers["provider1"].RpcUrls[0] = "https://mutated.example.com"

	got.Pricing["provider1"] = IaasPricing{Cores: 9.0}

	monthly := got.ServiceRequests.Requests["rpc"]
	stats := monthly["2026-04"]
	stats.DNS.Requests = 999
	monthly["2026-04"] = stats

	got.Alerts.Matrix.Members["provider1"][0] = "@mutated:example.org"

	if cfg.data.Local.DnsApi.AuthKeys["primary"] != "secret" {
		t.Fatalf("expected original auth key to remain unchanged")
	}
	if cfg.data.Local.Checks[0].ExtraOptions["headers"].(map[string]interface{})["User-Agent"] != "ibp-monitor" {
		t.Fatalf("expected original nested extra options map to remain unchanged")
	}
	if cfg.data.StaticDNS[0].Content != "192.0.2.10" {
		t.Fatalf("expected original static dns record to remain unchanged")
	}
	if cfg.data.Members["provider1"].ServiceAssignments["rpc"][0] != "rpc.example.com" {
		t.Fatalf("expected original member assignments to remain unchanged")
	}
	if cfg.data.Services["rpc"].Providers["provider1"].RpcUrls[0] != "https://rpc.example.com:8443" {
		t.Fatalf("expected original service provider URLs to remain unchanged")
	}
	if cfg.data.Pricing["provider1"].Cores != 1.0 {
		t.Fatalf("expected original pricing to remain unchanged")
	}
	if cfg.data.ServiceRequests.Requests["rpc"]["2026-04"].DNS.Requests != 10 {
		t.Fatalf("expected original service request stats to remain unchanged")
	}
	if cfg.data.Alerts.Matrix.Members["provider1"][0] != "@ops:example.org" {
		t.Fatalf("expected original alert members to remain unchanged")
	}
}

func TestGetConfigConcurrentAccess(t *testing.T) {
	withTestConfig(t, seedTestConfig())

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				got := GetConfig()
				got.Local.DnsApi.AuthKeys["primary"] = "changed"
				member := got.Members["provider1"]
				member.ServiceAssignments["rpc"][0] = "changed.example.com"
			}
		}()
	}
	wg.Wait()
}

func TestGetMemberReturnsDeepCopy(t *testing.T) {
	withTestConfig(t, seedTestConfig())

	member, ok := GetMember("provider1")
	if !ok {
		t.Fatalf("expected test member to exist")
	}

	member.ServiceAssignments["rpc"][0] = "changed.example.com"
	if cfg.data.Members["provider1"].ServiceAssignments["rpc"][0] != "rpc.example.com" {
		t.Fatalf("expected original member assignments to remain unchanged")
	}
}

func TestListMembersReturnsDeepCopy(t *testing.T) {
	withTestConfig(t, seedTestConfig())

	members := ListMembers()
	member := members["provider1"]
	member.ServiceAssignments["rpc"][0] = "changed.example.com"

	if cfg.data.Members["provider1"].ServiceAssignments["rpc"][0] != "rpc.example.com" {
		t.Fatalf("expected original member assignments to remain unchanged")
	}
}
