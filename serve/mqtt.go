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

// systemSensors are published on the aggregated system device.
// Only whole-system statistics: summed values and shared-resource measurements.
var systemSensors = []sensorDef{
	// Battery (shared bank)
	{"battery_soc", "Battery SOC", "%", "battery", "measurement", "mdi:battery", "battery.soc"},
	{"battery_voltage", "Battery Voltage", "V", "voltage", "measurement", "mdi:flash", "battery.voltage"},
	{"battery_current", "Battery Current", "A", "current", "measurement", "mdi:current-dc", "battery.current"},
	{"battery_power", "Battery Power", "W", "power", "measurement", "mdi:battery-charging", "battery.total_charge_power"},
	{"battery_power_signed", "Battery Power (Signed)", "W", "power", "measurement", "mdi:battery-sync", "battery.signed_power"},
	{"charge_status", "Charge Status", "", "", "", "mdi:battery-sync", "battery.charge_status_name"},
	{"bms_voltage", "BMS Voltage", "V", "voltage", "measurement", "mdi:flash", "battery.bms_voltage"},
	{"bms_current", "BMS Current", "A", "current", "measurement", "mdi:current-dc", "battery.bms_current"},

	// PV (summed across all trackers and units)
	{"pv_total_power", "PV Total Power", "W", "power", "measurement", "mdi:solar-power", "pv.total_power"},

	// Load (summed)
	{"load_power", "Load Power", "W", "power", "measurement", "mdi:flash", "load.total_power"},
	{"load_apparent_power", "Load Apparent Power", "VA", "apparent_power", "measurement", "mdi:flash", "load.total_apparent_power"},
	{"load_dc_power", "DC Load Power", "W", "power", "measurement", "mdi:flash", "load.dc_power"},

	// Grid (shared grid connection)
	{"grid_frequency", "Grid Frequency", "Hz", "frequency", "measurement", "mdi:sine-wave", "grid.frequency"},
	{"mains_charge_current", "Mains Charge Current", "A", "current", "measurement", "mdi:transmission-tower-export", "grid.mains_charge_curr"},
	{"grid_power", "Grid Power", "W", "power", "measurement", "mdi:transmission-tower", "grid.total_power"},

	// Grid L1
	{"grid_voltage_l1", "Grid Voltage L1", "V", "voltage", "measurement", "mdi:transmission-tower", "grid.l1.grid_voltage"},
	{"grid_current_l1", "Grid Current L1", "A", "current", "measurement", "mdi:transmission-tower", "grid.l1.grid_current"},
	{"inverter_voltage_l1", "Inverter Voltage L1", "V", "voltage", "measurement", "mdi:sine-wave", "grid.l1.inverter_voltage"},
	{"inverter_current_l1", "Inverter Current L1", "A", "current", "measurement", "mdi:sine-wave", "grid.l1.inverter_current"},

	// Load L1
	{"load_power_l1", "Load Power L1", "W", "power", "measurement", "mdi:flash", "load.l1.power"},
	{"load_current_l1", "Load Current L1", "A", "current", "measurement", "mdi:flash", "load.l1.current"},
	{"load_apparent_power_l1", "Load Apparent Power L1", "VA", "apparent_power", "measurement", "mdi:flash", "load.l1.apparent_power"},
	{"load_ratio_l1", "Load Ratio L1", "%", "", "measurement", "mdi:gauge", "load.l1.ratio"},

	// Grid L2
	{"grid_voltage_l2", "Grid Voltage L2", "V", "voltage", "measurement", "mdi:transmission-tower", "grid.l2.grid_voltage"},
	{"grid_current_l2", "Grid Current L2", "A", "current", "measurement", "mdi:transmission-tower", "grid.l2.grid_current"},
	{"inverter_voltage_l2", "Inverter Voltage L2", "V", "voltage", "measurement", "mdi:sine-wave", "grid.l2.inverter_voltage"},
	{"inverter_current_l2", "Inverter Current L2", "A", "current", "measurement", "mdi:sine-wave", "grid.l2.inverter_current"},

	// Load L2
	{"load_power_l2", "Load Power L2", "W", "power", "measurement", "mdi:flash", "load.l2.power"},
	{"load_current_l2", "Load Current L2", "A", "current", "measurement", "mdi:flash", "load.l2.current"},
	{"load_apparent_power_l2", "Load Apparent Power L2", "VA", "apparent_power", "measurement", "mdi:flash", "load.l2.apparent_power"},
	{"load_ratio_l2", "Load Ratio L2", "%", "", "measurement", "mdi:gauge", "load.l2.ratio"},

	// System state
	{"inverter_state", "Inverter State", "", "", "", "mdi:state-machine", "inverter.machine_state_name"},

	// Stats — daily (summed across units)
	{"pv_generation_today", "PV Generation Today", "kWh", "energy", "total_increasing", "mdi:solar-power", "stats.pv_generation_today"},
	{"load_consumption_today", "Load Consumption Today", "kWh", "energy", "total_increasing", "mdi:flash", "stats.load_consumption_today"},
	{"battery_charge_today", "Battery Charge Today", "Ah", "", "total_increasing", "mdi:battery-plus", "stats.battery_charge_today"},
	{"battery_discharge_today", "Battery Discharge Today", "Ah", "", "total_increasing", "mdi:battery-minus", "stats.battery_discharge_today"},
	{"grid_charge_today", "Grid Charge Today", "Ah", "", "total_increasing", "mdi:transmission-tower-import", "stats.grid_charge_today"},
	{"battery_charge_energy", "Battery Charge Energy Today", "kWh", "energy", "total_increasing", "mdi:battery-plus", "stats.battery_charge_energy"},
	{"battery_discharge_energy", "Battery Discharge Energy Today", "kWh", "energy", "total_increasing", "mdi:battery-minus", "stats.battery_discharge_energy"},
	{"energy_import_today", "Energy Import Today", "kWh", "energy", "total_increasing", "mdi:transmission-tower-import", "stats.energy_import_today"},
	{"energy_export_today", "Energy Export Today", "kWh", "energy", "total_increasing", "mdi:transmission-tower-export", "stats.energy_export_today"},

	// Stats — lifetime (summed across units)
	{"accum_pv_generation", "PV Generation Total", "kWh", "energy", "total_increasing", "mdi:solar-power", "stats.accum_pv_generation"},
	{"accum_load_consumption", "Load Consumption Total", "kWh", "energy", "total_increasing", "mdi:flash", "stats.accum_load_consumption"},
	{"accum_battery_charge", "Battery Charge Total", "Ah", "", "total_increasing", "mdi:battery-plus", "stats.accum_battery_charge"},
	{"accum_battery_discharge", "Battery Discharge Total", "Ah", "", "total_increasing", "mdi:battery-minus", "stats.accum_battery_discharge"},
	{"accum_mains_charge", "Mains Charge Total", "Ah", "", "total_increasing", "mdi:transmission-tower-import", "stats.accum_mains_charge"},
	{"accum_energy_import", "Energy Import Total", "kWh", "energy", "total_increasing", "mdi:transmission-tower-import", "stats.accum_energy_import"},
}

// unitSensors are published on per-inverter devices.
// Includes everything: per-tracker PV, temperatures, diagnostics.
var unitSensors = []sensorDef{
	// Battery
	{"battery_soc", "Battery SOC", "%", "battery", "measurement", "mdi:battery", "battery.soc"},
	{"battery_voltage", "Battery Voltage", "V", "voltage", "measurement", "mdi:flash", "battery.voltage"},
	{"battery_current", "Battery Current", "A", "current", "measurement", "mdi:current-dc", "battery.current"},
	{"battery_power", "Battery Power", "W", "power", "measurement", "mdi:battery-charging", "battery.total_charge_power"},
	{"battery_temp", "Battery Temp", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "battery.battery_temp"},
	{"controller_temp", "Controller Temp", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "battery.controller_temp"},
	{"charge_status", "Charge Status", "", "", "", "mdi:battery-sync", "battery.charge_status_name"},
	{"bms_voltage", "BMS Voltage", "V", "voltage", "measurement", "mdi:flash", "battery.bms_voltage"},
	{"bms_current", "BMS Current", "A", "current", "measurement", "mdi:current-dc", "battery.bms_current"},
	{"bms_temperature", "BMS Temperature", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "battery.bms_temperature"},

	// PV (per-tracker detail)
	{"pv_total_power", "PV Total Power", "W", "power", "measurement", "mdi:solar-power", "pv.total_power"},
	{"pv1_voltage", "PV1 Voltage", "V", "voltage", "measurement", "mdi:solar-panel", "pv.pv1_voltage"},
	{"pv1_current", "PV1 Current", "A", "current", "measurement", "mdi:solar-panel", "pv.pv1_current"},
	{"pv1_power", "PV1 Power", "W", "power", "measurement", "mdi:solar-panel", "pv.pv1_power"},
	{"pv2_voltage", "PV2 Voltage", "V", "voltage", "measurement", "mdi:solar-panel", "pv.pv2_voltage"},
	{"pv2_current", "PV2 Current", "A", "current", "measurement", "mdi:solar-panel", "pv.pv2_current"},
	{"pv2_power", "PV2 Power", "W", "power", "measurement", "mdi:solar-panel", "pv.pv2_power"},

	// Load
	{"load_power", "Load Power", "W", "power", "measurement", "mdi:flash", "load.total_power"},
	{"load_apparent_power", "Load Apparent Power", "VA", "apparent_power", "measurement", "mdi:flash", "load.total_apparent_power"},
	{"load_power_factor", "Load Power Factor", "", "power_factor", "measurement", "mdi:angle-acute", "load.power_factor"},
	{"load_dc_power", "DC Load Power", "W", "power", "measurement", "mdi:flash", "load.dc_power"},

	// Grid
	{"grid_frequency", "Grid Frequency", "Hz", "frequency", "measurement", "mdi:sine-wave", "grid.frequency"},
	{"mains_charge_current", "Mains Charge Current", "A", "current", "measurement", "mdi:transmission-tower-export", "grid.mains_charge_curr"},

	// Grid L1
	{"grid_voltage_l1", "Grid Voltage L1", "V", "voltage", "measurement", "mdi:transmission-tower", "grid.l1.grid_voltage"},
	{"grid_current_l1", "Grid Current L1", "A", "current", "measurement", "mdi:transmission-tower", "grid.l1.grid_current"},
	{"inverter_voltage_l1", "Inverter Voltage L1", "V", "voltage", "measurement", "mdi:sine-wave", "grid.l1.inverter_voltage"},
	{"inverter_current_l1", "Inverter Current L1", "A", "current", "measurement", "mdi:sine-wave", "grid.l1.inverter_current"},

	// Load L1
	{"load_power_l1", "Load Power L1", "W", "power", "measurement", "mdi:flash", "load.l1.power"},
	{"load_current_l1", "Load Current L1", "A", "current", "measurement", "mdi:flash", "load.l1.current"},
	{"load_apparent_power_l1", "Load Apparent Power L1", "VA", "apparent_power", "measurement", "mdi:flash", "load.l1.apparent_power"},
	{"load_ratio_l1", "Load Ratio L1", "%", "", "measurement", "mdi:gauge", "load.l1.ratio"},

	// Grid L2
	{"grid_voltage_l2", "Grid Voltage L2", "V", "voltage", "measurement", "mdi:transmission-tower", "grid.l2.grid_voltage"},
	{"grid_current_l2", "Grid Current L2", "A", "current", "measurement", "mdi:transmission-tower", "grid.l2.grid_current"},
	{"inverter_voltage_l2", "Inverter Voltage L2", "V", "voltage", "measurement", "mdi:sine-wave", "grid.l2.inverter_voltage"},
	{"inverter_current_l2", "Inverter Current L2", "A", "current", "measurement", "mdi:sine-wave", "grid.l2.inverter_current"},

	// Load L2
	{"load_power_l2", "Load Power L2", "W", "power", "measurement", "mdi:flash", "load.l2.power"},
	{"load_current_l2", "Load Current L2", "A", "current", "measurement", "mdi:flash", "load.l2.current"},
	{"load_apparent_power_l2", "Load Apparent Power L2", "VA", "apparent_power", "measurement", "mdi:flash", "load.l2.apparent_power"},
	{"load_ratio_l2", "Load Ratio L2", "%", "", "measurement", "mdi:gauge", "load.l2.ratio"},

	// Inverter (per-unit diagnostics)
	{"inverter_state", "Inverter State", "", "", "", "mdi:state-machine", "inverter.machine_state_name"},
	{"bus_voltage", "Bus Voltage", "V", "voltage", "measurement", "mdi:flash", "inverter.bus_voltage"},
	{"inverter_frequency", "Inverter Frequency", "Hz", "frequency", "measurement", "mdi:sine-wave", "inverter.frequency"},
	{"heatsink_temp_a", "Heatsink Temp DC-DC", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "inverter.heatsink_temp_a"},
	{"heatsink_temp_b", "Heatsink Temp DC-AC", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "inverter.heatsink_temp_b"},
	{"heatsink_temp_c", "Heatsink Temp Transformer", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "inverter.heatsink_temp_c"},
	{"heatsink_temp_d", "Heatsink Temp Ambient", "\u00b0C", "temperature", "measurement", "mdi:thermometer", "inverter.heatsink_temp_d"},

	// Stats — daily
	{"pv_generation_today", "PV Generation Today", "kWh", "energy", "total_increasing", "mdi:solar-power", "stats.pv_generation_today"},
	{"load_consumption_today", "Load Consumption Today", "kWh", "energy", "total_increasing", "mdi:flash", "stats.load_consumption_today"},
	{"battery_charge_today", "Battery Charge Today", "Ah", "", "total_increasing", "mdi:battery-plus", "stats.battery_charge_today"},
	{"battery_discharge_today", "Battery Discharge Today", "Ah", "", "total_increasing", "mdi:battery-minus", "stats.battery_discharge_today"},
	{"grid_charge_today", "Grid Charge Today", "Ah", "", "total_increasing", "mdi:transmission-tower-import", "stats.grid_charge_today"},
	{"battery_charge_energy", "Battery Charge Energy Today", "kWh", "energy", "total_increasing", "mdi:battery-plus", "stats.battery_charge_energy"},
	{"battery_discharge_energy", "Battery Discharge Energy Today", "kWh", "energy", "total_increasing", "mdi:battery-minus", "stats.battery_discharge_energy"},
	{"energy_import_today", "Energy Import Today", "kWh", "energy", "total_increasing", "mdi:transmission-tower-import", "stats.energy_import_today"},
	{"energy_export_today", "Energy Export Today", "kWh", "energy", "total_increasing", "mdi:transmission-tower-export", "stats.energy_export_today"},

	// Stats — lifetime
	{"accum_pv_generation", "PV Generation Total", "kWh", "energy", "total_increasing", "mdi:solar-power", "stats.accum_pv_generation"},
	{"accum_load_consumption", "Load Consumption Total", "kWh", "energy", "total_increasing", "mdi:flash", "stats.accum_load_consumption"},
	{"accum_battery_charge", "Battery Charge Total", "Ah", "", "total_increasing", "mdi:battery-plus", "stats.accum_battery_charge"},
	{"accum_battery_discharge", "Battery Discharge Total", "Ah", "", "total_increasing", "mdi:battery-minus", "stats.accum_battery_discharge"},
	{"accum_mains_charge", "Mains Charge Total", "Ah", "", "total_increasing", "mdi:transmission-tower-import", "stats.accum_mains_charge"},
	{"accum_energy_import", "Energy Import Total", "kWh", "energy", "total_increasing", "mdi:transmission-tower-import", "stats.accum_energy_import"},
	{"total_running_days", "Total Running Days", "d", "duration", "", "mdi:calendar-clock", "stats.total_running_days"},
}

// MQTTPublisher publishes snapshots to an MQTT broker with HA auto-discovery.
type MQTTPublisher struct {
	client     mqtt.Client
	prefix     string
	hub        *Hub
	unitInfos  []inverter.UnitInfo
	mpptLabels map[string][2]string // host -> [mppt1, mppt2] display labels
}

// NewMQTTPublisher creates and connects an MQTT publisher.
// mpptLabels is optional; when present, per-unit PV1/PV2 entity names are rewritten
// to use the configured labels (e.g. "Roof East Voltage" instead of "PV1 Voltage").
func NewMQTTPublisher(cfg *MQTTConfig, hub *Hub, unitInfos []inverter.UnitInfo, mpptLabels map[string][2]string) (*MQTTPublisher, error) {
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
		prefix:     cfg.TopicPrefix,
		hub:        hub,
		unitInfos:  unitInfos,
		mpptLabels: mpptLabels,
	}

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		slog.Info("mqtt connected, publishing discovery", "broker", cfg.Broker)
		pub.publishDiscovery()
		pub.subscribeControls(c)
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
			p.publishControlState()
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

	// Control entities (switches, numbers, selects)
	p.publishControlDiscovery()
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

	for _, s := range unitSensors {
		configTopic := fmt.Sprintf("%s/sensor/%s/%s/config", p.prefix, deviceID, s.Key)

		payload := map[string]any{
			"name":               p.mpptRename(info.Host, s.Key, s.Name),
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

func (p *MQTTPublisher) publishControlDiscovery() {
	deviceID := "srne_system"
	device := map[string]any{
		"identifiers":  []string{deviceID},
		"name":         "SRNE Solar System",
		"manufacturer": "SRNE",
	}
	if len(p.unitInfos) > 0 {
		device["model"] = p.unitInfos[0].Model
	}
	availTopic := fmt.Sprintf("%s/sensor/%s/availability", p.prefix, deviceID)

	// Switches
	for _, sw := range controlSwitches {
		configTopic := fmt.Sprintf("%s/switch/%s/%s/config", p.prefix, deviceID, sw.Key)
		payload := map[string]any{
			"name":               sw.Name,
			"unique_id":          fmt.Sprintf("%s_%s", deviceID, sw.Key),
			"command_topic":      fmt.Sprintf("%s/switch/%s/%s/set", p.prefix, deviceID, sw.Key),
			"state_topic":        fmt.Sprintf("%s/switch/%s/%s/state", p.prefix, deviceID, sw.Key),
			"payload_on":         "ON",
			"payload_off":        "OFF",
			"availability_topic": availTopic,
			"device":             device,
			"icon":               sw.Icon,
		}
		data, _ := json.Marshal(payload)
		p.client.Publish(configTopic, 1, true, data)
	}

	// Numbers
	for _, num := range controlNumbers {
		configTopic := fmt.Sprintf("%s/number/%s/%s/config", p.prefix, deviceID, num.Key)
		payload := map[string]any{
			"name":                num.Name,
			"unique_id":           fmt.Sprintf("%s_%s", deviceID, num.Key),
			"command_topic":       fmt.Sprintf("%s/number/%s/%s/set", p.prefix, deviceID, num.Key),
			"state_topic":         fmt.Sprintf("%s/number/%s/%s/state", p.prefix, deviceID, num.Key),
			"min":                 num.Min,
			"max":                 num.Max,
			"step":                num.Step,
			"unit_of_measurement": num.Unit,
			"availability_topic":  availTopic,
			"device":              device,
			"icon":                num.Icon,
		}
		data, _ := json.Marshal(payload)
		p.client.Publish(configTopic, 1, true, data)
	}

	// Selects
	for _, sel := range controlSelects {
		configTopic := fmt.Sprintf("%s/select/%s/%s/config", p.prefix, deviceID, sel.Key)
		payload := map[string]any{
			"name":               sel.Name,
			"unique_id":          fmt.Sprintf("%s_%s", deviceID, sel.Key),
			"command_topic":      fmt.Sprintf("%s/select/%s/%s/set", p.prefix, deviceID, sel.Key),
			"state_topic":        fmt.Sprintf("%s/select/%s/%s/state", p.prefix, deviceID, sel.Key),
			"options":            sel.Options,
			"availability_topic": availTopic,
			"device":             device,
			"icon":               sel.Icon,
		}
		data, _ := json.Marshal(payload)
		p.client.Publish(configTopic, 1, true, data)
	}

	// Diagnostics (read-only sensors sourced from settings)
	for _, diag := range controlDiagnostics {
		configTopic := fmt.Sprintf("%s/sensor/%s/%s/config", p.prefix, deviceID, diag.Key)
		payload := map[string]any{
			"name":               diag.Name,
			"unique_id":          fmt.Sprintf("%s_%s", deviceID, diag.Key),
			"state_topic":        fmt.Sprintf("%s/sensor/%s/%s/state", p.prefix, deviceID, diag.Key),
			"entity_category":    "diagnostic",
			"availability_topic": availTopic,
			"device":             device,
			"icon":               diag.Icon,
		}
		data, _ := json.Marshal(payload)
		p.client.Publish(configTopic, 1, true, data)
	}
}

func (p *MQTTPublisher) subscribeControls(c mqtt.Client) {
	deviceID := "srne_system"
	topic := fmt.Sprintf("%s/+/%s/+/set", p.prefix, deviceID)
	c.Subscribe(topic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		// Parse topic: {prefix}/{component}/{deviceID}/{key}/set
		parts := strings.Split(msg.Topic(), "/")
		if len(parts) < 5 {
			return
		}
		key := parts[len(parts)-2]
		payload := string(msg.Payload())

		slog.Info("mqtt control command", "key", key, "value", payload)

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), controlWriteTimeout)
			defer cancel()
			if err := executeControl(ctx, p.hub, key, payload); err != nil {
				slog.Error("mqtt control write failed", "key", key, "error", err)
			} else {
				slog.Info("mqtt control written", "key", key, "value", payload)
				p.publishControlState()
			}
		}()
	})
}

func (p *MQTTPublisher) publishControlState() {
	settings := p.hub.Settings()
	if settings == nil {
		return
	}

	p.hub.InitMainsChargeCurrent(settings)
	deviceID := "srne_system"

	for _, sw := range controlSwitches {
		topic := fmt.Sprintf("%s/switch/%s/%s/state", p.prefix, deviceID, sw.Key)
		p.client.Publish(topic, 0, true, sw.StateFunc(settings))
	}
	for _, num := range controlNumbers {
		topic := fmt.Sprintf("%s/number/%s/%s/state", p.prefix, deviceID, num.Key)
		p.client.Publish(topic, 0, true, num.StateFunc(settings))
	}
	for _, sel := range controlSelects {
		topic := fmt.Sprintf("%s/select/%s/%s/state", p.prefix, deviceID, sel.Key)
		p.client.Publish(topic, 0, true, sel.StateFunc(settings))
	}
	for _, diag := range controlDiagnostics {
		topic := fmt.Sprintf("%s/sensor/%s/%s/state", p.prefix, deviceID, diag.Key)
		p.client.Publish(topic, 0, true, diag.StateFunc(settings))
	}
}

// mpptRename rewrites the display name for pv1_*/pv2_* sensors using the
// configured MPPT label for this host. Entity keys and unique_ids are left
// untouched so existing HA entities survive a config change.
func (p *MQTTPublisher) mpptRename(host, key, name string) string {
	labels, ok := p.mpptLabels[host]
	if !ok {
		return name
	}
	switch {
	case strings.HasPrefix(key, "pv1_") && labels[0] != "":
		return strings.Replace(name, "PV1", labels[0], 1)
	case strings.HasPrefix(key, "pv2_") && labels[1] != "":
		return strings.Replace(name, "PV2", labels[1], 1)
	}
	return name
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
