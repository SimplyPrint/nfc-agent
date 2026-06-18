package core

import "testing"

func TestDesfireErrorMessage(t *testing.T) {
	withStatus := &DesfireError{Msg: "authentication failed", Status: 0x91AE}
	if got, want := withStatus.Error(), "authentication failed (status 0x91AE)"; got != want {
		t.Errorf("with status: got %q, want %q", got, want)
	}

	noStatus := &DesfireError{Msg: "reader does not support DESFire transparent sessions"}
	if got, want := noStatus.Error(), "reader does not support DESFire transparent sessions"; got != want {
		t.Errorf("no status: got %q, want %q", got, want)
	}
}

func TestSplitStatusWord(t *testing.T) {
	tests := []struct {
		name    string
		rsp     []byte
		wantSW1 byte
		wantSW2 byte
		wantOK  bool
	}{
		{"additional frame", []byte{0xAA, 0xBB, 0x91, 0xAF}, 0x91, 0xAF, true},
		{"success only", []byte{0x91, 0x00}, 0x91, 0x00, true},
		{"too short", []byte{0x91}, 0, 0, false},
		{"empty", []byte{}, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sw1, sw2, ok := SplitStatusWord(tt.rsp)
			if sw1 != tt.wantSW1 || sw2 != tt.wantSW2 || ok != tt.wantOK {
				t.Errorf("SplitStatusWord(%X) = (%02X, %02X, %v), want (%02X, %02X, %v)",
					tt.rsp, sw1, sw2, ok, tt.wantSW1, tt.wantSW2, tt.wantOK)
			}
		})
	}
}
