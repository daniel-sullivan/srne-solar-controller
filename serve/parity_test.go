package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

// messageStore collects MQTT messages published to the embedded broker.
type messageStore struct {
	mu       sync.Mutex
	messages map[string][]byte // topic → last payload
}

func newMessageStore() *messageStore {
	return &messageStore{messages: make(map[string][]byte)}
}

func (ms *messageStore) handler(_ *mqtt.Client, _ packets.Subscription, pk packets.Packet) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.messages[pk.TopicName] = pk.Payload
}

func (ms *messageStore) get(topic string) ([]byte, bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	v, ok := ms.messages[topic]
	return v, ok
}

func (ms *messageStore) topics() []string {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	topics := make([]string, 0, len(ms.messages))
	for t := range ms.messages {
		topics = append(topics, t)
	}
	return topics
}

// startTestBroker starts an embedded mochi-mqtt broker on a random port.
// Returns the broker, its TCP address, and a message store that captures all published messages.
func startTestBroker(t *testing.T) (*mqtt.Server, string, *messageStore) {
	t.Helper()

	broker := mqtt.New(&mqtt.Options{InlineClient: true})
	require.NoError(t, broker.AddHook(new(auth.AllowHook), nil))
	tcp := listeners.NewTCP(listeners.Config{ID: "test", Address: "127.0.0.1:0"})
	require.NoError(t, broker.AddListener(tcp))
	require.NoError(t, broker.Serve())

	store := newMessageStore()
	require.NoError(t, broker.Subscribe("#", 1, store.handler))

	t.Cleanup(func() { _ = broker.Close() })

	addr := tcp.Address()
	return broker, addr, store
}

// TestMQTTRESTSensorParity verifies that MQTT discovery and REST /api/entities
// expose the same sensor metadata and values from the same mock system state.
func TestMQTTRESTSensorParity(t *testing.T) {
	_, addr, store := startTestBroker(t)

	// Set up mock system with known values
	sys := newTestSystem(t) // SOC=85, PV=600+440, Load=500
	hub := NewHub(sys, 50*time.Millisecond, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	time.Sleep(150 * time.Millisecond) // wait for poll + settings refresh

	// Start MQTT publisher
	mqttCfg := &MQTTConfig{
		Broker:      fmt.Sprintf("tcp://%s", addr),
		ClientID:    "test-publisher",
		TopicPrefix: "homeassistant",
	}
	unitInfos := sys.Units()
	pub, err := NewMQTTPublisher(mqttCfg, hub, unitInfos)
	require.NoError(t, err)
	go pub.Run(ctx)

	// Wait for MQTT messages to propagate
	time.Sleep(300 * time.Millisecond)

	// --- REST: GET /api/entities ---
	ws := NewWebServer(hub, sys)
	req := httptest.NewRequest(http.MethodGet, "/api/entities", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var entities struct {
		Sensors  []json.RawMessage `json:"sensors"`
		Switches []json.RawMessage `json:"switches"`
		Numbers  []json.RawMessage `json:"numbers"`
		Selects  []json.RawMessage `json:"selects"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entities))

	// --- Verify sensor discovery parity ---
	t.Run("sensor_discovery", func(t *testing.T) {
		for _, s := range systemSensors {
			configTopic := fmt.Sprintf("homeassistant/sensor/srne_system/%s/config", s.Key)
			payload, ok := store.get(configTopic)
			if !assert.True(t, ok, "missing MQTT discovery for sensor %s", s.Key) {
				continue
			}

			var config map[string]any
			require.NoError(t, json.Unmarshal(payload, &config))

			assert.Equal(t, s.Name, config["name"], "sensor %s name mismatch", s.Key)
			assert.Contains(t, config["value_template"], s.ValuePath, "sensor %s value_template missing path", s.Key)

			if s.Unit != "" {
				assert.Equal(t, s.Unit, config["unit_of_measurement"], "sensor %s unit mismatch", s.Key)
			}
			if s.DeviceClass != "" {
				assert.Equal(t, s.DeviceClass, config["device_class"], "sensor %s device_class mismatch", s.Key)
			}
		}
	})

	// --- Verify sensor state values match between MQTT and REST ---
	t.Run("sensor_values", func(t *testing.T) {
		// Get MQTT state payload
		stateTopic := "homeassistant/sensor/srne_system/state"
		mqttState, ok := store.get(stateTopic)
		require.True(t, ok, "missing MQTT state topic")

		var mqttSnap map[string]any
		require.NoError(t, json.Unmarshal(mqttState, &mqttSnap))

		// REST sensor values come from /api/entities
		for _, raw := range entities.Sensors {
			var sensor struct {
				Key       string `json:"key"`
				ValuePath string `json:"value_path"`
				Value     any    `json:"value"`
			}
			require.NoError(t, json.Unmarshal(raw, &sensor))

			mqttValue := walkJSON(mqttSnap, sensor.ValuePath)
			assert.Equal(t, sensor.Value, mqttValue,
				"sensor %s value mismatch: REST=%v MQTT=%v", sensor.Key, sensor.Value, mqttValue)
		}
	})

	// --- Verify sensor count matches ---
	t.Run("sensor_count", func(t *testing.T) {
		assert.Equal(t, len(systemSensors), len(entities.Sensors),
			"REST sensor count should match systemSensors registry")
	})
}

// TestMQTTRESTControlParity verifies control entities (switches, numbers, selects)
// have matching MQTT discovery and REST representation.
func TestMQTTRESTControlParity(t *testing.T) {
	_, addr, store := startTestBroker(t)

	sys := newTestSystem(t)
	hub := NewHub(sys, 50*time.Millisecond, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	time.Sleep(150 * time.Millisecond)

	mqttCfg := &MQTTConfig{
		Broker:      fmt.Sprintf("tcp://%s", addr),
		ClientID:    "test-publisher",
		TopicPrefix: "homeassistant",
	}
	pub, err := NewMQTTPublisher(mqttCfg, hub, sys.Units())
	require.NoError(t, err)
	go pub.Run(ctx)
	time.Sleep(300 * time.Millisecond)

	// REST entities
	ws := NewWebServer(hub, sys)
	req := httptest.NewRequest(http.MethodGet, "/api/entities", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var entities struct {
		Switches []json.RawMessage `json:"switches"`
		Numbers  []json.RawMessage `json:"numbers"`
		Selects  []json.RawMessage `json:"selects"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entities))

	// --- Switch parity ---
	t.Run("switch_discovery", func(t *testing.T) {
		assert.Equal(t, len(controlSwitches), len(entities.Switches), "switch count mismatch")

		for _, sw := range controlSwitches {
			configTopic := fmt.Sprintf("homeassistant/switch/srne_system/%s/config", sw.Key)
			payload, ok := store.get(configTopic)
			require.True(t, ok, "missing MQTT discovery for switch %s", sw.Key)

			var config map[string]any
			require.NoError(t, json.Unmarshal(payload, &config))
			assert.Equal(t, sw.Name, config["name"], "switch %s name mismatch", sw.Key)
			assert.NotEmpty(t, config["command_topic"], "switch %s missing command_topic", sw.Key)
			assert.NotEmpty(t, config["state_topic"], "switch %s missing state_topic", sw.Key)
		}
	})

	t.Run("switch_state", func(t *testing.T) {
		settings := hub.Settings()
		require.NotNil(t, settings)

		for i, sw := range controlSwitches {
			// MQTT state
			stateTopic := fmt.Sprintf("homeassistant/switch/srne_system/%s/state", sw.Key)
			mqttState, ok := store.get(stateTopic)
			require.True(t, ok, "missing MQTT state for switch %s", sw.Key)

			// REST state
			var restSw struct {
				State string `json:"state"`
			}
			require.NoError(t, json.Unmarshal(entities.Switches[i], &restSw))

			assert.Equal(t, restSw.State, string(mqttState),
				"switch %s state mismatch: REST=%s MQTT=%s", sw.Key, restSw.State, string(mqttState))
		}
	})

	// --- Number parity ---
	t.Run("number_discovery", func(t *testing.T) {
		assert.Equal(t, len(controlNumbers), len(entities.Numbers), "number count mismatch")

		for _, num := range controlNumbers {
			configTopic := fmt.Sprintf("homeassistant/number/srne_system/%s/config", num.Key)
			payload, ok := store.get(configTopic)
			require.True(t, ok, "missing MQTT discovery for number %s", num.Key)

			var config map[string]any
			require.NoError(t, json.Unmarshal(payload, &config))
			assert.Equal(t, num.Name, config["name"], "number %s name mismatch", num.Key)
			assert.Equal(t, num.Min, config["min"], "number %s min mismatch", num.Key)
			assert.Equal(t, num.Max, config["max"], "number %s max mismatch", num.Key)
			assert.Equal(t, num.Step, config["step"], "number %s step mismatch", num.Key)
			assert.NotEmpty(t, config["command_topic"], "number %s missing command_topic", num.Key)
		}
	})

	t.Run("number_state", func(t *testing.T) {
		for i, num := range controlNumbers {
			stateTopic := fmt.Sprintf("homeassistant/number/srne_system/%s/state", num.Key)
			mqttState, ok := store.get(stateTopic)
			require.True(t, ok, "missing MQTT state for number %s", num.Key)

			var restNum struct {
				Value string `json:"value"`
			}
			require.NoError(t, json.Unmarshal(entities.Numbers[i], &restNum))

			assert.Equal(t, restNum.Value, string(mqttState),
				"number %s value mismatch: REST=%s MQTT=%s", num.Key, restNum.Value, string(mqttState))
		}
	})

	// --- Select parity ---
	t.Run("select_discovery", func(t *testing.T) {
		assert.Equal(t, len(controlSelects), len(entities.Selects), "select count mismatch")

		for _, sel := range controlSelects {
			configTopic := fmt.Sprintf("homeassistant/select/srne_system/%s/config", sel.Key)
			payload, ok := store.get(configTopic)
			require.True(t, ok, "missing MQTT discovery for select %s", sel.Key)

			var config map[string]any
			require.NoError(t, json.Unmarshal(payload, &config))
			assert.Equal(t, sel.Name, config["name"], "select %s name mismatch", sel.Key)
			assert.NotEmpty(t, config["command_topic"], "select %s missing command_topic", sel.Key)

			// Verify options match
			opts, ok := config["options"].([]any)
			require.True(t, ok, "select %s options should be array", sel.Key)
			for j, opt := range sel.Options {
				assert.Equal(t, opt, opts[j], "select %s option %d mismatch", sel.Key, j)
			}
		}
	})

	t.Run("select_state", func(t *testing.T) {
		for i, sel := range controlSelects {
			stateTopic := fmt.Sprintf("homeassistant/select/srne_system/%s/state", sel.Key)
			mqttState, ok := store.get(stateTopic)
			require.True(t, ok, "missing MQTT state for select %s", sel.Key)

			var restSel struct {
				Value string `json:"value"`
			}
			require.NoError(t, json.Unmarshal(entities.Selects[i], &restSel))

			assert.Equal(t, restSel.Value, string(mqttState),
				"select %s value mismatch: REST=%s MQTT=%s", sel.Key, restSel.Value, string(mqttState))
		}
	})
}

// TestMQTTControlRoundTrip verifies that writing a control via MQTT command topic
// updates both the MQTT state and the REST state.
func TestMQTTControlRoundTrip(t *testing.T) {
	broker, addr, store := startTestBroker(t)

	sys := newTestSystem(t)
	hub := NewHub(sys, 50*time.Millisecond, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	time.Sleep(150 * time.Millisecond)

	mqttCfg := &MQTTConfig{
		Broker:      fmt.Sprintf("tcp://%s", addr),
		ClientID:    "test-publisher",
		TopicPrefix: "homeassistant",
	}
	pub, err := NewMQTTPublisher(mqttCfg, hub, sys.Units())
	require.NoError(t, err)
	go pub.Run(ctx)
	time.Sleep(300 * time.Millisecond)

	ws := NewWebServer(hub, sys)

	// --- Toggle charge_from_mains switch ON via MQTT ---
	t.Run("switch_round_trip", func(t *testing.T) {
		cmdTopic := "homeassistant/switch/srne_system/charge_from_mains/set"
		require.NoError(t, broker.Publish(cmdTopic, []byte("ON"), false, 0))

		// Wait for write to propagate
		time.Sleep(500 * time.Millisecond)

		// Check MQTT state
		stateTopic := "homeassistant/switch/srne_system/charge_from_mains/state"
		mqttState, ok := store.get(stateTopic)
		require.True(t, ok, "missing MQTT state after switch command")
		assert.Equal(t, "ON", string(mqttState))

		// Check REST state
		req := httptest.NewRequest(http.MethodGet, "/api/entities", nil)
		w := httptest.NewRecorder()
		ws.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var entities struct {
			Switches []struct {
				Key   string `json:"key"`
				State string `json:"state"`
			} `json:"switches"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entities))
		require.Len(t, entities.Switches, 1)
		assert.Equal(t, "ON", entities.Switches[0].State)
	})

	// --- Toggle charge_from_mains switch OFF → should restore previous priority ---
	t.Run("switch_restore", func(t *testing.T) {
		cmdTopic := "homeassistant/switch/srne_system/charge_from_mains/set"
		require.NoError(t, broker.Publish(cmdTopic, []byte("OFF"), false, 0))

		time.Sleep(500 * time.Millisecond)

		stateTopic := "homeassistant/switch/srne_system/charge_from_mains/state"
		mqttState, ok := store.get(stateTopic)
		require.True(t, ok)
		assert.Equal(t, "OFF", string(mqttState))

		// Charger priority should have been restored (not CUB)
		settings := hub.Settings()
		require.NotNil(t, settings)
		assert.NotEqual(t, uint16(1), settings.Inverter.ChargerPriority,
			"charger priority should not be CUB after switch OFF")
	})

	// --- Set a number control via REST and verify MQTT ---
	t.Run("number_rest_to_mqtt", func(t *testing.T) {
		body := strings.NewReader(`{"value":"90"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/controls/stop_charge_soc", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ws.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		// Wait for state to propagate via next poll
		time.Sleep(300 * time.Millisecond)

		stateTopic := "homeassistant/number/srne_system/stop_charge_soc/state"
		mqttState, ok := store.get(stateTopic)
		require.True(t, ok, "missing MQTT state after REST write")
		assert.Equal(t, "90", string(mqttState))
	})

	// --- Set a select control via MQTT and verify REST ---
	t.Run("select_mqtt_to_rest", func(t *testing.T) {
		cmdTopic := "homeassistant/select/srne_system/output_priority/set"
		require.NoError(t, broker.Publish(cmdTopic, []byte("UTI"), false, 0))

		time.Sleep(500 * time.Millisecond)

		req := httptest.NewRequest(http.MethodGet, "/api/entities", nil)
		w := httptest.NewRecorder()
		ws.Handler().ServeHTTP(w, req)

		var entities struct {
			Selects []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"selects"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entities))

		for _, sel := range entities.Selects {
			if sel.Key == "output_priority" {
				assert.Equal(t, "UTI", sel.Value)
				return
			}
		}
		t.Fatal("output_priority not found in REST entities")
	})
}
