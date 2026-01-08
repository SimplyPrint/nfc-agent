"""Exception classes for NFC Agent SDK."""


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


class APIError(NFCAgentError):
    """Raised when the API returns an error response."""

    def __init__(self, message: str, status_code: int):
        super().__init__(message)
        self.status_code = status_code


class TimeoutError(NFCAgentError):
    """Raised when a request times out."""

    pass
