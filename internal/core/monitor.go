package core

import (
	"strings"
	"time"

	"github.com/SimplyPrint/nfc-agent/internal/logging"
	"github.com/ebfe/scard"
)

// pnpNotificationReader is the special PC/SC pseudo-reader that reports changes
// to the *set* of connected readers (arrival/removal). Watching it with
// SCardGetStatusChange lets us react to readers appearing after startup without
// busy-polling. Supported by both winscard (Windows) and pcsc-lite (macOS/Linux).
const pnpNotificationReader = `\\?PnP?\Notification`

// Monitor tuning. The finite watch timeout is deliberate: it bounds how long we
// block on the PnP reader so that (a) non-PC/SC readers (Proxmark3) and platforms
// where PnP notifications are unsupported still get picked up by the periodic
// re-list, and (b) a stop signal is honoured promptly without needing to cancel
// the in-flight PC/SC call.
const (
	readerWatchTimeout   = 2 * time.Second
	readerMonitorBackoff = 3 * time.Second
)

// readerSetKey returns a stable string identifying the current set of readers,
// used to detect whether the reader list actually changed between polls.
func readerSetKey(readers []Reader) string {
	if len(readers) == 0 {
		return ""
	}
	parts := make([]string, len(readers))
	for i, r := range readers {
		parts[i] = r.Name + "|" + r.Type
	}
	return strings.Join(parts, "\x00")
}

// MonitorReaders blocks, watching for NFC reader arrival and removal, and calls
// onChange with the current reader list whenever the set of readers changes
// (including the first successful enumeration). It returns when stop is closed;
// pass nil to run for the lifetime of the process.
//
// It is resilient to the PC/SC daemon being unavailable at startup — the common
// failure on socket-activated pcscd (Debian Trixie, recent Raspberry Pi OS) and
// when a USB reader is enumerated by the kernel after the agent has already
// started. When the context can't be established it reports an empty reader list
// and retries with backoff, so the agent self-heals on a cold boot instead of
// being stuck at "0 readers" until restarted.
func MonitorReaders(onChange func([]Reader), stop <-chan struct{}) {
	defer logging.RecoverAndLog("ReaderMonitor", false)

	lastKey := "\x00uninitialized" // sentinel that never matches a real key, so the first emit always fires
	emit := func(readers []Reader) {
		key := readerSetKey(readers)
		if key == lastKey {
			return
		}
		lastKey = key
		logging.Info(logging.CatReader, "Reader set changed", map[string]any{
			"count": len(readers),
		})
		onChange(readers)
	}

	for {
		if stopped(stop) {
			return
		}

		ctx, err := scard.EstablishContext()
		if err != nil {
			// pcscd not up yet (socket-activated / not installed) or no driver.
			// Reflect "no readers" and retry — this is the cold-boot self-heal path.
			logging.Debug(logging.CatReader, "Reader monitor: PC/SC unavailable, will retry", map[string]any{
				"error": err.Error(),
			})
			emit(nil)
			if sleepOrStop(stop, readerMonitorBackoff) {
				return
			}
			continue
		}

		// Watch with this context until it breaks (pcscd restart/death) or we stop.
		stopRequested := watchReaders(ctx, stop, emit)
		ctx.Release()
		if stopRequested {
			return
		}
		// Context went bad; back off before re-establishing.
		if sleepOrStop(stop, readerMonitorBackoff) {
			return
		}
	}
}

// watchReaders runs the inner watch loop against an established context. It
// returns true if a stop was requested, or false if the context became invalid
// and the caller should re-establish it.
func watchReaders(ctx *scard.Context, stop <-chan struct{}, emit func([]Reader)) (stopRequested bool) {
	// Emit the current state immediately on (re)connect.
	emit(ListReaders())

	rs := []scard.ReaderState{{
		Reader:       pnpNotificationReader,
		CurrentState: scard.StateUnaware,
	}}

	for {
		if stopped(stop) {
			return true
		}

		err := ctx.GetStatusChange(rs, readerWatchTimeout)
		switch err {
		case nil:
			// The PnP reader signalled a change to the reader set.
			emit(ListReaders())
			// Acknowledge the new state so the next call blocks until the *next* change.
			rs[0].CurrentState = rs[0].EventState &^ scard.StateChanged

		case scard.ErrTimeout:
			// No PnP event within the window. Re-list anyway so Proxmark3 and any
			// platform without PnP support still converge (degrades to polling).
			emit(ListReaders())

		case scard.ErrCancelled, scard.ErrSystemCancelled, scard.ErrCancelledByUser:
			return true

		case scard.ErrNoService, scard.ErrServiceStopped, scard.ErrInvalidHandle:
			// The daemon/context is gone — re-establish from scratch.
			logging.Debug(logging.CatReader, "Reader monitor: PC/SC context lost, reconnecting", map[string]any{
				"error": err.Error(),
			})
			return false

		default:
			// e.g. ErrNoReadersAvailable / ErrReaderUnavailable, or an immediate-return
			// error. Re-list (usually empty) and pace ourselves to avoid a busy loop.
			emit(ListReaders())
			if sleepOrStop(stop, readerWatchTimeout) {
				return true
			}
		}
	}
}

// stopped reports whether the stop channel has been closed (non-blocking).
func stopped(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

// sleepOrStop waits for d, or returns true early if stop is closed.
func sleepOrStop(stop <-chan struct{}, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-stop:
		return true
	case <-t.C:
		return false
	}
}
