"""Sensor platform for SRNE Solar Controller."""

from __future__ import annotations

from homeassistant.components.sensor import SensorEntity
from homeassistant.core import HomeAssistant
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from . import SrneConfigEntry, SrneCoordinator
from .const import DOMAIN, SENSORS, SrneSensorEntityDescription


async def async_setup_entry(
    hass: HomeAssistant,
    entry: SrneConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up SRNE sensors from a config entry."""
    coordinator = entry.runtime_data
    async_add_entities(
        SrneSensor(coordinator, description, entry)
        for description in SENSORS
    )


class SrneSensor(CoordinatorEntity[SrneCoordinator], SensorEntity):
    """Representation of an SRNE sensor."""

    entity_description: SrneSensorEntityDescription
    _attr_has_entity_name = True

    def __init__(
        self,
        coordinator: SrneCoordinator,
        description: SrneSensorEntityDescription,
        entry: SrneConfigEntry,
    ) -> None:
        """Initialize the sensor."""
        super().__init__(coordinator)
        self.entity_description = description
        self._attr_unique_id = f"{entry.entry_id}_{description.key}"
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, entry.entry_id)},
            name="SRNE Solar System",
            manufacturer="SRNE",
            configuration_url=coordinator.base_url,
        )

    @property
    def native_value(self) -> float | str | None:
        """Return the sensor value from the snapshot."""
        if self.coordinator.data is None:
            return None
        try:
            return self.entity_description.value_fn(self.coordinator.data)
        except (KeyError, TypeError):
            return None
