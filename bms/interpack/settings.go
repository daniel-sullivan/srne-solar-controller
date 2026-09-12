package interpack

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const (
	// FunctionCodeSettings is the JBD function code for configuration queries.
	FunctionCodeSettings = 0x78

	// RegBalanceStart and RegBalanceEnd define the register block for balance configuration.
	RegBalanceStart = 0x1C00
	RegBalanceEnd   = 0x1CA0

	// BalancePayloadLength is the verified payload length for UP16S balance configuration.
	BalancePayloadLength = 134
	// BalanceFrameSize is the full wire frame length (8-byte header + 134-byte payload + 2-byte CRC).
	BalanceFrameSize = 8 + BalancePayloadLength + 2 // 144

	// RegProtectionStart and RegProtectionEnd define the register block for OVP limits.
	RegProtectionStart = 0x1800
	RegProtectionEnd   = 0x1900

	// ProtectionPayloadLength is the verified payload length for protection configuration.
	ProtectionPayloadLength = 208
	// ProtectionFrameSize is the full wire frame length (8-byte header + 208-byte payload + 2-byte CRC).
	ProtectionFrameSize = 8 + ProtectionPayloadLength + 2 // 218

	// SettingsRequestSize is the byte length of a read-only 0x78 request frame.
	SettingsRequestSize = 10
)

var (
	ErrSettingsFrameSize = errors.New("interpack: invalid settings frame size")
	ErrSettingsHeader    = errors.New("interpack: invalid settings header")
	ErrSettingsBlock     = errors.New("interpack: unsupported settings block")
	ErrSettingsRange     = errors.New("interpack: settings value out of valid range")
)

// BuildSettingsRequest constructs a strictly read-only 10-byte 0x78 settings query.
// Wire format: [address, 0x78, startBE16, endBE16, 0x00, 0x00, CRC16-Modbus LE].
func BuildSettingsRequest(address uint8, start, end uint16) [10]byte {
	var req [10]byte
	req[0] = address
	req[1] = FunctionCodeSettings
	binary.BigEndian.PutUint16(req[2:4], start)
	binary.BigEndian.PutUint16(req[4:6], end)
	req[6] = 0
	req[7] = 0
	binary.LittleEndian.PutUint16(req[8:10], CRC16(req[:8]))
	return req
}

// BalanceSettingsRequest returns a read-only 0x78 request for the balance settings block.
func BalanceSettingsRequest(address uint8) [10]byte {
	return BuildSettingsRequest(address, RegBalanceStart, RegBalanceEnd)
}

// ProtectionSettingsRequest returns a read-only 0x78 request for the protection settings block.
func ProtectionSettingsRequest(address uint8) [10]byte {
	return BuildSettingsRequest(address, RegProtectionStart, RegProtectionEnd)
}

// BalanceSettings contains verified balance-related settings decoded from block 0x1C00-0x1CA0.
// BMSID is retained for physical identity tracking and anti-aliasing guards, but is omitted
// from public JSON (json:"-") to protect deployment hardware identifiers.
type BalanceSettings struct {
	BalanceStartMillivolts uint16  `json:"balance_start_mv"`
	BalanceDeltaMillivolts uint16  `json:"balance_delta_mv"`
	FullChargeCentivolts   uint16  `json:"full_charge_cv"`
	FullChargeVolts        float64 `json:"full_charge_volts"`
	BMSID                  string  `json:"-"`
}

// ProtectionSettings contains verified protection settings decoded from block 0x1800-0x1900.
type ProtectionSettings struct {
	CellOVPAlarmMillivolts      uint16  `json:"cell_ovp_alarm_mv"`
	CellOVPProtectionMillivolts uint16  `json:"cell_ovp_protection_mv"`
	PackOVPAlarmCentivolts      uint16  `json:"pack_ovp_alarm_cv"`
	PackOVPProtectionCentivolts uint16  `json:"pack_ovp_protection_cv"`
	PackOVPAlarmVolts           float64 `json:"pack_ovp_alarm_volts"`
	PackOVPProtectionVolts      float64 `json:"pack_ovp_protection_volts"`
}

// SettingsBlockType classifies the decoded settings block.
type SettingsBlockType int

const (
	BlockTypeUnknown SettingsBlockType = iota
	BlockTypeBalance
	BlockTypeProtection
)

// SettingsFrame represents a validated 0x78 settings reply from a pack.
type SettingsFrame struct {
	Address    uint8
	BlockType  SettingsBlockType
	Start      uint16
	End        uint16
	PayloadLen uint16
	Balance    *BalanceSettings
	Protection *ProtectionSettings
}

// ParseSettingsFrame validates and decodes a single 0x78 settings reply.
func ParseSettingsFrame(data []byte) (SettingsFrame, error) {
	if len(data) < 10 {
		return SettingsFrame{}, ErrSettingsFrameSize
	}
	if data[1] != FunctionCodeSettings {
		return SettingsFrame{}, ErrSettingsHeader
	}
	start := binary.BigEndian.Uint16(data[2:4])
	end := binary.BigEndian.Uint16(data[4:6])
	declaredLen := binary.BigEndian.Uint16(data[6:8])

	expectedTotal := 8 + int(declaredLen) + 2
	if len(data) != expectedTotal {
		return SettingsFrame{}, fmt.Errorf("%w: got %d bytes, expected %d", ErrSettingsFrameSize, len(data), expectedTotal)
	}

	gotCRC := binary.LittleEndian.Uint16(data[len(data)-2:])
	if wantCRC := CRC16(data[:len(data)-2]); gotCRC != wantCRC {
		return SettingsFrame{}, fmt.Errorf("%w: got 0x%04x, want 0x%04x", ErrCRC, gotCRC, wantCRC)
	}

	payload := data[8 : 8+declaredLen]
	frame := SettingsFrame{
		Address:    data[0],
		Start:      start,
		End:        end,
		PayloadLen: declaredLen,
	}

	switch {
	case start == RegBalanceStart && end == RegBalanceEnd:
		// Actual installed UP16S firmware uses 134 bytes; known reference variants use 136.
		// Layout offsets 4:6 (balance start), 6:8 (balance delta), 12:14 (full charge),
		// and 16:46 (BMS serial/identifier) are identical across both 134 and 136 variants
		// because the two extra reference bytes reside at the tail of the block.
		// Arbitrary shorter lengths are strictly rejected to prevent offset misalignment.
		if declaredLen != BalancePayloadLength && declaredLen != 136 {
			return SettingsFrame{}, fmt.Errorf("%w: balance payload length must be %d or 136, got %d",
				ErrSettingsFrameSize, BalancePayloadLength, declaredLen)
		}
		balStart := binary.BigEndian.Uint16(payload[4:6])
		balDelta := binary.BigEndian.Uint16(payload[6:8])
		fullCharge := binary.BigEndian.Uint16(payload[12:14])
		bmsID := strings.TrimRight(string(payload[16:46]), "\x00 \t\r\n")

		// Plausibility boundaries for 16S LiFePO4 pack:
		// Balance start: 2000 - 4500 mV (actual 3400 mV)
		// Balance delta: 5 - 500 mV (actual 30 mV)
		// Full charge voltage: 4000 - 6500 cV (actual 5680 cV = 56.80 V)
		if balStart < 2000 || balStart > 4500 || balDelta < 5 || balDelta > 500 || fullCharge < 4000 || fullCharge > 6500 {
			return SettingsFrame{}, fmt.Errorf("%w: balance values invalid (start=%d mV, delta=%d mV, fullCharge=%d cV)",
				ErrSettingsRange, balStart, balDelta, fullCharge)
		}

		frame.BlockType = BlockTypeBalance
		frame.Balance = &BalanceSettings{
			BalanceStartMillivolts: balStart,
			BalanceDeltaMillivolts: balDelta,
			FullChargeCentivolts:   fullCharge,
			FullChargeVolts:        float64(fullCharge) / 100.0,
			BMSID:                  bmsID,
		}
		return frame, nil

	case start == RegProtectionStart && end == RegProtectionEnd:
		// Actual installed UP16S protection block is strictly 208 bytes.
		// Shorter arbitrary lengths are rejected.
		if declaredLen != ProtectionPayloadLength {
			return SettingsFrame{}, fmt.Errorf("%w: protection payload length must be %d, got %d",
				ErrSettingsFrameSize, ProtectionPayloadLength, declaredLen)
		}
		cellOVPAlarm := binary.BigEndian.Uint16(payload[0:2])
		cellOVPProt := binary.BigEndian.Uint16(payload[6:8])
		packOVPAlarm := binary.BigEndian.Uint16(payload[24:26])
		packOVPProt := binary.BigEndian.Uint16(payload[30:32])

		// Plausibility boundaries for 16S LiFePO4 pack:
		// Cell OVP alarm: 2500 - 4500 mV (actual 3600 mV)
		// Cell OVP protection: 2500 - 4500 mV (actual 3650 mV)
		// Pack OVP alarm: 4000 - 7000 cV (actual 5760 cV = 57.60 V)
		// Pack OVP protection: 4000 - 7000 cV (actual 5840 cV = 58.40 V)
		if cellOVPAlarm < 2500 || cellOVPAlarm > 4500 || cellOVPProt < 2500 || cellOVPProt > 4500 ||
			packOVPAlarm < 4000 || packOVPAlarm > 7000 || packOVPProt < 4000 || packOVPProt > 7000 {
			return SettingsFrame{}, fmt.Errorf("%w: protection values invalid (cellAlarm=%d, cellProt=%d, packAlarm=%d, packProt=%d)",
				ErrSettingsRange, cellOVPAlarm, cellOVPProt, packOVPAlarm, packOVPProt)
		}

		frame.BlockType = BlockTypeProtection
		frame.Protection = &ProtectionSettings{
			CellOVPAlarmMillivolts:      cellOVPAlarm,
			CellOVPProtectionMillivolts: cellOVPProt,
			PackOVPAlarmCentivolts:      packOVPAlarm,
			PackOVPProtectionCentivolts: packOVPProt,
			PackOVPAlarmVolts:           float64(packOVPAlarm) / 100.0,
			PackOVPProtectionVolts:      float64(packOVPProt) / 100.0,
		}
		return frame, nil

	default:
		return SettingsFrame{}, fmt.Errorf("%w: start 0x%04x, end 0x%04x", ErrSettingsBlock, start, end)
	}
}

// matchSettingsHeader tests whether a byte slice starts with a recognized 0x78 settings response header.
// It returns the expected full frame length and true if a valid header is found.
func matchSettingsHeader(data []byte) (int, bool) {
	if len(data) < 8 {
		return 0, false
	}
	if data[1] != FunctionCodeSettings {
		return 0, false
	}
	start := binary.BigEndian.Uint16(data[2:4])
	end := binary.BigEndian.Uint16(data[4:6])
	declaredLen := binary.BigEndian.Uint16(data[6:8])

	if (start == RegBalanceStart && end == RegBalanceEnd && (declaredLen == BalancePayloadLength || declaredLen == 136)) ||
		(start == RegProtectionStart && end == RegProtectionEnd && declaredLen == ProtectionPayloadLength) {
		return 8 + int(declaredLen) + 2, true
	}
	return 0, false
}
