"""WebSocket client for real-time NFC Agent communication."""

from __future__ import annotations

import asyncio
import contextlib
import json
import re
from dataclasses import dataclass
from typing import Any, Callable

import websockets
from websockets.asyncio.client import ClientConnection

from .exceptions import CardError, ConnectionError, NFCAgentError, TimeoutError
from .types import (
    Card,
    CardDataType,
    CardDetectedEvent,
    CardRemovedEvent,
    DerivedKeyData,
    HealthInfo,
    MifareBatchWriteResult,
    MifareBlockData,
    MifareBlockWriteResult,
    MifareKeyType,
    NDEFRecord,
    Reader,
    UltralightBatchWriteResult,
    UltralightPageData,
    UltralightPageWriteResult,
    VersionInfo,
)

DEFAULT_WS_URL = "ws://127.0.0.1:32145/v1/ws"
DEFAULT_WSS_URL = "wss://127.0.0.1:32145/v1/ws"
DEFAULT_TIMEOUT = 5.0
DEFAULT_RECONNECT_INTERVAL = 3.0

# Callback type aliases
CardDetectedCallback = Callable[[CardDetectedEvent], None]
CardRemovedCallback = Callable[[CardRemovedEvent], None]
ConnectionCallback = Callable[[], None]
ErrorCallback = Callable[[Exception], None]


def _to_snake_case(name: str) -> str:
    """Convert camelCase to snake_case."""
    return re.sub(r"(?<!^)(?=[A-Z])", "_", name).lower()


def _snake_case_keys(d: dict[str, Any]) -> dict[str, Any]:
    """Convert all keys in a dict from camelCase to snake_case."""
    return {_to_snake_case(k): v for k, v in d.items()}


@dataclass
class _PendingRequest:
    """Internal pending request tracker."""

    future: asyncio.Future[Any]
    timeout_handle: asyncio.TimerHandle


class NFCWebSocket:
    """
    WebSocket client for real-time NFC Agent communication.

    Provides persistent connection with automatic reconnection,
    request/response correlation, and event subscriptions.

    Example:
        async with NFCWebSocket() as ws:
            await ws.subscribe(0)

            @ws.on_card_detected
            def handle_card(event):
                print(f"Card detected: {event.card.uid}")

            # Keep running
            await asyncio.sleep(60)
    """

    def __init__(
        self,
        url: str | None = None,
        *,
        timeout: float = DEFAULT_TIMEOUT,
        auto_reconnect: bool = True,
        reconnect_interval: float = DEFAULT_RECONNECT_INTERVAL,
        secure: bool = False,
    ):
        """
        Create a new WebSocket client.

        Args:
            url: WebSocket URL (default: ws://127.0.0.1:32145/v1/ws)
            timeout: Request timeout in seconds (default: 5.0)
            auto_reconnect: Auto-reconnect on disconnect (default: True)
            reconnect_interval: Reconnect interval in seconds (default: 3.0)
            secure: Use wss:// instead of ws:// (default: False)
        """
        default_url = DEFAULT_WSS_URL if secure else DEFAULT_WS_URL
        self.url = url or default_url
        self.timeout = timeout
        self.auto_reconnect = auto_reconnect
        self.reconnect_interval = reconnect_interval

        self._ws: ClientConnection | None = None
        self._request_id = 0
        self._pending: dict[str, _PendingRequest] = {}
        self._is_closing = False
        self._reconnect_task: asyncio.Task[None] | None = None
        self._receive_task: asyncio.Task[None] | None = None

        # Event listeners
        self._on_card_detected: list[CardDetectedCallback] = []
        self._on_card_removed: list[CardRemovedCallback] = []
        self._on_connected: list[ConnectionCallback] = []
        self._on_disconnected: list[ConnectionCallback] = []
        self._on_error: list[ErrorCallback] = []

    @property
    def is_connected(self) -> bool:
        """Check if connected to the server."""
        return self._ws is not None and not self._ws.close_code

    async def __aenter__(self) -> NFCWebSocket:
        """Async context manager entry."""
        await self.connect()
        return self

    async def __aexit__(
        self,
        exc_type: Any,
        exc_val: Any,
        exc_tb: Any,
    ) -> None:
        """Async context manager exit."""
        await self.disconnect()

    async def connect(self) -> None:
        """Connect to the NFC Agent WebSocket server."""
        if self.is_connected:
            return

        self._is_closing = False

        try:
            self._ws = await websockets.connect(self.url)
            self._receive_task = asyncio.create_task(self._receive_loop())
            self._emit_connected()
        except Exception as e:
            raise ConnectionError(f"WebSocket connection failed: {e}") from e

    async def disconnect(self) -> None:
        """Disconnect from the server."""
        self._is_closing = True

        if self._reconnect_task:
            self._reconnect_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await self._reconnect_task
            self._reconnect_task = None

        if self._receive_task:
            self._receive_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await self._receive_task
            self._receive_task = None

        if self._ws:
            await self._ws.close()
            self._ws = None

        # Reject pending requests
        for _req_id, pending in self._pending.items():
            pending.timeout_handle.cancel()
            if not pending.future.done():
                pending.future.set_exception(ConnectionError("Connection closed"))
        self._pending.clear()

    # =========================================================================
    # Event Registration (Decorator Style)
    # =========================================================================

    def on_card_detected(
        self, callback: CardDetectedCallback
    ) -> CardDetectedCallback:
        """
        Register a card detected event handler.

        Can be used as a decorator:
            @ws.on_card_detected
            def handle_card(event):
                print(event.card.uid)
        """
        self._on_card_detected.append(callback)
        return callback

    def on_card_removed(self, callback: CardRemovedCallback) -> CardRemovedCallback:
        """Register a card removed event handler."""
        self._on_card_removed.append(callback)
        return callback

    def on_connected(self, callback: ConnectionCallback) -> ConnectionCallback:
        """Register a connected event handler."""
        self._on_connected.append(callback)
        return callback

    def on_disconnected(self, callback: ConnectionCallback) -> ConnectionCallback:
        """Register a disconnected event handler."""
        self._on_disconnected.append(callback)
        return callback

    def on_error(self, callback: ErrorCallback) -> ErrorCallback:
        """Register an error event handler."""
        self._on_error.append(callback)
        return callback

    # =========================================================================
    # API Methods
    # =========================================================================

    async def get_readers(self) -> list[Reader]:
        """List available NFC readers."""
        response = await self._request("list_readers")
        return [Reader(**r) for r in response]

    async def read_card(self, reader_index: int) -> Card:
        """
        Read card from a reader.

        Args:
            reader_index: Index of the reader (0-based)

        Raises:
            CardError: If read fails
        """
        try:
            response = await self._request("read_card", {"readerIndex": reader_index})
            return self._parse_card(response)
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def write_card(
        self,
        reader_index: int,
        *,
        data: str | None = None,
        data_type: str,
        url: str | None = None,
    ) -> None:
        """
        Write data to a card.

        Args:
            reader_index: Index of the reader
            data: Data to write
            data_type: Type of data ('text', 'json', 'binary', 'url')
            url: Optional URL
        """
        try:
            await self._request(
                "write_card",
                {
                    "readerIndex": reader_index,
                    "data": data,
                    "dataType": data_type,
                    "url": url,
                },
            )
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def erase_card(self, reader_index: int) -> None:
        """Erase NDEF data from a card."""
        try:
            await self._request("erase_card", {"readerIndex": reader_index})
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def lock_card(self, reader_index: int) -> None:
        """
        Lock a card permanently (IRREVERSIBLE!).

        Args:
            reader_index: Index of the reader
        """
        try:
            await self._request(
                "lock_card", {"readerIndex": reader_index, "confirm": True}
            )
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def set_password(self, reader_index: int, password: str) -> None:
        """
        Set password protection on an NTAG card.

        Args:
            reader_index: Index of the reader
            password: Password to set
        """
        try:
            await self._request(
                "set_password", {"readerIndex": reader_index, "password": password}
            )
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def remove_password(self, reader_index: int, password: str) -> None:
        """
        Remove password protection from an NTAG card.

        Args:
            reader_index: Index of the reader
            password: Current password
        """
        try:
            await self._request(
                "remove_password", {"readerIndex": reader_index, "password": password}
            )
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def write_records(
        self, reader_index: int, records: list[NDEFRecord]
    ) -> None:
        """
        Write multiple NDEF records to a card.

        Args:
            reader_index: Index of the reader
            records: List of NDEF records to write
        """
        record_dicts = []
        for r in records:
            d: dict[str, Any] = {"type": r.type, "data": r.data}
            if r.mime_type:
                d["mimeType"] = r.mime_type
            record_dicts.append(d)

        try:
            await self._request(
                "write_records",
                {"readerIndex": reader_index, "records": record_dicts},
            )
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def subscribe(self, reader_index: int) -> None:
        """Subscribe to card events on a reader."""
        await self._request("subscribe", {"readerIndex": reader_index})

    async def unsubscribe(self, reader_index: int) -> None:
        """Unsubscribe from card events on a reader."""
        await self._request("unsubscribe", {"readerIndex": reader_index})

    async def get_version(self) -> VersionInfo:
        """Get agent version information."""
        response = await self._request("version")
        return VersionInfo(
            version=response.get("version", ""),
            build_time=response.get("buildTime", ""),
            git_commit=response.get("gitCommit", ""),
            update_available=response.get("updateAvailable"),
            latest_version=response.get("latestVersion"),
            release_url=response.get("releaseUrl"),
        )

    async def health(self) -> HealthInfo:
        """Health check."""
        response = await self._request("health")
        return HealthInfo(
            status=response.get("status", "ok"),
            uptime=response.get("uptime"),
        )

    # =========================================================================
    # MIFARE Classic Methods
    # =========================================================================

    async def read_mifare_block(
        self,
        reader_index: int,
        block: int,
        *,
        key: str | None = None,
        key_type: MifareKeyType | None = None,
    ) -> MifareBlockData:
        """Read a raw 16-byte block from a MIFARE Classic card."""
        payload: dict[str, Any] = {"readerIndex": reader_index, "block": block}
        if key:
            payload["key"] = key
        if key_type:
            payload["keyType"] = key_type.value

        try:
            response = await self._request("read_mifare_block", payload)
            return MifareBlockData(block=response["block"], data=response["data"])
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def write_mifare_block(
        self,
        reader_index: int,
        block: int,
        *,
        data: str,
        key: str | None = None,
        key_type: MifareKeyType | None = None,
    ) -> None:
        """Write a raw 16-byte block to a MIFARE Classic card."""
        payload: dict[str, Any] = {
            "readerIndex": reader_index,
            "block": block,
            "data": data,
        }
        if key:
            payload["key"] = key
        if key_type:
            payload["keyType"] = key_type.value

        try:
            await self._request("write_mifare_block", payload)
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def write_mifare_blocks(
        self,
        reader_index: int,
        blocks: list[dict[str, Any]],
        *,
        key: str | None = None,
        key_type: MifareKeyType | None = None,
    ) -> MifareBatchWriteResult:
        """Write multiple blocks to a MIFARE Classic card."""
        payload: dict[str, Any] = {"readerIndex": reader_index, "blocks": blocks}
        if key:
            payload["key"] = key
        if key_type:
            payload["keyType"] = key_type.value

        try:
            response = await self._request("write_mifare_blocks", payload)
            results = [
                MifareBlockWriteResult(
                    block=r["block"],
                    success=r["success"],
                    error=r.get("error"),
                )
                for r in response.get("results", [])
            ]
            return MifareBatchWriteResult(
                results=results,
                written=response.get("written", 0),
                total=response.get("total", 0),
            )
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    # =========================================================================
    # MIFARE Ultralight Methods
    # =========================================================================

    async def read_ultralight_page(
        self,
        reader_index: int,
        page: int,
        *,
        password: str | None = None,
    ) -> UltralightPageData:
        """Read a 4-byte page from a MIFARE Ultralight card."""
        payload: dict[str, Any] = {"readerIndex": reader_index, "page": page}
        if password:
            payload["password"] = password

        try:
            response = await self._request("read_ultralight_page", payload)
            return UltralightPageData(page=response["page"], data=response["data"])
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def write_ultralight_page(
        self,
        reader_index: int,
        page: int,
        *,
        data: str,
        password: str | None = None,
    ) -> None:
        """Write a 4-byte page to a MIFARE Ultralight card."""
        payload: dict[str, Any] = {
            "readerIndex": reader_index,
            "page": page,
            "data": data,
        }
        if password:
            payload["password"] = password

        try:
            await self._request("write_ultralight_page", payload)
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def write_ultralight_pages(
        self,
        reader_index: int,
        pages: list[dict[str, Any]],
        *,
        password: str | None = None,
    ) -> UltralightBatchWriteResult:
        """Write multiple pages to a MIFARE Ultralight/NTAG card."""
        payload: dict[str, Any] = {"readerIndex": reader_index, "pages": pages}
        if password:
            payload["password"] = password

        try:
            response = await self._request("write_ultralight_pages", payload)
            results = [
                UltralightPageWriteResult(
                    page=r["page"],
                    success=r["success"],
                    error=r.get("error"),
                )
                for r in response.get("results", [])
            ]
            return UltralightBatchWriteResult(
                results=results,
                written=response.get("written", 0),
                total=response.get("total", 0),
            )
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    # =========================================================================
    # AES/Key Derivation Methods
    # =========================================================================

    async def derive_uid_key_aes(
        self, reader_index: int, aes_key: str
    ) -> DerivedKeyData:
        """Derive a 6-byte MIFARE sector key from the card's UID using AES."""
        try:
            response = await self._request(
                "derive_uid_key_aes",
                {"readerIndex": reader_index, "aesKey": aes_key},
            )
            return DerivedKeyData(key=response["key"])
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def aes_encrypt_and_write_block(
        self,
        reader_index: int,
        block: int,
        *,
        data: str,
        aes_key: str,
        auth_key: str,
        auth_key_type: MifareKeyType | None = None,
    ) -> None:
        """Encrypt data with AES-128-ECB and write to a MIFARE Classic block."""
        payload: dict[str, Any] = {
            "readerIndex": reader_index,
            "block": block,
            "data": data,
            "aesKey": aes_key,
            "authKey": auth_key,
        }
        if auth_key_type:
            payload["authKeyType"] = auth_key_type.value

        try:
            await self._request("aes_encrypt_and_write_block", payload)
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    async def write_mifare_sector_trailer(
        self,
        reader_index: int,
        block: int,
        *,
        key_a: str,
        key_b: str,
        auth_key: str,
        access_bits: str | None = None,
        auth_key_type: MifareKeyType | None = None,
    ) -> None:
        """Write a MIFARE Classic sector trailer with new keys."""
        payload: dict[str, Any] = {
            "readerIndex": reader_index,
            "block": block,
            "keyA": key_a,
            "keyB": key_b,
            "authKey": auth_key,
        }
        if access_bits:
            payload["accessBits"] = access_bits
        if auth_key_type:
            payload["authKeyType"] = auth_key_type.value

        try:
            await self._request("write_mifare_sector_trailer", payload)
        except NFCAgentError as e:
            raise CardError(str(e)) from e

    # =========================================================================
    # Internal Methods
    # =========================================================================

    async def _request(
        self, msg_type: str, payload: dict[str, Any] | None = None
    ) -> Any:
        """Send a request and wait for response."""
        if not self.is_connected:
            raise ConnectionError("Not connected to NFC Agent")

        self._request_id += 1
        req_id = f"req-{self._request_id}"

        loop = asyncio.get_event_loop()
        future: asyncio.Future[Any] = loop.create_future()

        def on_timeout() -> None:
            if not future.done():
                future.set_exception(TimeoutError("Request timed out"))
                self._pending.pop(req_id, None)

        timeout_handle = loop.call_later(self.timeout, on_timeout)
        self._pending[req_id] = _PendingRequest(future, timeout_handle)

        message = json.dumps({"type": msg_type, "id": req_id, "payload": payload})

        assert self._ws is not None
        await self._ws.send(message)
        return await future

    async def _receive_loop(self) -> None:
        """Background task to receive WebSocket messages."""
        try:
            assert self._ws is not None
            async for message in self._ws:
                if isinstance(message, str):
                    await self._handle_message(message)
        except websockets.ConnectionClosed:
            pass
        finally:
            self._emit_disconnected()
            if self.auto_reconnect and not self._is_closing:
                self._schedule_reconnect()

    async def _handle_message(self, raw: str) -> None:
        """Handle an incoming WebSocket message."""
        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            return

        msg_type = data.get("type")

        # Handle events
        if msg_type == "card_detected":
            detected_payload = data.get("payload", {})
            card = self._parse_card(detected_payload.get("card", {}))
            detected_event = CardDetectedEvent(
                reader=detected_payload.get("reader", 0), card=card
            )
            for detected_cb in self._on_card_detected:
                with contextlib.suppress(Exception):
                    detected_cb(detected_event)
            return

        if msg_type == "card_removed":
            removed_payload = data.get("payload", {})
            removed_event = CardRemovedEvent(reader=removed_payload.get("reader", 0))
            for removed_cb in self._on_card_removed:
                with contextlib.suppress(Exception):
                    removed_cb(removed_event)
            return

        # Handle responses
        req_id = data.get("id")
        if req_id and req_id in self._pending:
            pending = self._pending.pop(req_id)
            pending.timeout_handle.cancel()

            if data.get("error"):
                pending.future.set_exception(NFCAgentError(data["error"]))
            else:
                pending.future.set_result(data.get("payload"))

    def _schedule_reconnect(self) -> None:
        """Schedule a reconnection attempt."""

        async def reconnect() -> None:
            await asyncio.sleep(self.reconnect_interval)
            try:  # noqa: SIM105 - contextlib.suppress doesn't work with await
                await self.connect()
            except ConnectionError:
                pass  # Will try again

        self._reconnect_task = asyncio.create_task(reconnect())

    def _emit_connected(self) -> None:
        """Emit connected event to all listeners."""
        for callback in self._on_connected:
            with contextlib.suppress(Exception):
                callback()

    def _emit_disconnected(self) -> None:
        """Emit disconnected event to all listeners."""
        for callback in self._on_disconnected:
            with contextlib.suppress(Exception):
                callback()

    @staticmethod
    def _parse_card(data: dict[str, Any]) -> Card:
        """Parse card data from response."""
        data_type = None
        if "dataType" in data:
            try:
                data_type = CardDataType(data["dataType"])
            except ValueError:
                data_type = CardDataType.UNKNOWN

        return Card(
            uid=data.get("uid", ""),
            atr=data.get("atr"),
            type=data.get("type"),
            protocol=data.get("protocol"),
            protocol_iso=data.get("protocolISO"),
            size=data.get("size"),
            writable=data.get("writable"),
            data=data.get("data"),
            data_type=data_type,
        )
