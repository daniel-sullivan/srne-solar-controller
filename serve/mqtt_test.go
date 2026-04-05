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
	for _, s := range systemSensors {
		assert.NotEmpty(t, s.Key, "sensor missing key")
		assert.NotEmpty(t, s.Name, "sensor %s missing name", s.Key)
		assert.NotEmpty(t, s.ValuePath, "sensor %s missing value path", s.Key)
	}
}

func TestDiscoveryPayloadStructure(t *testing.T) {
	prefix := "homeassistant"
	deviceID := "srne_system"
	s := systemSensors[0] // battery_soc

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
