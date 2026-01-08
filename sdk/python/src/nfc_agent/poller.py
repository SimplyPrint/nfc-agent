"""Card polling utility for automatic card detection."""

from __future__ import annotations

import asyncio
import contextlib
from typing import TYPE_CHECKING, Callable

from .exceptions import CardError
from .types import Card

if TYPE_CHECKING:
    from .client import NFCClient

# Callback type aliases
CardCallback = Callable[[Card], None]
RemovedCallback = Callable[[], None]
ErrorCallback = Callable[[Exception], None]

DEFAULT_INTERVAL = 1.0


class CardPoller:
    """
    Polls a reader for card presence and emits events on card detection/removal.

    Example:
        poller = client.poll_card(0, interval=0.5)

        @poller.on_card
        def handle_card(card):
            print(f"Card detected: {card.uid}")

        @poller.on_removed
        def handle_removed():
            print("Card removed")

        await poller.start()
        # ... later ...
        poller.stop()
    """

    def __init__(
        self,
        client: NFCClient,
        reader_index: int,
        *,
        interval: float = DEFAULT_INTERVAL,
    ):
        """
        Create a new card poller.

        Args:
            client: NFCClient instance to use for polling
            reader_index: Index of the reader to poll
            interval: Polling interval in seconds (default: 1.0)
        """
        self._client = client
        self._reader_index = reader_index
        self._interval = interval
        self._task: asyncio.Task[None] | None = None
        self._last_card_uid: str | None = None

        self._on_card: list[CardCallback] = []
        self._on_removed: list[RemovedCallback] = []
        self._on_error: list[ErrorCallback] = []

    @property
    def is_running(self) -> bool:
        """Check if the poller is currently running."""
        return self._task is not None and not self._task.done()

    def on_card(self, callback: CardCallback) -> CardCallback:
        """
        Register a card detected callback.

        Can be used as a decorator:
            @poller.on_card
            def handle_card(card):
                print(card.uid)
        """
        self._on_card.append(callback)
        return callback

    def on_removed(self, callback: RemovedCallback) -> RemovedCallback:
        """
        Register a card removed callback.

        Can be used as a decorator:
            @poller.on_removed
            def handle_removed():
                print("Card removed")
        """
        self._on_removed.append(callback)
        return callback

    def on_error(self, callback: ErrorCallback) -> ErrorCallback:
        """
        Register an error callback.

        Can be used as a decorator:
            @poller.on_error
            def handle_error(e):
                print(f"Error: {e}")
        """
        self._on_error.append(callback)
        return callback

    async def start(self) -> None:
        """Start polling for cards."""
        if self.is_running:
            return

        self._task = asyncio.create_task(self._poll_loop())

    def stop(self) -> None:
        """Stop polling."""
        if self._task:
            self._task.cancel()
            self._task = None
        self._last_card_uid = None

    async def _poll_loop(self) -> None:
        """Internal polling loop."""
        while True:
            await self._poll()
            await asyncio.sleep(self._interval)

    async def _poll(self) -> None:
        """Perform a single poll."""
        try:
            card = await self._client.read_card(self._reader_index)

            # New card detected or card changed
            if self._last_card_uid != card.uid:
                self._last_card_uid = card.uid
                for card_cb in self._on_card:
                    with contextlib.suppress(Exception):
                        card_cb(card)

        except CardError as e:
            # Card was removed or read failed
            if self._last_card_uid is not None:
                self._last_card_uid = None
                for removed_cb in self._on_removed:
                    with contextlib.suppress(Exception):
                        removed_cb()

            # Only emit error for non-card-related issues
            error_msg = str(e).lower()
            if "no card" not in error_msg:
                for error_cb in self._on_error:
                    with contextlib.suppress(Exception):
                        error_cb(e)

        except Exception as e:
            for error_cb in self._on_error:
                with contextlib.suppress(Exception):
                    error_cb(e)
