package proxmark3

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
)

// GetCardInfo reads card UID and type information
// Uses fast "reader" commands instead of "info" for better performance (~1.3s vs ~6s)
// Tries ISO 15693 first (faster, common for industrial tags), then falls back to ISO 14443A
func (c *Client) GetCardInfo(ctx context.Context) (*CardInfo, error) {
	// Try ISO 15693 first - it's faster (~1.3s) and common for industrial NFC tags
	output15, err15 := c.Execute(ctx, "hf 15 reader")
	if err15 == nil {
		if info, parseErr := ParseHF15Info(output15); parseErr == nil {
			return info, nil
		}
	}

	// Fall back to ISO 14443A (MIFARE, NTAG, etc.) - takes ~2.6s if no card
	output14a, err14a := c.Execute(ctx, "hf 14a reader")
	if err14a == nil {
		if info, parseErr := ParseHF14AInfo(output14a); parseErr == nil {
			return info, nil
		}
	}

	// Both protocols failed
	if err15 != nil && err14a != nil {
		return nil, fmt.Errorf("failed to get card info: %w", err15)
	}

	return nil, fmt.Errorf("failed to get card info: %w", ErrNoCard)
}

// ReadMifareBlock reads a 16-byte block from a MIFARE Classic card
// block: block number (0-63 for 1K, 0-255 for 4K)
// key: 6-byte authentication key
// keyType: 'A' or 'B' for key type
func (c *Client) ReadMifareBlock(ctx context.Context, block int, key []byte, keyType byte) ([]byte, error) {
	if len(key) != 6 {
		return nil, fmt.Errorf("key must be 6 bytes, got %d", len(key))
	}

	keyHex := hex.EncodeToString(key)
	keyFlag := "-a"
	if keyType == 'B' || keyType == 'b' {
		keyFlag = "-b"
	}

	cmd := fmt.Sprintf("hf mf rdbl --blk %d -k %s %s", block, keyHex, keyFlag)
	output, err := c.Execute(ctx, cmd)
	if err != nil {
		return nil, err
	}

	return ParseBlockData(output)
}

// WriteMifareBlock writes 16 bytes to a MIFARE Classic block
// block: block number (0-63 for 1K, 0-255 for 4K)
// data: 16 bytes to write
// key: 6-byte authentication key
// keyType: 'A' or 'B' for key type
func (c *Client) WriteMifareBlock(ctx context.Context, block int, data []byte, key []byte, keyType byte) error {
	if len(data) != 16 {
		return fmt.Errorf("data must be 16 bytes, got %d", len(data))
	}
	if len(key) != 6 {
		return fmt.Errorf("key must be 6 bytes, got %d", len(key))
	}

	dataHex := hex.EncodeToString(data)
	keyHex := hex.EncodeToString(key)
	keyFlag := "-a"
	if keyType == 'B' || keyType == 'b' {
		keyFlag = "-b"
	}

	cmd := fmt.Sprintf("hf mf wrbl --blk %d -k %s %s -d %s", block, keyHex, keyFlag, dataHex)
	output, err := c.Execute(ctx, cmd)
	if err != nil {
		return err
	}

	if !IsWriteSuccess(output) {
		return fmt.Errorf("%w: %s", ErrWriteFailed, strings.TrimSpace(output))
	}

	return nil
}

// ReadMifareSector reads all blocks in a MIFARE Classic sector
// sector: sector number (0-15 for 1K, 0-39 for 4K)
// key: 6-byte authentication key
// keyType: 'A' or 'B' for key type
func (c *Client) ReadMifareSector(ctx context.Context, sector int, key []byte, keyType byte) ([][]byte, error) {
	if len(key) != 6 {
		return nil, fmt.Errorf("key must be 6 bytes, got %d", len(key))
	}

	keyHex := hex.EncodeToString(key)
	keyFlag := "-a"
	if keyType == 'B' || keyType == 'b' {
		keyFlag = "-b"
	}

	cmd := fmt.Sprintf("hf mf rdsc --sec %d -k %s %s", sector, keyHex, keyFlag)
	output, err := c.Execute(ctx, cmd)
	if err != nil {
		return nil, err
	}

	// Parse multiple blocks from sector read output
	var blocks [][]byte
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if data, err := ParseBlockData(line); err == nil && len(data) == 16 {
			blocks = append(blocks, data)
		}
	}

	if len(blocks) == 0 {
		return nil, fmt.Errorf("%w: no blocks found in sector", ErrParseError)
	}

	return blocks, nil
}

// ReadUltralightPage reads a 4-byte page from a MIFARE Ultralight/NTAG card
// page: page number (0-44 for NTAG213, 0-134 for NTAG215, 0-230 for NTAG216)
// password: optional 4-byte password (nil for no authentication)
func (c *Client) ReadUltralightPage(ctx context.Context, page int, password []byte) ([]byte, error) {
	var cmd string
	if len(password) == 4 {
		cmd = fmt.Sprintf("hf mfu rdbl -b %d -k %s", page, hex.EncodeToString(password))
	} else {
		cmd = fmt.Sprintf("hf mfu rdbl -b %d", page)
	}

	output, err := c.Execute(ctx, cmd)
	if err != nil {
		return nil, err
	}

	return ParseMFUPage(output)
}

// WriteUltralightPage writes 4 bytes to a MIFARE Ultralight/NTAG page
// page: page number
// data: 4 bytes to write
// password: optional 4-byte password (nil for no authentication)
func (c *Client) WriteUltralightPage(ctx context.Context, page int, data []byte, password []byte) error {
	if len(data) != 4 {
		return fmt.Errorf("data must be 4 bytes, got %d", len(data))
	}

	var cmd string
	dataHex := hex.EncodeToString(data)
	if len(password) == 4 {
		cmd = fmt.Sprintf("hf mfu wrbl -b %d -d %s -k %s", page, dataHex, hex.EncodeToString(password))
	} else {
		cmd = fmt.Sprintf("hf mfu wrbl -b %d -d %s", page, dataHex)
	}

	output, err := c.Execute(ctx, cmd)
	if err != nil {
		return err
	}

	if !IsWriteSuccess(output) {
		return fmt.Errorf("%w: %s", ErrWriteFailed, strings.TrimSpace(output))
	}

	return nil
}

// ReadUltralightPages reads multiple consecutive pages from a MIFARE Ultralight/NTAG card
// startPage: starting page number
// count: number of pages to read
// password: optional 4-byte password (nil for no authentication)
func (c *Client) ReadUltralightPages(ctx context.Context, startPage, count int, password []byte) ([]byte, error) {
	var result []byte

	for i := 0; i < count; i++ {
		page, err := c.ReadUltralightPage(ctx, startPage+i, password)
		if err != nil {
			return nil, fmt.Errorf("failed to read page %d: %w", startPage+i, err)
		}
		result = append(result, page...)
	}

	return result, nil
}

// WriteUltralightPages writes data to multiple consecutive pages
// startPage: starting page number
// data: data to write (must be multiple of 4 bytes)
// password: optional 4-byte password (nil for no authentication)
func (c *Client) WriteUltralightPages(ctx context.Context, startPage int, data []byte, password []byte) error {
	if len(data)%4 != 0 {
		return fmt.Errorf("data length must be multiple of 4, got %d", len(data))
	}

	numPages := len(data) / 4
	for i := 0; i < numPages; i++ {
		pageData := data[i*4 : (i+1)*4]
		if err := c.WriteUltralightPage(ctx, startPage+i, pageData, password); err != nil {
			return fmt.Errorf("failed to write page %d: %w", startPage+i, err)
		}
	}

	return nil
}

// ReadNDEF reads NDEF data from a MIFARE Ultralight/NTAG card
func (c *Client) ReadNDEF(ctx context.Context) ([]byte, error) {
	output, err := c.Execute(ctx, "hf mfu ndefread")
	if err != nil {
		return nil, err
	}

	// The ndefread command outputs hex data - parse it
	// Look for hex data in the output
	lines := strings.Split(output, "\n")
	var ndefData []byte

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip lines that don't look like hex data
		if !strings.HasPrefix(line, "[") && len(line) > 0 {
			// Try to decode as hex
			cleaned := strings.ReplaceAll(line, " ", "")
			if data, err := hex.DecodeString(cleaned); err == nil {
				ndefData = append(ndefData, data...)
			}
		}
	}

	if len(ndefData) == 0 {
		return nil, fmt.Errorf("%w: no NDEF data found", ErrParseError)
	}

	return ndefData, nil
}

// GetUltralightInfo reads MIFARE Ultralight/NTAG tag information
func (c *Client) GetUltralightInfo(ctx context.Context) (*CardInfo, error) {
	output, err := c.Execute(ctx, "hf mfu info")
	if err != nil {
		return nil, err
	}

	return ParseHF14AInfo(output)
}

// ListDevices returns a list of connected Proxmark3 devices
func (c *Client) ListDevices(ctx context.Context) ([]string, error) {
	// Try pm3 --list first
	output, err := c.Execute(ctx, "--list")
	if err == nil {
		devices := ParseDeviceList(output)
		if len(devices) > 0 {
			return devices, nil
		}
	}

	// If --list doesn't work, return empty (device detection will be handled elsewhere)
	return nil, nil
}

// WriteISO15693Block writes a 4-byte block to an ISO 15693 tag
func (c *Client) WriteISO15693Block(ctx context.Context, block int, data []byte) error {
	if len(data) != 4 {
		return fmt.Errorf("ISO 15693 block must be exactly 4 bytes, got %d", len(data))
	}

	// Use hf 15 wrbl command with -* to scan for tag
	// Format: hf 15 wrbl -* -b <block> -d <data>
	cmd := fmt.Sprintf("hf 15 wrbl -* -b %d -d %s", block, strings.ToUpper(hex.EncodeToString(data)))
	output, err := c.Execute(ctx, cmd)
	if err != nil {
		return err
	}

	// Check for error patterns in output
	lower := strings.ToLower(output)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "no tag found") {
		return fmt.Errorf("write failed: %s", output)
	}

	return nil
}
