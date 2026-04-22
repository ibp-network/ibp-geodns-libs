package requestschema

import "testing"

func TestHasExpectedUniqueIndex(t *testing.T) {
	if !HasExpectedUniqueIndex(ExpectedUniqueIndexColumns()) {
		t.Fatal("expected canonical requests index columns to validate")
	}

	if HasExpectedUniqueIndex([]string{"date", "domain_name"}) {
		t.Fatal("expected incomplete requests index to be rejected")
	}
}
