package core

import (
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/SimplyPrint/nfc-agent/internal/logging"
	"github.com/ebfe/scard"
)

// DesfireError is a typed error for DESFire transparent-session operations.
//
// Status carries the DESFire/ISO status word returned by the card (e.g. 0x91AF
// "additional frame", 0x911C "illegal command") when one is applicable, so SDK
// consumers can branch on it. It is 0 for transport/reader errors that never
// produced a card status.
type DesfireError struct {
	Msg    string
	Status uint16
}

func (e *DesfireError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s (status 0x%04X)", e.Msg, e.Status)
	}
	return e.Msg
}

// DesfireSession is a transparent APDU pipe to a single card on one reader. It
// holds the PC/SC connection open across many Transmit calls so a stateful,
// interactive protocol — DESFire AuthenticateEV2First and the secure-messaging
// commands that follow — can be driven turn-by-turn by an external party (e.g.
// the SimplyPrint backend, which holds the keys in its HSM).
//
// Deliberately, the agent performs NO DESFire cryptography and holds NO keys.
// It forwards the caller's APDU bytes verbatim and returns the card's raw
// response. All session secrets (transaction id, command counter, session
// keys) live with whoever drives the handshake — never here, never on disk,
// never in the logs. This keeps DESFire support aligned with the project's
// "expose raw access, don't bake in app logic" philosophy and means there is
// no hand-rolled secure messaging in the agent to get wrong.
type DesfireSession struct {
	readerName string
	ctx        *scard.Context
	card       *scard.Card
	mu         sync.Mutex
	closed     bool

	// UID is read once at open via the standard FF CA pseudo-APDU. It may be a
	// random UID on privacy-enabled cards; the real UID is obtained by the
	// driver post-authentication and is not the agent's concern.
	UID string
	// ATR is the card's answer-to-reset (hex), best-effort.
	ATR string
}

// OpenDesfireSession establishes a held PC/SC connection to the card on the
// given reader and reads its UID + ATR. It does not authenticate or send any
// DESFire command — that is the caller's job via Transmit. The session must be
// closed with Close to release the reader.
func OpenDesfireSession(readerName string) (*DesfireSession, error) {
	// Transparent sessions are a PC/SC-only feature. Proxmark3 readers are
	// driven through a separate path and are not supported here.
	if IsProxmark3Reader(readerName) {
		return nil, &DesfireError{Msg: "reader does not support DESFire transparent sessions"}
	}

	// Serialize the open against any in-flight detection on this reader.
	rmu := getReaderMutex(readerName)
	rmu.Lock()
	defer rmu.Unlock()

	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, &DesfireError{Msg: fmt.Sprintf("failed to establish PC/SC context: %v", err)}
	}

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		ctx.Release()
		return nil, &DesfireError{Msg: fmt.Sprintf("failed to connect to reader (is a card present?): %v", err)}
	}

	s := &DesfireSession{readerName: readerName, ctx: ctx, card: card}

	if status, err := card.Status(); err == nil {
		s.ATR = hex.EncodeToString(status.Atr)
	}
	// FF CA 00 00 00 — get UID. Same pseudo-APDU used throughout the agent.
	// Non-fatal on failure (random-UID cards mask it).
	if rsp, err := card.Transmit([]byte{0xFF, 0xCA, 0x00, 0x00, 0x00}); err == nil && len(rsp) >= 2 {
		if rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
			s.UID = hex.EncodeToString(rsp[:len(rsp)-2])
		}
	}

	logging.Info(logging.CatCard, "DESFire session opened", map[string]any{
		"reader": readerName,
		"uid":    s.UID,
	})
	return s, nil
}

// Transmit forwards a single raw APDU to the card and returns the full raw
// response, including the trailing status word. The agent does not interpret
// the APDU or its response.
//
// A non-nil error means the byte exchange itself failed (transport/reader);
// a card error *status* (e.g. 0x91xx) is a valid response and is returned to
// the caller to interpret — it is not an error here.
func (s *DesfireSession) Transmit(apdu []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, &DesfireError{Msg: "DESFire session is closed"}
	}

	// Serialize each exchange against the reader's other PC/SC users (e.g. the
	// subscription polling loop) for the duration of this single transmit.
	rmu := getReaderMutex(s.readerName)
	rmu.Lock()
	rsp, err := s.card.Transmit(apdu)
	rmu.Unlock()
	if err != nil {
		return nil, &DesfireError{Msg: fmt.Sprintf("APDU transmit failed: %v", err)}
	}
	return rsp, nil
}

// Close disconnects the card and releases the PC/SC context. Safe to call more
// than once.
func (s *DesfireSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.card != nil {
		s.card.Disconnect(scard.LeaveCard)
	}
	if s.ctx != nil {
		s.ctx.Release()
	}
	logging.Info(logging.CatCard, "DESFire session closed", map[string]any{
		"reader": s.readerName,
	})
}

// SplitStatusWord returns the trailing SW1/SW2 of an APDU response, or
// ok=false if the response is too short to contain a status word.
func SplitStatusWord(rsp []byte) (sw1 byte, sw2 byte, ok bool) {
	if len(rsp) < 2 {
		return 0, 0, false
	}
	return rsp[len(rsp)-2], rsp[len(rsp)-1], true
}
