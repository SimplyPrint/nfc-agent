"""REST API client for NFC Agent."""

from __future__ import annotations

import re
from typing import TYPE_CHECKING, Any

import httpx

from .exceptions import APIError, CardError, ConnectionError, TimeoutError
from .types import (
    Card,
    CardDataType,
    DerivedKeyData,
    MifareBatchWriteResult,
    MifareBlockData,
    MifareBlockWriteResult,
    MifareKeyType,
    Reader,
    SupportedReader,
    SupportedReaderCapabilities,
    SupportedReadersResponse,
    UltralightBatchWriteResult,
    UltralightPageData,
    UltralightPageWriteResult,
    VersionInfo,
)

if TYPE_CHECKING:
    from .poller import CardPoller

DEFAULT_BASE_URL = "http://127.0.0.1:32145"
DEFAULT_TIMEOUT = 5.0


def _to_snake_case(name: str) -> str:
    """Convert camelCase to snake_case."""
    return re.sub(r"(?<!^)(?=[A-Z])", "_", name).lower()


def _snake_case_keys(d: dict[str, Any]) -> dict[str, Any]:
    """Convert all keys in a dict from camelCase to snake_case."""
    return {_to_snake_case(k): v for k, v in d.items()}


class NFCClient:
    """
    Client for interacting with the NFC Agent local server.

    Supports both synchronous and asynchronous usage via context managers.

    Example (async):
        async with NFCClient() as client:
            readers = await client.get_readers()
            card = await client.read_card(0)

    Example (sync):
        with NFCClient() as client:
            readers = client.get_readers_sync()
            card = client.read_card_sync(0)
    """

    def __init__(
        self,
        base_url: str = DEFAULT_BASE_URL,
        timeout: float = DEFAULT_TIMEOUT,
    ):
        """
        Create a new NFC Agent client.

        Args:
            base_url: Base URL of the nfc-agent server (default: "http://127.0.0.1:32145")
            timeout: Request timeout in seconds (default: 5.0)
        """
        self.base_url = base_url
        self.timeout = timeout
        self._async_client: httpx.AsyncClient | None = None
        self._sync_client: httpx.Client | None = None

    async def __aenter__(self) -> NFCClient:
        """Async context manager entry."""
        self._async_client = httpx.AsyncClient(
            base_url=self.base_url,
            timeout=self.timeout,
        )
        return self

    async def __aexit__(
        self,
        exc_type: Any,
        exc_val: Any,
        exc_tb: Any,
    ) -> None:
        """Async context manager exit."""
        if self._async_client:
            await self._async_client.aclose()
            self._async_client = None

    def __enter__(self) -> NFCClient:
        """Sync context manager entry."""
        self._sync_client = httpx.Client(
            base_url=self.base_url,
            timeout=self.timeout,
        )
        return self

    def __exit__(
        self,
        exc_type: Any,
        exc_val: Any,
        exc_tb: Any,
    ) -> None:
        """Sync context manager exit."""
        if self._sync_client:
            self._sync_client.close()
            self._sync_client = None

    async def _request(
        self,
        method: str,
        path: str,
        *,
        params: dict[str, Any] | None = None,
        json: dict[str, Any] | None = None,
    ) -> Any:
        """Make an async HTTP request."""
        client = self._async_client
        should_close = False

        if client is None:
            client = httpx.AsyncClient(
                base_url=self.base_url,
                timeout=self.timeout,
            )
            should_close = True

        try:
            response = await client.request(
                method,
                path,
                params=params,
                json=json,
                headers={"Content-Type": "application/json"},
            )

            data = response.json()

            if not response.is_success:
                error_msg = data.get("error", f"HTTP {response.status_code}")
                raise APIError(error_msg, response.status_code)

            return data

        except httpx.TimeoutException as e:
            raise TimeoutError("Request timed out") from e
        except httpx.ConnectError as e:
            raise ConnectionError(
                "Failed to connect to nfc-agent. Is the agent running?"
            ) from e
        finally:
            if should_close:
                await client.aclose()

    def _request_sync(
        self,
        method: str,
        path: str,
        *,
        params: dict[str, Any] | None = None,
        json: dict[str, Any] | None = None,
    ) -> Any:
        """Make a sync HTTP request."""
        client = self._sync_client
        should_close = False

        if client is None:
            client = httpx.Client(
                base_url=self.base_url,
                timeout=self.timeout,
            )
            should_close = True

        try:
            response = client.request(
                method,
                path,
                params=params,
                json=json,
                headers={"Content-Type": "application/json"},
            )

            data = response.json()

            if not response.is_success:
                error_msg = data.get("error", f"HTTP {response.status_code}")
                raise APIError(error_msg, response.status_code)

            return data

        except httpx.TimeoutException as e:
            raise TimeoutError("Request timed out") from e
        except httpx.ConnectError as e:
            raise ConnectionError(
                "Failed to connect to nfc-agent. Is the agent running?"
            ) from e
        finally:
            if should_close:
                client.close()

    # =========================================================================
    # Async Methods
    # =========================================================================

    async def is_connected(self) -> bool:
        """Check if the nfc-agent server is running and accessible."""
        try:
            await self.get_readers()
            return True
        except Exception:
            return False

    async def get_readers(self) -> list[Reader]:
        """Get a list of available NFC readers."""
        data = await self._request("GET", "/v1/readers")
        return [Reader(**r) for r in data]

    async def read_card(self, reader_index: int) -> Card:
        """
        Read card data from a specific reader.

        Args:
            reader_index: Index of the reader (0-based)

        Returns:
            Card data if a card is present

        Raises:
            CardError: If no card is present or read fails
        """
        try:
            data = await self._request("GET", f"/v1/readers/{reader_index}/card")
            return self._parse_card(data)
        except APIError as e:
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
        Write data to a card on a specific reader.

        Args:
            reader_index: Index of the reader (0-based)
            data: Data to write
            data_type: Type of data ('text', 'json', 'binary', 'url')
            url: Optional URL to write as first NDEF record

        Raises:
            CardError: If write fails
        """
        body: dict[str, Any] = {"dataType": data_type}

        if data_type == "url":
            body["data"] = data or url or ""
        else:
            if data:
                body["data"] = data
            if url:
                body["url"] = url

        try:
            await self._request("POST", f"/v1/readers/{reader_index}/card", json=body)
        except APIError as e:
            raise CardError(str(e)) from e

    async def get_version(self) -> VersionInfo:
        """Get agent version information."""
        data = await self._request("GET", "/v1/version")
        return self._parse_version_info(data)

    async def get_supported_readers(self) -> SupportedReadersResponse:
        """Get information about supported reader hardware."""
        data = await self._request("GET", "/v1/supported-readers")
        readers = []
        for r in data.get("readers", []):
            caps = r.get("capabilities", {})
            readers.append(
                SupportedReader(
                    name=r["name"],
                    manufacturer=r["manufacturer"],
                    description=r["description"],
                    supported_tags=r.get("supportedTags", []),
                    capabilities=SupportedReaderCapabilities(
                        read=caps.get("read", False),
                        write=caps.get("write", False),
                        ndef=caps.get("ndef", False),
                    ),
                    limitations=r.get("limitations"),
                )
            )
        return SupportedReadersResponse(readers=readers)

    async def read_mifare_block(
        self,
        reader_index: int,
        block: int,
        *,
        key: str | None = None,
        key_type: MifareKeyType | None = None,
    ) -> MifareBlockData:
        """
        Read a raw 16-byte block from a MIFARE Classic card.

        Args:
            reader_index: Index of the reader (0-based)
            block: Block number to read (0-63 for 1K, 0-255 for 4K)
            key: Authentication key as hex string (12 chars = 6 bytes)
            key_type: Key type 'A' or 'B' (default: 'A')

        Returns:
            Block data (16 bytes as hex string)

        Raises:
            CardError: If read fails or authentication fails
        """
        params: dict[str, Any] = {}
        if key:
            params["key"] = key
        if key_type:
            params["keyType"] = key_type.value

        try:
            data = await self._request(
                "GET",
                f"/v1/readers/{reader_index}/mifare/{block}",
                params=params if params else None,
            )
            return MifareBlockData(block=data["block"], data=data["data"])
        except APIError as e:
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
        """
        Write a raw 16-byte block to a MIFARE Classic card.

        Args:
            reader_index: Index of the reader (0-based)
            block: Block number to write
            data: Block data as hex string (32 chars = 16 bytes)
            key: Authentication key as hex string
            key_type: Key type 'A' or 'B'

        Raises:
            CardError: If write fails or authentication fails
        """
        body: dict[str, Any] = {"data": data}
        if key:
            body["key"] = key
        if key_type:
            body["keyType"] = key_type.value

        try:
            await self._request(
                "POST",
                f"/v1/readers/{reader_index}/mifare/{block}",
                json=body,
            )
        except APIError as e:
            raise CardError(str(e)) from e

    async def write_mifare_blocks(
        self,
        reader_index: int,
        blocks: list[dict[str, Any]],
        *,
        key: str | None = None,
        key_type: MifareKeyType | None = None,
    ) -> MifareBatchWriteResult:
        """
        Write multiple blocks to a MIFARE Classic card in a single session.

        Args:
            reader_index: Index of the reader (0-based)
            blocks: List of {"block": int, "data": str} dicts
            key: Authentication key
            key_type: Key type

        Returns:
            Batch write results

        Raises:
            CardError: If batch write fails
        """
        body: dict[str, Any] = {"blocks": blocks}
        if key:
            body["key"] = key
        if key_type:
            body["keyType"] = key_type.value

        try:
            data = await self._request(
                "POST",
                f"/v1/readers/{reader_index}/mifare/batch",
                json=body,
            )
            return self._parse_mifare_batch_result(data)
        except APIError as e:
            raise CardError(str(e)) from e

    async def read_ultralight_page(
        self,
        reader_index: int,
        page: int,
        *,
        password: str | None = None,
    ) -> UltralightPageData:
        """
        Read a 4-byte page from a MIFARE Ultralight card.

        Args:
            reader_index: Index of the reader (0-based)
            page: Page number to read
            password: Optional password for EV1 cards

        Returns:
            Page data (4 bytes as hex string)

        Raises:
            CardError: If read fails
        """
        params: dict[str, Any] = {}
        if password:
            params["password"] = password

        try:
            data = await self._request(
                "GET",
                f"/v1/readers/{reader_index}/ultralight/{page}",
                params=params if params else None,
            )
            return UltralightPageData(page=data["page"], data=data["data"])
        except APIError as e:
            raise CardError(str(e)) from e

    async def write_ultralight_page(
        self,
        reader_index: int,
        page: int,
        *,
        data: str,
        password: str | None = None,
    ) -> None:
        """
        Write a 4-byte page to a MIFARE Ultralight card.

        Args:
            reader_index: Index of the reader (0-based)
            page: Page number to write
            data: Page data as hex string (8 chars = 4 bytes)
            password: Optional password

        Raises:
            CardError: If write fails
        """
        body: dict[str, Any] = {"data": data}
        if password:
            body["password"] = password

        try:
            await self._request(
                "POST",
                f"/v1/readers/{reader_index}/ultralight/{page}",
                json=body,
            )
        except APIError as e:
            raise CardError(str(e)) from e

    async def write_ultralight_pages(
        self,
        reader_index: int,
        pages: list[dict[str, Any]],
        *,
        password: str | None = None,
    ) -> UltralightBatchWriteResult:
        """
        Write multiple pages to a MIFARE Ultralight/NTAG card.

        Args:
            reader_index: Index of the reader
            pages: List of {"page": int, "data": str} dicts
            password: Optional password

        Returns:
            Batch write results
        """
        body: dict[str, Any] = {"pages": pages}
        if password:
            body["password"] = password

        try:
            data = await self._request(
                "POST",
                f"/v1/readers/{reader_index}/ultralight/batch",
                json=body,
            )
            return self._parse_ultralight_batch_result(data)
        except APIError as e:
            raise CardError(str(e)) from e

    async def derive_uid_key_aes(
        self,
        reader_index: int,
        aes_key: str,
    ) -> DerivedKeyData:
        """
        Derive a 6-byte MIFARE sector key from the card's UID using AES-128-ECB.

        Args:
            reader_index: Index of the reader
            aes_key: AES-128 key as hex string (32 chars = 16 bytes)

        Returns:
            Derived 6-byte key as hex string

        Raises:
            CardError: If derivation fails
        """
        try:
            data = await self._request(
                "POST",
                f"/v1/readers/{reader_index}/mifare/derive-key",
                json={"aesKey": aes_key},
            )
            return DerivedKeyData(key=data["key"])
        except APIError as e:
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
        """
        Encrypt data with AES-128-ECB and write to a MIFARE Classic block.

        Args:
            reader_index: Index of the reader
            block: Block number to write
            data: Block data as hex string (will be encrypted)
            aes_key: AES-128 encryption key
            auth_key: MIFARE authentication key
            auth_key_type: Key type for authentication

        Raises:
            CardError: If write fails
        """
        body: dict[str, Any] = {
            "data": data,
            "aesKey": aes_key,
            "authKey": auth_key,
        }
        if auth_key_type:
            body["authKeyType"] = auth_key_type.value

        try:
            await self._request(
                "POST",
                f"/v1/readers/{reader_index}/mifare/aes-write/{block}",
                json=body,
            )
        except APIError as e:
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
        """
        Write a MIFARE Classic sector trailer with new keys.

        Args:
            reader_index: Index of the reader
            block: Sector trailer block number (3, 7, 11, 15, ...)
            key_a: New Key A as hex string
            key_b: New Key B as hex string
            auth_key: Authentication key
            access_bits: Access bits as hex string (optional)
            auth_key_type: Key type for authentication

        Raises:
            CardError: If write fails
        """
        body: dict[str, Any] = {
            "keyA": key_a,
            "keyB": key_b,
            "authKey": auth_key,
        }
        if access_bits:
            body["accessBits"] = access_bits
        if auth_key_type:
            body["authKeyType"] = auth_key_type.value

        try:
            await self._request(
                "POST",
                f"/v1/readers/{reader_index}/mifare/sector-trailer/{block}",
                json=body,
            )
        except APIError as e:
            raise CardError(str(e)) from e

    def poll_card(self, reader_index: int, *, interval: float = 1.0) -> CardPoller:
        """
        Create a card poller for automatic card detection.

        Args:
            reader_index: Index of the reader to poll
            interval: Polling interval in seconds (default: 1.0)

        Returns:
            CardPoller instance
        """
        from .poller import CardPoller

        return CardPoller(self, reader_index, interval=interval)

    # =========================================================================
    # Sync Methods
    # =========================================================================

    def is_connected_sync(self) -> bool:
        """Check if the nfc-agent server is running (sync version)."""
        try:
            self.get_readers_sync()
            return True
        except Exception:
            return False

    def get_readers_sync(self) -> list[Reader]:
        """Get a list of available NFC readers (sync version)."""
        data = self._request_sync("GET", "/v1/readers")
        return [Reader(**r) for r in data]

    def read_card_sync(self, reader_index: int) -> Card:
        """Read card data from a specific reader (sync version)."""
        try:
            data = self._request_sync("GET", f"/v1/readers/{reader_index}/card")
            return self._parse_card(data)
        except APIError as e:
            raise CardError(str(e)) from e

    def write_card_sync(
        self,
        reader_index: int,
        *,
        data: str | None = None,
        data_type: str,
        url: str | None = None,
    ) -> None:
        """Write data to a card (sync version)."""
        body: dict[str, Any] = {"dataType": data_type}

        if data_type == "url":
            body["data"] = data or url or ""
        else:
            if data:
                body["data"] = data
            if url:
                body["url"] = url

        try:
            self._request_sync("POST", f"/v1/readers/{reader_index}/card", json=body)
        except APIError as e:
            raise CardError(str(e)) from e

    def get_version_sync(self) -> VersionInfo:
        """Get agent version information (sync version)."""
        data = self._request_sync("GET", "/v1/version")
        return self._parse_version_info(data)

    # =========================================================================
    # Helper Methods
    # =========================================================================

    @staticmethod
    def _parse_card(data: dict[str, Any]) -> Card:
        """Parse card data from API response."""
        data_type = None
        if "dataType" in data:
            try:
                data_type = CardDataType(data["dataType"])
            except ValueError:
                data_type = CardDataType.UNKNOWN

        return Card(
            uid=data["uid"],
            atr=data.get("atr"),
            type=data.get("type"),
            protocol=data.get("protocol"),
            protocol_iso=data.get("protocolISO"),
            size=data.get("size"),
            writable=data.get("writable"),
            data=data.get("data"),
            data_type=data_type,
        )

    @staticmethod
    def _parse_version_info(data: dict[str, Any]) -> VersionInfo:
        """Parse version info from API response."""
        return VersionInfo(
            version=data["version"],
            build_time=data.get("buildTime", ""),
            git_commit=data.get("gitCommit", ""),
            update_available=data.get("updateAvailable"),
            latest_version=data.get("latestVersion"),
            release_url=data.get("releaseUrl"),
        )

    @staticmethod
    def _parse_mifare_batch_result(data: dict[str, Any]) -> MifareBatchWriteResult:
        """Parse MIFARE batch write result."""
        results = [
            MifareBlockWriteResult(
                block=r["block"],
                success=r["success"],
                error=r.get("error"),
            )
            for r in data.get("results", [])
        ]
        return MifareBatchWriteResult(
            results=results,
            written=data.get("written", 0),
            total=data.get("total", 0),
        )

    @staticmethod
    def _parse_ultralight_batch_result(
        data: dict[str, Any],
    ) -> UltralightBatchWriteResult:
        """Parse Ultralight batch write result."""
        results = [
            UltralightPageWriteResult(
                page=r["page"],
                success=r["success"],
                error=r.get("error"),
            )
            for r in data.get("results", [])
        ]
        return UltralightBatchWriteResult(
            results=results,
            written=data.get("written", 0),
            total=data.get("total", 0),
        )
