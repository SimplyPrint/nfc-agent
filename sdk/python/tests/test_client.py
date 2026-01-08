"""Tests for NFCClient."""

import pytest
import respx
from httpx import Response

from nfc_agent import (
    CardError,
    NFCClient,
)


class TestNFCClient:
    """Tests for NFCClient class."""

    @respx.mock
    @pytest.mark.asyncio
    async def test_get_readers(self, mock_readers):
        """Test getting list of readers."""
        respx.get("http://127.0.0.1:32145/v1/readers").mock(
            return_value=Response(200, json=mock_readers)
        )

        async with NFCClient() as client:
            readers = await client.get_readers()

        assert len(readers) == 2
        assert readers[0].id == "0"
        assert readers[0].name == "ACR122U"
        assert readers[0].type == "picc"

    @respx.mock
    @pytest.mark.asyncio
    async def test_read_card(self, mock_card):
        """Test reading a card."""
        respx.get("http://127.0.0.1:32145/v1/readers/0/card").mock(
            return_value=Response(200, json=mock_card)
        )

        async with NFCClient() as client:
            card = await client.read_card(0)

        assert card.uid == "04AABBCCDD"
        assert card.type == "NTAG215"
        assert card.data == "Hello World"

    @respx.mock
    @pytest.mark.asyncio
    async def test_read_card_no_card(self):
        """Test reading when no card is present."""
        respx.get("http://127.0.0.1:32145/v1/readers/0/card").mock(
            return_value=Response(404, json={"error": "no card present"})
        )

        async with NFCClient() as client:
            with pytest.raises(CardError) as exc_info:
                await client.read_card(0)

        assert "no card present" in str(exc_info.value)

    @respx.mock
    @pytest.mark.asyncio
    async def test_write_card(self):
        """Test writing to a card."""
        respx.post("http://127.0.0.1:32145/v1/readers/0/card").mock(
            return_value=Response(200, json={"success": "data written"})
        )

        async with NFCClient() as client:
            await client.write_card(0, data="Test", data_type="text")

    @respx.mock
    @pytest.mark.asyncio
    async def test_write_card_url(self):
        """Test writing URL to a card."""
        route = respx.post("http://127.0.0.1:32145/v1/readers/0/card").mock(
            return_value=Response(200, json={"success": "data written"})
        )

        async with NFCClient() as client:
            await client.write_card(0, data="https://example.com", data_type="url")

        request = route.calls.last.request
        import json

        body = json.loads(request.content)
        assert body["dataType"] == "url"
        assert body["data"] == "https://example.com"

    @respx.mock
    @pytest.mark.asyncio
    async def test_get_version(self, mock_version):
        """Test getting version info."""
        respx.get("http://127.0.0.1:32145/v1/version").mock(
            return_value=Response(200, json=mock_version)
        )

        async with NFCClient() as client:
            version = await client.get_version()

        assert version.version == "1.0.0"
        assert version.git_commit == "abc123"
        assert version.update_available is False

    @respx.mock
    @pytest.mark.asyncio
    async def test_is_connected_true(self, mock_readers):
        """Test is_connected when server is available."""
        respx.get("http://127.0.0.1:32145/v1/readers").mock(
            return_value=Response(200, json=mock_readers)
        )

        async with NFCClient() as client:
            assert await client.is_connected() is True

    @respx.mock
    @pytest.mark.asyncio
    async def test_is_connected_false(self):
        """Test is_connected when server is unavailable."""
        respx.get("http://127.0.0.1:32145/v1/readers").mock(
            return_value=Response(500, json={"error": "server error"})
        )

        async with NFCClient() as client:
            assert await client.is_connected() is False

    @respx.mock
    @pytest.mark.asyncio
    async def test_read_mifare_block(self, mock_mifare_block):
        """Test reading a MIFARE block."""
        respx.get("http://127.0.0.1:32145/v1/readers/0/mifare/4").mock(
            return_value=Response(200, json=mock_mifare_block)
        )

        async with NFCClient() as client:
            block = await client.read_mifare_block(0, 4)

        assert block.block == 4
        assert block.data == "00112233445566778899AABBCCDDEEFF"

    @respx.mock
    @pytest.mark.asyncio
    async def test_write_mifare_block(self):
        """Test writing a MIFARE block."""
        respx.post("http://127.0.0.1:32145/v1/readers/0/mifare/4").mock(
            return_value=Response(200, json={"success": "block written"})
        )

        async with NFCClient() as client:
            await client.write_mifare_block(
                0, 4, data="00112233445566778899AABBCCDDEEFF"
            )

    @respx.mock
    @pytest.mark.asyncio
    async def test_read_ultralight_page(self, mock_ultralight_page):
        """Test reading an Ultralight page."""
        respx.get("http://127.0.0.1:32145/v1/readers/0/ultralight/4").mock(
            return_value=Response(200, json=mock_ultralight_page)
        )

        async with NFCClient() as client:
            page = await client.read_ultralight_page(0, 4)

        assert page.page == 4
        assert page.data == "00112233"

    @respx.mock
    @pytest.mark.asyncio
    async def test_derive_uid_key_aes(self):
        """Test deriving UID key."""
        respx.post("http://127.0.0.1:32145/v1/readers/0/mifare/derive-key").mock(
            return_value=Response(200, json={"key": "AABBCCDDEEFF"})
        )

        async with NFCClient() as client:
            result = await client.derive_uid_key_aes(
                0, "00112233445566778899AABBCCDDEEFF"
            )

        assert result.key == "AABBCCDDEEFF"


class TestNFCClientSync:
    """Tests for NFCClient sync methods."""

    @respx.mock
    def test_get_readers_sync(self, mock_readers):
        """Test getting list of readers (sync)."""
        respx.get("http://127.0.0.1:32145/v1/readers").mock(
            return_value=Response(200, json=mock_readers)
        )

        with NFCClient() as client:
            readers = client.get_readers_sync()

        assert len(readers) == 2
        assert readers[0].name == "ACR122U"

    @respx.mock
    def test_read_card_sync(self, mock_card):
        """Test reading a card (sync)."""
        respx.get("http://127.0.0.1:32145/v1/readers/0/card").mock(
            return_value=Response(200, json=mock_card)
        )

        with NFCClient() as client:
            card = client.read_card_sync(0)

        assert card.uid == "04AABBCCDD"

    @respx.mock
    def test_write_card_sync(self):
        """Test writing to a card (sync)."""
        respx.post("http://127.0.0.1:32145/v1/readers/0/card").mock(
            return_value=Response(200, json={"success": "data written"})
        )

        with NFCClient() as client:
            client.write_card_sync(0, data="Test", data_type="text")

    @respx.mock
    def test_get_version_sync(self, mock_version):
        """Test getting version info (sync)."""
        respx.get("http://127.0.0.1:32145/v1/version").mock(
            return_value=Response(200, json=mock_version)
        )

        with NFCClient() as client:
            version = client.get_version_sync()

        assert version.version == "1.0.0"
