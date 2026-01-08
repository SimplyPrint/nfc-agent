package api

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/SimplyPrint/nfc-agent/internal/core"
	"github.com/SimplyPrint/nfc-agent/internal/data"
	"github.com/SimplyPrint/nfc-agent/internal/logging"
	"github.com/SimplyPrint/nfc-agent/internal/openprinttag"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local use
	},
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type    string          `json:"type"`              // Message type
	ID      string          `json:"id,omitempty"`      // Request ID for request/response matching
	Payload json.RawMessage `json:"payload,omitempty"` // Message payload
	Error   string          `json:"error,omitempty"`   // Error message if any
}

// WSClient represents a connected WebSocket client
type WSClient struct {
	conn        *websocket.Conn
	send        chan []byte
	hub         *WSHub
	mu          sync.Mutex
	closed      bool                      // True when client is disconnected
	subscribed  map[string]bool           // Track subscribed readers for auto-read
	pollTickers map[string]*time.Ticker
	lastUIDs    map[string]string // Track last seen UID per reader
}

// WSHub manages all WebSocket connections
type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan []byte
	register   chan *WSClient
	unregister chan *WSClient
	mu         sync.RWMutex
}

// NewWSHub creates a new WebSocket hub
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

// Run starts the hub's main loop
func (h *WSHub) Run() {
	// Re-panic after logging since hub crash is fatal
	defer logging.RecoverAndLog("WebSocket hub", true)

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.mu.Lock()
				client.closed = true
				close(client.send)
				client.mu.Unlock()
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Global hub instance
var wsHub *WSHub

// InitWebSocket initializes the WebSocket hub and returns the handler
func InitWebSocket() http.HandlerFunc {
	wsHub = NewWSHub()
	go wsHub.Run()

	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logging.Error(logging.CatWebSocket, "WebSocket upgrade failed", map[string]any{
				"error":      err.Error(),
				"remoteAddr": r.RemoteAddr,
			})
			return
		}

		logging.Info(logging.CatWebSocket, "Client connected", map[string]any{
			"remoteAddr": r.RemoteAddr,
		})

		client := &WSClient{
			conn:        conn,
			send:        make(chan []byte, 256),
			hub:         wsHub,
			subscribed:  make(map[string]bool),
			pollTickers: make(map[string]*time.Ticker),
			lastUIDs:    make(map[string]string),
		}

		wsHub.register <- client

		go client.writePump()
		go client.readPump()
	}
}

func (c *WSClient) readPump() {
	// Recover from panics (runs last due to LIFO)
	defer logging.RecoverAndLog("WebSocket readPump", false)
	// Cleanup (runs first)
	defer func() {
		// Stop all polling
		c.mu.Lock()
		for _, ticker := range c.pollTickers {
			ticker.Stop()
		}
		c.mu.Unlock()

		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512 * 1024) // 512KB max message size
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logging.Warn(logging.CatWebSocket, "WebSocket unexpected close", map[string]any{
					"error": err.Error(),
				})
			} else {
				logging.Debug(logging.CatWebSocket, "Client disconnected", nil)
			}
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.sendError("", "invalid message format")
			continue
		}

		c.handleMessage(msg)
	}
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	// Recover from panics (runs last due to LIFO)
	defer logging.RecoverAndLog("WebSocket writePump", false)
	// Cleanup (runs first)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				return
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *WSClient) handleMessage(msg WSMessage) {
	logging.Debug(logging.CatWebSocket, "Received message", map[string]any{
		"type": msg.Type,
		"id":   msg.ID,
	})

	switch msg.Type {
	case "list_readers":
		c.handleListReaders(msg.ID)
	case "read_card":
		c.handleReadCard(msg.ID, msg.Payload)
	case "write_card":
		c.handleWriteCard(msg.ID, msg.Payload)
	case "erase_card":
		c.handleEraseCard(msg.ID, msg.Payload)
	case "lock_card":
		c.handleLockCard(msg.ID, msg.Payload)
	case "set_password":
		c.handleSetPassword(msg.ID, msg.Payload)
	case "remove_password":
		c.handleRemovePassword(msg.ID, msg.Payload)
	case "write_records":
		c.handleWriteRecords(msg.ID, msg.Payload)
	case "subscribe":
		c.handleSubscribe(msg.ID, msg.Payload)
	case "unsubscribe":
		c.handleUnsubscribe(msg.ID, msg.Payload)
	case "supported_readers":
		c.handleSupportedReaders(msg.ID)
	case "version":
		c.handleVersion(msg.ID)
	case "health":
		c.handleHealth(msg.ID)
	case "read_mifare_block":
		c.handleReadMifareBlock(msg.ID, msg.Payload)
	case "write_mifare_block":
		c.handleWriteMifareBlock(msg.ID, msg.Payload)
	case "write_mifare_blocks":
		c.handleWriteMifareBlocks(msg.ID, msg.Payload)
	case "read_ultralight_page":
		c.handleReadUltralightPage(msg.ID, msg.Payload)
	case "write_ultralight_page":
		c.handleWriteUltralightPage(msg.ID, msg.Payload)
	case "write_ultralight_pages":
		c.handleWriteUltralightPages(msg.ID, msg.Payload)
	case "derive_uid_key_aes":
		c.handleDeriveUIDKeyAES(msg.ID, msg.Payload)
	case "aes_encrypt_and_write_block":
		c.handleAESEncryptAndWriteBlock(msg.ID, msg.Payload)
	case "write_mifare_sector_trailer":
		c.handleWriteMifareSectorTrailer(msg.ID, msg.Payload)
	default:
		logging.Warn(logging.CatWebSocket, "Unknown message type", map[string]any{
			"type": msg.Type,
		})
		c.sendError(msg.ID, "unknown message type: "+msg.Type)
	}
}

func (c *WSClient) sendResponse(id string, msgType string, payload interface{}) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	payloadBytes, _ := json.Marshal(payload)
	response := WSMessage{
		Type:    msgType,
		ID:      id,
		Payload: payloadBytes,
	}
	responseBytes, _ := json.Marshal(response)

	// Use non-blocking send to avoid panic if channel closes between check and send
	select {
	case c.send <- responseBytes:
	default:
		// Channel full or closed, drop the message
	}
}

func (c *WSClient) sendError(id string, errMsg string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	response := WSMessage{
		Type:  "error",
		ID:    id,
		Error: errMsg,
	}
	responseBytes, _ := json.Marshal(response)

	// Use non-blocking send to avoid panic if channel closes between check and send
	select {
	case c.send <- responseBytes:
	default:
		// Channel full or closed, drop the message
	}
}

func (c *WSClient) handleListReaders(id string) {
	readers := core.ListReaders()
	c.sendResponse(id, "readers", readers)
}

func (c *WSClient) handleReadCard(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int `json:"readerIndex"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	card, err := core.GetCardUID(readers[req.ReaderIndex].Name)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "card", card)
}

func (c *WSClient) handleWriteCard(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		Data        string `json:"data"`
		DataType    string `json:"dataType"`
		URL         string `json:"url"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	if req.DataType == "" {
		req.DataType = "text"
	}

	var dataBytes []byte
	switch req.DataType {
	case "text", "json", "url":
		dataBytes = []byte(req.Data)
	case "binary":
		var err error
		dataBytes, err = base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			c.sendError(id, "invalid base64 data")
			return
		}
	case "openprinttag":
		// Validate JSON structure for openprinttag
		var input openprinttag.Input
		if err := json.Unmarshal([]byte(req.Data), &input); err != nil {
			c.sendError(id, "invalid openprinttag JSON format: "+err.Error())
			return
		}
		dataBytes = []byte(req.Data)
	default:
		c.sendError(id, "invalid dataType (must be 'text', 'json', 'binary', 'url', or 'openprinttag')")
		return
	}

	if err := core.WriteDataWithURL(readers[req.ReaderIndex].Name, dataBytes, req.DataType, req.URL); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "write_success", map[string]string{"success": "data written"})
}

func (c *WSClient) handleEraseCard(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int `json:"readerIndex"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	if err := core.EraseCard(readers[req.ReaderIndex].Name); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "erase_success", map[string]string{"success": "card erased"})
}

func (c *WSClient) handleLockCard(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int  `json:"readerIndex"`
		Confirm     bool `json:"confirm"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	if !req.Confirm {
		c.sendError(id, "must set confirm=true to lock card (WARNING: IRREVERSIBLE)")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	if err := core.LockCard(readers[req.ReaderIndex].Name); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "lock_success", map[string]string{"success": "card locked permanently"})
}

func (c *WSClient) handleSetPassword(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		Password    string `json:"password"`
		Pack        string `json:"pack"`
		StartPage   int    `json:"startPage"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	password, err := hex.DecodeString(req.Password)
	if err != nil || len(password) != 4 {
		c.sendError(id, "password must be 8 hex characters (4 bytes)")
		return
	}

	pack, err := hex.DecodeString(req.Pack)
	if err != nil || len(pack) != 2 {
		c.sendError(id, "pack must be 4 hex characters (2 bytes)")
		return
	}

	if req.StartPage < 4 {
		req.StartPage = 4
	}

	if err := core.SetPassword(readers[req.ReaderIndex].Name, password, pack, byte(req.StartPage)); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "password_set", map[string]string{"success": "password set"})
}

func (c *WSClient) handleRemovePassword(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		Password    string `json:"password"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	password, err := hex.DecodeString(req.Password)
	if err != nil || len(password) != 4 {
		c.sendError(id, "password must be 8 hex characters (4 bytes)")
		return
	}

	if err := core.RemovePassword(readers[req.ReaderIndex].Name, password); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "password_removed", map[string]string{"success": "password removed"})
}

func (c *WSClient) handleWriteRecords(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int               `json:"readerIndex"`
		Records     []core.NDEFRecord `json:"records"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	if len(req.Records) == 0 {
		c.sendError(id, "records array cannot be empty")
		return
	}

	if err := core.WriteMultipleRecords(readers[req.ReaderIndex].Name, req.Records); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "records_written", map[string]string{"success": "records written"})
}

func (c *WSClient) handleSubscribe(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int `json:"readerIndex"`
		IntervalMs  int `json:"intervalMs"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	if req.IntervalMs < 100 {
		req.IntervalMs = 500 // Default 500ms
	}

	readerKey := readers[req.ReaderIndex].Name

	c.mu.Lock()
	// Stop existing ticker if any
	if ticker, ok := c.pollTickers[readerKey]; ok {
		ticker.Stop()
	}

	c.subscribed[readerKey] = true
	ticker := time.NewTicker(time.Duration(req.IntervalMs) * time.Millisecond)
	c.pollTickers[readerKey] = ticker
	c.mu.Unlock()

	// Start polling goroutine
	go func() {
		defer logging.RecoverAndLog("WebSocket poll goroutine", false)

		for range ticker.C {
			c.mu.Lock()
			if !c.subscribed[readerKey] {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()

			card, err := core.GetCardUID(readerKey)
			if err != nil {
				// Card removed - send event if we previously had a card
				c.mu.Lock()
				if c.lastUIDs[readerKey] != "" {
					c.lastUIDs[readerKey] = ""
					c.mu.Unlock()
					logging.Info(logging.CatCard, "Card removed", map[string]any{
						"reader": readerKey,
					})
					c.sendResponse("", "card_removed", map[string]interface{}{
						"readerIndex": req.ReaderIndex,
						"readerName":  readerKey,
					})
				} else {
					c.mu.Unlock()
				}
				continue
			}

			// Check if this is a new card
			c.mu.Lock()
			lastUID := c.lastUIDs[readerKey]
			if card.UID != lastUID {
				c.lastUIDs[readerKey] = card.UID
				c.mu.Unlock()
				logData := map[string]any{
					"reader": readerKey,
					"uid":    card.UID,
					"type":   card.Type,
				}
				if card.Data != "" {
					logData["data"] = card.Data
					logData["dataType"] = card.DataType
				}
				if card.URL != "" {
					logData["url"] = card.URL
				}
				logging.Info(logging.CatCard, "Tag read", logData)
				c.sendResponse("", "card_detected", map[string]interface{}{
					"readerIndex": req.ReaderIndex,
					"readerName":  readerKey,
					"card":        card,
				})
			} else {
				c.mu.Unlock()
			}
		}
	}()

	logging.Info(logging.CatWebSocket, "Client subscribed to reader", map[string]any{
		"reader":     readerKey,
		"intervalMs": req.IntervalMs,
	})
	c.sendResponse(id, "subscribed", map[string]interface{}{
		"readerIndex": req.ReaderIndex,
		"intervalMs":  req.IntervalMs,
	})
}

func (c *WSClient) handleUnsubscribe(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int `json:"readerIndex"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	readerKey := readers[req.ReaderIndex].Name

	c.mu.Lock()
	c.subscribed[readerKey] = false
	if ticker, ok := c.pollTickers[readerKey]; ok {
		ticker.Stop()
		delete(c.pollTickers, readerKey)
	}
	c.mu.Unlock()

	logging.Info(logging.CatWebSocket, "Client unsubscribed from reader", map[string]any{
		"reader": readerKey,
	})
	c.sendResponse(id, "unsubscribed", map[string]interface{}{
		"readerIndex": req.ReaderIndex,
	})
}

func (c *WSClient) handleSupportedReaders(id string) {
	readers, err := data.GetSupportedReaders()
	if err != nil {
		c.sendError(id, "failed to load supported readers")
		return
	}
	c.sendResponse(id, "supported_readers", map[string]interface{}{
		"readers": readers,
	})
}

func (c *WSClient) handleVersion(id string) {
	response := map[string]interface{}{
		"version":   Version,
		"buildTime": BuildTime,
		"gitCommit": GitCommit,
	}

	// Include update info if available (for JS SDK / SimplyPrint integration)
	if updateChecker != nil {
		info := updateChecker.Check(false) // Use cached result
		response["updateAvailable"] = info.Available
		if info.LatestVersion != "" {
			response["latestVersion"] = info.LatestVersion
		}
		if info.ReleaseURL != "" {
			response["releaseUrl"] = info.ReleaseURL
		}
	}

	c.sendResponse(id, "version", response)
}

func (c *WSClient) handleHealth(id string) {
	readers := core.ListReaders()
	c.sendResponse(id, "health", map[string]interface{}{
		"status":      "ok",
		"readerCount": len(readers),
	})
}

func (c *WSClient) handleReadMifareBlock(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		Block       int    `json:"block"`
		Key         string `json:"key"`     // Optional, hex string
		KeyType     string `json:"keyType"` // Optional, "A" or "B"
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	key, err := parseMifareKey(req.Key)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}
	keyType := parseMifareKeyType(req.KeyType)

	data, err := core.ReadMifareBlock(readers[req.ReaderIndex].Name, req.Block, key, keyType)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "mifare_block", map[string]interface{}{
		"block": req.Block,
		"data":  hex.EncodeToString(data),
	})
}

func (c *WSClient) handleWriteMifareBlock(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		Block       int    `json:"block"`
		Data        string `json:"data"`    // Hex string, 32 chars = 16 bytes
		Key         string `json:"key"`     // Optional, hex string
		KeyType     string `json:"keyType"` // Optional, "A" or "B"
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	// Parse data
	data, err := hex.DecodeString(req.Data)
	if err != nil || len(data) != 16 {
		c.sendError(id, "invalid data (must be 32 hex characters for 16 bytes)")
		return
	}

	key, err := parseMifareKey(req.Key)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}
	keyType := parseMifareKeyType(req.KeyType)

	if err := core.WriteMifareBlock(readers[req.ReaderIndex].Name, req.Block, data, key, keyType); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "mifare_write_success", map[string]interface{}{
		"success": true,
		"block":   req.Block,
	})
}

func (c *WSClient) handleWriteMifareBlocks(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int `json:"readerIndex"`
		Blocks      []struct {
			Block int    `json:"block"`
			Data  string `json:"data"` // Hex string, 32 chars = 16 bytes
		} `json:"blocks"`
		Key     string `json:"key"`     // Hex string, 12 chars = 6 bytes
		KeyType string `json:"keyType"` // "A" or "B"
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	if len(req.Blocks) == 0 {
		c.sendError(id, "no blocks provided")
		return
	}

	// Convert to core types
	blocks := make([]core.MifareBlockWrite, len(req.Blocks))
	for i, b := range req.Blocks {
		data, err := hex.DecodeString(b.Data)
		if err != nil || len(data) != 16 {
			c.sendError(id, fmt.Sprintf("block %d: invalid data (must be 32 hex characters for 16 bytes)", b.Block))
			return
		}
		blocks[i] = core.MifareBlockWrite{
			Block: b.Block,
			Data:  data,
		}
	}

	key, err := parseMifareKey(req.Key)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}
	keyType := parseMifareKeyType(req.KeyType)

	results, err := core.WriteMifareBlocks(readers[req.ReaderIndex].Name, blocks, key, keyType)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}

	// Count successes
	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		}
	}

	c.sendResponse(id, "mifare_write_blocks_success", map[string]interface{}{
		"results": results,
		"written": successCount,
		"total":   len(results),
	})
}

func (c *WSClient) handleReadUltralightPage(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		Page        int    `json:"page"`
		Password    string `json:"password"` // Optional, hex string, 8 chars = 4 bytes
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	password, err := parseUltralightPassword(req.Password)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}

	data, err := core.ReadUltralightPage(readers[req.ReaderIndex].Name, req.Page, password)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "ultralight_page", map[string]interface{}{
		"page": req.Page,
		"data": hex.EncodeToString(data),
	})
}

func (c *WSClient) handleWriteUltralightPage(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		Page        int    `json:"page"`
		Data        string `json:"data"`     // Hex string, 8 chars = 4 bytes
		Password    string `json:"password"` // Optional, hex string, 8 chars = 4 bytes
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	data, err := hex.DecodeString(req.Data)
	if err != nil || len(data) != 4 {
		c.sendError(id, "invalid data (must be 8 hex characters for 4 bytes)")
		return
	}

	password, err := parseUltralightPassword(req.Password)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}

	if err := core.WriteUltralightPage(readers[req.ReaderIndex].Name, req.Page, data, password); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "ultralight_write_success", map[string]interface{}{
		"success": true,
		"page":    req.Page,
	})
}

func (c *WSClient) handleWriteUltralightPages(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int `json:"readerIndex"`
		Pages       []struct {
			Page int    `json:"page"`
			Data string `json:"data"` // Hex string, 8 chars = 4 bytes
		} `json:"pages"`
		Password string `json:"password"` // Optional, hex string, 8 chars = 4 bytes
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	if len(req.Pages) == 0 {
		c.sendError(id, "no pages provided")
		return
	}

	// Convert to core types
	pages := make([]core.UltralightPageWrite, len(req.Pages))
	for i, p := range req.Pages {
		data, err := hex.DecodeString(p.Data)
		if err != nil || len(data) != 4 {
			c.sendError(id, fmt.Sprintf("page %d: invalid data (must be 8 hex characters for 4 bytes)", p.Page))
			return
		}
		pages[i] = core.UltralightPageWrite{
			Page: p.Page,
			Data: data,
		}
	}

	password, err := parseUltralightPassword(req.Password)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}

	results, err := core.WriteUltralightPages(readers[req.ReaderIndex].Name, pages, password)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}

	// Count successes
	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	c.sendResponse(id, "ultralight_write_pages_success", map[string]interface{}{
		"results": results,
		"written": successCount,
		"total":   len(results),
	})
}

func (c *WSClient) handleDeriveUIDKeyAES(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		AESKey      string `json:"aesKey"` // Hex string, 32 chars = 16 bytes
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	aesKey, err := hex.DecodeString(req.AESKey)
	if err != nil || len(aesKey) != 16 {
		c.sendError(id, "invalid aesKey (must be 32 hex characters for 16 bytes)")
		return
	}

	key, err := core.DeriveUIDKeyAES(readers[req.ReaderIndex].Name, aesKey)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "uid_key_derived", map[string]interface{}{
		"key": hex.EncodeToString(key),
	})
}

func (c *WSClient) handleAESEncryptAndWriteBlock(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		Block       int    `json:"block"`
		Data        string `json:"data"`        // Hex string, 32 chars = 16 bytes
		AESKey      string `json:"aesKey"`      // Hex string, 32 chars = 16 bytes
		AuthKey     string `json:"authKey"`     // Hex string, 12 chars = 6 bytes
		AuthKeyType string `json:"authKeyType"` // "A" or "B"
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	data, err := hex.DecodeString(req.Data)
	if err != nil || len(data) != 16 {
		c.sendError(id, "invalid data (must be 32 hex characters for 16 bytes)")
		return
	}

	aesKey, err := hex.DecodeString(req.AESKey)
	if err != nil || len(aesKey) != 16 {
		c.sendError(id, "invalid aesKey (must be 32 hex characters for 16 bytes)")
		return
	}

	authKey, err := parseMifareKey(req.AuthKey)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}
	authKeyType := parseMifareKeyType(req.AuthKeyType)

	if err := core.AESEncryptAndWriteBlock(readers[req.ReaderIndex].Name, req.Block, data, aesKey, authKey, authKeyType); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "aes_write_success", map[string]interface{}{
		"success": true,
		"block":   req.Block,
	})
}

func (c *WSClient) handleWriteMifareSectorTrailer(id string, payload json.RawMessage) {
	var req struct {
		ReaderIndex int    `json:"readerIndex"`
		Block       int    `json:"block"`       // Sector trailer block number
		KeyA        string `json:"keyA"`        // New Key A, 12 hex chars = 6 bytes
		KeyB        string `json:"keyB"`        // New Key B, 12 hex chars = 6 bytes
		AccessBits  string `json:"accessBits"`  // Access bits, 6 or 8 hex chars (optional, preserves existing if empty)
		AuthKey     string `json:"authKey"`     // Key for authentication, 12 hex chars = 6 bytes
		AuthKeyType string `json:"authKeyType"` // "A" or "B"
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(id, "invalid payload")
		return
	}

	readers := core.ListReaders()
	if req.ReaderIndex < 0 || req.ReaderIndex >= len(readers) {
		c.sendError(id, "reader index out of range")
		return
	}

	keyA, err := hex.DecodeString(req.KeyA)
	if err != nil || len(keyA) != 6 {
		c.sendError(id, "invalid keyA (must be 12 hex characters for 6 bytes)")
		return
	}

	keyB, err := hex.DecodeString(req.KeyB)
	if err != nil || len(keyB) != 6 {
		c.sendError(id, "invalid keyB (must be 12 hex characters for 6 bytes)")
		return
	}

	// Parse optional access bits (nil if empty, preserves existing)
	var accessBits []byte
	if req.AccessBits != "" {
		accessBits, err = hex.DecodeString(req.AccessBits)
		if err != nil || (len(accessBits) != 3 && len(accessBits) != 4) {
			c.sendError(id, "invalid accessBits (must be 6 or 8 hex characters for 3 or 4 bytes)")
			return
		}
	}

	authKey, err := parseMifareKey(req.AuthKey)
	if err != nil {
		c.sendError(id, err.Error())
		return
	}
	authKeyType := parseMifareKeyType(req.AuthKeyType)

	if err := core.WriteSectorTrailer(readers[req.ReaderIndex].Name, req.Block, keyA, keyB, accessBits, authKey, authKeyType); err != nil {
		c.sendError(id, err.Error())
		return
	}

	c.sendResponse(id, "mifare_sector_trailer_written", map[string]interface{}{
		"success": true,
		"block":   req.Block,
	})
}
