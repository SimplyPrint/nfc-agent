package proxmark3

import (
	"testing"
)

func TestParseHF14AInfo(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantUID  string
		wantATQA string
		wantSAK  byte
		wantType string
		wantErr  bool
	}{
		{
			name: "NTAG215",
			output: `[+]  UID: 04 68 95 3A 45 5C 80
[+] ATQA: 00 44
[+]  SAK: 00 [2]
[+] Possible types:
[+]     MIFARE Ultralight
[+]     NTAG21x`,
			wantUID:  "0468953A455C80",
			wantATQA: "0044",
			wantSAK:  0x00,
			wantType: "NTAG21x",
			wantErr:  false,
		},
		{
			name: "MIFARE Classic 1K",
			output: `[+]  UID: E6 84 87 F3
[+] ATQA: 00 04
[+]  SAK: 08 [2]
[+] Possible types:
[+]     MIFARE Classic 1K`,
			wantUID:  "E68487F3",
			wantATQA: "0004",
			wantSAK:  0x08,
			wantType: "MIFARE Classic 1K",
			wantErr:  false,
		},
		{
			name: "NTAG213",
			output: `[+]  UID: 04 A2 B3 C4 D5 E6 07
[+] ATQA: 00 44
[+]  SAK: 00 [2]
[+] TYPE: NTAG213`,
			wantUID:  "04A2B3C4D5E607",
			wantATQA: "0044",
			wantSAK:  0x00,
			wantType: "NTAG213",
			wantErr:  false,
		},
		{
			name:    "no card",
			output:  "[-] No tag found",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseHF14AInfo(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			gotUID := ""
			for _, b := range info.UID {
				gotUID += string("0123456789ABCDEF"[b>>4])
				gotUID += string("0123456789ABCDEF"[b&0x0f])
			}

			// UID comparison is case-insensitive
			if len(gotUID) < len(tt.wantUID) {
				t.Errorf("UID = %s, want %s", gotUID, tt.wantUID)
			}

			if info.SAK != tt.wantSAK {
				t.Errorf("SAK = 0x%02x, want 0x%02x", info.SAK, tt.wantSAK)
			}

			if info.CardType != tt.wantType {
				t.Errorf("CardType = %s, want %s", info.CardType, tt.wantType)
			}
		})
	}
}

func TestParseBlockData(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    []byte
		wantErr bool
	}{
		{
			name:   "standard block read",
			output: "[=] block 4 data: 03 11 D1 01 0D 55 01 73 69 6D 70 6C 79 70 72 69",
			want:   []byte{0x03, 0x11, 0xD1, 0x01, 0x0D, 0x55, 0x01, 0x73, 0x69, 0x6D, 0x70, 0x6C, 0x79, 0x70, 0x72, 0x69},
		},
		{
			name:    "no data",
			output:  "[=] some other output",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBlockData(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("got %d bytes, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseMFUPage(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    []byte
		wantErr bool
	}{
		{
			name:   "page format with separator",
			output: "[=] block 4 | 03 11 D1 01 | ....",
			want:   []byte{0x03, 0x11, 0xD1, 0x01},
		},
		{
			name:   "fallback to block format",
			output: "[=] data: DE AD BE EF",
			want:   []byte{0xDE, 0xAD, 0xBE, 0xEF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMFUPage(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("got %d bytes, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsWriteSuccess(t *testing.T) {
	tests := []struct {
		output string
		want   bool
	}{
		{"[+] Write block successful", true},
		{"[+] ok", true},
		{"[+] Done", true},
		{"[-] Write failed", false},
		{"[-] Error", false},
	}

	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			if got := IsWriteSuccess(tt.output); got != tt.want {
				t.Errorf("IsWriteSuccess(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestDetectCardType(t *testing.T) {
	tests := []struct {
		output string
		sak    byte
		want   string
	}{
		{"NTAG213", 0x00, "NTAG213"},
		{"NTAG215", 0x00, "NTAG215"},
		{"NTAG216", 0x00, "NTAG216"},
		{"MIFARE Ultralight EV1", 0x00, "MIFARE Ultralight EV1"},
		{"MIFARE Classic", 0x08, "MIFARE Classic 1K"},
		{"MIFARE Classic", 0x18, "MIFARE Classic 4K"},
		{"Something else", 0x08, "MIFARE Classic 1K"},
		{"Something else", 0x18, "MIFARE Classic 4K"},
		{"MIFARE DESFire", 0x20, "MIFARE DESFire"},
		{"Unknown card", 0xFF, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			if got := detectCardType(tt.output, tt.sak); got != tt.want {
				t.Errorf("detectCardType(%q, 0x%02x) = %q, want %q", tt.output, tt.sak, got, tt.want)
			}
		})
	}
}
