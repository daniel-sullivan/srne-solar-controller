"""The SRNE Solar Controller integration."""

from __future__ import annotations

from datetime import timedelta
import logging

import aiohttp

from homeassistant.config_entries import ConfigEntry
from homeassistant.const import Platform
from homeassistant.core import HomeAssistant
from homeassistant.helpers.aiohttp_client import async_get_clientsession
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .const import CONF_BASE_URL, DEFAULT_SCAN_INTERVAL, DOMAIN

_LOGGER = logging.getLogger(__name__)

PLATFORMS = [Platform.SENSOR]

type SrneConfigEntry = ConfigEntry[SrneCoordinator]


class SrneCoordinator(DataUpdateCoordinator[dict]):
    """Coordinator that polls the SRNE Solar Controller REST API."""

    def __init__(self, hass: HomeAssistant, entry: SrneConfigEntry) -> None:
        """Initialize the coordinator."""
        super().__init__(
            hass,
            _LOGGER,
            name=DOMAIN,
            update_interval=timedelta(seconds=DEFAULT_SCAN_INTERVAL),
        )
        self.base_url = entry.data[CONF_BASE_URL].rstrip("/")
        self.session = async_get_clientsession(hass)

    async def _async_update_data(self) -> dict:
        """Fetch snapshot from the SRNE controller."""
        url = f"{self.base_url}/api/snapshot"
        try:
            async with self.session.get(url, timeout=aiohttp.ClientTimeout(total=10)) as resp:
                resp.raise_for_status()
                return await resp.json()
        except (aiohttp.ClientError, TimeoutError) as err:
            raise UpdateFailed(f"Error fetching snapshot: {err}") from err


async def async_setup_entry(hass: HomeAssistant, entry: SrneConfigEntry) -> bool:
    """Set up SRNE Solar Controller from a config entry."""
    coordinator = SrneCoordinator(hass, entry)
    await coordinator.async_config_entry_first_refresh()
    entry.runtime_data = coordinator
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: SrneConfigEntry) -> bool:
    """Unload a config entry."""
    return await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
