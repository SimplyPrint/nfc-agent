package core

// SmartCardContext represents a PC/SC context for listing readers
type SmartCardContext interface {
	ListReaders() ([]string, error)
	Connect(reader string, shareMode uint32, protocol uint32) (SmartCard, error)
	Release() error
}

// SmartCard represents a connected smart card for transmitting commands
type SmartCard interface {
	Transmit(cmd []byte) ([]byte, error)
	Status() (SmartCardStatus, error)
	Disconnect(disposition uint32) error
}

// SmartCardStatus represents the status of a smart card
type SmartCardStatus struct {
	Reader         string
	State          uint32
	ActiveProtocol uint32
	Atr            []byte
}

// ContextFactory creates SmartCardContext instances
// This allows for dependency injection and mocking in tests
type ContextFactory interface {
	EstablishContext() (SmartCardContext, error)
}

// DefaultContextFactory is the production factory that uses real PC/SC
type DefaultContextFactory struct{}

// cardOperations holds the operations that can be mocked for testing
// This is package-level to allow tests to override behavior
var cardOps = &realCardOperations{}

// CardOperations defines the interface for card-related operations
type CardOperations interface {
	GetCardUID(readerName string) (*Card, error)
	WriteData(readerName string, data []byte, dataType string) error
	WriteDataWithURL(readerName string, data []byte, dataType string, url string) error
	EraseCard(readerName string) error
	LockCard(readerName string) error
	SetPassword(readerName string, password []byte, pack []byte, startPage byte) error
	RemovePassword(readerName string, password []byte) error
	WriteMultipleRecords(readerName string, records []NDEFRecord) error
}

// ReaderOperations defines the interface for reader-related operations
type ReaderOperations interface {
	ListReaders() []Reader
}

// realCardOperations implements CardOperations using actual hardware
type realCardOperations struct{}

func (r *realCardOperations) GetCardUID(readerName string) (*Card, error) {
	return GetCardUID(readerName)
}

func (r *realCardOperations) WriteData(readerName string, data []byte, dataType string) error {
	return WriteData(readerName, data, dataType)
}

func (r *realCardOperations) WriteDataWithURL(readerName string, data []byte, dataType string, url string) error {
	return WriteDataWithURL(readerName, data, dataType, url)
}

func (r *realCardOperations) EraseCard(readerName string) error {
	return EraseCard(readerName)
}

func (r *realCardOperations) LockCard(readerName string) error {
	return LockCard(readerName)
}

func (r *realCardOperations) SetPassword(readerName string, password []byte, pack []byte, startPage byte) error {
	return SetPassword(readerName, password, pack, startPage)
}

func (r *realCardOperations) RemovePassword(readerName string, password []byte) error {
	return RemovePassword(readerName, password)
}

func (r *realCardOperations) WriteMultipleRecords(readerName string, records []NDEFRecord) error {
	return WriteMultipleRecords(readerName, records)
}
