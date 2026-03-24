package modbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReadHoldingRegisters(t *testing.T) {
	frame := BuildReadHoldingRegisters(1, 0x0100, 2)
	require.Len(t, frame, 8)
	assert.Equal(t, byte(1), frame[0], "slave ID")
	assert.Equal(t, byte(0x03), frame[1], "function code")
	assert.Equal(t, byte(0x01), frame[2])
	assert.Equal(t, byte(0x00), frame[3])
	assert.Equal(t, byte(0x00), frame[4])
	assert.Equal(t, byte(0x02), frame[5])
	assert.True(t, ValidateCRC(frame))
}

func TestBuildWriteSingleRegister(t *testing.T) {
	frame := BuildWriteSingleRegister(1, 0xE204, 0x0001)
	require.Len(t, frame, 8)
	assert.Equal(t, byte(0x06), frame[1], "function code")
	assert.True(t, ValidateCRC(frame))
}

func TestBuildWriteMultipleRegisters(t *testing.T) {
	frame := BuildWriteMultipleRegisters(1, 0xE034, []uint16{0x1A02, 0x0315})
	require.Len(t, frame, 13)
	assert.Equal(t, byte(0x10), frame[1], "function code")
	assert.Equal(t, byte(4), frame[6], "byte count")
	assert.True(t, ValidateCRC(frame))
}

func TestParseReadResponse(t *testing.T) {
	pdu := []byte{0x01, 0x03, 0x04, 0x00, 0x64, 0x01, 0xF4}
	frame := AppendCRC(pdu)

	values, err := ParseReadResponse(frame)
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Equal(t, uint16(0x0064), values[0])
	assert.Equal(t, uint16(0x01F4), values[1])
}

func TestParseReadResponse_Error(t *testing.T) {
	pdu := []byte{0x01, 0x83, 0x02}
	frame := AppendCRC(pdu)

	_, err := ParseReadResponse(frame)
	require.Error(t, err)

	var modbusErr *ModbusError
	require.ErrorAs(t, err, &modbusErr)
	assert.Equal(t, byte(0x02), modbusErr.ExceptionCode)
}

func TestParseWriteResponse(t *testing.T) {
	pdu := []byte{0x01, 0x06, 0xE2, 0x04, 0x00, 0x01}
	frame := AppendCRC(pdu)

	addr, value, err := ParseWriteResponse(frame)
	require.NoError(t, err)
	assert.Equal(t, uint16(0xE204), addr)
	assert.Equal(t, uint16(0x0001), value)
}
