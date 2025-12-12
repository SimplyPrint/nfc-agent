package api

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	subscribed  map[string]bool // Track subscribed readers for auto-read
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
				close(client.send)
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
	default:
		logging.Warn(logging.CatWebSocket, "Unknown message type", map[string]any{
			"type": msg.Type,
		})
		c.sendError(msg.ID, "unknown message type: "+msg.Type)
	}
}

func (c *WSClient) sendResponse(id string, msgType string, payload interface{}) {
	payloadBytes, _ := json.Marshal(payload)
	response := WSMessage{
		Type:    msgType,
		ID:      id,
		Payload: payloadBytes,
	}
	responseBytes, _ := json.Marshal(response)
	c.send <- responseBytes
}

func (c *WSClient) sendError(id string, errMsg string) {
	response := WSMessage{
		Type:  "error",
		ID:    id,
		Error: errMsg,
	}
	responseBytes, _ := json.Marshal(response)
	c.send <- responseBytes
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
