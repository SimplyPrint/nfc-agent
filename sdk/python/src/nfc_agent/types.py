"""Type definitions for NFC Agent SDK."""

from dataclasses import dataclass
from enum import Enum
from typing import Literal, Optional


class CardDataType(str, Enum):
    """Data type for card content."""

    TEXT = "text"
    JSON = "json"
    BINARY = "binary"
    URL = "url"
    UNKNOWN = "unknown"


class MifareKeyType(str, Enum):
    """MIFARE Classic key type for authentication."""

    A = "A"
    B = "B"


@dataclass
class Reader:
    """Represents an NFC reader device."""

    id: str
    """Unique identifier for the reader."""

    name: str
    """Human-readable name of the reader."""

    type: str
    """Reader type (e.g., 'picc' for contactless)."""


@dataclass
class Card:
    """Represents an NFC card/tag."""

    uid: str
    """Unique identifier (hex encoded)."""

    atr: Optional[str] = None
    """Answer To Reset (hex encoded)."""

    type: Optional[str] = None
    """Card type (e.g., 'NTAG213', 'NTAG215', 'MIFARE Classic')."""

    protocol: Optional[str] = None
    """Short protocol name (e.g., 'NFC-A', 'NFC-V')."""

    protocol_iso: Optional[str] = None
    """Full ISO protocol name (e.g., 'ISO 14443-3A', 'ISO 15693')."""

    size: Optional[int] = None
    """Memory size in bytes."""

    writable: Optional[bool] = None
    """Whether the card can be written to."""

    data: Optional[str] = None
    """Data stored on the card."""

    data_type: Optional[CardDataType] = None
    """Type of data stored."""


@dataclass
class VersionInfo:
    """Version information from the agent."""

    version: str
    """Current version string."""

    build_time: str
    """Build timestamp."""

    git_commit: str
    """Git commit hash."""

    update_available: Optional[bool] = None
    """Whether a newer version is available."""

    latest_version: Optional[str] = None
    """Latest available version (if update_available is True)."""

    release_url: Optional[str] = None
    """URL to download the latest release."""


@dataclass
class HealthInfo:
    """Health check response."""

    status: Literal["ok", "degraded", "error"]
    """Current health status."""

    uptime: Optional[int] = None
    """Uptime in seconds."""


@dataclass
class CardDetectedEvent:
    """Card detected event payload."""

    reader: int
    """Reader index where card was detected."""

    card: Card
    """Card information."""


@dataclass
class CardRemovedEvent:
    """Card removed event payload."""

    reader: int
    """Reader index where card was removed."""


@dataclass
class NDEFRecord:
    """Single NDEF record for write_records."""

    type: Literal["text", "url", "json", "binary", "mime"]
    """Record type."""

    data: str
    """Record data."""

    mime_type: Optional[str] = None
    """MIME type (for 'mime' type records)."""


@dataclass
class MifareBlockData:
    """Response from reading a MIFARE Classic block."""

    block: int
    """Block number that was read."""

    data: str
    """Block data as hex string (32 characters = 16 bytes)."""


@dataclass
class MifareBlockWriteOp:
    """A single block write operation for batch writes."""

    block: int
    """Block number (0-255, excluding sector trailers)."""

    data: str
    """Block data as hex string (32 characters = 16 bytes)."""


@dataclass
class MifareBlockWriteResult:
    """Result of a single block write in a batch operation."""

    block: int
    """Block number."""

    success: bool
    """Whether the write succeeded."""

    error: Optional[str] = None
    """Error message if write failed."""


@dataclass
class MifareBatchWriteResult:
    """Response from batch writing MIFARE Classic blocks."""

    results: list[MifareBlockWriteResult]
    """Results for each block write."""

    written: int
    """Number of blocks successfully written."""

    total: int
    """Total number of blocks attempted."""


@dataclass
class UltralightPageData:
    """Response from reading a MIFARE Ultralight page."""

    page: int
    """Page number that was read."""

    data: str
    """Page data as hex string (8 characters = 4 bytes)."""


@dataclass
class UltralightPageWriteOp:
    """A single page write operation for batch writes."""

    page: int
    """Page number (4-255 for user data)."""

    data: str
    """Page data as hex string (8 characters = 4 bytes)."""


@dataclass
class UltralightPageWriteResult:
    """Result of a single page write in a batch operation."""

    page: int
    """Page number."""

    success: bool
    """Whether the write succeeded."""

    error: Optional[str] = None
    """Error message if write failed."""


@dataclass
class UltralightBatchWriteResult:
    """Response from batch writing MIFARE Ultralight pages."""

    results: list[UltralightPageWriteResult]
    """Results for each page write."""

    written: int
    """Number of pages successfully written."""

    total: int
    """Total number of pages attempted."""


@dataclass
class DerivedKeyData:
    """Response from deriving a UID key via AES."""

    key: str
    """Derived 6-byte MIFARE key as hex string (12 characters)."""


@dataclass
class SupportedReaderCapabilities:
    """Capabilities of a supported reader."""

    read: bool
    write: bool
    ndef: bool


@dataclass
class SupportedReader:
    """Information about a supported reader model."""

    name: str
    manufacturer: str
    description: str
    supported_tags: list[str]
    capabilities: SupportedReaderCapabilities
    limitations: Optional[list[str]] = None


@dataclass
class SupportedReadersResponse:
    """Response from the supported readers endpoint."""

    readers: list[SupportedReader]
