package main

import (
	"encoding/hex"
	"fmt"
	"log"

	"github.com/ebfe/scard"
)

func main() {
	ctx, err := scard.EstablishContext()
	if err != nil {
		log.Fatal(err)
	}
	defer ctx.Release()

	readers, err := ctx.ListReaders()
	if err != nil {
		log.Fatal(err)
	}

	// Find ACR1252U
	var readerName string
	for _, name := range readers {
		if name == "ACS ACR1252 Dual Reader PICC" {
			readerName = name
			break
		}
	}

	if readerName == "" {
		log.Fatal("ACR1252U not found")
	}

	fmt.Printf("Connecting to: %s\n\n", readerName)

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Try different commands to see which ones work on ACR1252U

	fmt.Println("=== Testing GET_VERSION command ===")
	testCommand(card, "GET_VERSION", []byte{0xFF, 0x00, 0x00, 0x00, 0x02, 0x60, 0x00})

	fmt.Println("\n=== Testing READ CC (page 3) ===")
	testCommand(card, "READ CC", []byte{0xFF, 0xB0, 0x00, 0x03, 0x10})

	fmt.Println("\n=== Testing direct GET_VERSION (no wrapper) ===")
	testCommand(card, "Direct GET_VERSION", []byte{0x60})

	fmt.Println("\n=== Testing READ page 0 ===")
	testCommand(card, "READ page 0", []byte{0xFF, 0xB0, 0x00, 0x00, 0x10})

	fmt.Println("\n=== Testing READ page 1 ===")
	testCommand(card, "READ page 1", []byte{0xFF, 0xB0, 0x00, 0x01, 0x10})

	fmt.Println("\n=== Testing READ page 2 ===")
	testCommand(card, "READ page 2", []byte{0xFF, 0xB0, 0x00, 0x02, 0x10})

	fmt.Println("\n=== Testing alternative GET_VERSION wrapper ===")
	testCommand(card, "Alt GET_VERSION", []byte{0xFF, 0x00, 0x00, 0x00, 0x01, 0x60})

	fmt.Println("\n=== Testing READ with extended length ===")
	testCommand(card, "READ extended", []byte{0xFF, 0xB0, 0x00, 0x03, 0x20})
}

func testCommand(card *scard.Card, name string, cmd []byte) {
	fmt.Printf("%s: %s\n", name, hex.EncodeToString(cmd))
	rsp, err := card.Transmit(cmd)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}

	fmt.Printf("  Response (%d bytes): %s\n", len(rsp), hex.EncodeToString(rsp))
	if len(rsp) >= 2 {
		sw1 := rsp[len(rsp)-2]
		sw2 := rsp[len(rsp)-1]
		fmt.Printf("  Status: %02X %02X", sw1, sw2)
		if sw1 == 0x90 && sw2 == 0x00 {
			fmt.Printf(" (SUCCESS)")
		} else {
			fmt.Printf(" (FAILED)")
		}
		fmt.Println()
	}
}
