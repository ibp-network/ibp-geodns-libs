package data

import (
	"path/filepath"
	"testing"

	cfg "github.com/ibp-network/ibp-geodns-libs/config"
)

func currentOfficialResultsState() OfficialResults {
	Official.Mu.RLock()
	defer Official.Mu.RUnlock()

	return OfficialResults{
		SiteResults:     cloneSiteResults(Official.SiteResults),
		DomainResults:   cloneDomainResults(Official.DomainResults),
		EndpointResults: cloneEndpointResults(Official.EndpointResults),
	}
}

func currentLocalResultsState() LocalResults {
	Local.Mu.RLock()
	defer Local.Mu.RUnlock()

	return LocalResults{
		SiteResults:     cloneSiteResults(Local.SiteResults),
		DomainResults:   cloneDomainResults(Local.DomainResults),
		EndpointResults: cloneEndpointResults(Local.EndpointResults),
	}
}

func sampleSiteResults() []SiteResult {
	return []SiteResult{
		{
			Check:  cfg.Check{Name: "ping"},
			IsIPv6: false,
			Results: []Result{
				{
					Member: cfg.Member{
						Details: cfg.MemberDetails{Name: "provider1"},
					},
					Status: true,
				},
			},
		},
	}
}

func TestSetOfficialSiteResultsRefreshesSnapshot(t *testing.T) {
	originalOfficial := currentOfficialResultsState()
	originalSnapshot := currentOfficialSnapshot()
	t.Cleanup(func() {
		Official.Mu.Lock()
		Official.SiteResults = cloneSiteResults(originalOfficial.SiteResults)
		Official.DomainResults = cloneDomainResults(originalOfficial.DomainResults)
		Official.EndpointResults = cloneEndpointResults(originalOfficial.EndpointResults)
		publishSnapshotLocked()
		Official.Mu.Unlock()
		SetOfficialSnapshot(originalSnapshot)
	})

	SetOfficialSiteResults(sampleSiteResults())

	sites, _, _ := GetOfficialResults()
	if len(sites) != 1 {
		t.Fatalf("expected official snapshot to expose 1 site result, got %d", len(sites))
	}
	if got := sites[0].Check.Name; got != "ping" {
		t.Fatalf("expected site result to be visible through snapshot, got %q", got)
	}
}

func TestLoadCachesFromFilesRefreshesOfficialSnapshot(t *testing.T) {
	originalOfficial := currentOfficialResultsState()
	originalLocal := currentLocalResultsState()
	originalSnapshot := currentOfficialSnapshot()
	t.Cleanup(func() {
		Official.Mu.Lock()
		Official.SiteResults = cloneSiteResults(originalOfficial.SiteResults)
		Official.DomainResults = cloneDomainResults(originalOfficial.DomainResults)
		Official.EndpointResults = cloneEndpointResults(originalOfficial.EndpointResults)
		publishSnapshotLocked()
		Official.Mu.Unlock()

		Local.Mu.Lock()
		Local.SiteResults = cloneSiteResults(originalLocal.SiteResults)
		Local.DomainResults = cloneDomainResults(originalLocal.DomainResults)
		Local.EndpointResults = cloneEndpointResults(originalLocal.EndpointResults)
		Local.Mu.Unlock()

		SetOfficialSnapshot(originalSnapshot)
	})

	tmpDir := t.TempDir()
	officialFile := filepath.Join(tmpDir, officialCacheFile)
	localFile := filepath.Join(tmpDir, localCacheFile)

	officialCache := OfficialResults{
		SiteResults: sampleSiteResults(),
	}
	localCache := LocalResults{
		SiteResults: sampleSiteResults(),
	}

	if err := SaveCache(officialFile, &officialCache); err != nil {
		t.Fatalf("failed to seed official cache: %v", err)
	}
	if err := SaveCache(localFile, &localCache); err != nil {
		t.Fatalf("failed to seed local cache: %v", err)
	}

	Official.Mu.Lock()
	Official.SiteResults = nil
	Official.DomainResults = nil
	Official.EndpointResults = nil
	publishSnapshotLocked()
	Official.Mu.Unlock()

	Local.Mu.Lock()
	Local.SiteResults = nil
	Local.DomainResults = nil
	Local.EndpointResults = nil
	Local.Mu.Unlock()

	loadCachesFromFiles(officialFile, localFile, true)

	sites, _, _ := GetOfficialResults()
	if len(sites) != 1 {
		t.Fatalf("expected cache-loaded official snapshot to expose 1 site result, got %d", len(sites))
	}
	if got := sites[0].Results[0].Member.Details.Name; got != "provider1" {
		t.Fatalf("expected cache-loaded official snapshot member to be provider1, got %q", got)
	}

	Local.Mu.RLock()
	defer Local.Mu.RUnlock()
	if len(Local.SiteResults) != 1 {
		t.Fatalf("expected local cache to load 1 site result, got %d", len(Local.SiteResults))
	}
}
