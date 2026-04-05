package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
)

// sensorDef defines a Home Assistant MQTT sensor entity.
type sensorDef struct {
	Key         string
	Name        string
	Unit        string
	DeviceClass string
	StateClass  string
	Icon        string
	ValuePath   string // JSON path for value_template (e.g., "battery.soc")
}

// systemSensors defines all HA sensor entities for the aggregated system.
var systemSensors = []sensorDef{
	{"battery_soc", "Battery SOC", "%", "battery", "measurement", "mdi:battery", "battery.soc"},
	{"battery_voltage", "Battery Voltage", "V", "voltage", "measurement", "mdi:flash", "battery.voltage"},
	{"battery_current", "Battery Current", "A", "current", "measurement", "mdi:current-dc", "battery.current"},
	{"battery_power", "Charge Power", "W", "power", "measurement", "mdi:battery-charging", "battery.total_charge_power"},
	{"battery_temp", "Battery Temp", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "battery.battery_temp"},
	{"controller_temp", "Controller Temp", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "battery.controller_temp"},
	{"pv_total_power", "PV Total Power", "W", "power", "measurement", "mdi:solar-power", "pv.total_power"},
	{"pv1_power", "PV1 Power", "W", "power", "measurement", "mdi:solar-panel", "pv.pv1_power"},
	{"pv2_power", "PV2 Power", "W", "power", "measurement", "mdi:solar-panel", "pv.pv2_power"},
	{"load_power", "Load Power", "W", "power", "measurement", "mdi:flash", "load.total_power"},
	{"grid_frequency", "Grid Frequency", "Hz", "frequency", "measurement", "mdi:sine-wave", "grid.frequency"},
	{"inverter_state", "Inverter State", "", "", "", "mdi:state-machine", "inverter.machine_state_name"},
	{"bus_voltage", "Bus Voltage", "V", "voltage", "measurement", "mdi:flash", "inverter.bus_voltage"},
	{"inverter_frequency", "Inverter Frequency", "Hz", "frequency", "measurement", "mdi:sine-wave", "inverter.frequency"},
	{"heatsink_temp_a", "Heatsink Temp DC-DC", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "inverter.heatsink_temp_a"},
	{"heatsink_temp_b", "Heatsink Temp DC-AC", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "inverter.heatsink_temp_b"},
	{"pv_generation_today", "PV Generation Today", "kWh", "energy", "total_increasing", "mdi:solar-power", "stats.pv_generation_today"},
	{"load_consumption_today", "Load Consumption Today", "kWh", "energy", "total_increasing", "mdi:flash", "stats.load_consumption_today"},
	{"battery_charge_today", "Battery Charge Today", "Ah", "", "total_increasing", "mdi:battery-plus", "stats.battery_charge_today"},
	{"battery_discharge_today", "Battery Discharge Today", "Ah", "", "total_increasing", "mdi:battery-minus", "stats.battery_discharge_today"},
}

// MQTTPublisher publishes snapshots to an MQTT broker with HA auto-discovery.
type MQTTPublisher struct {
	client    mqtt.Client
	prefix    string
	hub       *Hub
	unitInfos []inverter.UnitInfo
}

// NewMQTTPublisher creates and connects an MQTT publisher.
func NewMQTTPublisher(cfg *MQTTConfig, hub *Hub, unitInfos []inverter.UnitInfo) (*MQTTPublisher, error) {
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetKeepAlive(30 * time.Second).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	// LWT: mark system offline on disconnect
	availTopic := fmt.Sprintf("%s/sensor/srne_system/availability", cfg.TopicPrefix)
	opts.SetWill(availTopic, "offline", 1, true)

	pub := &MQTTPublisher{
		prefix:    cfg.TopicPrefix,
		hub:       hub,
		unitInfos: unitInfos,
	}

	opts.SetOnConnectHandler(func(_ mqtt.Client) {
		slog.Info("mqtt connected, publishing discovery", "broker", cfg.Broker)
		pub.publishDiscovery()
		pub.publishAvailability("online")
	})

	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		slog.Warn("mqtt connection lost", "error", err)
	})

	opts.SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
		slog.Info("mqtt reconnecting")
	})

	pub.client = mqtt.NewClient(opts)
	token := pub.client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("mqtt connect: %w", err)
	}

	return pub, nil
}

// Run subscribes to the hub and publishes state on each snapshot. Blocks until ctx is done.
func (p *MQTTPublisher) Run(ctx context.Context) {
	sub := p.hub.Subscribe()
	defer p.hub.Unsubscribe(sub)

	for {
		select {
		case <-ctx.Done():
			p.publishAvailability("offline")
			p.client.Disconnect(1000)
			return
		case snap, ok := <-sub.C:
			if !ok {
				return
			}
			p.publishState(snap)
		}
	}
}

func (p *MQTTPublisher) publishDiscovery() {
	deviceID := "srne_system"
	device := map[string]any{
		"identifiers":  []string{deviceID},
		"name":         "SRNE Solar System",
		"manufacturer": "SRNE",
	}
	if len(p.unitInfos) > 0 {
		device["model"] = p.unitInfos[0].Model
	}

	stateTopic := fmt.Sprintf("%s/sensor/%s/state", p.prefix, deviceID)
	availTopic := fmt.Sprintf("%s/sensor/%s/availability", p.prefix, deviceID)

	for _, s := range systemSensors {
		configTopic := fmt.Sprintf("%s/sensor/%s/%s/config", p.prefix, deviceID, s.Key)

		payload := map[string]any{
			"name":               s.Name,
			"unique_id":          fmt.Sprintf("%s_%s", deviceID, s.Key),
			"state_topic":        stateTopic,
			"value_template":     fmt.Sprintf("{{ value_json.%s }}", s.ValuePath),
			"availability_topic": availTopic,
			"device":             device,
		}
		if s.Unit != "" {
			payload["unit_of_measurement"] = s.Unit
		}
		if s.DeviceClass != "" {
			payload["device_class"] = s.DeviceClass
		}
		if s.StateClass != "" {
			payload["state_class"] = s.StateClass
		}
		if s.Icon != "" {
			payload["icon"] = s.Icon
		}

		data, _ := json.Marshal(payload)
		p.client.Publish(configTopic, 1, true, data)
	}

	// Per-unit discovery
	for _, info := range p.unitInfos {
		p.publishUnitDiscovery(info)
	}
}

func (p *MQTTPublisher) publishUnitDiscovery(info inverter.UnitInfo) {
	serial := sanitizeSerial(info.Serial)
	deviceID := fmt.Sprintf("srne_%s", serial)

	device := map[string]any{
		"identifiers":  []string{deviceID},
		"name":         fmt.Sprintf("SRNE %s", info.Host),
		"manufacturer": "SRNE",
		"model":        info.Model,
		"via_device":   "srne_system",
	}

	stateTopic := fmt.Sprintf("%s/sensor/%s/state", p.prefix, deviceID)
	availTopic := fmt.Sprintf("%s/sensor/srne_system/availability", p.prefix)

	for _, s := range systemSensors {
		configTopic := fmt.Sprintf("%s/sensor/%s/%s/config", p.prefix, deviceID, s.Key)

		payload := map[string]any{
			"name":               s.Name,
			"unique_id":          fmt.Sprintf("%s_%s", deviceID, s.Key),
			"state_topic":        stateTopic,
			"value_template":     fmt.Sprintf("{{ value_json.%s }}", s.ValuePath),
			"availability_topic": availTopic,
			"device":             device,
		}
		if s.Unit != "" {
			payload["unit_of_measurement"] = s.Unit
		}
		if s.DeviceClass != "" {
			payload["device_class"] = s.DeviceClass
		}
		if s.StateClass != "" {
			payload["state_class"] = s.StateClass
		}
		if s.Icon != "" {
			payload["icon"] = s.Icon
		}

		data, _ := json.Marshal(payload)
		p.client.Publish(configTopic, 1, true, data)
	}
}

func (p *MQTTPublisher) publishState(snap *inverter.Snapshot) {
	// System-level state
	stateTopic := fmt.Sprintf("%s/sensor/srne_system/state", p.prefix)
	data, err := json.Marshal(snap)
	if err != nil {
		slog.Error("mqtt state marshal failed", "error", err)
		return
	}
	p.client.Publish(stateTopic, 0, false, data)

	// Per-unit state
	for _, unit := range snap.Units {
		serial := sanitizeSerial(unit.Serial)
		unitTopic := fmt.Sprintf("%s/sensor/srne_%s/state", p.prefix, serial)
		unitData, err := json.Marshal(unit)
		if err != nil {
			continue
		}
		p.client.Publish(unitTopic, 0, false, unitData)
	}
}

func (p *MQTTPublisher) publishAvailability(status string) {
	topic := fmt.Sprintf("%s/sensor/srne_system/availability", p.prefix)
	p.client.Publish(topic, 1, true, status)
}

// sanitizeSerial replaces characters not safe for MQTT topics.
func sanitizeSerial(serial string) string {
	serial = strings.ReplaceAll(serial, "/", "_")
	serial = strings.ReplaceAll(serial, "+", "_")
	serial = strings.ReplaceAll(serial, "#", "_")
	if serial == "" {
		return "unknown"
	}
	return serial
}
