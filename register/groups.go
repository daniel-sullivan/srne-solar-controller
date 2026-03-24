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
		{Address: AddrMaxVoltageRatedCurrent, Name: "Max Voltage / Rated Current", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrProductType, Name: "Product Type", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrProductModel, Name: "Product Model", Unit: "", Type: ASCII, Access: ReadOnly, Count: 8},
		{Address: AddrSoftwareVersionCPU1, Name: "Software Version CPU1", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrSoftwareVersionCPU2, Name: "Software Version CPU2", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrHardwareVersionControl, Name: "Hardware Version (Control)", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrHardwareVersionPower, Name: "Hardware Version (Power)", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrRS485Address, Name: "RS485 Address", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrModelCode, Name: "Model Code", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrProtocolVersion, Name: "Protocol Version", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrManufactureDate, Name: "Manufacture Date", Unit: "", Type: U32, Access: ReadOnly, Count: 2, Optional: true},
		{Address: AddrCompilationDateTime, Name: "Compilation Date/Time", Unit: "", Type: ASCII, Access: ReadOnly, Count: 20, Optional: true},
		{Address: AddrSerialNumber, Name: "Serial Number", Unit: "", Type: ASCIILoByte, Access: ReadOnly, Count: 20},
	},
}

// BatteryData — P01: Controller/battery realtime data (0x0100-0x011D)
var BatteryData = Group{
	Name: "Battery / PV Data",
	Registers: []Register{
		{Address: AddrBatterySOC, Name: "Battery SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBatteryVoltage, Name: "Battery Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBatteryCurrent, Name: "Battery Current", Scale: Mul01, Unit: "A", Type: S16, Access: ReadOnly, Count: 1},
		{Address: AddrTemps, Name: "Temps (Controller/Battery)", Unit: "°C", Type: PackedTemp, Access: ReadOnly, Count: 1},
		{Address: AddrDCLoadVoltage, Name: "DC Load Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrDCLoadCurrent, Name: "DC Load Current", Scale: Mul001, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrDCLoadPower, Name: "DC Load Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPV1Voltage, Name: "PV1 Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPV1Current, Name: "PV1 Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPV1Power, Name: "PV1 Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrDCLoadOnOff, Name: "DC Load On/Off", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrChargeStatus, Name: "Charge Status", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrFaultAlarmBits, Name: "Fault/Alarm Bits", Unit: "", Type: U32, Access: ReadOnly, Count: 2},
		{Address: AddrTotalChargePower, Name: "Total Charge Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPV2Voltage, Name: "PV2 Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPV2Current, Name: "PV2 Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPV2Power, Name: "PV2 Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		// BMS data (V2.08+)
		{Address: AddrBMSBatteryVoltage, Name: "BMS Battery Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrBMSBatteryCurrent, Name: "BMS Battery Current", Scale: Mul01, Unit: "A", Type: S16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrBMSBatteryTemperature, Name: "BMS Battery Temperature", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrBMSChargeLimitVoltage, Name: "BMS Charge Limit Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrBMSDischargeLimitVolt, Name: "BMS Discharge Limit Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrBMSChargeLimitCurrent, Name: "BMS Charge Limit Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrBMSAlarmBits, Name: "BMS Alarm Bits", Unit: "", Type: U32, Access: ReadOnly, Count: 2, Optional: true},
		{Address: AddrBMSProtectBits, Name: "BMS Protect Bits", Unit: "", Type: U32, Access: ReadOnly, Count: 2, Optional: true},
		{Address: AddrBattery2Voltage, Name: "Battery 2 Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrBattery2Current, Name: "Battery 2 Current", Scale: Mul01, Unit: "A", Type: S16, Access: ReadOnly, Count: 1, Optional: true},
	},
}

// InverterData — P02: Inverter/grid/load realtime data (0x0200-0x0239)
var InverterData = Group{
	Name: "Inverter Data",
	Registers: []Register{
		{Address: AddrFaultBits, Name: "Fault Bits", Unit: "", Type: U32, Access: ReadOnly, Count: 2},
		{Address: AddrFaultBitsExt, Name: "Fault Bits (ext)", Unit: "", Type: U32, Access: ReadOnly, Count: 2},
		{Address: AddrFaultCode1, Name: "Fault Code 1", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrFaultCode2, Name: "Fault Code 2", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrFaultCode3, Name: "Fault Code 3", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrFaultCode4, Name: "Fault Code 4", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrCurrentTime, Name: "Current Time", Unit: "", Type: PackedTime, Access: ReadWrite, Count: 3},
		{Address: AddrMachineState, Name: "Machine State", Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPasswordStatus, Name: "Password Status", Unit: "", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrBusVoltage, Name: "Bus Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrGridVoltageL1, Name: "Grid Voltage L1", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrGridCurrentL1, Name: "Grid Current L1", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrGridFrequency, Name: "Grid Frequency", Scale: Mul001, Unit: "Hz", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrInverterVoltageL1, Name: "Inverter Voltage L1", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrInverterCurrentL1, Name: "Inverter Current L1", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrInverterFrequency, Name: "Inverter Frequency", Scale: Mul001, Unit: "Hz", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadCurrentL1, Name: "Load Current L1", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadPowerFactor, Name: "Load Power Factor", Scale: Mul001, Unit: "", Type: S16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadPowerL1, Name: "Load Power L1", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadApparentPowerL1, Name: "Load Apparent Power L1", Scale: Mul1, Unit: "VA", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrInverterDCComponent, Name: "Inverter DC Component", Scale: Mul1, Unit: "mV", Type: S16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrMainsChargeCurrent, Name: "Mains Charge Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadRatioL1, Name: "Load Ratio L1", Scale: Mul1, Unit: "%", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrHeatsinkATemp, Name: "Heatsink A Temp (DC-DC)", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1},
		{Address: AddrHeatsinkBTemp, Name: "Heatsink B Temp (DC-AC)", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1},
		{Address: AddrHeatsinkCTemp, Name: "Heatsink C Temp (Transformer)", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1},
		{Address: AddrHeatsinkDTemp, Name: "Heatsink D Temp (Ambient)", Scale: Mul01, Unit: "°C", Type: S16, Access: ReadOnly, Count: 1},
		{Address: AddrPVChargeCurrentBatt, Name: "PV Charge Current (Batt Side)", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1},
		// L2/L3 — split-phase and 3-phase models
		{Address: AddrGridVoltageL2, Name: "Grid Voltage L2", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrGridCurrentL2, Name: "Grid Current L2", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrInverterVoltageL2, Name: "Inverter Voltage L2", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrInverterCurrentL2, Name: "Inverter Current L2", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrLoadCurrentL2, Name: "Load Current L2", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrLoadPowerL2, Name: "Load Power L2", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrLoadApparentPowerL2, Name: "Load Apparent Power L2", Scale: Mul1, Unit: "VA", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrLoadRatioL2, Name: "Load Ratio L2", Scale: Mul1, Unit: "%", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrGridVoltageL3, Name: "Grid Voltage L3", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrGridCurrentL3, Name: "Grid Current L3", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrInverterVoltageL3, Name: "Inverter Voltage L3", Scale: Mul01, Unit: "V", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrInverterCurrentL3, Name: "Inverter Current L3", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrLoadCurrentL3, Name: "Load Current L3", Scale: Mul01, Unit: "A", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrLoadPowerL3, Name: "Load Power L3", Scale: Mul1, Unit: "W", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrLoadApparentPowerL3, Name: "Load Apparent Power L3", Scale: Mul1, Unit: "VA", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrLoadRatioL3, Name: "Load Ratio L3", Scale: Mul1, Unit: "%", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
	},
}

// BatterySettings — P05: Battery parameter settings (0xE001-0xE039)
var BatterySettings = Group{
	Name: "Battery Settings",
	Registers: []Register{
		{Address: AddrPVChargeCurrentLimit, Name: "PV Charge Current Limit", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrNominalBatteryCapAH, Name: "Nominal Battery Capacity", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrSystemVoltage, Name: "System Voltage", Scale: Mul1, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrBatteryType, Name: "Battery Type", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrOverVoltageProtection, Name: "Over Voltage Protection", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrLimitedChargeVoltage, Name: "Limited Charge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrEqualizingChargeVolt, Name: "Equalizing Charge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrBoostChargeVoltage, Name: "Boost Charge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrFloatChargeVoltage, Name: "Float Charge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrBoostReturnVoltage, Name: "Boost Return Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrOverDischargeReturnV, Name: "Over Discharge Return Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrUnderVoltageWarning, Name: "Under Voltage Warning", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrOverDischargeVoltage, Name: "Over Discharge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrLimitedDischargeVolt, Name: "Limited Discharge Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrCutoffSOC, Name: "Cutoff SOC (Charge/Discharge)", Unit: "%", Type: PackedHiLo, Access: ReadWrite, Count: 1},
		{Address: AddrOverDischargeDelay, Name: "Over Discharge Delay", Scale: Mul1, Unit: "s", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrEqualizingChargeTime, Name: "Equalizing Charge Time", Scale: Mul1, Unit: "min", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrBoostChargeTime, Name: "Boost Charge Time", Scale: Mul1, Unit: "min", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrEqualizingChargeIntv, Name: "Equalizing Charge Interval", Scale: Mul1, Unit: "days", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrTempCompensationCoeff, Name: "Temp Compensation Coeff", Scale: Mul1, Unit: "mV/°C/2V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrChargeMaxTemp, Name: "Charge Max Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrChargeMinTemp, Name: "Charge Min Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischargeMaxTemp, Name: "Discharge Max Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischargeMinTemp, Name: "Discharge Min Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrBatteryHeaterStart, Name: "Battery Heater Start Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrBatteryHeaterStop, Name: "Battery Heater Stop Temp", Scale: Mul1, Unit: "°C", Type: S16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrMainsSwitchingVoltage, Name: "Mains Switching Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrStopChargeCurrentLi, Name: "Stop Charge Current (Li)", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrStopChargeSOC, Name: "Stop Charge SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrLowSOCAlarm, Name: "Low SOC Alarm", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrSOCSwitchToMains, Name: "SOC Switch To Mains", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrSOCSwitchToBattery, Name: "SOC Switch To Battery", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrInverterSwitchingVolt, Name: "Inverter Switching Voltage", Scale: Voltage12V, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrEqualizingChargeTO, Name: "Equalizing Charge Timeout", Scale: Mul1, Unit: "min", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrLiActivationCurrent, Name: "Li Activation Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrBMSChargeLCMode, Name: "BMS Charge LC Mode", Scale: Mul1, Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		// Extended settings (may not exist on all models)
		{Address: AddrGridConnectedEnable, Name: "Grid-Connected Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrGFCIEnable, Name: "GFCI Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrPVPowerPriority, Name: "PV Power Priority", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
	},
}

// TimedChargeDischarge — P05 cont: Sectional timed charge/discharge (0xE026-0xE04C)
var TimedChargeDischarge = Group{
	Name: "Timed Charge/Discharge",
	Registers: []Register{
		// Charge time sections
		{Address: AddrChargeStartTime1, Name: "Charge Start Time 1", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrChargeEndTime1, Name: "Charge End Time 1", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrChargeStartTime2, Name: "Charge Start Time 2", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrChargeEndTime2, Name: "Charge End Time 2", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrChargeStartTime3, Name: "Charge Start Time 3", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrChargeEndTime3, Name: "Charge End Time 3", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrTimedChargeEnable, Name: "Timed Charge Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		// Discharge time sections
		{Address: AddrDischargeStartTime1, Name: "Discharge Start Time 1", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrDischargeEndTime1, Name: "Discharge End Time 1", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrDischargeStartTime2, Name: "Discharge Start Time 2", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrDischargeEndTime2, Name: "Discharge End Time 2", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrDischargeStartTime3, Name: "Discharge Start Time 3", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrDischargeEndTime3, Name: "Discharge End Time 3", Unit: "", Type: PackedHourMin, Access: ReadWrite, Count: 1},
		{Address: AddrTimedDischargeEnable, Name: "Timed Discharge Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		// Per-section SOC cutoffs (ASF V1.94+, may not exist on all models/firmware)
		{Address: AddrCharge1StopSOC, Name: "Charge 1 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrCharge2StopSOC, Name: "Charge 2 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrCharge3StopSOC, Name: "Charge 3 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischarge1StopSOC, Name: "Discharge 1 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischarge2StopSOC, Name: "Discharge 2 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischarge3StopSOC, Name: "Discharge 3 Stop SOC", Scale: Mul1, Unit: "%", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		// Per-section voltage cutoffs + discharge power (ASF V1.95+)
		{Address: AddrCharge1StopVoltage, Name: "Charge 1 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrCharge2StopVoltage, Name: "Charge 2 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrCharge3StopVoltage, Name: "Charge 3 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischarge1StopVolt, Name: "Discharge 1 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischarge2StopVolt, Name: "Discharge 2 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischarge3StopVolt, Name: "Discharge 3 Stop Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischarge1MaxPower, Name: "Discharge 1 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischarge2MaxPower, Name: "Discharge 2 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDischarge3MaxPower, Name: "Discharge 3 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		// Charge power limits + source (ASF V1.96+)
		{Address: AddrCharge1MaxPower, Name: "Charge 1 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrCharge2MaxPower, Name: "Charge 2 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrCharge3MaxPower, Name: "Charge 3 Max Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
	},
}

// InverterSettings — P07: Inverter user settings (0xE200-0xE221)
var InverterSettings = Group{
	Name: "Inverter Settings",
	Registers: []Register{
		{Address: AddrInvRS485Address, Name: "RS485 Address", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrParallelMode, Name: "Parallel Mode", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrOutputPriority, Name: "Output Priority", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrMainsChargeCurrentLim, Name: "Mains Charge Current Limit", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrEqualizingChargeEn, Name: "Equalizing Charge Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrNGFunctionEnable, Name: "N-G Function Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrOutputVoltage, Name: "Output Voltage", Scale: Mul01, Unit: "V", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrOutputFrequency, Name: "Output Frequency", Scale: Mul001, Unit: "Hz", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrMaxChargeCurrent, Name: "Max Charge Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrACInputRange, Name: "AC Input Range", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrPowerSavingMode, Name: "Power Saving Mode", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrOverloadAutoRestart, Name: "Overload Auto Restart", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrOverTempAutoRestart, Name: "Over Temp Auto Restart", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrChargerPriority, Name: "Charger Priority", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrAlarmEnable, Name: "Alarm Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrAlarmOnInputLoss, Name: "Alarm On Input Loss", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrOverloadBypassEnable, Name: "Overload Bypass Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrRecordFaultEnable, Name: "Record Fault Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrBMSErrorStopEnable, Name: "BMS Error Stop Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrBMSCommunicationEn, Name: "BMS Communication Enable", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		{Address: AddrDCLoadSwitch, Name: "DC Load Switch", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrDeratePower, Name: "Derate Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrBMSProtocol, Name: "BMS Protocol", Unit: "", Type: U16, Access: ReadWrite, Count: 1},
		// Extended inverter settings (may not exist on all models)
		{Address: AddrMaxLineCurrent, Name: "Max Line Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrMaxLinePower, Name: "Max Line Power", Scale: Mul10, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrOutputPhase, Name: "Output Phase", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrGeneratorWorkMode, Name: "Generator Work Mode", Unit: "", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrGenChargeMaxCurrent, Name: "Generator Charge Max Current", Scale: Mul01, Unit: "A", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
		{Address: AddrGeneratorRatedPower, Name: "Generator Rated Power", Scale: Mul1, Unit: "W", Type: U16, Access: ReadWrite, Count: 1, Optional: true},
	},
}

// Statistics — P08: Power statistics (0xF000-0xF04B)
var Statistics = Group{
	Name: "Statistics",
	Registers: []Register{
		// Last 7 days history (index 0 = today-6, index 6 = yesterday)
		{Address: AddrPVGenDay7, Name: "PV Gen Day-7", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPVGenDay6, Name: "PV Gen Day-6", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPVGenDay5, Name: "PV Gen Day-5", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPVGenDay4, Name: "PV Gen Day-4", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPVGenDay3, Name: "PV Gen Day-3", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPVGenDay2, Name: "PV Gen Day-2", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPVGenDay1, Name: "PV Gen Day-1", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattChargeDay7, Name: "Batt Charge Day-7", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattChargeDay6, Name: "Batt Charge Day-6", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattChargeDay5, Name: "Batt Charge Day-5", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattChargeDay4, Name: "Batt Charge Day-4", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattChargeDay3, Name: "Batt Charge Day-3", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattChargeDay2, Name: "Batt Charge Day-2", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattChargeDay1, Name: "Batt Charge Day-1", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattDischargeDay7, Name: "Batt Discharge Day-7", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattDischargeDay6, Name: "Batt Discharge Day-6", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattDischargeDay5, Name: "Batt Discharge Day-5", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattDischargeDay4, Name: "Batt Discharge Day-4", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattDischargeDay3, Name: "Batt Discharge Day-3", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattDischargeDay2, Name: "Batt Discharge Day-2", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBattDischargeDay1, Name: "Batt Discharge Day-1", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsChargeDay7, Name: "Mains Charge Day-7", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsChargeDay6, Name: "Mains Charge Day-6", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsChargeDay5, Name: "Mains Charge Day-5", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsChargeDay4, Name: "Mains Charge Day-4", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsChargeDay3, Name: "Mains Charge Day-3", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsChargeDay2, Name: "Mains Charge Day-2", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsChargeDay1, Name: "Mains Charge Day-1", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadDay7, Name: "Load Day-7", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadDay6, Name: "Load Day-6", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadDay5, Name: "Load Day-5", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadDay4, Name: "Load Day-4", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadDay3, Name: "Load Day-3", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadDay2, Name: "Load Day-2", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadDay1, Name: "Load Day-1", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsLoadDay7, Name: "Mains Load Day-7", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsLoadDay6, Name: "Mains Load Day-6", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsLoadDay5, Name: "Mains Load Day-5", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsLoadDay4, Name: "Mains Load Day-4", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsLoadDay3, Name: "Mains Load Day-3", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsLoadDay2, Name: "Mains Load Day-2", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrMainsLoadDay1, Name: "Mains Load Day-1", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		// Today / accumulated
		{Address: AddrEnergyExportToday, Name: "Energy Export Today", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrBatteryChargeToday, Name: "Battery Charge Today", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrBatteryDischargeToday, Name: "Battery Discharge Today", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrPVGenerationToday, Name: "PV Generation Today", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrLoadConsumptionToday, Name: "Load Consumption Today", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrTotalRunningDays, Name: "Total Running Days", Scale: Mul1, Unit: "days", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrTotalOverdischargeCount, Name: "Total Overdischarge Count", Scale: Mul1, Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrTotalFullChargeCount, Name: "Total Full Charge Count", Scale: Mul1, Unit: "", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrAccumBatteryCharge, Name: "Accumulated Battery Charge", Scale: Mul1, Unit: "AH", Type: U32, Access: ReadOnly, Count: 2},
		{Address: AddrAccumBatteryDischarge, Name: "Accumulated Battery Discharge", Scale: Mul1, Unit: "AH", Type: U32, Access: ReadOnly, Count: 2},
		{Address: AddrAccumPVGeneration, Name: "Accumulated PV Generation", Scale: Mul01, Unit: "kWh", Type: U32, Access: ReadOnly, Count: 2},
		{Address: AddrAccumLoadConsumption, Name: "Accumulated Load Consumption", Scale: Mul01, Unit: "kWh", Type: U32, Access: ReadOnly, Count: 2},
		{Address: AddrGridChargeToday, Name: "Grid Charge Today", Scale: Mul1, Unit: "AH", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrEnergyImportToday, Name: "Energy Import Today", Scale: Mul01, Unit: "kWh", Type: U16, Access: ReadOnly, Count: 1, Optional: true},
		{Address: AddrAccumMainsCharge, Name: "Accumulated Mains Charge", Scale: Mul01, Unit: "kWh", Type: U32, Access: ReadOnly, Count: 2},
		{Address: AddrAccumInverterHours, Name: "Accumulated Inverter Hours", Scale: Mul1, Unit: "h", Type: U16, Access: ReadOnly, Count: 1},
		{Address: AddrAccumBypassHours, Name: "Accumulated Bypass Hours", Scale: Mul1, Unit: "h", Type: U16, Access: ReadOnly, Count: 1},
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
