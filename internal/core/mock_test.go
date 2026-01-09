package core

import (
	"encoding/hex"
	"errors"
	"sync"
)

// MockSmartCardContext implements SmartCardContext for testing
type MockSmartCardContext struct {
	readers     []string
	cards       map[string]*MockSmartCard
	shouldError bool
	errorMsg    string
}

// MockSmartCard implements SmartCard for testing
type MockSmartCard struct {
	mu           sync.Mutex
	atr          []byte
	uid          []byte
	cardType     string
	ndefData     []byte
	responses    map[string][]byte // command hex -> response
	shouldError  bool
	errorMsg     string
	disconnected bool
}

// NewMockContext creates a new mock context with predefined readers
func NewMockContext() *MockSmartCardContext {
	return &MockSmartCardContext{
		readers: []string{
			"ACS ACR122U PICC Interface",
			"ACS ACR1552 1S CL Reader PICC",
			"ACS ACR1252 Dual Reader PICC",
		},
		cards: make(map[string]*MockSmartCard),
	}
}

// WithReaders sets the readers for the mock context
func (m *MockSmartCardContext) WithReaders(readers []string) *MockSmartCardContext {
	m.readers = readers
	return m
}

// WithCard adds a mock card to a specific reader
func (m *MockSmartCardContext) WithCard(readerName string, card *MockSmartCard) *MockSmartCardContext {
	m.cards[readerName] = card
	return m
}

// WithError makes the context return errors
func (m *MockSmartCardContext) WithError(msg string) *MockSmartCardContext {
	m.shouldError = true
	m.errorMsg = msg
	return m
}

func (m *MockSmartCardContext) ListReaders() ([]string, error) {
	if m.shouldError {
		return nil, errors.New(m.errorMsg)
	}
	return m.readers, nil
}

func (m *MockSmartCardContext) Connect(reader string, shareMode uint32, protocol uint32) (SmartCard, error) {
	if m.shouldError {
		return nil, errors.New(m.errorMsg)
	}
	card, ok := m.cards[reader]
	if !ok {
		return nil, errors.New("no card present")
	}
	return card, nil
}

func (m *MockSmartCardContext) Release() error {
	return nil
}

// NewMockCard creates a mock card with realistic data
func NewMockCard(cardType string) *MockSmartCard {
	card := &MockSmartCard{
		responses: make(map[string][]byte),
	}

	switch cardType {
	case "MIFARE Classic":
		card.atr, _ = hex.DecodeString("3b8f8001804f0ca000000306030001000000006a")
		card.uid, _ = hex.DecodeString("932bae0e")
		card.cardType = "MIFARE Classic"
		card.setupMIFAREResponses()
	case "ISO 15693":
		card.atr, _ = hex.DecodeString("3b8f8001804f0ca0000003060b00140000000077")
		card.uid, _ = hex.DecodeString("80391566080104e0")
		card.cardType = "ISO 15693"
		card.setupISO15693Responses()
	case "NTAG213":
		card.atr, _ = hex.DecodeString("3b8f8001804f0ca0000003060300030000000068")
		card.uid, _ = hex.DecodeString("0442488a837280")
		card.cardType = "NTAG213"
		card.setupNTAG213Responses()
	case "NTAG215":
		// Real data from ACR1252 reader
		card.atr, _ = hex.DecodeString("3b8f8001804f0ca0000003060300030000000068")
		card.uid, _ = hex.DecodeString("04635d6bc22a81")
		card.cardType = "NTAG215"
		card.setupNTAG215Responses()
	case "NTAG216":
		// Real data from ACR122U reader
		card.atr, _ = hex.DecodeString("3b8f8001804f0ca0000003060300030000000068")
		card.uid, _ = hex.DecodeString("5397e01aa20001")
		card.cardType = "NTAG216"
		card.setupNTAG216Responses()
	case "MIFARE Ultralight":
		// Real data from ACR1552 reader - plain Ultralight does NOT support GET_VERSION
		card.atr, _ = hex.DecodeString("3b8f8001804f0ca0000003060300030000000068")
		card.uid, _ = hex.DecodeString("ff0f39c8d60000")
		card.cardType = "MIFARE Ultralight"
		card.setupMIFAREUltralightResponses()
	case "NTAG216-MemoryProbe":
		// Simulates NTAG216 where GET_VERSION fails but memory probe succeeds
		// This tests the fallback detection path
		card.atr, _ = hex.DecodeString("3b8f8001804f0ca0000003060300030000000068")
		card.uid, _ = hex.DecodeString("04eeed91ca2a81") // Real NTAG216 UID from customer
		card.cardType = "NTAG216"
		card.setupNTAG216MemoryProbeResponses()
	case "NTAG215-MemoryProbe":
		// Simulates NTAG215 where GET_VERSION fails but memory probe succeeds
		card.atr, _ = hex.DecodeString("3b8f8001804f0ca0000003060300030000000068")
		card.uid, _ = hex.DecodeString("045d4737c62a81") // Real NTAG215 UID
		card.cardType = "NTAG215"
		card.setupNTAG215MemoryProbeResponses()
	default:
		card.atr, _ = hex.DecodeString("3b8f8001804f0ca0000003060300030000000068")
		card.uid, _ = hex.DecodeString("04000000000000")
		card.cardType = "Unknown"
	}

	return card
}

func (m *MockSmartCard) setupMIFAREResponses() {
	// GET UID command response
	uidResponse := append(m.uid, 0x90, 0x00)
	m.responses["ffca000000"] = uidResponse

	// GET_VERSION fails on MIFARE Classic
	m.responses["ff00000002600"] = []byte{0x6A, 0x81} // Command not supported
}

func (m *MockSmartCard) setupISO15693Responses() {
	// GET UID command response
	uidResponse := append(m.uid, 0x90, 0x00)
	m.responses["ffca000000"] = uidResponse

	// GET_VERSION fails on ISO 15693
	m.responses["ff0000000260"] = []byte{0x6A, 0x81}
}

func (m *MockSmartCard) setupNTAG213Responses() {
	// GET UID command response
	uidResponse := append(m.uid, 0x90, 0x00)
	m.responses["ffca000000"] = uidResponse

	// GET_VERSION response for NTAG213 (storage size 0x0F)
	m.responses["ff000000026000"] = []byte{0x00, 0x04, 0x04, 0x02, 0x01, 0x00, 0x0F, 0x03, 0x90, 0x00}
	m.responses["ff0000000160"] = []byte{0x00, 0x04, 0x04, 0x02, 0x01, 0x00, 0x0F, 0x03, 0x90, 0x00}

	// Read page 3 (capability container) - NTAG213 has CC size 0x12
	m.responses["ffb0000310"] = []byte{0xE1, 0x10, 0x12, 0x00, 0x01, 0x03, 0xA0, 0x0C, 0x34, 0x03, 0x00, 0xFE, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}

	// Read page 1 (for CC detection method 2a)
	m.responses["ffb0000110"] = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xE1, 0x10, 0x12, 0x00, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}

	// Read pages 4+ for NDEF data (empty by default)
	m.responses["ffb0000440"] = append(make([]byte, 64), 0x90, 0x00)

	// Write responses (success)
	m.responses["ffd6000404"] = []byte{0x90, 0x00}
}

func (m *MockSmartCard) setupNTAG215Responses() {
	uidResponse := append(m.uid, 0x90, 0x00)
	m.responses["ffca000000"] = uidResponse

	// GET_VERSION response for NTAG215 (storage size 0x11)
	m.responses["ff000000026000"] = []byte{0x00, 0x04, 0x04, 0x02, 0x01, 0x00, 0x11, 0x03, 0x90, 0x00}
	m.responses["ff0000000160"] = []byte{0x00, 0x04, 0x04, 0x02, 0x01, 0x00, 0x11, 0x03, 0x90, 0x00}

	// CC with size 0x3E for NTAG215
	m.responses["ffb0000310"] = []byte{0xE1, 0x10, 0x3E, 0x00, 0x03, 0x00, 0xFE, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}
}

func (m *MockSmartCard) setupNTAG216Responses() {
	uidResponse := append(m.uid, 0x90, 0x00)
	m.responses["ffca000000"] = uidResponse

	// GET_VERSION response for NTAG216 (storage size 0x13)
	// Note: Some NTAG216 tags have vendor 0x53 instead of 0x04
	m.responses["ff000000026000"] = []byte{0x00, 0x53, 0x04, 0x02, 0x01, 0x00, 0x13, 0x03, 0x90, 0x00}
	m.responses["ff0000000160"] = []byte{0x00, 0x53, 0x04, 0x02, 0x01, 0x00, 0x13, 0x03, 0x90, 0x00}

	// Transparent exchange GET_VERSION (ACR1552) - real captured data
	m.responses["ffc2000002810"] = []byte{0xC0, 0x03, 0x00, 0x90, 0x00} // Start session OK
	m.responses["ffc20002048f020003"] = []byte{0xC0, 0x03, 0x00, 0x90, 0x00} // Set protocol OK
	m.responses["ffc2000103950160"] = []byte{0xC0, 0x03, 0x00, 0x90, 0x00, 0x92, 0x01, 0x00, 0x96, 0x02, 0x00, 0x00, 0x97, 0x08, 0x00, 0x53, 0x04, 0x02, 0x01, 0x00, 0x13, 0x03, 0x90, 0x00}
	m.responses["ffc2000002820"] = []byte{0xC0, 0x03, 0x00, 0x90, 0x00} // End session OK

	// CC with size 0x6D for NTAG216
	m.responses["ffb0000310"] = []byte{0xE1, 0x10, 0x6D, 0x00, 0x03, 0x00, 0xFE, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}
}

func (m *MockSmartCard) setupMIFAREUltralightResponses() {
	uidResponse := append(m.uid, 0x90, 0x00)
	m.responses["ffca000000"] = uidResponse

	// GET_VERSION fails on plain MIFARE Ultralight - returns 6900 (command not allowed)
	m.responses["ff000000026000"] = []byte{0x69, 0x00}
	m.responses["ff0000000160"] = []byte{0x69, 0x00}

	// Transparent exchange GET_VERSION returns garbage data (real captured data)
	// Plain Ultralight doesn't support GET_VERSION, so it returns random bytes
	m.responses["ffc2000002810"] = []byte{0xC0, 0x03, 0x00, 0x90, 0x00} // Start session OK
	m.responses["ffc20002048f020003"] = []byte{0xC0, 0x03, 0x00, 0x90, 0x00} // Set protocol OK
	// Garbage response: header=0x0f (invalid), vendor=0xff (invalid) - should be rejected
	m.responses["ffc2000103950160"] = []byte{0xC0, 0x03, 0x00, 0x90, 0x00, 0x92, 0x01, 0x00, 0x96, 0x02, 0x00, 0x00, 0x97, 0x08, 0x0F, 0xFF, 0x04, 0x02, 0x01, 0x00, 0x0F, 0x03, 0x90, 0x00}
	m.responses["ffc2000002820"] = []byte{0xC0, 0x03, 0x00, 0x90, 0x00} // End session OK

	// CC - plain Ultralight typically doesn't have NDEF formatted, or has invalid CC
	// This simulates a tag without valid NDEF magic byte
	m.responses["ffb0000310"] = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}
	m.responses["ffb0000110"] = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}
}

// setupNTAG216MemoryProbeResponses simulates an NTAG216 where GET_VERSION fails
// but memory probe succeeds (can read page 135). This tests the fallback detection.
func (m *MockSmartCard) setupNTAG216MemoryProbeResponses() {
	uidResponse := append(m.uid, 0x90, 0x00)
	m.responses["ffca000000"] = uidResponse

	// GET_VERSION fails with 6300 (simulates intermittent communication failure)
	m.responses["ff000000026000"] = []byte{0x63, 0x00}
	m.responses["ff0000000160"] = []byte{0x63, 0x00}

	// CC reading fails - no valid NDEF magic byte (simulates CC read failure)
	m.responses["ffb0000310"] = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}
	m.responses["ffb0000110"] = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}

	// Memory probe: page 135 succeeds (NTAG216 has 231 pages)
	// Read page 135: FF B0 00 87 04
	m.responses["ffb0008704"] = []byte{0x00, 0x00, 0x00, 0x00, 0x90, 0x00}
}

// setupNTAG215MemoryProbeResponses simulates an NTAG215 where GET_VERSION fails
// but memory probe succeeds (page 135 fails, page 45 succeeds).
func (m *MockSmartCard) setupNTAG215MemoryProbeResponses() {
	uidResponse := append(m.uid, 0x90, 0x00)
	m.responses["ffca000000"] = uidResponse

	// GET_VERSION fails with 6300
	m.responses["ff000000026000"] = []byte{0x63, 0x00}
	m.responses["ff0000000160"] = []byte{0x63, 0x00}

	// CC reading fails
	m.responses["ffb0000310"] = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}
	m.responses["ffb0000110"] = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x90, 0x00}

	// Memory probe: page 135 fails (NTAG215 only has 135 pages, 0-134)
	m.responses["ffb0008704"] = []byte{0x6A, 0x82} // Page out of range

	// Memory probe: page 45 succeeds (NTAG215 has pages 0-134)
	// Read page 45: FF B0 00 2D 04
	m.responses["ffb0002d04"] = []byte{0x00, 0x00, 0x00, 0x00, 0x90, 0x00}
}

// WithNDEFData sets NDEF data on the mock card
func (m *MockSmartCard) WithNDEFData(ndefType, data string) *MockSmartCard {
	m.mu.Lock()
	defer m.mu.Unlock()

	var ndefBytes []byte
	switch ndefType {
	case "text":
		// Create text NDEF record
		payload := []byte{0x02} // Language length
		payload = append(payload, []byte("en")...)
		payload = append(payload, []byte(data)...)
		ndefBytes = createNDEFRecord(0xD1, []byte("T"), payload, true, true)
	case "url":
		prefixCode, remainder := findURIPrefix(data)
		payload := []byte{prefixCode}
		payload = append(payload, []byte(remainder)...)
		ndefBytes = createNDEFRecord(0xD1, []byte("U"), payload, true, true)
	}

	m.ndefData = ndefBytes

	// Update read responses to return this NDEF data
	readResponse := make([]byte, 64)
	copy(readResponse, ndefBytes)
	readResponse = append(readResponse, 0x90, 0x00)
	m.responses["ffb0000440"] = readResponse

	return m
}

// WithError makes the card return errors
func (m *MockSmartCard) WithError(msg string) *MockSmartCard {
	m.shouldError = true
	m.errorMsg = msg
	return m
}

func (m *MockSmartCard) Transmit(cmd []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return nil, errors.New(m.errorMsg)
	}

	if m.disconnected {
		return nil, errors.New("card disconnected")
	}

	// Look up response by command hex
	cmdHex := hex.EncodeToString(cmd)

	// Try exact match first
	if resp, ok := m.responses[cmdHex]; ok {
		return resp, nil
	}

	// Try prefix match for commands with variable data
	for prefix, resp := range m.responses {
		if len(cmdHex) >= len(prefix) && cmdHex[:len(prefix)] == prefix {
			return resp, nil
		}
	}

	// Handle GET UID command
	if len(cmd) >= 5 && cmd[0] == 0xFF && cmd[1] == 0xCA && cmd[2] == 0x00 && cmd[3] == 0x00 {
		return append(m.uid, 0x90, 0x00), nil
	}

	// Handle write commands - return success
	if len(cmd) >= 5 && cmd[0] == 0xFF && cmd[1] == 0xD6 {
		return []byte{0x90, 0x00}, nil
	}

	// Handle read commands - return zeros with success
	if len(cmd) >= 5 && cmd[0] == 0xFF && cmd[1] == 0xB0 {
		length := int(cmd[4])
		resp := make([]byte, length)
		return append(resp, 0x90, 0x00), nil
	}

	// Default: command not supported
	return []byte{0x6A, 0x81}, nil
}

func (m *MockSmartCard) Status() (SmartCardStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return SmartCardStatus{}, errors.New(m.errorMsg)
	}

	return SmartCardStatus{
		Reader:         "Mock Reader",
		State:          0,
		ActiveProtocol: 1,
		Atr:            m.atr,
	}, nil
}

func (m *MockSmartCard) Disconnect(disposition uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disconnected = true
	return nil
}

// MockReaderOperations implements ReaderOperations for testing
type MockReaderOperations struct {
	readers []Reader
}

func NewMockReaderOperations(readers []Reader) *MockReaderOperations {
	return &MockReaderOperations{readers: readers}
}

func (m *MockReaderOperations) ListReaders() []Reader {
	return m.readers
}

// MockCardOperations implements CardOperations for testing
type MockCardOperations struct {
	mu            sync.Mutex
	cards         map[string]*Card
	writeData     map[string][]byte
	erased        map[string]bool
	locked        map[string]bool
	passwords     map[string][]byte
	shouldError   bool
	errorMsg      string
	writeRecords  map[string][]NDEFRecord
}

func NewMockCardOperations() *MockCardOperations {
	return &MockCardOperations{
		cards:        make(map[string]*Card),
		writeData:    make(map[string][]byte),
		erased:       make(map[string]bool),
		locked:       make(map[string]bool),
		passwords:    make(map[string][]byte),
		writeRecords: make(map[string][]NDEFRecord),
	}
}

func (m *MockCardOperations) WithCard(readerName string, card *Card) *MockCardOperations {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cards[readerName] = card
	return m
}

func (m *MockCardOperations) WithError(msg string) *MockCardOperations {
	m.shouldError = true
	m.errorMsg = msg
	return m
}

func (m *MockCardOperations) GetCardUID(readerName string) (*Card, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return nil, errors.New(m.errorMsg)
	}

	card, ok := m.cards[readerName]
	if !ok {
		return nil, errors.New("no card present")
	}
	return card, nil
}

func (m *MockCardOperations) WriteData(readerName string, data []byte, dataType string) error {
	return m.WriteDataWithURL(readerName, data, dataType, "")
}

func (m *MockCardOperations) WriteDataWithURL(readerName string, data []byte, dataType string, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return errors.New(m.errorMsg)
	}

	m.writeData[readerName] = data
	return nil
}

func (m *MockCardOperations) EraseCard(readerName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return errors.New(m.errorMsg)
	}

	m.erased[readerName] = true
	return nil
}

func (m *MockCardOperations) LockCard(readerName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return errors.New(m.errorMsg)
	}

	m.locked[readerName] = true
	return nil
}

func (m *MockCardOperations) SetPassword(readerName string, password []byte, pack []byte, startPage byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return errors.New(m.errorMsg)
	}

	m.passwords[readerName] = password
	return nil
}

func (m *MockCardOperations) RemovePassword(readerName string, password []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return errors.New(m.errorMsg)
	}

	delete(m.passwords, readerName)
	return nil
}

func (m *MockCardOperations) WriteMultipleRecords(readerName string, records []NDEFRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return errors.New(m.errorMsg)
	}

	m.writeRecords[readerName] = records
	return nil
}

// Helper methods for assertions in tests
func (m *MockCardOperations) GetWrittenData(readerName string) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeData[readerName]
}

func (m *MockCardOperations) WasErased(readerName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.erased[readerName]
}

func (m *MockCardOperations) WasLocked(readerName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.locked[readerName]
}

func (m *MockCardOperations) GetPassword(readerName string) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.passwords[readerName]
}

func (m *MockCardOperations) GetWrittenRecords(readerName string) []NDEFRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeRecords[readerName]
}
