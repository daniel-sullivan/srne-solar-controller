package modbus

import "testing"

func TestCRC16(t *testing.T) {
	// Known MODBUS CRC test vector: slave 1, func 0x03, addr 0x0100, count 1
	frame := []byte{0x01, 0x03, 0x01, 0x00, 0x00, 0x01}
	crc := CRC16(frame)
	// Verify by appending and validating
	withCRC := AppendCRC(frame)
	if len(withCRC) != 8 {
		t.Fatalf("expected 8 bytes, got %d", len(withCRC))
	}
	if !ValidateCRC(withCRC) {
		t.Errorf("CRC validation failed for known frame, CRC=0x%04X", crc)
	}
}

func TestCRC16_KnownValue(t *testing.T) {
	// Standard MODBUS test: "123456789" should give CRC 0x4B37
	data := []byte("123456789")
	crc := CRC16(data)
	if crc != 0x4B37 {
		t.Errorf("expected CRC 0x4B37, got 0x%04X", crc)
	}
}

func TestValidateCRC_TooShort(t *testing.T) {
	if ValidateCRC([]byte{0x01, 0x02}) {
		t.Error("expected false for short frame")
	}
}

func TestValidateCRC_Invalid(t *testing.T) {
	frame := AppendCRC([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01})
	frame[len(frame)-1] ^= 0xFF // corrupt CRC
	if ValidateCRC(frame) {
		t.Error("expected false for corrupted CRC")
	}
}
