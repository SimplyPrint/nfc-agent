package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"time"

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

	// Try Method 1a
	fmt.Println("=== Method 1a ===")
	rsp, _ := card.Transmit([]byte{0xFF, 0x00, 0x00, 0x00, 0x02, 0x60, 0x00})
	fmt.Printf("Response: %s\n", hex.EncodeToString(rsp))

	// Try with delay
	fmt.Println("\n=== Method 1b with 100ms delay ===")
	time.Sleep(100 * time.Millisecond)
	rsp, _ = card.Transmit([]byte{0xFF, 0x00, 0x00, 0x00, 0x01, 0x60})
	fmt.Printf("Response: %s\n", hex.EncodeToString(rsp))

	// Try reconnecting
	fmt.Println("\n=== Disconnect and reconnect ===")
	card.Disconnect(scard.ResetCard)
	card, err = ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		log.Fatalf("Failed to reconnect: %v", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	fmt.Println("\n=== Method 1b after reconnect ===")
	rsp, _ = card.Transmit([]byte{0xFF, 0x00, 0x00, 0x00, 0x01, 0x60})
	fmt.Printf("Response: %s\n", hex.EncodeToString(rsp))

	// Try different approach: swap order
	fmt.Println("\n=== Fresh connection, try 1b FIRST ===")
	card.Disconnect(scard.ResetCard)
	card, err = ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		log.Fatalf("Failed to reconnect: %v", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	rsp, _ = card.Transmit([]byte{0xFF, 0x00, 0x00, 0x00, 0x01, 0x60})
	fmt.Printf("Response: %s\n", hex.EncodeToString(rsp))
}
