"""Pytest configuration and fixtures."""

import pytest


@pytest.fixture
def mock_readers():
    """Sample reader data."""
    return [
        {"id": "0", "name": "ACR122U", "type": "picc"},
        {"id": "1", "name": "ACS ACR1252", "type": "picc"},
    ]


@pytest.fixture
def mock_card():
    """Sample card data."""
    return {
        "uid": "04AABBCCDD",
        "atr": "3B8F8001804F0CA000000306030001000000006A",
        "type": "NTAG215",
        "protocol": "NFC-A",
        "protocolISO": "ISO 14443-3A",
        "size": 504,
        "writable": True,
        "data": "Hello World",
        "dataType": "text",
    }


@pytest.fixture
def mock_version():
    """Sample version data."""
    return {
        "version": "1.0.0",
        "buildTime": "2024-01-01T00:00:00Z",
        "gitCommit": "abc123",
        "updateAvailable": False,
    }


@pytest.fixture
def mock_mifare_block():
    """Sample MIFARE block data."""
    return {
        "block": 4,
        "data": "00112233445566778899AABBCCDDEEFF",
    }


@pytest.fixture
def mock_ultralight_page():
    """Sample Ultralight page data."""
    return {
        "page": 4,
        "data": "00112233",
    }
