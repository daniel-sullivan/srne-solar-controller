package register

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
)

func stubLookup(regs map[uint16]uint16) modbus.Lookup {
	return func(addr uint16) (uint16, error) {
		v, ok := regs[addr]
		if !ok {
			return 0, &modbus.ModbusError{FunctionCode: 0x03, ExceptionCode: 0x02}
		}
		return v, nil
	}
}

func TestFormatValueU16(t *testing.T) {
	reg := Register{Type: U16, Scale: Mul01, Unit: "V"}
	assert.Equal(t, "53.2 V", FormatValue(reg, []uint16{532}, nil))
}

func TestFormatValueU16_Identity(t *testing.T) {
	reg := Register{Type: U16, Scale: Mul1, Unit: "%"}
	assert.Equal(t, "85 %", FormatValue(reg, []uint16{85}, nil))
}

func TestFormatValueU16_NoScale(t *testing.T) {
	reg := Register{Type: U16, Unit: ""}
	assert.Equal(t, "42 ", FormatValue(reg, []uint16{42}, nil))
}

func TestFormatValueS16(t *testing.T) {
	reg := Register{Type: S16, Scale: Mul01, Unit: "A"}
	assert.Equal(t, "-10.0 A", FormatValue(reg, []uint16{0xFF9C}, nil))
}

func TestFormatValueU32(t *testing.T) {
	reg := Register{Type: U32, Scale: Mul1, Unit: "Wh", Count: 2}
	assert.Equal(t, "70196 Wh", FormatValue(reg, []uint16{0x1234, 0x0001}, nil))
}

func TestFormatValueU32_Incomplete(t *testing.T) {
	reg := Register{Type: U32, Count: 2}
	assert.Equal(t, "<incomplete>", FormatValue(reg, []uint16{0x1234}, nil))
}

func TestFormatValueASCII(t *testing.T) {
	reg := Register{Type: ASCII, Count: 2}
	assert.Equal(t, "ABCD", FormatValue(reg, []uint16{0x4142, 0x4344}, nil))
}

func TestFormatValueASCII_NullPadding(t *testing.T) {
	reg := Register{Type: ASCII, Count: 2}
	assert.Equal(t, "AB", FormatValue(reg, []uint16{0x4142, 0x0000}, nil))
}

func TestFormatValuePackedTemp(t *testing.T) {
	reg := Register{Type: PackedTemp}
	assert.Equal(t, "30°C / 25°C", FormatValue(reg, []uint16{0x1E19}, nil))
}

func TestFormatValuePackedTemp_Negative(t *testing.T) {
	reg := Register{Type: PackedTemp}
	assert.Equal(t, "-5°C / 10°C", FormatValue(reg, []uint16{0xFB0A}, nil))
}

func TestFormatValuePackedTime(t *testing.T) {
	reg := Register{Type: PackedTime, Count: 3}
	assert.Equal(t, "2025-03-15 14:30:00", FormatValue(reg, []uint16{0x1903, 0x0F0E, 0x1E00}, nil))
}

func TestFormatValuePackedTime_Incomplete(t *testing.T) {
	reg := Register{Type: PackedTime, Count: 3}
	assert.Equal(t, "<incomplete>", FormatValue(reg, []uint16{0x1903}, nil))
}

func TestFormatValuePackedHourMin(t *testing.T) {
	reg := Register{Type: PackedHourMin}
	assert.Equal(t, "14:30", FormatValue(reg, []uint16{0x0E1E}, nil))
}

func TestFormatValuePackedHiLo(t *testing.T) {
	reg := Register{Type: PackedHiLo, Unit: "%"}
	assert.Equal(t, "90 / 20 %", FormatValue(reg, []uint16{0x5A14}, nil))
}

func TestFormatValueNoData(t *testing.T) {
	reg := Register{Type: U16}
	assert.Equal(t, "<no data>", FormatValue(reg, nil, nil))
}

func TestVoltage12VScaling(t *testing.T) {
	lookup := stubLookup(map[uint16]uint16{AddrSystemVoltage: 48})
	reg := Register{Type: U16, Scale: Voltage12V, Unit: "V"}
	assert.Equal(t, "216.0 V", FormatValue(reg, []uint16{540}, lookup))
}

func TestVoltage12VScaling_NoLookup(t *testing.T) {
	reg := Register{Type: U16, Scale: Voltage12V, Unit: "V"}
	assert.Equal(t, "54.0 V", FormatValue(reg, []uint16{540}, nil))
}

func TestMul001(t *testing.T) {
	reg := Register{Type: U16, Scale: Mul001, Unit: "A"}
	assert.Equal(t, "12.3 A", FormatValue(reg, []uint16{1234}, nil))
}

func TestMul10(t *testing.T) {
	reg := Register{Type: U16, Scale: Mul10, Unit: "Wh"}
	assert.Equal(t, "1000.0 Wh", FormatValue(reg, []uint16{100}, nil))
}

func TestRegCount(t *testing.T) {
	tests := []struct {
		name     string
		reg      Register
		expected uint16
	}{
		{"default U16", Register{Type: U16}, 1},
		{"default S16", Register{Type: S16}, 1},
		{"default U32", Register{Type: U32}, 2},
		{"explicit count", Register{Type: ASCII, Count: 8}, 8},
		{"explicit count overrides", Register{Type: U32, Count: 4}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.reg.RegCount())
		})
	}
}

func TestParseFaultRecord(t *testing.T) {
	regs := [16]uint16{}
	regs[0] = 14 // OverloadInverter
	regs[1] = 0x1903
	regs[2] = 0x0F0E
	regs[3] = 0x1E00
	regs[9] = 532
	regs[12] = 1200
	regs[13] = 1201
	regs[14] = 3800
	regs[15] = 6000

	r := ParseFaultRecord(0, regs[:])

	assert.Equal(t, uint16(14), r.FaultCode)
	assert.Equal(t, "2025-03-15 14:30:00", r.Timestamp)
	assert.InDelta(t, 53.2, r.BatteryVoltage, 0.01)
	assert.InDelta(t, 60.00, r.Frequency, 0.01)
	assert.False(t, r.IsEmpty())
}

func TestParseFaultRecord_Empty(t *testing.T) {
	regs := [16]uint16{}
	assert.True(t, ParseFaultRecord(0, regs[:]).IsEmpty())
}

func TestFaultCodeName(t *testing.T) {
	assert.Equal(t, "OverloadInverter", FaultCodeName(14))
	assert.True(t, strings.HasPrefix(FaultCodeName(999), "Unknown"))
}

func TestFormatFaultRecord_Empty(t *testing.T) {
	assert.Empty(t, FormatFaultRecord(FaultRecord{}))
}

func TestFormatFaultRecord_NonEmpty(t *testing.T) {
	r := FaultRecord{
		Index: 0, FaultCode: 14, Timestamp: "2025-03-15 14:30:00",
		BatteryVoltage: 53.2, InverterVoltL1: 120.0, InverterVoltL2: 120.1,
		BusVoltage: 380.0, Frequency: 60.00,
	}
	got := FormatFaultRecord(r)
	require.Contains(t, got, "OverloadInverter")
	assert.Contains(t, got, "53.2V")
}
