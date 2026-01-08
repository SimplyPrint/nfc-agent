"""Tests for CardPoller."""

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from nfc_agent import CardPoller, NFCClient
from nfc_agent.exceptions import CardError
from nfc_agent.types import Card


class TestCardPoller:
    """Tests for CardPoller class."""

    def test_init(self):
        """Test poller initialization."""
        client = MagicMock(spec=NFCClient)
        poller = CardPoller(client, 0, interval=0.5)

        assert poller._client is client
        assert poller._reader_index == 0
        assert poller._interval == 0.5
        assert poller.is_running is False

    def test_on_card_decorator(self):
        """Test on_card decorator registration."""
        client = MagicMock(spec=NFCClient)
        poller = CardPoller(client, 0)
        cards_detected = []

        @poller.on_card
        def handle_card(card):
            cards_detected.append(card)

        assert len(poller._on_card) == 1
        assert handle_card in poller._on_card

    def test_on_removed_decorator(self):
        """Test on_removed decorator registration."""
        client = MagicMock(spec=NFCClient)
        poller = CardPoller(client, 0)
        removals = []

        @poller.on_removed
        def handle_removed():
            removals.append(True)

        assert len(poller._on_removed) == 1

    def test_on_error_decorator(self):
        """Test on_error decorator registration."""
        client = MagicMock(spec=NFCClient)
        poller = CardPoller(client, 0)
        errors = []

        @poller.on_error
        def handle_error(e):
            errors.append(e)

        assert len(poller._on_error) == 1

    @pytest.mark.asyncio
    async def test_poll_detects_new_card(self):
        """Test that poll detects a new card."""
        client = MagicMock(spec=NFCClient)
        card = Card(uid="04AABBCCDD")
        client.read_card = AsyncMock(return_value=card)

        poller = CardPoller(client, 0)
        cards_detected = []

        @poller.on_card
        def handle_card(c):
            cards_detected.append(c)

        await poller._poll()

        assert len(cards_detected) == 1
        assert cards_detected[0].uid == "04AABBCCDD"
        assert poller._last_card_uid == "04AABBCCDD"

    @pytest.mark.asyncio
    async def test_poll_ignores_same_card(self):
        """Test that poll ignores the same card."""
        client = MagicMock(spec=NFCClient)
        card = Card(uid="04AABBCCDD")
        client.read_card = AsyncMock(return_value=card)

        poller = CardPoller(client, 0)
        poller._last_card_uid = "04AABBCCDD"  # Already seen this card
        cards_detected = []

        @poller.on_card
        def handle_card(c):
            cards_detected.append(c)

        await poller._poll()

        assert len(cards_detected) == 0  # Should not trigger

    @pytest.mark.asyncio
    async def test_poll_detects_card_removal(self):
        """Test that poll detects card removal."""
        client = MagicMock(spec=NFCClient)
        client.read_card = AsyncMock(side_effect=CardError("no card present"))

        poller = CardPoller(client, 0)
        poller._last_card_uid = "04AABBCCDD"  # Card was present
        removals = []

        @poller.on_removed
        def handle_removed():
            removals.append(True)

        await poller._poll()

        assert len(removals) == 1
        assert poller._last_card_uid is None

    @pytest.mark.asyncio
    async def test_poll_no_removal_if_no_card_was_present(self):
        """Test that poll doesn't emit removal if no card was present."""
        client = MagicMock(spec=NFCClient)
        client.read_card = AsyncMock(side_effect=CardError("no card present"))

        poller = CardPoller(client, 0)
        poller._last_card_uid = None  # No card was present
        removals = []

        @poller.on_removed
        def handle_removed():
            removals.append(True)

        await poller._poll()

        assert len(removals) == 0

    @pytest.mark.asyncio
    async def test_poll_emits_error_for_connection_issues(self):
        """Test that poll emits errors for connection issues."""
        client = MagicMock(spec=NFCClient)
        client.read_card = AsyncMock(side_effect=CardError("connection refused"))

        poller = CardPoller(client, 0)
        errors = []

        @poller.on_error
        def handle_error(e):
            errors.append(e)

        await poller._poll()

        assert len(errors) == 1
        assert "connection refused" in str(errors[0])

    @pytest.mark.asyncio
    async def test_start_creates_task(self):
        """Test that start creates an async task."""
        client = MagicMock(spec=NFCClient)
        client.read_card = AsyncMock(return_value=Card(uid="test"))

        poller = CardPoller(client, 0, interval=0.1)

        await poller.start()
        assert poller.is_running is True

        # Let it run briefly
        await asyncio.sleep(0.05)

        poller.stop()
        assert poller.is_running is False

    @pytest.mark.asyncio
    async def test_stop_cancels_task(self):
        """Test that stop cancels the polling task."""
        client = MagicMock(spec=NFCClient)
        client.read_card = AsyncMock(return_value=Card(uid="test"))

        poller = CardPoller(client, 0, interval=0.1)
        await poller.start()

        poller.stop()

        assert poller._task is None
        assert poller._last_card_uid is None

    @pytest.mark.asyncio
    async def test_start_idempotent(self):
        """Test that calling start twice doesn't create duplicate tasks."""
        client = MagicMock(spec=NFCClient)
        client.read_card = AsyncMock(return_value=Card(uid="test"))

        poller = CardPoller(client, 0, interval=0.1)

        await poller.start()
        task1 = poller._task

        await poller.start()
        task2 = poller._task

        assert task1 is task2

        poller.stop()
