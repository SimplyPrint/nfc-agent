package core

import (
	"encoding/hex"
	"testing"
)

// Tests using mock card data that simulates real hardware responses

func TestMockSmartCard_NTAG213(t *testing.T) {
	card := NewMockCard("NTAG213")

	// Test Status returns correct ATR
	status, err := card.Status()
	if err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}

	expectedATR := "3b8f8001804f0ca0000003060300030000000068"
	actualATR := hex.EncodeToString(status.Atr)
	if actualATR != expectedATR {
		t.Errorf("expected ATR %s, got %s", expectedATR, actualATR)
	}

	// Test GET UID command
	getUIDCmd := []byte{0xFF, 0xCA, 0x00, 0x00, 0x00}
	resp, err := card.Transmit(getUIDCmd)
	if err != nil {
		t.Fatalf("Transmit(GET_UID) returned error: %v", err)
	}

	// Response should be UID + 9000
	if len(resp) < 3 {
		t.Fatalf("response too short: %d bytes", len(resp))
	}

	// Check status words
	sw1, sw2 := resp[len(resp)-2], resp[len(resp)-1]
	if sw1 != 0x90 || sw2 != 0x00 {
		t.Errorf("expected status 9000, got %02X%02X", sw1, sw2)
	}

	// Check UID
	uid := resp[:len(resp)-2]
	expectedUID := "0442488a837280"
	actualUID := hex.EncodeToString(uid)
	if actualUID != expectedUID {
		t.Errorf("expected UID %s, got %s", expectedUID, actualUID)
	}
}

func TestMockSmartCard_MIFAREClassic(t *testing.T) {
	card := NewMockCard("MIFARE Classic")

	status, err := card.Status()
	if err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}

	expectedATR := "3b8f8001804f0ca000000306030001000000006a"
	actualATR := hex.EncodeToString(status.Atr)
	if actualATR != expectedATR {
		t.Errorf("expected ATR %s, got %s", expectedATR, actualATR)
	}

	// Test GET UID
	getUIDCmd := []byte{0xFF, 0xCA, 0x00, 0x00, 0x00}
	resp, err := card.Transmit(getUIDCmd)
	if err != nil {
		t.Fatalf("Transmit(GET_UID) returned error: %v", err)
	}

	uid := resp[:len(resp)-2]
	expectedUID := "932bae0e"
	actualUID := hex.EncodeToString(uid)
	if actualUID != expectedUID {
		t.Errorf("expected UID %s, got %s", expectedUID, actualUID)
	}
}

func TestMockSmartCard_ISO15693(t *testing.T) {
	card := NewMockCard("ISO 15693")

	status, err := card.Status()
	if err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}

	expectedATR := "3b8f8001804f0ca0000003060b00140000000077"
	actualATR := hex.EncodeToString(status.Atr)
	if actualATR != expectedATR {
		t.Errorf("expected ATR %s, got %s", expectedATR, actualATR)
	}

	// Check for ISO 15693 pattern in ATR
	if !contains(actualATR, "03060b") {
		t.Error("ATR should contain ISO 15693 pattern '03060b'")
	}
}

func TestMockSmartCard_NTAG215(t *testing.T) {
	card := NewMockCard("NTAG215")

	// Test Status returns correct ATR
	status, err := card.Status()
	if err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}

	expectedATR := "3b8f8001804f0ca0000003060300030000000068"
	actualATR := hex.EncodeToString(status.Atr)
	if actualATR != expectedATR {
		t.Errorf("expected ATR %s, got %s", expectedATR, actualATR)
	}

	// Test GET UID command
	getUIDCmd := []byte{0xFF, 0xCA, 0x00, 0x00, 0x00}
	resp, err := card.Transmit(getUIDCmd)
	if err != nil {
		t.Fatalf("Transmit(GET_UID) returned error: %v", err)
	}

	// Check status words
	sw1, sw2 := resp[len(resp)-2], resp[len(resp)-1]
	if sw1 != 0x90 || sw2 != 0x00 {
		t.Errorf("expected status 9000, got %02X%02X", sw1, sw2)
	}

	// Check UID - real data from ACR1252 reader
	uid := resp[:len(resp)-2]
	expectedUID := "04635d6bc22a81"
	actualUID := hex.EncodeToString(uid)
	if actualUID != expectedUID {
		t.Errorf("expected UID %s, got %s", expectedUID, actualUID)
	}
}

func TestMockSmartCard_NTAG216(t *testing.T) {
	card := NewMockCard("NTAG216")

	// Test Status returns correct ATR
	status, err := card.Status()
	if err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}

	expectedATR := "3b8f8001804f0ca0000003060300030000000068"
	actualATR := hex.EncodeToString(status.Atr)
	if actualATR != expectedATR {
		t.Errorf("expected ATR %s, got %s", expectedATR, actualATR)
	}

	// Test GET UID command
	getUIDCmd := []byte{0xFF, 0xCA, 0x00, 0x00, 0x00}
	resp, err := card.Transmit(getUIDCmd)
	if err != nil {
		t.Fatalf("Transmit(GET_UID) returned error: %v", err)
	}

	// Check status words
	sw1, sw2 := resp[len(resp)-2], resp[len(resp)-1]
	if sw1 != 0x90 || sw2 != 0x00 {
		t.Errorf("expected status 9000, got %02X%02X", sw1, sw2)
	}

	// Check UID - real data from ACR122U reader
	uid := resp[:len(resp)-2]
	expectedUID := "5397e01aa20001"
	actualUID := hex.EncodeToString(uid)
	if actualUID != expectedUID {
		t.Errorf("expected UID %s, got %s", expectedUID, actualUID)
	}
}

func TestMockSmartCard_NTAGSizes(t *testing.T) {
	tests := []struct {
		cardType     string
		expectedSize int // CC size field indicates capacity
	}{
		{"NTAG213", 0x12}, // 144 bytes usable
		{"NTAG215", 0x3E}, // 496 bytes usable
		{"NTAG216", 0x6D}, // 872 bytes usable
	}

	for _, tt := range tests {
		t.Run(tt.cardType, func(t *testing.T) {
			card := NewMockCard(tt.cardType)

			// Read capability container (page 3)
			readCCCmd := []byte{0xFF, 0xB0, 0x00, 0x03, 0x10}
			resp, err := card.Transmit(readCCCmd)
			if err != nil {
				t.Fatalf("Transmit(READ CC) returned error: %v", err)
			}

			// CC format: E1 10 [size] 00
			if resp[0] != 0xE1 {
				t.Errorf("expected CC magic 0xE1, got 0x%02X", resp[0])
			}
			if resp[2] != byte(tt.expectedSize) {
				t.Errorf("expected CC size 0x%02X, got 0x%02X", tt.expectedSize, resp[2])
			}
		})
	}
}

func TestMockSmartCard_WriteCommand(t *testing.T) {
	card := NewMockCard("NTAG213")

	// Test write command
	writeCmd := []byte{0xFF, 0xD6, 0x00, 0x04, 0x04, 0x01, 0x02, 0x03, 0x04}
	resp, err := card.Transmit(writeCmd)
	if err != nil {
		t.Fatalf("Transmit(WRITE) returned error: %v", err)
	}

	// Should return success
	if len(resp) < 2 {
		t.Fatal("response too short")
	}

	sw1, sw2 := resp[len(resp)-2], resp[len(resp)-1]
	if sw1 != 0x90 || sw2 != 0x00 {
		t.Errorf("expected status 9000, got %02X%02X", sw1, sw2)
	}
}

func TestMockSmartCard_ReadCommand(t *testing.T) {
	card := NewMockCard("NTAG213")

	// Test read command (16 bytes from page 4)
	readCmd := []byte{0xFF, 0xB0, 0x00, 0x04, 0x10}
	resp, err := card.Transmit(readCmd)
	if err != nil {
		t.Fatalf("Transmit(READ) returned error: %v", err)
	}

	// Should return data + status
	if len(resp) < 2 {
		t.Fatal("response too short")
	}

	sw1, sw2 := resp[len(resp)-2], resp[len(resp)-1]
	if sw1 != 0x90 || sw2 != 0x00 {
		t.Errorf("expected status 9000, got %02X%02X", sw1, sw2)
	}
}

func TestMockSmartCard_WithNDEFData(t *testing.T) {
	card := NewMockCard("NTAG213").WithNDEFData("text", "Hello World")

	// Read NDEF data
	readCmd := []byte{0xFF, 0xB0, 0x00, 0x04, 0x40}
	resp, err := card.Transmit(readCmd)
	if err != nil {
		t.Fatalf("Transmit(READ) returned error: %v", err)
	}

	// Should return NDEF data + status
	if len(resp) < 10 {
		t.Fatalf("response too short: %d bytes", len(resp))
	}

	// Check for NDEF TLV marker
	if resp[0] != 0x03 {
		t.Errorf("expected NDEF TLV marker 0x03, got 0x%02X", resp[0])
	}
}

func TestMockSmartCard_WithError(t *testing.T) {
	card := NewMockCard("NTAG213").WithError("simulated error")

	_, err := card.Transmit([]byte{0xFF, 0xCA, 0x00, 0x00, 0x00})
	if err == nil {
		t.Error("expected error, got nil")
	}
	if err.Error() != "simulated error" {
		t.Errorf("expected 'simulated error', got '%s'", err.Error())
	}

	_, err = card.Status()
	if err == nil {
		t.Error("expected error from Status(), got nil")
	}
}

func TestMockSmartCard_Disconnect(t *testing.T) {
	card := NewMockCard("NTAG213")

	err := card.Disconnect(0)
	if err != nil {
		t.Fatalf("Disconnect() returned error: %v", err)
	}

	// After disconnect, Transmit should fail
	_, err = card.Transmit([]byte{0xFF, 0xCA, 0x00, 0x00, 0x00})
	if err == nil {
		t.Error("expected error after disconnect, got nil")
	}
}

func TestMockContext_ListReaders(t *testing.T) {
	ctx := NewMockContext()

	readers, err := ctx.ListReaders()
	if err != nil {
		t.Fatalf("ListReaders() returned error: %v", err)
	}

	expected := 3
	if len(readers) != expected {
		t.Errorf("expected %d readers, got %d", expected, len(readers))
	}

	// Check reader names
	expectedNames := []string{
		"ACS ACR122U PICC Interface",
		"ACS ACR1552 1S CL Reader PICC",
		"ACS ACR1252 Dual Reader PICC",
	}
	for i, name := range expectedNames {
		if readers[i] != name {
			t.Errorf("reader[%d] expected %s, got %s", i, name, readers[i])
		}
	}
}

func TestMockContext_WithReaders(t *testing.T) {
	customReaders := []string{"Reader 1", "Reader 2"}
	ctx := NewMockContext().WithReaders(customReaders)

	readers, err := ctx.ListReaders()
	if err != nil {
		t.Fatalf("ListReaders() returned error: %v", err)
	}

	if len(readers) != 2 {
		t.Errorf("expected 2 readers, got %d", len(readers))
	}
}

func TestMockContext_Connect(t *testing.T) {
	card := NewMockCard("NTAG213")
	ctx := NewMockContext().WithCard("ACS ACR122U PICC Interface", card)

	smartCard, err := ctx.Connect("ACS ACR122U PICC Interface", 0, 0)
	if err != nil {
		t.Fatalf("Connect() returned error: %v", err)
	}

	// Verify we can use the card
	resp, err := smartCard.Transmit([]byte{0xFF, 0xCA, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatalf("Transmit() returned error: %v", err)
	}

	if len(resp) < 3 {
		t.Fatal("response too short")
	}
}

func TestMockContext_ConnectNoCard(t *testing.T) {
	ctx := NewMockContext() // No cards added

	_, err := ctx.Connect("ACS ACR122U PICC Interface", 0, 0)
	if err == nil {
		t.Error("expected error when no card present, got nil")
	}
}

func TestMockContext_WithError(t *testing.T) {
	ctx := NewMockContext().WithError("context error")

	_, err := ctx.ListReaders()
	if err == nil {
		t.Error("expected error from ListReaders(), got nil")
	}

	_, err = ctx.Connect("test", 0, 0)
	if err == nil {
		t.Error("expected error from Connect(), got nil")
	}
}

func TestMockCardOperations_GetCardUID(t *testing.T) {
	mockCard := &Card{
		UID:      "932bae0e",
		ATR:      "3b8f8001804f0ca000000306030001000000006a",
		Type:     "MIFARE Classic",
		Size:     1024,
		Writable: true,
	}

	ops := NewMockCardOperations().WithCard("reader1", mockCard)

	card, err := ops.GetCardUID("reader1")
	if err != nil {
		t.Fatalf("GetCardUID() returned error: %v", err)
	}

	if card.UID != "932bae0e" {
		t.Errorf("expected UID 932bae0e, got %s", card.UID)
	}
	if card.Type != "MIFARE Classic" {
		t.Errorf("expected type MIFARE Classic, got %s", card.Type)
	}
}

func TestMockCardOperations_GetCardUID_NoCard(t *testing.T) {
	ops := NewMockCardOperations()

	_, err := ops.GetCardUID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent reader, got nil")
	}
}

func TestMockCardOperations_WriteData(t *testing.T) {
	ops := NewMockCardOperations()

	data := []byte("test data")
	err := ops.WriteData("reader1", data, "text")
	if err != nil {
		t.Fatalf("WriteData() returned error: %v", err)
	}

	written := ops.GetWrittenData("reader1")
	if string(written) != "test data" {
		t.Errorf("expected 'test data', got '%s'", string(written))
	}
}

func TestMockCardOperations_EraseCard(t *testing.T) {
	ops := NewMockCardOperations()

	err := ops.EraseCard("reader1")
	if err != nil {
		t.Fatalf("EraseCard() returned error: %v", err)
	}

	if !ops.WasErased("reader1") {
		t.Error("card should be marked as erased")
	}
}

func TestMockCardOperations_LockCard(t *testing.T) {
	ops := NewMockCardOperations()

	err := ops.LockCard("reader1")
	if err != nil {
		t.Fatalf("LockCard() returned error: %v", err)
	}

	if !ops.WasLocked("reader1") {
		t.Error("card should be marked as locked")
	}
}

func TestMockCardOperations_SetPassword(t *testing.T) {
	ops := NewMockCardOperations()

	password := []byte{0x01, 0x02, 0x03, 0x04}
	pack := []byte{0xAB, 0xCD}
	err := ops.SetPassword("reader1", password, pack, 4)
	if err != nil {
		t.Fatalf("SetPassword() returned error: %v", err)
	}

	storedPwd := ops.GetPassword("reader1")
	if len(storedPwd) != 4 {
		t.Errorf("expected 4 byte password, got %d bytes", len(storedPwd))
	}
}

func TestMockCardOperations_RemovePassword(t *testing.T) {
	ops := NewMockCardOperations()

	// Set password first
	ops.SetPassword("reader1", []byte{0x01, 0x02, 0x03, 0x04}, []byte{0xAB, 0xCD}, 4)

	// Remove password
	err := ops.RemovePassword("reader1", []byte{0x01, 0x02, 0x03, 0x04})
	if err != nil {
		t.Fatalf("RemovePassword() returned error: %v", err)
	}

	storedPwd := ops.GetPassword("reader1")
	if storedPwd != nil {
		t.Error("password should be removed")
	}
}

func TestMockCardOperations_WriteMultipleRecords(t *testing.T) {
	ops := NewMockCardOperations()

	records := []NDEFRecord{
		{Type: "url", Data: "https://example.com"},
		{Type: "text", Data: "Hello"},
	}

	err := ops.WriteMultipleRecords("reader1", records)
	if err != nil {
		t.Fatalf("WriteMultipleRecords() returned error: %v", err)
	}

	written := ops.GetWrittenRecords("reader1")
	if len(written) != 2 {
		t.Errorf("expected 2 records, got %d", len(written))
	}
}

func TestMockCardOperations_WithError(t *testing.T) {
	ops := NewMockCardOperations().WithError("operation failed")

	_, err := ops.GetCardUID("reader1")
	if err == nil {
		t.Error("expected error, got nil")
	}

	err = ops.WriteData("reader1", []byte("data"), "text")
	if err == nil {
		t.Error("expected error, got nil")
	}

	err = ops.EraseCard("reader1")
	if err == nil {
		t.Error("expected error, got nil")
	}

	err = ops.LockCard("reader1")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestMockReaderOperations(t *testing.T) {
	readers := []Reader{
		{ID: "reader-0", Name: "ACS ACR122U PICC Interface", Type: "picc"},
		{ID: "reader-1", Name: "ACS ACR1252 Dual Reader PICC", Type: "picc"},
	}

	ops := NewMockReaderOperations(readers)

	result := ops.ListReaders()
	if len(result) != 2 {
		t.Errorf("expected 2 readers, got %d", len(result))
	}

	if result[0].Name != "ACS ACR122U PICC Interface" {
		t.Errorf("unexpected first reader name: %s", result[0].Name)
	}
}

// Benchmark mocks
func BenchmarkMockCardTransmit(b *testing.B) {
	card := NewMockCard("NTAG213")
	cmd := []byte{0xFF, 0xCA, 0x00, 0x00, 0x00}

	for i := 0; i < b.N; i++ {
		card.Transmit(cmd)
	}
}

func BenchmarkMockContextConnect(b *testing.B) {
	card := NewMockCard("NTAG213")
	ctx := NewMockContext().WithCard("reader", card)

	for i := 0; i < b.N; i++ {
		ctx.Connect("reader", 0, 0)
	}
}
