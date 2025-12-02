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

	// Simulate what happens in the real code: try Method 1a first
	fmt.Println("=== Method 1a (ACR1552U format) ===")
	getVersionCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x02, 0x60, 0x00}
	fmt.Printf("Sending: %s\n", hex.EncodeToString(getVersionCmd))
	rsp, err := card.Transmit(getVersionCmd)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Response: %s\n", hex.EncodeToString(rsp))
	}

	// Now try Method 1b immediately after (like in the code)
	fmt.Println("\n=== Method 1b (ACR1252U format) - IMMEDIATELY AFTER ===")
	getVersionCmd2 := []byte{0xFF, 0x00, 0x00, 0x00, 0x01, 0x60}
	fmt.Printf("Sending: %s\n", hex.EncodeToString(getVersionCmd2))
	rsp, err = card.Transmit(getVersionCmd2)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Response: %s\n", hex.EncodeToString(rsp))
		if len(rsp) >= 2 {
			fmt.Printf("Status: %02X %02X\n", rsp[len(rsp)-2], rsp[len(rsp)-1])
		}
	}
}
