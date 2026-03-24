package register

import "fmt"

// GroupEntry pairs a short name with its group definition.
type GroupEntry struct {
	Key   string
	Group Group
}

// OrderedGroups returns all groups in display order.
func OrderedGroups() []GroupEntry {
	return []GroupEntry{
		{"battery", BatteryData},
		{"inverter", InverterData},
		{"settings", BatterySettings},
		{"timed", TimedChargeDischarge},
		{"inverter-settings", InverterSettings},
		{"stats", Statistics},
	}
}

// AllGroups returns all known register groups keyed by name.
func AllGroups() map[string]Group {
	m := make(map[string]Group)
	for _, e := range OrderedGroups() {
		m[e.Key] = e.Group
	}
	return m
}

// ProductInfo — P00: Product information (0x000A-0x0048)
var ProductInfo = Group{
	Name: "Product Info",
	Registers: []Register{
		{Address: 0x000A, Name: "Max Voltage / Rated Current", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x000B, Name: "Product Type", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x000C, Name: "Product Model", Unit: "", Type: ASCII, Access: ReadOnly, Count: 8},
		{Address: 0x0014, Name: "Software Version CPU1", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0015, Name: "Software Version CPU2", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0016, Name: "Hardware Version (Control)", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0017, Name: "Hardware Version (Power)", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x001A, Name: "RS485 Address", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x001B, Name: "Model Code", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x001C, Name: "Protocol Version", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x001E, Name: "Manufacture Date", Unit: "", Type: U32, Access: ReadOnly, Count: 2, Optional: true},
		{Address: 0x0021, Name: "Compilation Date/Time", Unit: "", Type: ASCII, Access: ReadOnly, Count: 20, Optional: true},
		{Address: 0x0035, Name: "Serial Number", Unit: "", Type: ASCII, Access: ReadOnly, Count: 20},
	},
}

// BatteryData — P01: Controller/battery realtime data (0x0100-0x0111)
var BatteryData = Group{
	Name: "Battery / PV Data",
	Registers: []Register{
		{Address: 0x0100, Name: "Battery SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0101, Name: "Battery Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0102, Name: "Battery Current", Scale: Mul01, Unit: "A", Type: S16, Access: ReadOnly, Count: 1},
		{Address: 0x0103, Name: "Temps (Controller/Battery)", Unit: "°C", Type: PackedTemp, Access: ReadOnly, Count: 1},
		{Address: 0x0104, Name: "DC Load Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0105, Name: "DC Load Current", Scale: Mul001, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0106, Name: "DC Load Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0107, Name: "PV1 Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0108, Name: "PV1 Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0109, Name: "PV1 Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x010A, Name: "DC Load On/Off", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0x010B, Name: "Charge Status", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x010C, Name: "Fault/Alarm Bits", Unit: "", Type: U32, Access: ReadOnly, Count: 2},
		{Address: 0x010E, Name: "Total Charge Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x010F, Name: "PV2 Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0110, Name: "PV2 Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0111, Name: "PV2 Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		// BMS data (V2.08+)
		{Address: 0x0112, Name: "BMS Battery Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0113, Name: "BMS Battery Current", Scale: Mul01, Unit: "A", Type: S16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0114, Name: "BMS Battery Temperature", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0115, Name: "BMS Charge Limit Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0116, Name: "BMS Discharge Limit Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0117, Name: "BMS Charge Limit Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0118, Name: "BMS Alarm Bits", Unit: "", Type: U32, Access: ReadOnly, Count: 2, Optional: true},
		{Address: 0x011A, Name: "BMS Protect Bits", Unit: "", Type: U32, Access: ReadOnly, Count: 2, Optional: true},
		{Address: 0x011C, Name: "Battery 2 Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x011D, Name: "Battery 2 Current", Scale: Mul01, Unit: "A", Type: S16, Access: ReadOnly, Count: 1, Optional: true},
	},
}

// InverterData — P02: Inverter/grid/load realtime data (0x0200-0x0237)
var InverterData = Group{
	Name: "Inverter Data",
	Registers: []Register{
		{Address: 0x0200, Name: "Fault Bits", Unit: "", Type: U32, Access: ReadOnly, Count: 2},
		{Address: 0x0202, Name: "Fault Bits (ext)", Unit: "", Type: U32, Access: ReadOnly, Count: 2},
		{Address: 0x0204, Name: "Fault Code 1", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0205, Name: "Fault Code 2", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0206, Name: "Fault Code 3", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0207, Name: "Fault Code 4", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x020C, Name: "Current Time", Unit: "", Type: PackedTime, Access: ReadWrite, Count: 3},
		{Address: 0x0210, Name: "Machine State", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0211, Name: "Password Status", Unit: "", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0212, Name: "Bus Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0213, Name: "Grid Voltage L1", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0214, Name: "Grid Current L1", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0215, Name: "Grid Frequency", Scale: Mul001, Unit: "Hz", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0216, Name: "Inverter Voltage L1", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0217, Name: "Inverter Current L1", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0218, Name: "Inverter Frequency", Scale: Mul001, Unit: "Hz", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0219, Name: "Load Current L1", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x021A, Name: "Load Power Factor", Scale: Mul001, Unit: "", Type: S16, Access: ReadOnly, Count: 1},
		{Address: 0x021B, Name: "Load Power L1", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x021C, Name: "Load Apparent Power L1", Scale: Mul1, Unit: "VA", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x021D, Name: "Inverter DC Component", Scale: Mul1, Unit: "mV", Type: S16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x021E, Name: "Mains Charge Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x021F, Name: "Load Ratio L1", Scale: Mul1, Unit: "%", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0x0220, Name: "Heatsink A Temp (DC-DC)", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1},
		{Address: 0x0221, Name: "Heatsink B Temp (DC-AC)", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1},
		{Address: 0x0222, Name: "Heatsink C Temp (Transformer)", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1},
		{Address: 0x0223, Name: "Heatsink D Temp (Ambient)", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1},
		{Address: 0x0224, Name: "PV Charge Current (Batt Side)", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		// L2/L3 — split-phase and 3-phase models
		{Address: 0x022A, Name: "Grid Voltage L2", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x022B, Name: "Grid Current L2", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x022C, Name: "Inverter Voltage L2", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x022D, Name: "Inverter Current L2", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x022E, Name: "Load Current L2", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x022F, Name: "Load Power L2", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0230, Name: "Load Apparent Power L2", Scale: Mul1, Unit: "VA", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0231, Name: "Load Ratio L2", Scale: Mul1, Unit: "%", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0232, Name: "Grid Voltage L3", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0233, Name: "Grid Current L3", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0234, Name: "Inverter Voltage L3", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0235, Name: "Inverter Current L3", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0236, Name: "Load Current L3", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0237, Name: "Load Power L3", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0238, Name: "Load Apparent Power L3", Scale: Mul1, Unit: "VA", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0x0239, Name: "Load Ratio L3", Scale: Mul1, Unit: "%", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
	},
}

// BatterySettings — P05: Battery parameter settings (0xE001-0xE025)
var BatterySettings = Group{
	Name: "Battery Settings",
	Registers: []Register{
		{Address: 0xE001, Name: "PV Charge Current Limit", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE002, Name: "Nominal Battery Capacity", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE003, Name: "System Voltage", Scale: Mul1, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE004, Name: "Battery Type", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE005, Name: "Over Voltage Protection", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE006, Name: "Limited Charge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE007, Name: "Equalizing Charge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE008, Name: "Boost Charge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE009, Name: "Float Charge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE00A, Name: "Boost Return Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE00B, Name: "Over Discharge Return Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE00C, Name: "Under Voltage Warning", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE00D, Name: "Over Discharge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE00E, Name: "Limited Discharge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE00F, Name: "Cutoff SOC (Charge/Discharge)", Unit: "%", Type: PackedHiLo, Access: ReadWrite, Count: 1},
		{Address: 0xE010, Name: "Over Discharge Delay", Scale: Mul1, Unit: "s", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE011, Name: "Equalizing Charge Time", Scale: Mul1, Unit: "min", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE012, Name: "Boost Charge Time", Scale: Mul1, Unit: "min", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE013, Name: "Equalizing Charge Interval", Scale: Mul1, Unit: "days", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE014, Name: "Temp Compensation Coeff", Scale: Mul1, Unit: "mV/°C/2V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE015, Name: "Charge Max Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE016, Name: "Charge Min Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE017, Name: "Discharge Max Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE018, Name: "Discharge Min Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE019, Name: "Battery Heater Start Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE01A, Name: "Battery Heater Stop Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE01B, Name: "Mains Switching Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE01C, Name: "Stop Charge Current (Li)", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE01D, Name: "Stop Charge SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE01E, Name: "Low SOC Alarm", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE01F, Name: "SOC Switch To Mains", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE020, Name: "SOC Switch To Battery", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE022, Name: "Inverter Switching Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE023, Name: "Equalizing Charge Timeout", Scale: Mul1, Unit: "min", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE024, Name: "Li Activation Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE025, Name: "BMS Charge LC Mode", Scale: Mul1, Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		// Extended settings (may not exist on all models)
		{Address: 0xE037, Name: "Grid-Connected Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE038, Name: "GFCI Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE039, Name: "PV Power Priority", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
	},
}

// TimedChargeDischarge — P05 cont: Sectional timed charge/discharge (0xE026-0xE04D)
var TimedChargeDischarge = Group{
	Name: "Timed Charge/Discharge",
	Registers: []Register{
		// Charge time sections
		{Address: 0xE026, Name: "Charge Start Time 1", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE027, Name: "Charge End Time 1", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE028, Name: "Charge Start Time 2", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE029, Name: "Charge End Time 2", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE02A, Name: "Charge Start Time 3", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE02B, Name: "Charge End Time 3", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE02C, Name: "Timed Charge Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		// Discharge time sections
		{Address: 0xE02D, Name: "Discharge Start Time 1", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE02E, Name: "Discharge End Time 1", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE02F, Name: "Discharge Start Time 2", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE030, Name: "Discharge End Time 2", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE031, Name: "Discharge Start Time 3", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE032, Name: "Discharge End Time 3", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: 0xE033, Name: "Timed Discharge Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		// Per-section SOC cutoffs (ASF V1.94+, may not exist on all models/firmware)
		{Address: 0xE03B, Name: "Charge 1 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE03C, Name: "Charge 2 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE03D, Name: "Charge 3 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE03E, Name: "Discharge 1 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE03F, Name: "Discharge 2 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE040, Name: "Discharge 3 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		// Per-section voltage cutoffs + discharge power (ASF V1.95+)
		{Address: 0xE041, Name: "Charge 1 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE042, Name: "Charge 2 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE043, Name: "Charge 3 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE044, Name: "Discharge 1 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE045, Name: "Discharge 2 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE046, Name: "Discharge 3 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE047, Name: "Discharge 1 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE048, Name: "Discharge 2 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE049, Name: "Discharge 3 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		// Charge power limits + source (ASF V1.96+)
		{Address: 0xE04A, Name: "Charge 1 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE04B, Name: "Charge 2 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE04C, Name: "Charge 3 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
	},
}

// InverterSettings — P07: Inverter user settings (0xE200-0xE221)
var InverterSettings = Group{
	Name: "Inverter Settings",
	Registers: []Register{
		{Address: 0xE200, Name: "RS485 Address", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE201, Name: "Parallel Mode", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE204, Name: "Output Priority", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE205, Name: "Mains Charge Current Limit", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE206, Name: "Equalizing Charge Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE207, Name: "N-G Function Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE208, Name: "Output Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE209, Name: "Output Frequency", Scale: Mul001, Unit: "Hz", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE20A, Name: "Max Charge Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE20B, Name: "AC Input Range", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE20C, Name: "Power Saving Mode", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE20D, Name: "Overload Auto Restart", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE20E, Name: "Over Temp Auto Restart", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE20F, Name: "Charger Priority", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE210, Name: "Alarm Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE211, Name: "Alarm On Input Loss", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE212, Name: "Overload Bypass Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE213, Name: "Record Fault Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE214, Name: "BMS Error Stop Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE215, Name: "BMS Communication Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: 0xE216, Name: "DC Load Switch", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE218, Name: "Derate Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE21B, Name: "BMS Protocol", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		// Extended inverter settings (may not exist on all models)
		{Address: 0xE21C, Name: "Max Line Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE21D, Name: "Max Line Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE21E, Name: "Output Phase", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE21F, Name: "Generator Work Mode", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE220, Name: "Generator Charge Max Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: 0xE221, Name: "Generator Rated Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
	},
}

// Statistics — P08: Power statistics (0xF000-0xF04B)
var Statistics = Group{
	Name: "Statistics",
	Registers: []Register{
		// Last 7 days history (index 0 = today-6, index 6 = yesterday)
		{Address: 0xF000, Name: "PV Gen Day-7", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF001, Name: "PV Gen Day-6", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF002, Name: "PV Gen Day-5", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF003, Name: "PV Gen Day-4", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF004, Name: "PV Gen Day-3", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF005, Name: "PV Gen Day-2", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF006, Name: "PV Gen Day-1", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF007, Name: "Batt Charge Day-7", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF008, Name: "Batt Charge Day-6", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF009, Name: "Batt Charge Day-5", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF00A, Name: "Batt Charge Day-4", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF00B, Name: "Batt Charge Day-3", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF00C, Name: "Batt Charge Day-2", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF00D, Name: "Batt Charge Day-1", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF00E, Name: "Batt Discharge Day-7", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF00F, Name: "Batt Discharge Day-6", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF010, Name: "Batt Discharge Day-5", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF011, Name: "Batt Discharge Day-4", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF012, Name: "Batt Discharge Day-3", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF013, Name: "Batt Discharge Day-2", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF014, Name: "Batt Discharge Day-1", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF015, Name: "Mains Charge Day-7", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF016, Name: "Mains Charge Day-6", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF017, Name: "Mains Charge Day-5", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF018, Name: "Mains Charge Day-4", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF019, Name: "Mains Charge Day-3", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF01A, Name: "Mains Charge Day-2", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF01B, Name: "Mains Charge Day-1", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF01C, Name: "Load Day-7", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF01D, Name: "Load Day-6", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF01E, Name: "Load Day-5", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF01F, Name: "Load Day-4", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF020, Name: "Load Day-3", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF021, Name: "Load Day-2", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF022, Name: "Load Day-1", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF023, Name: "Mains Load Day-7", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF024, Name: "Mains Load Day-6", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF025, Name: "Mains Load Day-5", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF026, Name: "Mains Load Day-4", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF027, Name: "Mains Load Day-3", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF028, Name: "Mains Load Day-2", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF029, Name: "Mains Load Day-1", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		// Today / accumulated
		{Address: 0xF02C, Name: "Energy Export Today", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0xF02D, Name: "Battery Charge Today", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF02E, Name: "Battery Discharge Today", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF02F, Name: "PV Generation Today", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF030, Name: "Load Consumption Today", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF031, Name: "Total Running Days", Scale: Mul1, Unit: "days", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF032, Name: "Total Overdischarge Count", Scale: Mul1, Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF033, Name: "Total Full Charge Count", Scale: Mul1, Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF034, Name: "Accumulated Battery Charge", Scale: Mul1, Unit: "AH", Type: U32, Access: ReadOnly, Count: 2},
		{Address: 0xF036, Name: "Accumulated Battery Discharge", Scale: Mul1, Unit: "AH", Type: U32, Access: ReadOnly, Count: 2},
		{Address: 0xF038, Name: "Accumulated PV Generation", Scale: Mul01, Unit: "kWh", Type: U32, Access: ReadOnly, Count: 2},
		{Address: 0xF03A, Name: "Accumulated Load Consumption", Scale: Mul01, Unit: "kWh", Type: U32, Access: ReadOnly, Count: 2},
		{Address: 0xF03C, Name: "Grid Charge Today", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0xF03D, Name: "Energy Import Today", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: 0xF046, Name: "Accumulated Mains Charge", Scale: Mul01, Unit: "kWh", Type: U32, Access: ReadOnly, Count: 2},
		{Address: 0xF04A, Name: "Accumulated Inverter Hours", Scale: Mul1, Unit: "h", Type: U16, Access: ReadOnly, Count: 1},
		{Address: 0xF04B, Name: "Accumulated Bypass Hours", Scale: Mul1, Unit: "h", Type: U16, Access: ReadOnly, Count: 1},
	},
}

// MachineState returns a human-readable string for the machine state register.
func MachineState(value uint16) string {
	states := map[uint16]string{
		0: "Standby (Delay)", 1: "Waiting", 2: "Initializing", 3: "Soft Start",
		4: "Mains Mode", 5: "Inverter Mode", 6: "Inverter → Mains", 7: "Mains → Inverter",
		8: "Battery Activate", 9: "Shutdown", 10: "Fault",
	}
	if s, ok := states[value]; ok {
		return s
	}
	return fmt.Sprintf("Unknown (%d)", value)
}

// ChargeStatus returns a human-readable string for the charge status register.
func ChargeStatus(value uint16) string {
	statuses := map[uint16]string{
		0: "Off", 1: "Quick Charge", 2: "Constant Voltage", 4: "Float", 6: "Li Activate",
	}
	if s, ok := statuses[value]; ok {
		return s
	}
	return fmt.Sprintf("Unknown (%d)", value)
}

// BatteryType returns a human-readable string for the battery type register.
func BatteryType(value uint16) string {
	types := map[uint16]string{
		0: "User Defined", 1: "Sealed Lead-Acid", 2: "Flooded Lead-Acid",
		3: "GEL", 4: "LiFePO4", 5: "NCA", 6: "LiFePO4 (BMS)",
	}
	if s, ok := types[value]; ok {
		return s
	}
	return fmt.Sprintf("Unknown (%d)", value)
}

// OutputPriority returns a human-readable string.
func OutputPriority(value uint16) string {
	p := map[uint16]string{
		0: "SOL (Solar First)", 1: "UTI (Utility/Mains First)", 2: "SBU (Solar → Battery → Utility)",
	}
	if s, ok := p[value]; ok {
		return s
	}
	return fmt.Sprintf("Unknown (%d)", value)
}

// ChargerPriority returns a human-readable string.
func ChargerPriority(value uint16) string {
	p := map[uint16]string{
		0: "CSO (PV Preferred)", 1: "CUB (Mains Preferred)", 2: "SNU (Hybrid)", 3: "OSO (PV Only)",
	}
	if s, ok := p[value]; ok {
		return s
	}
	return fmt.Sprintf("Unknown (%d)", value)
}
