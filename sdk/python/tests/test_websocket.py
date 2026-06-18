"""Tests for NFCWebSocket."""

import asyncio
import json

import pytest

from nfc_agent import ConnectionError, DesfireError, NFCWebSocket
from nfc_agent.types import CardDetectedEvent, DesfireResponse, DesfireSessionInfo


class _FakeWS:
    """Minimal stand-in for a websockets ClientConnection.

    Captures sent frames and lets a test feed a reply back through the client's
    own _handle_message so request/response correlation runs end to end.
    """

    def __init__(self, client: NFCWebSocket):
        self._client = client
        self.sent: list[dict] = []
        self.close_code = None
        # Optional canned reply builder: (request dict) -> response dict
        self.reply = None

    async def send(self, message: str) -> None:
        request = json.loads(message)
        self.sent.append(request)
        if self.reply is not None:
            response = self.reply(request)
            await self._client._handle_message(json.dumps(response))


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
    async def test_handle_readers_changed_event(self):
        """Test handling readers_changed event."""
        ws = NFCWebSocket()
        events_received = []

        @ws.on_readers_changed
        def handle_readers(event):
            events_received.append(event)

        message = json.dumps(
            {
                "type": "readers_changed",
                "payload": {
                    "readers": [
                        {"id": "reader-0", "name": "ACR122U PICC", "type": "picc"}
                    ]
                },
            }
        )

        await ws._handle_message(message)

        assert len(events_received) == 1
        assert len(events_received[0].readers) == 1
        assert events_received[0].readers[0].name == "ACR122U PICC"
        assert events_received[0].readers[0].type == "picc"

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

    # =========================================================================
    # DESFire Transparent Session
    # =========================================================================

    @pytest.mark.asyncio
    async def test_desfire_session_lifecycle(self):
        """Test open/transmit/close send the right messages and parse replies."""
        ws = NFCWebSocket()
        fake = _FakeWS(ws)
        ws._ws = fake

        replies = {
            "desfire_session_open": lambda req: {
                "type": "desfire_session_opened",
                "id": req["id"],
                "payload": {
                    "readerIndex": req["payload"]["readerIndex"],
                    "readerName": "ACR122U PICC",
                    "uid": "04AABBCCDD",
                    "atr": "3B8180018080",
                },
            },
            "desfire_transmit": lambda req: {
                "type": "desfire_response",
                "id": req["id"],
                "payload": {"response": "0102910091AF", "sw1": 145, "sw2": 175},
            },
            "desfire_session_close": lambda req: {
                "type": "desfire_session_closed",
                "id": req["id"],
                "payload": {
                    "readerIndex": req["payload"]["readerIndex"],
                    "readerName": "ACR122U PICC",
                },
            },
        }
        fake.reply = lambda req: replies[req["type"]](req)

        # open
        info = await ws.open_desfire_session(0)
        assert isinstance(info, DesfireSessionInfo)
        assert info.reader_name == "ACR122U PICC"
        assert info.uid == "04AABBCCDD"
        assert info.atr == "3B8180018080"
        assert fake.sent[0]["type"] == "desfire_session_open"
        assert fake.sent[0]["payload"] == {"readerIndex": 0}

        # transmit
        resp = await ws.desfire_transmit(0, apdu="9070000000")
        assert isinstance(resp, DesfireResponse)
        assert resp.response == "0102910091AF"
        assert resp.sw1 == 145
        assert resp.sw2 == 175
        assert fake.sent[1]["type"] == "desfire_transmit"
        assert fake.sent[1]["payload"] == {"readerIndex": 0, "apdu": "9070000000"}

        # close
        result = await ws.close_desfire_session(0)
        assert result is None
        assert fake.sent[2]["type"] == "desfire_session_close"
        assert fake.sent[2]["payload"] == {"readerIndex": 0}

    @pytest.mark.asyncio
    async def test_desfire_transmit_batch(self):
        """Test batch transmit sends apdus list and parses responses list."""
        ws = NFCWebSocket()
        fake = _FakeWS(ws)
        ws._ws = fake

        fake.reply = lambda req: {
            "type": "desfire_responses",
            "id": req["id"],
            "payload": {
                "responses": [
                    {"response": "9100", "sw1": 145, "sw2": 0},
                    {"response": "AF", "sw1": None, "sw2": None},
                ]
            },
        }

        responses = await ws.desfire_transmit_batch(0, apdus=["9070000000", "90AF000000"])
        assert fake.sent[0]["type"] == "desfire_transmit_batch"
        assert fake.sent[0]["payload"] == {
            "readerIndex": 0,
            "apdus": ["9070000000", "90AF000000"],
        }
        assert len(responses) == 2
        assert responses[0].response == "9100"
        assert responses[0].sw1 == 145
        assert responses[1].response == "AF"
        assert responses[1].sw1 is None

    @pytest.mark.asyncio
    async def test_desfire_error_parses_status_code(self):
        """Test a desfire error reply raises DesfireError with parsed status."""
        ws = NFCWebSocket()
        fake = _FakeWS(ws)
        ws._ws = fake

        fake.reply = lambda req: {
            "type": "error",
            "id": req["id"],
            "error": "transmit failed: status 0xAE (authentication error)",
        }

        with pytest.raises(DesfireError) as exc_info:
            await ws.desfire_transmit(0, apdu="9070000000")

        assert exc_info.value.status_code == 0xAE
        assert "0xAE" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_desfire_error_without_status_code(self):
        """Test a desfire error without a status word leaves status_code None."""
        ws = NFCWebSocket()
        fake = _FakeWS(ws)
        ws._ws = fake

        fake.reply = lambda req: {
            "type": "error",
            "id": req["id"],
            "error": "no DESFire session open for this reader",
        }

        with pytest.raises(DesfireError) as exc_info:
            await ws.desfire_transmit(0, apdu="9070000000")

        assert exc_info.value.status_code is None
