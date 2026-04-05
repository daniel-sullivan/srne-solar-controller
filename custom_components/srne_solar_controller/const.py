"""Constants for the SRNE Solar Controller integration."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable

from homeassistant.components.sensor import (
    SensorDeviceClass,
    SensorEntityDescription,
    SensorStateClass,
)
from homeassistant.const import (
    PERCENTAGE,
    UnitOfElectricCurrent,
    UnitOfElectricPotential,
    UnitOfEnergy,
    UnitOfFrequency,
    UnitOfPower,
    UnitOfTemperature,
)

DOMAIN = "srne_solar_controller"
DEFAULT_SCAN_INTERVAL = 10
CONF_BASE_URL = "base_url"


@dataclass(frozen=True, kw_only=True)
class SrneSensorEntityDescription(SensorEntityDescription):
    """Describe an SRNE sensor."""

    value_fn: Callable[[dict[str, Any]], Any]


SENSORS: tuple[SrneSensorEntityDescription, ...] = (
    # Battery
    SrneSensorEntityDescription(
        key="battery_soc",
        translation_key="battery_soc",
        native_unit_of_measurement=PERCENTAGE,
        device_class=SensorDeviceClass.BATTERY,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["battery"]["soc"],
    ),
    SrneSensorEntityDescription(
        key="battery_voltage",
        translation_key="battery_voltage",
        native_unit_of_measurement=UnitOfElectricPotential.VOLT,
        device_class=SensorDeviceClass.VOLTAGE,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["battery"]["voltage"],
    ),
    SrneSensorEntityDescription(
        key="battery_current",
        translation_key="battery_current",
        native_unit_of_measurement=UnitOfElectricCurrent.AMPERE,
        device_class=SensorDeviceClass.CURRENT,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["battery"]["current"],
    ),
    SrneSensorEntityDescription(
        key="battery_power",
        translation_key="battery_power",
        native_unit_of_measurement=UnitOfPower.WATT,
        device_class=SensorDeviceClass.POWER,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["battery"]["total_charge_power"],
    ),
    SrneSensorEntityDescription(
        key="battery_temp",
        translation_key="battery_temp",
        native_unit_of_measurement=UnitOfTemperature.CELSIUS,
        device_class=SensorDeviceClass.TEMPERATURE,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["battery"]["battery_temp"],
    ),
    SrneSensorEntityDescription(
        key="controller_temp",
        translation_key="controller_temp",
        native_unit_of_measurement=UnitOfTemperature.CELSIUS,
        device_class=SensorDeviceClass.TEMPERATURE,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["battery"]["controller_temp"],
    ),
    # PV
    SrneSensorEntityDescription(
        key="pv_total_power",
        translation_key="pv_total_power",
        native_unit_of_measurement=UnitOfPower.WATT,
        device_class=SensorDeviceClass.POWER,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["pv"]["total_power"],
    ),
    SrneSensorEntityDescription(
        key="pv1_power",
        translation_key="pv1_power",
        native_unit_of_measurement=UnitOfPower.WATT,
        device_class=SensorDeviceClass.POWER,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["pv"]["pv1_power"],
    ),
    SrneSensorEntityDescription(
        key="pv2_power",
        translation_key="pv2_power",
        native_unit_of_measurement=UnitOfPower.WATT,
        device_class=SensorDeviceClass.POWER,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["pv"]["pv2_power"],
    ),
    # Load
    SrneSensorEntityDescription(
        key="load_power",
        translation_key="load_power",
        native_unit_of_measurement=UnitOfPower.WATT,
        device_class=SensorDeviceClass.POWER,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["load"]["total_power"],
    ),
    # Grid
    SrneSensorEntityDescription(
        key="grid_frequency",
        translation_key="grid_frequency",
        native_unit_of_measurement=UnitOfFrequency.HERTZ,
        device_class=SensorDeviceClass.FREQUENCY,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["grid"]["frequency"],
    ),
    # Inverter
    SrneSensorEntityDescription(
        key="inverter_state",
        translation_key="inverter_state",
        value_fn=lambda d: d["inverter"]["machine_state_name"],
    ),
    SrneSensorEntityDescription(
        key="bus_voltage",
        translation_key="bus_voltage",
        native_unit_of_measurement=UnitOfElectricPotential.VOLT,
        device_class=SensorDeviceClass.VOLTAGE,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["inverter"]["bus_voltage"],
    ),
    SrneSensorEntityDescription(
        key="inverter_frequency",
        translation_key="inverter_frequency",
        native_unit_of_measurement=UnitOfFrequency.HERTZ,
        device_class=SensorDeviceClass.FREQUENCY,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["inverter"]["frequency"],
    ),
    SrneSensorEntityDescription(
        key="heatsink_temp_a",
        translation_key="heatsink_temp_a",
        native_unit_of_measurement=UnitOfTemperature.CELSIUS,
        device_class=SensorDeviceClass.TEMPERATURE,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["inverter"]["heatsink_temp_a"],
    ),
    SrneSensorEntityDescription(
        key="heatsink_temp_b",
        translation_key="heatsink_temp_b",
        native_unit_of_measurement=UnitOfTemperature.CELSIUS,
        device_class=SensorDeviceClass.TEMPERATURE,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda d: d["inverter"]["heatsink_temp_b"],
    ),
    # Stats
    SrneSensorEntityDescription(
        key="pv_generation_today",
        translation_key="pv_generation_today",
        native_unit_of_measurement=UnitOfEnergy.KILO_WATT_HOUR,
        device_class=SensorDeviceClass.ENERGY,
        state_class=SensorStateClass.TOTAL_INCREASING,
        value_fn=lambda d: d["stats"]["pv_generation_today"],
    ),
    SrneSensorEntityDescription(
        key="load_consumption_today",
        translation_key="load_consumption_today",
        native_unit_of_measurement=UnitOfEnergy.KILO_WATT_HOUR,
        device_class=SensorDeviceClass.ENERGY,
        state_class=SensorStateClass.TOTAL_INCREASING,
        value_fn=lambda d: d["stats"]["load_consumption_today"],
    ),
    SrneSensorEntityDescription(
        key="battery_charge_today",
        translation_key="battery_charge_today",
        native_unit_of_measurement="Ah",
        state_class=SensorStateClass.TOTAL_INCREASING,
        value_fn=lambda d: d["stats"]["battery_charge_today"],
    ),
    SrneSensorEntityDescription(
        key="battery_discharge_today",
        translation_key="battery_discharge_today",
        native_unit_of_measurement="Ah",
        state_class=SensorStateClass.TOTAL_INCREASING,
        value_fn=lambda d: d["stats"]["battery_discharge_today"],
    ),
)
