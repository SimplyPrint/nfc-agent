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
)

func listCustomGroupReaders() []string {
	var context uintptr
	result, _, _ := procEstablishCtx.Call(2, 0, 0, uintptr(unsafe.Pointer(&context)))
	if result != 0 {
		return nil
	}
	defer procReleaseCtx.Call(context)

	group, err := syscall.UTF16FromString("NavyNFCReaders")
	if err != nil {
		return nil
	}

	var bufferLength uint32
	result, _, _ = procListReaders.Call(
		context,
		uintptr(unsafe.Pointer(&group[0])),
		0,
		uintptr(unsafe.Pointer(&bufferLength)),
	)
	if result != 0x80100008 || bufferLength == 0 {
		return nil
	}

	buffer := make([]uint16, bufferLength)
	result, _, _ = procListReaders.Call(
		context,
		uintptr(unsafe.Pointer(&group[0])),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&bufferLength)),
	)
	if result != 0 {
		return nil
	}

	var readers []string
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
	return readers
}
