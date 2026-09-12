package interpack

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const FrameSize = 126

var (
	ErrFrameSize = errors.New("interpack: frame must be 126 bytes")
	ErrHeader    = errors.New("interpack: invalid telemetry header")
	ErrCRC       = errors.New("interpack: invalid frame CRC")
	ErrCells     = errors.New("interpack: invalid cell voltage")
)

var telemetryHeader = [9]byte{0x45, 0, 0, 0, 0x54, 0, 0x74, 0, 0x10}

// ProtectionInfo captures decoded 0x45 protection and alarm bitfields.
// Bits verified against JBD UP16S register specifications are exposed directly.
// UnresolvedBits is set to true to explicitly identify that remaining bitfields
// across firmware variants remain unverified.
type ProtectionInfo struct {
	FaultMask      uint32 `json:"fault_mask"`
	MOSState       uint16 `json:"mos_state"`
	WarningMask    uint16 `json:"warning_mask"`
	CellOVP        bool   `json:"cell_ovp"`
	CellUVP        bool   `json:"cell_uvp"`
	PackOVP        bool   `json:"pack_ovp"`
	PackUVP        bool   `json:"pack_uvp"`
	AlarmsClear    bool   `json:"alarms_clear"`
	UnresolvedBits bool   `json:"unresolved_bits"`
}

// Frame contains one CRC-checked pack report. Fields whose meaning or units have
// not been verified are retained as raw data.
type Frame struct {
	Address                  uint8
	CellMillivolts           [16]uint16
	Summary1Millivolts       uint16
	Summary2Millivolts       uint16
	PackVoltageCentivolts    uint16
	CurrentRaw               [4]byte
	TemperaturesDecikelvin   [2]uint16
	FullCapacityCentiAh      uint16
	RemainingCapacityCentiAh uint16
	CycleCount               uint16
	SOCPercent               uint16
	SOHPercent               uint16
	StatusRaw                [8]byte
	Protection               ProtectionInfo
	BatchRaw                 [12]byte
	ReservedRaw              [18]byte
	Unknown104               uint16
	Unknown106               uint16
	Unknown108               uint16
	Temperatures2Decikelvin  [2]uint16
	Unknown114               uint16
	Temperatures4Decikelvin  [4]uint16
}

// ParseFrame validates and decodes a single JBD UP16S inter-pack telemetry reply.
func ParseFrame(data []byte) (Frame, error) {
	if len(data) != FrameSize {
		return Frame{}, ErrFrameSize
	}
	if !hasTelemetryHeader(data) {
		return Frame{}, ErrHeader
	}
	got := binary.LittleEndian.Uint16(data[124:126])
	if want := CRC16(data[:124]); got != want {
		return Frame{}, fmt.Errorf("%w: got 0x%04x, want 0x%04x", ErrCRC, got, want)
	}

	frame := Frame{Address: data[0]}
	for i := range frame.CellMillivolts {
		mv := binary.BigEndian.Uint16(data[10+2*i : 12+2*i])
		if mv < 1000 || mv > 5000 {
			return Frame{}, fmt.Errorf("%w: cell %d = %d mV", ErrCells, i+1, mv)
		}
		frame.CellMillivolts[i] = mv
	}
	frame.Summary1Millivolts = binary.BigEndian.Uint16(data[42:44])
	frame.Summary2Millivolts = binary.BigEndian.Uint16(data[44:46])
	frame.PackVoltageCentivolts = binary.BigEndian.Uint16(data[46:48])
	copy(frame.CurrentRaw[:], data[48:52])
	for i := range frame.TemperaturesDecikelvin {
		frame.TemperaturesDecikelvin[i] = binary.BigEndian.Uint16(data[52+2*i : 54+2*i])
	}
	frame.FullCapacityCentiAh = binary.BigEndian.Uint16(data[56:58])
	frame.RemainingCapacityCentiAh = binary.BigEndian.Uint16(data[58:60])
	frame.CycleCount = binary.BigEndian.Uint16(data[60:62])
	frame.SOCPercent = binary.BigEndian.Uint16(data[62:64])
	frame.SOHPercent = binary.BigEndian.Uint16(data[64:66])

	// Status/Protection: bytes 66-73 (8 bytes total)
	copy(frame.StatusRaw[:], data[66:74])
	faultMask := binary.BigEndian.Uint32(data[66:70])
	mosState := binary.BigEndian.Uint16(data[70:72])
	warningMask := binary.BigEndian.Uint16(data[72:74])
	frame.Protection = ProtectionInfo{
		FaultMask:      faultMask,
		MOSState:       mosState,
		WarningMask:    warningMask,
		CellOVP:        faultMask&1 != 0,
		CellUVP:        faultMask&2 != 0,
		PackOVP:        faultMask&4 != 0,
		PackUVP:        faultMask&8 != 0,
		AlarmsClear:    faultMask == 0 && warningMask == 0,
		UnresolvedBits: true, // Remaining bitfield positions across firmware variants remain unresolved
	}

	copy(frame.BatchRaw[:], data[74:86])
	copy(frame.ReservedRaw[:], data[86:104])
	frame.Unknown104 = binary.BigEndian.Uint16(data[104:106])
	frame.Unknown106 = binary.BigEndian.Uint16(data[106:108])
	frame.Unknown108 = binary.BigEndian.Uint16(data[108:110])
	for i := range frame.Temperatures2Decikelvin {
		frame.Temperatures2Decikelvin[i] = binary.BigEndian.Uint16(data[110+2*i : 112+2*i])
	}
	frame.Unknown114 = binary.BigEndian.Uint16(data[114:116])
	for i := range frame.Temperatures4Decikelvin {
		frame.Temperatures4Decikelvin[i] = binary.BigEndian.Uint16(data[116+2*i : 118+2*i])
	}
	return frame, nil
}

func hasTelemetryHeader(data []byte) bool {
	if len(data) < 10 {
		return false
	}
	for i, b := range telemetryHeader {
		if data[i+1] != b {
			return false
		}
	}
	return true
}
