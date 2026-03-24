package modbus

import (
	"encoding/binary"
	"fmt"
)

// Function codes
const (
	FuncReadHoldingRegisters   = 0x03
	FuncWriteSingleRegister    = 0x06
	FuncWriteMultipleRegisters = 0x10
	FuncResetFactory           = 0x78
	FuncClearHistory           = 0x79
)

// SRNE-specific error codes
var errorMessages = map[byte]string{
	0x01: "illegal function",
	0x02: "illegal address",
	0x03: "illegal data value",
	0x04: "operation failed",
	0x05: "password error",
	0x06: "frame error",
	0x07: "parameter read-only",
	0x08: "cannot change while running",
	0x09: "password protection locked",
	0x0A: "length error",
	0x0B: "permission denied",
}

// ModbusError represents an error response from the device.
type ModbusError struct {
	FunctionCode  byte
	ExceptionCode byte
}

func (e *ModbusError) Error() string {
	msg, ok := errorMessages[e.ExceptionCode]
	if !ok {
		msg = fmt.Sprintf("unknown error 0x%02X", e.ExceptionCode)
	}
	return fmt.Sprintf("modbus error (function 0x%02X): %s", e.FunctionCode, msg)
}

// BuildReadHoldingRegisters builds a MODBUS RTU frame for function 0x03.
func BuildReadHoldingRegisters(slaveID byte, startAddr uint16, count uint16) []byte {
	frame := make([]byte, 6)
	frame[0] = slaveID
	frame[1] = FuncReadHoldingRegisters
	binary.BigEndian.PutUint16(frame[2:4], startAddr)
	binary.BigEndian.PutUint16(frame[4:6], count)
	return AppendCRC(frame)
}

// BuildWriteSingleRegister builds a MODBUS RTU frame for function 0x06.
func BuildWriteSingleRegister(slaveID byte, addr uint16, value uint16) []byte {
	frame := make([]byte, 6)
	frame[0] = slaveID
	frame[1] = FuncWriteSingleRegister
	binary.BigEndian.PutUint16(frame[2:4], addr)
	binary.BigEndian.PutUint16(frame[4:6], value)
	return AppendCRC(frame)
}

// BuildWriteMultipleRegisters builds a MODBUS RTU frame for function 0x10.
func BuildWriteMultipleRegisters(slaveID byte, startAddr uint16, values []uint16) []byte {
	count := len(values)
	frame := make([]byte, 7+count*2)
	frame[0] = slaveID
	frame[1] = FuncWriteMultipleRegisters
	binary.BigEndian.PutUint16(frame[2:4], startAddr)
	binary.BigEndian.PutUint16(frame[4:6], uint16(count))
	frame[6] = byte(count * 2)
	for i, v := range values {
		binary.BigEndian.PutUint16(frame[7+i*2:9+i*2], v)
	}
	return AppendCRC(frame)
}

// ParseReadResponse parses a response to a read holding registers request.
// Returns the register values. The input should be a complete RTU frame with CRC.
func ParseReadResponse(frame []byte) ([]uint16, error) {
	if len(frame) < 5 {
		return nil, fmt.Errorf("response too short: %d bytes", len(frame))
	}
	if !ValidateCRC(frame) {
		return nil, fmt.Errorf("CRC validation failed")
	}

	// Strip CRC for parsing
	pdu := frame[1 : len(frame)-2]

	// Check for error response (function code has high bit set)
	if pdu[0]&0x80 != 0 {
		return nil, &ModbusError{
			FunctionCode:  pdu[0] & 0x7F,
			ExceptionCode: pdu[1],
		}
	}

	if pdu[0] != FuncReadHoldingRegisters {
		return nil, fmt.Errorf("unexpected function code: 0x%02X", pdu[0])
	}

	byteCount := int(pdu[1])
	if len(pdu) < 2+byteCount {
		return nil, fmt.Errorf("response data too short: expected %d bytes, got %d", byteCount, len(pdu)-2)
	}

	regCount := byteCount / 2
	values := make([]uint16, regCount)
	for i := 0; i < regCount; i++ {
		values[i] = binary.BigEndian.Uint16(pdu[2+i*2 : 4+i*2])
	}
	return values, nil
}

// ParseWriteResponse parses a response to a write single/multiple register request.
// Returns the address and value/count written.
func ParseWriteResponse(frame []byte) (addr uint16, value uint16, err error) {
	if len(frame) < 5 {
		return 0, 0, fmt.Errorf("response too short: %d bytes", len(frame))
	}
	if !ValidateCRC(frame) {
		return 0, 0, fmt.Errorf("CRC validation failed")
	}

	pdu := frame[1 : len(frame)-2]

	if pdu[0]&0x80 != 0 {
		return 0, 0, &ModbusError{
			FunctionCode:  pdu[0] & 0x7F,
			ExceptionCode: pdu[1],
		}
	}

	if pdu[0] != FuncWriteSingleRegister && pdu[0] != FuncWriteMultipleRegisters {
		return 0, 0, fmt.Errorf("unexpected function code: 0x%02X", pdu[0])
	}

	if len(pdu) < 5 {
		return 0, 0, fmt.Errorf("response PDU too short")
	}

	addr = binary.BigEndian.Uint16(pdu[1:3])
	value = binary.BigEndian.Uint16(pdu[3:5])
	return addr, value, nil
}
