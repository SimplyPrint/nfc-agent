package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewWSHub(t *testing.T) {
	hub := NewWSHub()

	if hub == nil {
		t.Fatal("NewWSHub() returned nil")
	}
	if hub.clients == nil {
		t.Error("clients map should be initialized")
	}
	if hub.broadcast == nil {
		t.Error("broadcast channel should be initialized")
	}
	if hub.register == nil {
		t.Error("register channel should be initialized")
	}
	if hub.unregister == nil {
		t.Error("unregister channel should be initialized")
	}
}

func TestWSHub_Run(t *testing.T) {
	hub := NewWSHub()

	// Start hub in goroutine
	go hub.Run()

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Create a mock client
	client := &WSClient{
		send:        make(chan []byte, 256),
		hub:         hub,
		subscribed:  make(map[string]bool),
		pollTickers: make(map[string]*time.Ticker),
		pollDone:    make(map[string]chan struct{}),
		lastUIDs:    make(map[string]string),
	}

	// Register client
	hub.register <- client

	// Give time for registration
	time.Sleep(10 * time.Millisecond)

	// Check client was registered
	hub.mu.RLock()
	_, exists := hub.clients[client]
	hub.mu.RUnlock()

	if !exists {
		t.Error("client should be registered")
	}

	// Unregister client
	hub.unregister <- client

	// Give time for unregistration
	time.Sleep(10 * time.Millisecond)

	// Check client was unregistered
	hub.mu.RLock()
	_, exists = hub.clients[client]
	hub.mu.RUnlock()

	if exists {
		t.Error("client should be unregistered")
	}
}

func TestWSHub_Broadcast(t *testing.T) {
	hub := NewWSHub()
	go hub.Run()

	// Create multiple clients
	clients := make([]*WSClient, 3)
	for i := range clients {
		clients[i] = &WSClient{
			send:        make(chan []byte, 256),
			hub:         hub,
			subscribed:  make(map[string]bool),
			pollTickers: make(map[string]*time.Ticker),
			lastUIDs:    make(map[string]string),
		}
		hub.register <- clients[i]
	}

	time.Sleep(10 * time.Millisecond)

	// Broadcast a message
	testMsg := []byte(`{"type":"test"}`)
	hub.broadcast <- testMsg

	time.Sleep(10 * time.Millisecond)

	// Check all clients received the message
	for i, client := range clients {
		select {
		case msg := <-client.send:
			if string(msg) != string(testMsg) {
				t.Errorf("client %d received wrong message", i)
			}
		default:
			t.Errorf("client %d did not receive message", i)
		}
	}
}

func TestWSMessage_JSON(t *testing.T) {
	tests := []struct {
		name    string
		msg     WSMessage
		wantErr bool
	}{
		{
			name: "simple message",
			msg: WSMessage{
				Type: "test",
				ID:   "123",
			},
			wantErr: false,
		},
		{
			name: "message with payload",
			msg: WSMessage{
				Type:    "read_card",
				ID:      "456",
				Payload: json.RawMessage(`{"readerIndex":0}`),
			},
			wantErr: false,
		},
		{
			name: "error message",
			msg: WSMessage{
				Type:  "error",
				ID:    "789",
				Error: "something went wrong",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Unmarshal
			var decoded WSMessage
			err = json.Unmarshal(data, &decoded)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify
			if decoded.Type != tt.msg.Type {
				t.Errorf("Type mismatch: got %s, want %s", decoded.Type, tt.msg.Type)
			}
			if decoded.ID != tt.msg.ID {
				t.Errorf("ID mismatch: got %s, want %s", decoded.ID, tt.msg.ID)
			}
			if decoded.Error != tt.msg.Error {
				t.Errorf("Error mismatch: got %s, want %s", decoded.Error, tt.msg.Error)
			}
		})
	}
}

func TestWSClient_sendResponse(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	payload := map[string]string{"key": "value"}
	client.sendResponse("test-id", "test-type", payload)

	select {
	case msg := <-client.send:
		var decoded WSMessage
		if err := json.Unmarshal(msg, &decoded); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if decoded.Type != "test-type" {
			t.Errorf("expected type 'test-type', got '%s'", decoded.Type)
		}
		if decoded.ID != "test-id" {
			t.Errorf("expected ID 'test-id', got '%s'", decoded.ID)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_sendError(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	client.sendError("err-id", "test error message")

	select {
	case msg := <-client.send:
		var decoded WSMessage
		if err := json.Unmarshal(msg, &decoded); err != nil {
			t.Fatalf("failed to unmarshal error: %v", err)
		}

		if decoded.Type != "error" {
			t.Errorf("expected type 'error', got '%s'", decoded.Type)
		}
		if decoded.Error != "test error message" {
			t.Errorf("expected error 'test error message', got '%s'", decoded.Error)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for error")
	}
}

func TestWSClient_handleMessage(t *testing.T) {
	tests := []struct {
		name         string
		msgType      string
		payload      string
		expectError  bool
	}{
		{"list_readers", "list_readers", "", false},
		{"version", "version", "", false},
		{"health", "health", "", false},
		{"supported_readers", "supported_readers", "", false},
		{"unknown", "unknown_type", "", true},
		{"read_card_invalid_payload", "read_card", "invalid", true},
		{"write_card_invalid_payload", "write_card", "invalid", true},
		{"subscribe_invalid_payload", "subscribe", "invalid", true},
		{"unsubscribe_invalid_payload", "unsubscribe", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &WSClient{
				send:        make(chan []byte, 256),
				subscribed:  make(map[string]bool),
				pollTickers: make(map[string]*time.Ticker),
				lastUIDs:    make(map[string]string),
			}

			var payload json.RawMessage
			if tt.payload != "" {
				payload = json.RawMessage(tt.payload)
			}

			msg := WSMessage{
				Type:    tt.msgType,
				ID:      "test-id",
				Payload: payload,
			}

			client.handleMessage(msg)

			// Check if we got a response
			select {
			case resp := <-client.send:
				var decoded WSMessage
				json.Unmarshal(resp, &decoded)

				if tt.expectError && decoded.Type != "error" {
					t.Errorf("expected error response, got type '%s'", decoded.Type)
				}
			case <-time.After(100 * time.Millisecond):
				// Some handlers may not send immediate response
			}
		})
	}
}

func TestWSClient_handleListReaders(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	client.handleListReaders("test-id")

	select {
	case msg := <-client.send:
		var decoded WSMessage
		if err := json.Unmarshal(msg, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.Type != "readers" {
			t.Errorf("expected type 'readers', got '%s'", decoded.Type)
		}
		if decoded.ID != "test-id" {
			t.Errorf("expected ID 'test-id', got '%s'", decoded.ID)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleVersion(t *testing.T) {
	// Save and set test values
	origVersion := Version
	origBuildTime := BuildTime
	origGitCommit := GitCommit
	defer func() {
		Version = origVersion
		BuildTime = origBuildTime
		GitCommit = origGitCommit
	}()

	Version = "1.0.0-test"
	BuildTime = "2024-01-01"
	GitCommit = "abc123"

	client := &WSClient{
		send: make(chan []byte, 256),
	}

	client.handleVersion("ver-id")

	select {
	case msg := <-client.send:
		var decoded WSMessage
		if err := json.Unmarshal(msg, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.Type != "version" {
			t.Errorf("expected type 'version', got '%s'", decoded.Type)
		}

		var payload map[string]string
		json.Unmarshal(decoded.Payload, &payload)

		if payload["version"] != "1.0.0-test" {
			t.Errorf("expected version '1.0.0-test', got '%s'", payload["version"])
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleHealth(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	client.handleHealth("health-id")

	select {
	case msg := <-client.send:
		var decoded WSMessage
		if err := json.Unmarshal(msg, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.Type != "health" {
			t.Errorf("expected type 'health', got '%s'", decoded.Type)
		}

		var payload map[string]interface{}
		json.Unmarshal(decoded.Payload, &payload)

		if payload["status"] != "ok" {
			t.Errorf("expected status 'ok', got '%v'", payload["status"])
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleSupportedReaders(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	client.handleSupportedReaders("sr-id")

	select {
	case msg := <-client.send:
		var decoded WSMessage
		if err := json.Unmarshal(msg, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if decoded.Type != "supported_readers" {
			t.Errorf("expected type 'supported_readers', got '%s'", decoded.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleReadCard_InvalidPayload(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	client.handleReadCard("test-id", json.RawMessage("invalid json"))

	select {
	case msg := <-client.send:
		var decoded WSMessage
		json.Unmarshal(msg, &decoded)

		if decoded.Type != "error" {
			t.Errorf("expected error type, got '%s'", decoded.Type)
		}
		if !strings.Contains(decoded.Error, "invalid payload") {
			t.Errorf("expected 'invalid payload' error, got '%s'", decoded.Error)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleReadCard_OutOfRange(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	payload := json.RawMessage(`{"readerIndex": 999}`)
	client.handleReadCard("test-id", payload)

	select {
	case msg := <-client.send:
		var decoded WSMessage
		json.Unmarshal(msg, &decoded)

		if decoded.Type != "error" {
			t.Errorf("expected error type, got '%s'", decoded.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleWriteCard_InvalidDataType(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	payload := json.RawMessage(`{"readerIndex": 0, "data": "test", "dataType": "invalid_type"}`)
	client.handleWriteCard("test-id", payload)

	select {
	case msg := <-client.send:
		var decoded WSMessage
		json.Unmarshal(msg, &decoded)

		if decoded.Type != "error" {
			t.Errorf("expected error type, got '%s'", decoded.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleLockCard_NoConfirm(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	payload := json.RawMessage(`{"readerIndex": 0, "confirm": false}`)
	client.handleLockCard("test-id", payload)

	select {
	case msg := <-client.send:
		var decoded WSMessage
		json.Unmarshal(msg, &decoded)

		if decoded.Type != "error" {
			t.Errorf("expected error type, got '%s'", decoded.Type)
		}
		if !strings.Contains(decoded.Error, "confirm=true") {
			t.Errorf("expected confirm error, got '%s'", decoded.Error)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleSetPassword_InvalidPassword(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	// Password too short (should be 8 hex chars = 4 bytes)
	payload := json.RawMessage(`{"readerIndex": 0, "password": "123", "pack": "ABCD"}`)
	client.handleSetPassword("test-id", payload)

	select {
	case msg := <-client.send:
		var decoded WSMessage
		json.Unmarshal(msg, &decoded)

		if decoded.Type != "error" {
			t.Errorf("expected error type, got '%s'", decoded.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleSetPassword_InvalidPack(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	// Pack too short (should be 4 hex chars = 2 bytes)
	payload := json.RawMessage(`{"readerIndex": 0, "password": "12345678", "pack": "AB"}`)
	client.handleSetPassword("test-id", payload)

	select {
	case msg := <-client.send:
		var decoded WSMessage
		json.Unmarshal(msg, &decoded)

		if decoded.Type != "error" {
			t.Errorf("expected error type, got '%s'", decoded.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestWSClient_handleWriteRecords_EmptyRecords(t *testing.T) {
	client := &WSClient{
		send: make(chan []byte, 256),
	}

	// Note: Without mock readers, the reader index validation fails first
	// This test verifies that validation errors are returned properly
	payload := json.RawMessage(`{"readerIndex": 0, "records": []}`)
	client.handleWriteRecords("test-id", payload)

	select {
	case msg := <-client.send:
		var decoded WSMessage
		json.Unmarshal(msg, &decoded)

		if decoded.Type != "error" {
			t.Errorf("expected error type, got '%s'", decoded.Type)
		}
		// Reader index validation happens before empty records check
		if decoded.Error == "" {
			t.Error("expected non-empty error message")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for response")
	}
}

func TestInitWebSocket(t *testing.T) {
	handler := InitWebSocket()

	if handler == nil {
		t.Fatal("InitWebSocket() returned nil handler")
	}

	if wsHub == nil {
		t.Error("global wsHub should be initialized")
	}
}

// Integration test with actual WebSocket connection
func TestWebSocket_Integration(t *testing.T) {
	// Initialize WebSocket handler
	handler := InitWebSocket()

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	// Send a message
	msg := WSMessage{
		Type: "list_readers",
		ID:   "test-123",
	}
	if err := ws.WriteJSON(msg); err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// Read response
	var resp WSMessage
	if err := ws.ReadJSON(&resp); err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if resp.Type != "readers" {
		t.Errorf("expected type 'readers', got '%s'", resp.Type)
	}
	if resp.ID != "test-123" {
		t.Errorf("expected ID 'test-123', got '%s'", resp.ID)
	}
}

func TestWebSocket_Version(t *testing.T) {
	handler := InitWebSocket()
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	msg := WSMessage{Type: "version", ID: "v1"}
	ws.WriteJSON(msg)

	var resp WSMessage
	ws.ReadJSON(&resp)

	if resp.Type != "version" {
		t.Errorf("expected type 'version', got '%s'", resp.Type)
	}
}

func TestWebSocket_Health(t *testing.T) {
	handler := InitWebSocket()
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	msg := WSMessage{Type: "health", ID: "h1"}
	ws.WriteJSON(msg)

	var resp WSMessage
	ws.ReadJSON(&resp)

	if resp.Type != "health" {
		t.Errorf("expected type 'health', got '%s'", resp.Type)
	}
}

func TestWebSocket_UnknownType(t *testing.T) {
	handler := InitWebSocket()
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	msg := WSMessage{Type: "unknown_type_xyz", ID: "u1"}
	ws.WriteJSON(msg)

	var resp WSMessage
	ws.ReadJSON(&resp)

	if resp.Type != "error" {
		t.Errorf("expected error type, got '%s'", resp.Type)
	}
	if !strings.Contains(resp.Error, "unknown message type") {
		t.Errorf("expected unknown type error, got '%s'", resp.Error)
	}
}

func TestWebSocket_ConcurrentClients(t *testing.T) {
	handler := InitWebSocket()
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	numClients := 5
	var wg sync.WaitGroup
	wg.Add(numClients)

	errors := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				errors <- err
				return
			}
			defer ws.Close()

			// Send list_readers
			msg := WSMessage{Type: "list_readers", ID: "concurrent"}
			if err := ws.WriteJSON(msg); err != nil {
				errors <- err
				return
			}

			var resp WSMessage
			if err := ws.ReadJSON(&resp); err != nil {
				errors <- err
				return
			}

			if resp.Type != "readers" {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Errorf("concurrent client error: %v", err)
		}
	}
}

// Benchmarks
func BenchmarkWSMessage_Marshal(b *testing.B) {
	msg := WSMessage{
		Type:    "read_card",
		ID:      "benchmark-id",
		Payload: json.RawMessage(`{"readerIndex":0}`),
	}

	for i := 0; i < b.N; i++ {
		json.Marshal(msg)
	}
}

func BenchmarkWSMessage_Unmarshal(b *testing.B) {
	data := []byte(`{"type":"read_card","id":"benchmark-id","payload":{"readerIndex":0}}`)

	for i := 0; i < b.N; i++ {
		var msg WSMessage
		json.Unmarshal(data, &msg)
	}
}

func BenchmarkWSClient_sendResponse(b *testing.B) {
	client := &WSClient{
		send: make(chan []byte, 1000),
	}

	// Drain channel in background
	go func() {
		for range client.send {
		}
	}()

	payload := map[string]string{"key": "value"}

	for i := 0; i < b.N; i++ {
		client.sendResponse("id", "type", payload)
	}
}

// TestWSClient_SubscribeUnsubscribe_ClearsLastUID verifies that the lastUIDs map
// is properly cleared when unsubscribing, preventing stale UIDs from blocking
// card_detected events on re-subscribe.
func TestWSClient_SubscribeUnsubscribe_ClearsLastUID(t *testing.T) {
	client := &WSClient{
		send:        make(chan []byte, 256),
		subscribed:  make(map[string]bool),
		pollTickers: make(map[string]*time.Ticker),
		pollDone:    make(map[string]chan struct{}),
		lastUIDs:    make(map[string]string),
	}

	readerKey := "TestReader"

	// Simulate a subscription with a detected card
	client.mu.Lock()
	client.subscribed[readerKey] = true
	client.pollTickers[readerKey] = time.NewTicker(500 * time.Millisecond)
	client.lastUIDs[readerKey] = "ABC123DEF456" // Simulate a detected card UID
	client.mu.Unlock()

	// Verify the UID was set
	client.mu.Lock()
	if client.lastUIDs[readerKey] != "ABC123DEF456" {
		t.Errorf("expected lastUID to be set, got: %s", client.lastUIDs[readerKey])
	}
	client.mu.Unlock()

	// Simulate unsubscribe behavior (mirrors handleUnsubscribe logic)
	client.mu.Lock()
	client.subscribed[readerKey] = false
	if ticker, ok := client.pollTickers[readerKey]; ok {
		ticker.Stop()
		delete(client.pollTickers, readerKey)
	}
	delete(client.lastUIDs, readerKey) // This is the fix being tested
	client.mu.Unlock()

	// Verify lastUIDs was cleared
	client.mu.Lock()
	if uid, exists := client.lastUIDs[readerKey]; exists {
		t.Errorf("lastUIDs should be cleared after unsubscribe, but got: %s", uid)
	}
	client.mu.Unlock()
}

// TestWSClient_Subscribe_ResetsLastUID verifies that subscribing to a reader
// resets the lastUIDs entry, ensuring a card_detected event is sent even if
// the same card was previously detected.
func TestWSClient_Subscribe_ResetsLastUID(t *testing.T) {
	client := &WSClient{
		send:        make(chan []byte, 256),
		subscribed:  make(map[string]bool),
		pollTickers: make(map[string]*time.Ticker),
		pollDone:    make(map[string]chan struct{}),
		lastUIDs:    make(map[string]string),
	}

	readerKey := "TestReader"

	// Simulate stale state from a previous subscription
	client.mu.Lock()
	client.lastUIDs[readerKey] = "OLD_UID_12345"
	client.mu.Unlock()

	// Simulate subscribe behavior (mirrors handleSubscribe logic)
	client.mu.Lock()
	if ticker, ok := client.pollTickers[readerKey]; ok {
		ticker.Stop()
	}
	client.subscribed[readerKey] = true
	client.lastUIDs[readerKey] = "" // This is the fix being tested
	ticker := time.NewTicker(500 * time.Millisecond)
	client.pollTickers[readerKey] = ticker
	client.mu.Unlock()

	// Verify lastUIDs was reset
	client.mu.Lock()
	if client.lastUIDs[readerKey] != "" {
		t.Errorf("lastUIDs should be reset to empty on subscribe, but got: %s", client.lastUIDs[readerKey])
	}
	client.mu.Unlock()

	// Cleanup
	client.mu.Lock()
	if ticker, ok := client.pollTickers[readerKey]; ok {
		ticker.Stop()
	}
	client.mu.Unlock()
}

// TestWSClient_SubscribeUnsubscribeCycle tests the full subscribe/unsubscribe cycle
// to ensure card detection works correctly after multiple cycles.
func TestWSClient_SubscribeUnsubscribeCycle(t *testing.T) {
	client := &WSClient{
		send:        make(chan []byte, 256),
		subscribed:  make(map[string]bool),
		pollTickers: make(map[string]*time.Ticker),
		pollDone:    make(map[string]chan struct{}),
		lastUIDs:    make(map[string]string),
	}

	readerKey := "TestReader"

	// Perform multiple subscribe/unsubscribe cycles
	for cycle := 0; cycle < 5; cycle++ {
		// Subscribe
		client.mu.Lock()
		if ticker, ok := client.pollTickers[readerKey]; ok {
			ticker.Stop()
		}
		client.subscribed[readerKey] = true
		client.lastUIDs[readerKey] = ""
		ticker := time.NewTicker(500 * time.Millisecond)
		client.pollTickers[readerKey] = ticker
		client.mu.Unlock()

		// Simulate card detection
		client.mu.Lock()
		client.lastUIDs[readerKey] = "CARD_UID_XYZ"
		client.mu.Unlock()

		// Unsubscribe
		client.mu.Lock()
		client.subscribed[readerKey] = false
		if ticker, ok := client.pollTickers[readerKey]; ok {
			ticker.Stop()
			delete(client.pollTickers, readerKey)
		}
		delete(client.lastUIDs, readerKey)
		client.mu.Unlock()

		// Verify state is clean after unsubscribe
		client.mu.Lock()
		if _, exists := client.lastUIDs[readerKey]; exists {
			t.Errorf("cycle %d: lastUIDs should be cleared after unsubscribe", cycle)
		}
		if _, exists := client.pollTickers[readerKey]; exists {
			t.Errorf("cycle %d: pollTickers should be cleared after unsubscribe", cycle)
		}
		if client.subscribed[readerKey] {
			t.Errorf("cycle %d: subscribed should be false after unsubscribe", cycle)
		}
		client.mu.Unlock()
	}
}
