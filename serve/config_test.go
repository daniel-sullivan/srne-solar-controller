package serve

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadConfig_Minimal(t *testing.T) {
	path := writeConfig(t, `
[[inverter]]
host = "10.0.0.1"
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Len(t, cfg.Inverters, 1)
	assert.Equal(t, "10.0.0.1", cfg.Inverters[0].Host)
	assert.Equal(t, 8899, cfg.Inverters[0].Port)
	assert.Equal(t, 8080, cfg.Server.WebPort)
	assert.Nil(t, cfg.MQTT)

	d, err := cfg.PollIntervalDuration()
	require.NoError(t, err)
	assert.Equal(t, defaultPollInterval, d)
}

func TestLoadConfig_Full(t *testing.T) {
	path := writeConfig(t, `
[server]
poll_interval = "10s"
web_port = 9090
settings_refresh = "10m"

[[inverter]]
host = "10.0.0.1"
port = 8899

[[inverter]]
host = "10.0.0.2"
port = 8899
slave_id = 2

[mqtt]
broker = "tcp://mqtt:1883"
client_id = "test"
topic_prefix = "ha"
`)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Len(t, cfg.Inverters, 2)
	assert.Equal(t, 9090, cfg.Server.WebPort)
	assert.Equal(t, uint8(2), cfg.Inverters[1].SlaveID)
	require.NotNil(t, cfg.MQTT)
	assert.Equal(t, "tcp://mqtt:1883", cfg.MQTT.Broker)
	assert.Equal(t, "ha", cfg.MQTT.TopicPrefix)
}

func TestLoadConfig_NoInverters(t *testing.T) {
	path := writeConfig(t, `
[server]
web_port = 8080
`)
	_, err := LoadConfig(path)
	assert.ErrorContains(t, err, "at least one [[inverter]]")
}

func TestLoadConfig_MissingHost(t *testing.T) {
	path := writeConfig(t, `
[[inverter]]
port = 8899
`)
	_, err := LoadConfig(path)
	assert.ErrorContains(t, err, "missing host")
}

func TestLoadConfig_MQTTMissingBroker(t *testing.T) {
	path := writeConfig(t, `
[[inverter]]
host = "10.0.0.1"

[mqtt]
client_id = "test"
`)
	_, err := LoadConfig(path)
	assert.ErrorContains(t, err, "mqtt.broker is required")
}

func TestLoadConfig_InvalidDuration(t *testing.T) {
	path := writeConfig(t, `
[server]
poll_interval = "not-a-duration"

[[inverter]]
host = "10.0.0.1"
`)
	_, err := LoadConfig(path)
	assert.ErrorContains(t, err, "poll_interval")
}
