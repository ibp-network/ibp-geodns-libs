package data2

import "testing"

func TestShouldNotifyOffline(t *testing.T) {
	if !shouldNotifyOffline(false, 1) {
		t.Fatal("expected a fresh offline insert to notify")
	}
	if shouldNotifyOffline(false, 2) {
		t.Fatal("expected duplicate offline updates not to notify again")
	}
	if shouldNotifyOffline(false, 0) {
		t.Fatal("expected no-op offline writes not to notify")
	}
	if shouldNotifyOffline(true, 1) {
		t.Fatal("expected online status not to use offline notification path")
	}
}
