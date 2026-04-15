package matrix

import (
	"sync"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestClaimOutageAlertClearsInvalidCachedValue(t *testing.T) {
	offlineMap = sync.Map{}
	key := "provider1|site|ping|||false"
	offlineMap.Store(key, "invalid-event-type")

	if !claimOutageAlert(key) {
		t.Fatalf("expected to claim outage alert after removing invalid cached value")
	}

	raw, ok := offlineMap.Load(key)
	if !ok {
		t.Fatalf("expected sentinel entry to be stored")
	}

	evID, ok := storedEventID(raw)
	if !ok {
		t.Fatalf("expected stored value to be a Matrix event ID")
	}
	if evID != "" {
		t.Fatalf("expected in-flight sentinel event ID, got %q", evID)
	}
}

func TestClaimOutageAlertRejectsExistingEventID(t *testing.T) {
	offlineMap = sync.Map{}
	key := "provider1|site|ping|||false"
	offlineMap.Store(key, id.EventID("$event"))

	if claimOutageAlert(key) {
		t.Fatalf("expected existing event ID to prevent a duplicate outage alert")
	}
}
