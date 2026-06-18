"""Exception classes for NFC Agent SDK."""

from typing import Optional


class NFCAgentError(Exception):
    """Base exception for NFC Agent SDK errors."""

    pass


class ConnectionError(NFCAgentError):
    """Raised when connection to nfc-agent server fails."""

    def __init__(self, message: str = "Failed to connect to nfc-agent server"):
        super().__init__(message)


class ReaderError(NFCAgentError):
    """Raised for reader-related issues."""

    pass


class CardError(NFCAgentError):
    """Raised for card-related issues (read/write failures, no card present)."""

    pass


class DesfireError(CardError):
    """Raised for transparent DESFire session issues (open/transmit/close).

    The agent performs no DESFire crypto and holds no keys -- the caller drives
    the handshake -- so any failure surfaces here. When the agent reports a
    DESFire status word (e.g. ``status 0x91AF``), it is parsed into
    :attr:`status_code` for convenience.
    """

    def __init__(self, message: str, status_code: Optional[int] = None):
        super().__init__(message)
        self.status_code = status_code


class APIError(NFCAgentError):
    """Raised when the API returns an error response."""

    def __init__(self, message: str, status_code: int):
        super().__init__(message)
        self.status_code = status_code


class TimeoutError(NFCAgentError):
    """Raised when a request times out."""

    pass
