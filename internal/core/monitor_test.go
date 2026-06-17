package core

import "testing"

func TestReaderSetKey(t *testing.T) {
	empty := readerSetKey(nil)
	if empty != "" {
		t.Errorf("empty reader set should produce empty key, got %q", empty)
	}
	if readerSetKey([]Reader{}) != empty {
		t.Error("nil and empty slice should produce the same key")
	}

	one := []Reader{{ID: "reader-0", Name: "ACR122U PICC", Type: "picc"}}
	two := []Reader{
		{ID: "reader-0", Name: "ACR122U PICC", Type: "picc"},
		{ID: "reader-1", Name: "Proxmark3", Type: "proxmark3"},
	}

	if readerSetKey(one) == readerSetKey(two) {
		t.Error("different reader sets must produce different keys")
	}
	if readerSetKey(one) == empty {
		t.Error("non-empty set must differ from empty key")
	}

	// Key must be independent of the positional ID, but sensitive to name/type.
	sameSetDifferentID := []Reader{{ID: "reader-9", Name: "ACR122U PICC", Type: "picc"}}
	if readerSetKey(one) != readerSetKey(sameSetDifferentID) {
		t.Error("key should depend on name+type, not the positional ID")
	}

	renamed := []Reader{{ID: "reader-0", Name: "ACR1252U PICC", Type: "picc"}}
	if readerSetKey(one) == readerSetKey(renamed) {
		t.Error("a different reader name must change the key")
	}
}
