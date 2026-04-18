package serve

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
)

func TestSensorRegistry(t *testing.T) {
	// All sensors must have key, name, and value path
	for _, list := range [][]sensorDef{systemSensors, unitSensors} {
		for _, s := range list {
			assert.NotEmpty(t, s.Key, "sensor missing key")
			assert.NotEmpty(t, s.Name, "sensor %s missing name", s.Key)
			assert.NotEmpty(t, s.ValuePath, "sensor %s missing value path", s.Key)
		}
	}
	// System sensors should NOT have per-tracker PV entries
	for _, s := range systemSensors {
		assert.NotContains(t, s.Key, "pv1_", "system sensors should not have per-tracker PV")
		assert.NotContains(t, s.Key, "pv2_", "system sensors should not have per-tracker PV")
	}
	// Unit sensors should have per-tracker PV entries
	var hasPV1 bool
	for _, s := range unitSensors {
		if s.Key == "pv1_power" {
			hasPV1 = true
		}
	}
	assert.True(t, hasPV1, "unit sensors should have per-tracker PV")
}

func TestDiscoveryPayloadStructure(t *testing.T) {
	prefix := "homeassistant"
	deviceID := "srne_system"
	// Find the battery_soc sensor
	var s sensorDef
	for _, sensor := range systemSensors {
		if sensor.Key == "battery_soc" {
			s = sensor
			break
		}
	}
	require.NotEmpty(t, s.Key, "battery_soc sensor not found")

	payload := map[string]any{
		"name":               s.Name,
		"unique_id":          fmt.Sprintf("%s_%s", deviceID, s.Key),
		"state_topic":        fmt.Sprintf("%s/sensor/%s/state", prefix, deviceID),
		"value_template":     fmt.Sprintf("{{ value_json.%s }}", s.ValuePath),
		"availability_topic": fmt.Sprintf("%s/sensor/%s/availability", prefix, deviceID),
		"device": map[string]any{
			"identifiers":  []string{deviceID},
			"name":         "SRNE Solar System",
			"manufacturer": "SRNE",
		},
	}
	if s.Unit != "" {
		payload["unit_of_measurement"] = s.Unit
	}
	if s.DeviceClass != "" {
		payload["device_class"] = s.DeviceClass
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Verify it's valid JSON with expected fields
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "Battery SOC", decoded["name"])
	assert.Equal(t, "srne_system_battery_soc", decoded["unique_id"])
	assert.Equal(t, "%", decoded["unit_of_measurement"])
	assert.Equal(t, "battery", decoded["device_class"])
	assert.Contains(t, decoded["value_template"], "battery.soc")
	assert.Contains(t, decoded["state_topic"].(string), "srne_system/state")

	dev := decoded["device"].(map[string]any)
	assert.Equal(t, "SRNE Solar System", dev["name"])
}

func TestSanitizeSerial(t *testing.T) {
	assert.Equal(t, "ABC123", sanitizeSerial("ABC123"))
	assert.Equal(t, "A_B_C", sanitizeSerial("A/B+C"))
	assert.Equal(t, "unknown", sanitizeSerial(""))
}

func TestMPPTRename(t *testing.T) {
	p := &MQTTPublisher{
		mpptLabels: map[string][2]string{
			"10.0.0.1": {"Roof East", "Roof West"},
			"10.0.0.2": {"", "Garage"}, // only MPPT 2 configured
		},
	}

	tests := []struct {
		name     string
		host     string
		key      string
		input    string
		expected string
	}{
		{"pv1 rename", "10.0.0.1", "pv1_voltage", "PV1 Voltage", "Roof East Voltage"},
		{"pv2 rename", "10.0.0.1", "pv2_power", "PV2 Power", "Roof West Power"},
		{"partial: mppt1 blank falls through", "10.0.0.2", "pv1_voltage", "PV1 Voltage", "PV1 Voltage"},
		{"partial: mppt2 set renames", "10.0.0.2", "pv2_current", "PV2 Current", "Garage Current"},
		{"unknown host passes through", "10.9.9.9", "pv1_power", "PV1 Power", "PV1 Power"},
		{"non-pv sensor untouched", "10.0.0.1", "battery_soc", "Battery SOC", "Battery SOC"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, p.mpptRename(tc.host, tc.key, tc.input))
		})
	}

	// Nil labels map: pass-through.
	empty := &MQTTPublisher{}
	assert.Equal(t, "PV1 Voltage", empty.mpptRename("10.0.0.1", "pv1_voltage", "PV1 Voltage"))
}

func TestStatePayloadMatchesSnapshot(t *testing.T) {
	snap := &inverter.Snapshot{
		Battery: inverter.BatteryData{SOC: 85, Voltage: 53.2},
		PV:      inverter.PVData{TotalPower: 1040},
	}

	data, err := json.Marshal(snap)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	batt := decoded["battery"].(map[string]any)
	assert.Equal(t, float64(85), batt["soc"])
	assert.Equal(t, 53.2, batt["voltage"])

	pv := decoded["pv"].(map[string]any)
	assert.Equal(t, float64(1040), pv["total_power"])
}
