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

	// Test the alternative GET_VERSION
	getVersionCmd2 := []byte{0xFF, 0x00, 0x00, 0x00, 0x01, 0x60}
	fmt.Printf("Sending: %s\n", hex.EncodeToString(getVersionCmd2))

	rsp, err := card.Transmit(getVersionCmd2)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Response (%d bytes): %s\n", len(rsp), hex.EncodeToString(rsp))

	if len(rsp) >= 2 {
		sw1 := rsp[len(rsp)-2]
		sw2 := rsp[len(rsp)-1]
		fmt.Printf("Status: %02X %02X\n", sw1, sw2)
	}

	if len(rsp) >= 10 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		fmt.Println("\nCondition check: PASSED")
		fmt.Printf("Byte at index 6: %02X\n", rsp[6])

		storageSize := rsp[6]
		switch storageSize {
		case 0x0F:
			fmt.Println("Detected: NTAG213")
		case 0x11:
			fmt.Println("Detected: NTAG215")
		case 0x13:
			fmt.Println("Detected: NTAG216")
		default:
			fmt.Printf("Unknown storage size: %02X\n", storageSize)
		}
	} else {
		fmt.Println("\nCondition check: FAILED")
		fmt.Printf("len(rsp) = %d (need >= 10)\n", len(rsp))
		if len(rsp) >= 2 {
			fmt.Printf("rsp[len-2] = %02X (need 0x90)\n", rsp[len(rsp)-2])
			fmt.Printf("rsp[len-1] = %02X (need 0x00)\n", rsp[len(rsp)-1])
		}
	}
}
