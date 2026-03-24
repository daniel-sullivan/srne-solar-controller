package modbus

import (
	"errors"
	"testing"
)

func TestBuildReadHoldingRegisters(t *testing.T) {
	frame := BuildReadHoldingRegisters(1, 0x0100, 2)
	// slave=1, func=0x03, addr=0x0100, count=0x0002, + 2 CRC bytes
	if len(frame) != 8 {
		t.Fatalf("expected 8 bytes, got %d", len(frame))
	}
	if frame[0] != 1 || frame[1] != 0x03 {
		t.Errorf("unexpected header: %02X %02X", frame[0], frame[1])
	}
	if frame[2] != 0x01 || frame[3] != 0x00 {
		t.Errorf("unexpected addr: %02X%02X", frame[2], frame[3])
	}
	if frame[4] != 0x00 || frame[5] != 0x02 {
		t.Errorf("unexpected count: %02X%02X", frame[4], frame[5])
	}
	if !ValidateCRC(frame) {
		t.Error("CRC validation failed")
	}
}

func TestBuildWriteSingleRegister(t *testing.T) {
	frame := BuildWriteSingleRegister(1, 0xE204, 0x0001)
	if len(frame) != 8 {
		t.Fatalf("expected 8 bytes, got %d", len(frame))
	}
	if frame[1] != 0x06 {
		t.Errorf("expected func 0x06, got 0x%02X", frame[1])
	}
	if !ValidateCRC(frame) {
		t.Error("CRC validation failed")
	}
}

func TestBuildWriteMultipleRegisters(t *testing.T) {
	frame := BuildWriteMultipleRegisters(1, 0xE034, []uint16{0x1A02, 0x0315})
	// 1 + 1 + 2 + 2 + 1 + 4 + 2 = 13 bytes
	if len(frame) != 13 {
		t.Fatalf("expected 13 bytes, got %d", len(frame))
	}
	if frame[1] != 0x10 {
		t.Errorf("expected func 0x10, got 0x%02X", frame[1])
	}
	if frame[6] != 4 {
		t.Errorf("expected byte count 4, got %d", frame[6])
	}
	if !ValidateCRC(frame) {
		t.Error("CRC validation failed")
	}
}

func TestParseReadResponse(t *testing.T) {
	// Simulate: slave=1, func=0x03, byte_count=4, data=[0x0064, 0x01F4]
	pdu := []byte{0x01, 0x03, 0x04, 0x00, 0x64, 0x01, 0xF4}
	frame := AppendCRC(pdu)

	values, err := ParseReadResponse(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != 0x0064 {
		t.Errorf("expected 0x0064, got 0x%04X", values[0])
	}
	if values[1] != 0x01F4 {
		t.Errorf("expected 0x01F4, got 0x%04X", values[1])
	}
}

func TestParseReadResponse_Error(t *testing.T) {
	// Error response: func=0x83 (0x03 | 0x80), exception=0x02
	pdu := []byte{0x01, 0x83, 0x02}
	frame := AppendCRC(pdu)

	_, err := ParseReadResponse(frame)
	if err == nil {
		t.Fatal("expected error")
	}
	var modbusErr *ModbusError
	if !errors.As(err, &modbusErr) {
		t.Fatalf("expected ModbusError, got %T", err)
	}
	if modbusErr.ExceptionCode != 0x02 {
		t.Errorf("expected exception 0x02, got 0x%02X", modbusErr.ExceptionCode)
	}
}

func TestParseWriteResponse(t *testing.T) {
	// Write single response: echo of request
	pdu := []byte{0x01, 0x06, 0xE2, 0x04, 0x00, 0x01}
	frame := AppendCRC(pdu)

	addr, value, err := ParseWriteResponse(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != 0xE204 {
		t.Errorf("expected addr 0xE204, got 0x%04X", addr)
	}
	if value != 0x0001 {
		t.Errorf("expected value 0x0001, got 0x%04X", value)
	}
}
