package register

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
)

// Access defines read/write capability.
type Access int

const (
	ReadOnly Access = iota
	ReadWrite
	WriteOnly
)

// DataType defines how to interpret register values.
type DataType int

const (
	U16 DataType = iota
	S16
	U32 // two registers, low word at lower address
	ASCII
	PackedTemp    // high byte = temp1, low byte = temp2
	PackedTime    // packed year/month/day/hour/min/sec
	PackedHourMin // high byte = hours, low byte = minutes
	PackedHiLo    // high byte = value1, low byte = value2 (e.g., charge/discharge SOC)
)

// ScaleFunc converts a raw register value to a scaled float.
// The lookup parameter allows reading other registers on demand
// (e.g., system voltage for 12V-base scaling). Results are cached
// per session so repeated calls are cheap.
type ScaleFunc func(raw float64, lookup modbus.Lookup) float64

// Register describes a single MODBUS register.
type Register struct {
	Address  uint16
	Name     string
	Scale    ScaleFunc
	Unit     string
	Type     DataType
	Access   Access
	Count    uint16 // number of 16-bit registers (1 for U16/S16, 2 for U32, N for ASCII)
	Optional bool   // if true, read errors are silently ignored (register may not exist on all models/firmware)
}

// Group is a named collection of registers.
type Group struct {
	Name      string
	Registers []Register
}

// Common scale functions.
var (
	Mul1   ScaleFunc = func(raw float64, _ modbus.Lookup) float64 { return raw }
	Mul01  ScaleFunc = func(raw float64, _ modbus.Lookup) float64 { return raw * 0.1 }
	Mul001 ScaleFunc = func(raw float64, _ modbus.Lookup) float64 { return raw * 0.01 }
	Mul10  ScaleFunc = func(raw float64, _ modbus.Lookup) float64 { return raw * 10 }

	// Voltage12V scales a 12V-base voltage register by systemVoltage/12.
	// Reads register 0xE003 (system voltage) via the session cache.
	Voltage12V ScaleFunc = func(raw float64, lookup modbus.Lookup) float64 {
		v := raw * 0.1
		if lookup != nil {
			if sysV, err := lookup(0xE003); err == nil && sysV > 0 {
				v *= float64(sysV) / 12.0
			}
		}
		return v
	}
)

// FormatValue converts a raw register value to a human-readable string.
func FormatValue(reg Register, values []uint16, lookup modbus.Lookup) string {
	if len(values) == 0 {
		return "<no data>"
	}
	switch reg.Type {
	case S16:
		return formatScaled(float64(int16(values[0])), reg, lookup)
	case U32:
		if len(values) < 2 {
			return "<incomplete>"
		}
		raw := uint32(values[0]) | (uint32(values[1]) << 16)
		return formatScaled(float64(raw), reg, lookup)
	case ASCII:
		return decodeASCII(values)
	case PackedTemp:
		hi := int8(values[0] >> 8)
		lo := int8(values[0] & 0xFF)
		return fmt.Sprintf("%d°C / %d°C", hi, lo)
	case PackedTime:
		return decodePackedTime(values)
	case PackedHourMin:
		return fmt.Sprintf("%02d:%02d", values[0]>>8, values[0]&0xFF)
	case PackedHiLo:
		return fmt.Sprintf("%d / %d %s", values[0]>>8, values[0]&0xFF, reg.Unit)
	default: // U16
		return formatScaled(float64(values[0]), reg, lookup)
	}
}

func formatScaled(raw float64, reg Register, lookup modbus.Lookup) string {
	scale := reg.Scale
	if scale == nil {
		return fmt.Sprintf("%d %s", int64(raw), reg.Unit)
	}
	value := scale(raw, lookup)
	if value == raw {
		return fmt.Sprintf("%d %s", int64(raw), reg.Unit)
	}
	return fmt.Sprintf("%.1f %s", value, reg.Unit)
}

func decodeASCII(values []uint16) string {
	buf := make([]byte, len(values)*2)
	for i, v := range values {
		binary.BigEndian.PutUint16(buf[i*2:], v)
	}
	return strings.TrimRight(string(buf), "\x00 ")
}

func decodePackedTime(values []uint16) string {
	if len(values) < 3 {
		return "<incomplete>"
	}
	year := 2000 + int(values[0]>>8)
	month := values[0] & 0xFF
	day := values[1] >> 8
	hour := values[1] & 0xFF
	minute := values[2] >> 8
	sec := values[2] & 0xFF
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d", year, month, day, hour, minute, sec)
}

// RegCount returns the number of 16-bit registers this register occupies.
func (r Register) RegCount() uint16 {
	if r.Count > 0 {
		return r.Count
	}
	switch r.Type {
	case U32:
		return 2
	default:
		return 1
	}
}
