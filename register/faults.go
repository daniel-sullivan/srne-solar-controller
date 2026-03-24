package register

import (
	"fmt"
	"strings"
)

const (
	FaultHistoryBase  = 0xF800
	FaultRecordSize   = 16 // registers per record
	FaultRecordCount  = 16 // number of fault records
	StatusHistoryBase = 0xF900
)

// FaultRecord represents a single fault history entry.
type FaultRecord struct {
	Index          int
	FaultCode      uint16
	Timestamp      string // formatted date/time
	BatteryVoltage float64
	InverterVoltL1 float64
	InverterVoltL2 float64
	BusVoltage     float64
	Frequency      float64
	RawRegisters   [FaultRecordSize]uint16
}

// ParseFaultRecord interprets a 16-register fault history record.
func ParseFaultRecord(index int, regs []uint16) FaultRecord {
	if len(regs) < FaultRecordSize {
		return FaultRecord{Index: index}
	}

	r := FaultRecord{
		Index:     index,
		FaultCode: regs[0],
	}
	copy(r.RawRegisters[:], regs)

	// Timestamp at [+1..+3] — packed date/time
	// [+1] = year*256 + month, [+2] = day*256 + hour, [+3] = minute*256 + second (inferred)
	year := 2000 + int(regs[1]>>8)
	month := regs[1] & 0xFF
	day := regs[2] >> 8
	hour := regs[2] & 0xFF
	min := regs[3] >> 8
	sec := regs[3] & 0xFF
	r.Timestamp = fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d", year, month, day, hour, min, sec)

	// System snapshot (inferred from probing patterns)
	// [+9] = battery voltage ×0.1
	r.BatteryVoltage = float64(regs[9]) * 0.1
	// [+12] = inverter voltage L1 ×0.1
	r.InverterVoltL1 = float64(regs[12]) * 0.1
	// [+13] = inverter voltage L2 ×0.1
	r.InverterVoltL2 = float64(regs[13]) * 0.1
	// [+14] = bus voltage ×0.1
	r.BusVoltage = float64(regs[14]) * 0.1
	// [+15] = frequency ×0.01
	r.Frequency = float64(regs[15]) * 0.01

	return r
}

// IsEmpty returns true if this record has no fault logged.
func (r FaultRecord) IsEmpty() bool {
	return r.FaultCode == 0 && r.RawRegisters[1] == 0
}

// Fault code table.
// Source: SRNE ASP 8-10kW User Manual V1.3 (20250514)
// https://www.srnesolar.com/userfiles/files/2025/11/28/ASP%20_8-10kW_U_All-in-one%20solar%20charge%20inverter_V1.3[20250514].pdf
// Cross-referenced with HESP V2.3 manual and phinix-org fault code CSV.
// Note: codes 27, 33, 36, 46-55, 59 are not documented.
var faultCodes = map[uint16]string{
	1:  "BatVoltLow",
	2:  "BatOverCurrSw",
	3:  "BatOpen",
	4:  "BatLowEod",
	5:  "BatOverCurrHw",
	6:  "BatOverVolt",
	7:  "BusOverVoltHw",
	8:  "BusOverVoltSw",
	9:  "PvVoltHigh",
	10: "PvBuckOCHw",
	11: "PvBuckOCSw",
	12: "SpiCommErr",
	13: "OverloadBypass",
	14: "OverloadInverter",
	15: "AcOverCurrHw",
	16: "AuxDSpReqOffPWM",
	17: "InvShort",
	18: "BusSoftFailed",
	19: "OverTemperMppt",
	20: "OverTemperInv",
	21: "FanFail",
	22: "EEPROM",
	23: "ModelNumErr",
	24: "BusDiff",
	25: "BusShort",
	26: "RlyShort",
	28: "LinePhaseErr",
	29: "BusVoltLow",
	30: "BatCapacityLow1",
	31: "BatCapacityLow2",
	32: "BatCapacityLowStop",
	34: "CanCommFault",
	35: "ParaAddrErr",
	37: "ParaShareCurrErr",
	38: "ParaBattVoltDiff",
	39: "ParaAcSrcDiff",
	40: "ParaHwSynErr",
	41: "InvDcVoltErr",
	42: "SysFwVersionDiff",
	43: "ParaLineContErr",
	44: "SerialNumErr",
	45: "SplitPhaseErr",
	49: "GridOverVolt",
	50: "GridUnderVolt",
	51: "GridOverFreq",
	52: "GridUnderFreq",
	53: "GridLoss",
	54: "GridDcCurrOver",
	55: "GridStdUnInit",
	56: "LowInsulationRes",
	57: "LeakageCurrOver",
	58: "BMSComErr",
	59: "BMSErr",
	60: "BMSUnderTemp",
	61: "BMSOverTemp",
	62: "BMSOverCurr",
	63: "BMSUnderVolt",
	64: "BMSOverVolt",
}

// FaultCodeName returns the human-readable name for a fault code.
func FaultCodeName(code uint16) string {
	if name, ok := faultCodes[code]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", code)
}

// FormatFaultRecord returns a human-readable string for a fault record.
func FormatFaultRecord(r FaultRecord) string {
	if r.IsEmpty() {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "  #%-2d  %-20s  %s", r.Index+1, FaultCodeName(r.FaultCode), r.Timestamp)
	fmt.Fprintf(&b, "  Batt: %.1fV  L1: %.1fV  L2: %.1fV  Bus: %.1fV  Freq: %.2fHz",
		r.BatteryVoltage, r.InverterVoltL1, r.InverterVoltL2,
		r.BusVoltage, r.Frequency)
	return b.String()
}
