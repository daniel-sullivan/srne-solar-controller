package serve

import (
	"context"
	"fmt"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
)

// controlSwitch defines a Home Assistant switch entity.
type controlSwitch struct {
	Key         string
	Name        string
	Icon        string
	StateFunc   func(*inverter.Settings) string
	CommandFunc func(ctx context.Context, hub *Hub, payload string) error
}

// controlNumber defines a Home Assistant number entity.
type controlNumber struct {
	Key       string
	Name      string
	Icon      string
	Unit      string
	Field     string // WriteSetting field name
	Min       float64
	Max       float64
	Step      float64
	StateFunc func(*inverter.Settings) string
}

// controlSelect defines a Home Assistant select entity.
type controlSelect struct {
	Key       string
	Name      string
	Icon      string
	Field     string   // WriteSetting field name
	Options   []string // display names sent to/from HA
	Values    []string // corresponding WriteSetting values
	StateFunc func(*inverter.Settings) string
}

var controlSwitches = []controlSwitch{
	{
		Key:  "charge_from_mains",
		Name: "Charge from Mains",
		Icon: "mdi:transmission-tower-import",
		StateFunc: func(s *inverter.Settings) string {
			if s.Inverter.ChargerPriority == 1 {
				return "ON"
			}
			return "OFF"
		},
		CommandFunc: func(ctx context.Context, hub *Hub, payload string) error {
			switch payload {
			case "ON":
				// Save current priority before switching to CUB
				settings := hub.Settings()
				if settings != nil {
					hub.SaveChargerPriority(settings.Inverter.ChargerPriority)
				}
				return hub.WriteSetting(ctx, "charger_priority", "1")
			case "OFF":
				return hub.WriteSetting(ctx, "charger_priority", hub.RestoreChargerPriority())
			default:
				return fmt.Errorf("invalid switch payload: %q", payload)
			}
		},
	},
}

var controlNumbers = []controlNumber{
	{
		Key: "stop_charge_soc", Name: "Max Charge SOC", Icon: "mdi:battery-arrow-up",
		Unit: "%", Field: "stop_charge_soc", Min: 0, Max: 100, Step: 1,
		StateFunc: func(s *inverter.Settings) string {
			return fmt.Sprintf("%.0f", s.Battery.StopChargeSOC)
		},
	},
	{
		Key: "cutoff_discharge_soc", Name: "Discharge Cutoff SOC", Icon: "mdi:battery-arrow-down",
		Unit: "%", Field: "cutoff_discharge_soc", Min: 0, Max: 100, Step: 1,
		StateFunc: func(s *inverter.Settings) string {
			return fmt.Sprintf("%d", s.Battery.CutoffDischargeSOC)
		},
	},
	{
		Key: "soc_switch_to_mains", Name: "SOC Switch to Mains", Icon: "mdi:transmission-tower-import",
		Unit: "%", Field: "soc_switch_to_mains", Min: 0, Max: 100, Step: 1,
		StateFunc: func(s *inverter.Settings) string {
			return fmt.Sprintf("%.0f", s.Battery.SOCSwitchToMains)
		},
	},
	{
		Key: "soc_switch_to_battery", Name: "SOC Switch to Battery", Icon: "mdi:battery-charging",
		Unit: "%", Field: "soc_switch_to_battery", Min: 0, Max: 100, Step: 1,
		StateFunc: func(s *inverter.Settings) string {
			return fmt.Sprintf("%.0f", s.Battery.SOCSwitchToBattery)
		},
	},
	{
		Key: "mains_charge_current", Name: "Mains Charge Current", Icon: "mdi:transmission-tower-import",
		Unit: "A", Field: "mains_charge_current_lim", Min: 0, Max: 200, Step: 1,
		StateFunc: func(s *inverter.Settings) string {
			return fmt.Sprintf("%.0f", s.Inverter.MainsChargeCurrentLim)
		},
	},
}

var controlSelects = []controlSelect{
	{
		Key: "charger_priority", Name: "Charger Priority", Icon: "mdi:battery-charging",
		Field:   "charger_priority",
		Options: []string{"CSO", "CUB", "SNU", "OSO"},
		Values:  []string{"0", "1", "2", "3"},
		StateFunc: func(s *inverter.Settings) string {
			m := map[uint16]string{0: "CSO", 1: "CUB", 2: "SNU", 3: "OSO"}
			if name, ok := m[s.Inverter.ChargerPriority]; ok {
				return name
			}
			return "CSO"
		},
	},
	{
		Key: "output_priority", Name: "Output Priority", Icon: "mdi:power-plug",
		Field:   "output_priority",
		Options: []string{"SOL", "UTI", "SBU"},
		Values:  []string{"0", "1", "2"},
		StateFunc: func(s *inverter.Settings) string {
			m := map[uint16]string{0: "SOL", 1: "UTI", 2: "SBU"}
			if name, ok := m[s.Inverter.OutputPriority]; ok {
				return name
			}
			return "SOL"
		},
	},
}

// executeControl routes a control command by key. It handles switches, numbers, and selects.
func executeControl(ctx context.Context, hub *Hub, key, value string) error {
	if sw := findSwitch(key); sw != nil {
		return sw.CommandFunc(ctx, hub, value)
	}
	if num := findNumber(key); num != nil {
		return hub.WriteSetting(ctx, num.Field, value)
	}
	if sel := findSelect(key); sel != nil {
		// Translate option name to register value
		for i, opt := range sel.Options {
			if opt == value {
				return hub.WriteSetting(ctx, sel.Field, sel.Values[i])
			}
		}
		return fmt.Errorf("invalid option %q for %s", value, key)
	}
	return fmt.Errorf("unknown control: %q", key)
}

// controlWriteTimeout is the default timeout for control write operations.
const controlWriteTimeout = 30 * time.Second

func findSwitch(key string) *controlSwitch {
	for i := range controlSwitches {
		if controlSwitches[i].Key == key {
			return &controlSwitches[i]
		}
	}
	return nil
}

func findNumber(key string) *controlNumber {
	for i := range controlNumbers {
		if controlNumbers[i].Key == key {
			return &controlNumbers[i]
		}
	}
	return nil
}

func findSelect(key string) *controlSelect {
	for i := range controlSelects {
		if controlSelects[i].Key == key {
			return &controlSelects[i]
		}
	}
	return nil
}
