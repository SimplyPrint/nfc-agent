"""Tests for NFCWebSocket."""

import asyncio
import json

import pytest

from nfc_agent import ConnectionError, NFCWebSocket
from nfc_agent.types import CardDetectedEvent


class TestNFCWebSocket:
    """Tests for NFCWebSocket class."""

    @pytest.mark.asyncio
    async def test_init_defaults(self):
        """Test default initialization."""
        ws = NFCWebSocket()
        assert ws.url == "ws://127.0.0.1:32145/v1/ws"
        assert ws.timeout == 5.0
        assert ws.auto_reconnect is True
        assert ws.is_connected is False

    @pytest.mark.asyncio
    async def test_init_secure(self):
        """Test secure WebSocket initialization."""
        ws = NFCWebSocket(secure=True)
        assert ws.url == "wss://127.0.0.1:32145/v1/ws"

    @pytest.mark.asyncio
    async def test_init_custom_url(self):
        """Test custom URL initialization."""
        ws = NFCWebSocket(url="ws://localhost:8080/ws")
        assert ws.url == "ws://localhost:8080/ws"

    @pytest.mark.asyncio
    async def test_event_registration_decorator(self):
        """Test event registration as decorator."""
        ws = NFCWebSocket()
        events_received = []

        @ws.on_card_detected
        def handle_card(event):
            events_received.append(event)

        @ws.on_card_removed
        def handle_removed(event):
            events_received.append("removed")

        @ws.on_connected
        def handle_connected():
            events_received.append("connected")

        assert len(ws._on_card_detected) == 1
        assert len(ws._on_card_removed) == 1
        assert len(ws._on_connected) == 1

    @pytest.mark.asyncio
    async def test_handle_card_detected_event(self):
        """Test handling card_detected event."""
        ws = NFCWebSocket()
        events_received = []

        @ws.on_card_detected
        def handle_card(event: CardDetectedEvent):
            events_received.append(event)

        # Simulate receiving a card_detected message
        message = json.dumps(
            {
                "type": "card_detected",
                "payload": {
                    "reader": 0,
                    "card": {
                        "uid": "04AABBCCDD",
                        "type": "NTAG215",
                    },
                },
            }
        )

        await ws._handle_message(message)

        assert len(events_received) == 1
        assert events_received[0].reader == 0
        assert events_received[0].card.uid == "04AABBCCDD"

    @pytest.mark.asyncio
    async def test_handle_card_removed_event(self):
        """Test handling card_removed event."""
        ws = NFCWebSocket()
        events_received = []

        @ws.on_card_removed
        def handle_removed(event):
            events_received.append(event)

        message = json.dumps(
            {
                "type": "card_removed",
                "payload": {"reader": 0},
            }
        )

        await ws._handle_message(message)

        assert len(events_received) == 1
        assert events_received[0].reader == 0

    @pytest.mark.asyncio
    async def test_handle_response(self):
        """Test handling response to a request."""
        ws = NFCWebSocket()
        ws._request_id = 0

        # Create a pending request
        loop = asyncio.get_event_loop()
        future: asyncio.Future = loop.create_future()
        timeout_handle = loop.call_later(10, lambda: None)

        from nfc_agent.websocket import _PendingRequest

        ws._pending["req-1"] = _PendingRequest(future, timeout_handle)

        # Simulate response
        message = json.dumps(
            {
                "type": "list_readers",
                "id": "req-1",
                "payload": [{"id": "0", "name": "Test Reader", "type": "picc"}],
            }
        )

        await ws._handle_message(message)

        assert future.done()
        result = await future
        assert len(result) == 1
        assert result[0]["name"] == "Test Reader"

    @pytest.mark.asyncio
    async def test_handle_error_response(self):
        """Test handling error response."""
        ws = NFCWebSocket()

        loop = asyncio.get_event_loop()
        future: asyncio.Future = loop.create_future()
        timeout_handle = loop.call_later(10, lambda: None)

        from nfc_agent.websocket import _PendingRequest

        ws._pending["req-1"] = _PendingRequest(future, timeout_handle)

        message = json.dumps(
            {
                "type": "read_card",
                "id": "req-1",
                "error": "no card present",
            }
        )

        await ws._handle_message(message)

        assert future.done()
        with pytest.raises(Exception) as exc_info:
            await future
        assert "no card present" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_request_not_connected(self):
        """Test request when not connected."""
        ws = NFCWebSocket()

        with pytest.raises(ConnectionError) as exc_info:
            await ws._request("list_readers")

        assert "Not connected" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_invalid_json_ignored(self):
        """Test that invalid JSON is ignored."""
        ws = NFCWebSocket()

        # Should not raise
        await ws._handle_message("not valid json")
        await ws._handle_message("{incomplete")
