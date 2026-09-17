//go:build windows

package core

import (
	"syscall"
	"unsafe"
)

var (
	modWinSCard      = syscall.NewLazyDLL("winscard.dll")
	procEstablishCtx = modWinSCard.NewProc("SCardEstablishContext")
	procReleaseCtx   = modWinSCard.NewProc("SCardReleaseContext")
	procListReaders  = modWinSCard.NewProc("SCardListReadersW")
	procConnect      = modWinSCard.NewProc("SCardConnectW")
	procDisconnect   = modWinSCard.NewProc("SCardDisconnect")
)

func listCustomGroupReaders() []string {
	var context uintptr
	result, _, _ := procEstablishCtx.Call(0, 0, 0, uintptr(unsafe.Pointer(&context)))
	if result != 0 {
		return nil
	}
	defer procReleaseCtx.Call(context)

	group, err := syscall.UTF16FromString("NavyNFCReaders")
	if err != nil {
		return nil
	}

	readers := []string{}
	var bufferLength uint32
	result, _, _ = procListReaders.Call(
		context,
		uintptr(unsafe.Pointer(&group[0])),
		0,
		uintptr(unsafe.Pointer(&bufferLength)),
	)
	if result == 0x80100008 && bufferLength > 0 {
		buffer := make([]uint16, bufferLength)
		result, _, _ = procListReaders.Call(
			context,
			uintptr(unsafe.Pointer(&group[0])),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(unsafe.Pointer(&bufferLength)),
		)
		if result == 0 {
			for start := 0; start < len(buffer); {
				end := start
				for end < len(buffer) && buffer[end] != 0 {
					end++
				}
				if end == start {
					break
				}
				readers = append(readers, syscall.UTF16ToString(buffer[start:end]))
				start = end + 1
			}
		}
	}

	if len(readers) == 0 {
		reader, err := syscall.UTF16FromString("ASK LoGO 0")
		if err == nil {
			var card uintptr
			var protocol uint32
			result, _, _ = procConnect.Call(
				context,
				uintptr(unsafe.Pointer(&reader[0])),
				2,
				3,
				uintptr(unsafe.Pointer(&card)),
				uintptr(unsafe.Pointer(&protocol)),
			)
			if result == 0 {
				procDisconnect.Call(card, 0)
				readers = append(readers, "ASK LoGO 0")
			}
		}
	}
	return readers
}
