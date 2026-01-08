"""
NFC Agent Python SDK

A Python client for interacting with NFC readers via the nfc-agent local server.

Example:
    from nfc_agent import NFCClient, NFCWebSocket

    # REST API (simple operations)
    async with NFCClient() as client:
        readers = await client.get_readers()
        card = await client.read_card(0)
        print(f"Card UID: {card.uid}")

    # WebSocket (real-time events)
    async with NFCWebSocket() as ws:
        await ws.subscribe(0)

        @ws.on_card_detected
        def handle_card(event):
            print(f"Card: {event.card.uid}")

        await asyncio.sleep(60)
"""

from .client import NFCClient
from .exceptions import (
    APIError,
    CardError,
    ConnectionError,
    NFCAgentError,
    ReaderError,
    TimeoutError,
)
from .poller import CardPoller
from .types import (
    Card,
    CardDataType,
    CardDetectedEvent,
    CardRemovedEvent,
    DerivedKeyData,
    HealthInfo,
    MifareBatchWriteResult,
    MifareBlockData,
    MifareBlockWriteOp,
    MifareBlockWriteResult,
    MifareKeyType,
    NDEFRecord,
    Reader,
    SupportedReader,
    SupportedReaderCapabilities,
    SupportedReadersResponse,
    UltralightBatchWriteResult,
    UltralightPageData,
    UltralightPageWriteOp,
    UltralightPageWriteResult,
    VersionInfo,
)
from .websocket import NFCWebSocket

__version__ = "0.5.1"
__all__ = [
    # Clients
    "NFCClient",
    "NFCWebSocket",
    "CardPoller",
    # Exceptions
    "NFCAgentError",
    "ConnectionError",
    "ReaderError",
    "CardError",
    "APIError",
    "TimeoutError",
    # Types
    "Reader",
    "Card",
    "CardDataType",
    "VersionInfo",
    "HealthInfo",
    "CardDetectedEvent",
    "CardRemovedEvent",
    "NDEFRecord",
    "MifareKeyType",
    "MifareBlockData",
    "MifareBlockWriteOp",
    "MifareBlockWriteResult",
    "MifareBatchWriteResult",
    "UltralightPageData",
    "UltralightPageWriteOp",
    "UltralightPageWriteResult",
    "UltralightBatchWriteResult",
    "DerivedKeyData",
    "SupportedReader",
    "SupportedReaderCapabilities",
    "SupportedReadersResponse",
]
