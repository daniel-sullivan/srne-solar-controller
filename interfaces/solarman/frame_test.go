package solarman

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
)

func TestExtractAllFrames_Single(t *testing.T) {
	c := NewClient("127.0.0.1", 8899, 12345, 1)
	modbusFrame := modbus.BuildReadHoldingRegisters(1, 0x0100, 1)
	frame := c.wrapFrame(modbusFrame)

	frames := extractAllFrames(frame)
	require.Len(t, frames, 1)
	assert.Len(t, frames[0], len(frame))
}

func TestExtractAllFrames_Multiple(t *testing.T) {
	c := NewClient("127.0.0.1", 8899, 12345, 1)
	f1 := c.wrapFrame(modbus.BuildReadHoldingRegisters(1, 0x0100, 1))
	f2 := c.wrapFrame(modbus.BuildReadHoldingRegisters(1, 0x0200, 2))

	frames := extractAllFrames(append(f1, f2...))
	assert.Len(t, frames, 2)
}

func TestExtractAllFrames_Garbage(t *testing.T) {
	c := NewClient("127.0.0.1", 8899, 12345, 1)
	frame := c.wrapFrame(modbus.BuildReadHoldingRegisters(1, 0x0100, 1))

	data := append([]byte{0x00, 0xFF, 0x42}, frame...)
	frames := extractAllFrames(data)
	assert.Len(t, frames, 1)
}

func TestExtractAllFrames_Truncated(t *testing.T) {
	c := NewClient("127.0.0.1", 8899, 12345, 1)
	frame := c.wrapFrame(modbus.BuildReadHoldingRegisters(1, 0x0100, 1))
	assert.Empty(t, extractAllFrames(frame[:len(frame)-5]))
}

func TestExtractAllFrames_Empty(t *testing.T) {
	assert.Empty(t, extractAllFrames(nil))
}

func TestExtractAllFrames_TooShort(t *testing.T) {
	assert.Empty(t, extractAllFrames([]byte{startMarker, 0x01, 0x00}))
}

func TestWrapFrame_Structure(t *testing.T) {
	c := NewClient("127.0.0.1", 8899, 0xDEADBEEF, 1)
	c.connSerial = 0xDEADBEEF
	modbusFrame := modbus.BuildReadHoldingRegisters(1, 0x0100, 1)
	frame := c.wrapFrame(modbusFrame)

	assert.Equal(t, byte(startMarker), frame[0])
	assert.Equal(t, byte(endMarker), frame[len(frame)-1])

	payloadLen := int(frame[1]) | int(frame[2])<<8
	assert.Equal(t, 15+len(modbusFrame), payloadLen)
	assert.Len(t, frame, payloadLen+13)
	assert.Equal(t, byte(controlRequest), frame[4])

	serial := uint32(frame[7]) | uint32(frame[8])<<8 | uint32(frame[9])<<16 | uint32(frame[10])<<24
	assert.Equal(t, uint32(0xDEADBEEF), serial)

	var checksum byte
	for _, b := range frame[1 : len(frame)-2] {
		checksum += b
	}
	assert.Equal(t, checksum, frame[len(frame)-2], "V5 checksum")
}

func TestWrapFrame_SequenceIncrements(t *testing.T) {
	c := NewClient("127.0.0.1", 8899, 12345, 1)
	modbusFrame := modbus.BuildReadHoldingRegisters(1, 0x0100, 1)

	f1 := c.wrapFrame(modbusFrame)
	f2 := c.wrapFrame(modbusFrame)
	assert.Equal(t, f1[5]+1, f2[5], "sequence should increment")
}

func TestUnwrapFrame_Response(t *testing.T) {
	c := NewClient("127.0.0.1", 8899, 12345, 1)

	modbusFrame := modbus.AppendCRC([]byte{0x01, 0x03, 0x02, 0x00, 0x55})

	header := []byte{
		startMarker,
		0x00, 0x00,
		0x10, controlResponse,
		0x01, 0x00,
		0x39, 0x30, 0x00, 0x00,
	}
	body := make([]byte, 0, 14+len(modbusFrame))
	body = append(body, frameType)
	body = append(body, 0x00, 0x00)
	body = append(body, make([]byte, 11)...)
	body = append(body, modbusFrame...)

	header[1] = byte(len(body))
	header[2] = byte(len(body) >> 8)

	frame := make([]byte, 0, len(header)+len(body)+2)
	frame = append(frame, header...)
	frame = append(frame, body...)
	var checksum byte
	for _, b := range frame[1:] {
		checksum += b
	}
	frame = append(frame, checksum, endMarker)

	inner, err := c.unwrapFrame(frame)
	require.NoError(t, err)
	assert.Equal(t, modbusFrame, inner)
}

func TestUnwrapFrame_BadChecksum(t *testing.T) {
	c := NewClient("127.0.0.1", 8899, 12345, 1)
	c.connSerial = 12345
	wrapped := c.wrapFrame(modbus.BuildReadHoldingRegisters(1, 0x0100, 1))
	wrapped[len(wrapped)-2] ^= 0xFF

	_, err := c.unwrapFrame(wrapped)
	assert.Error(t, err)
}

func TestUnwrapFrame_TooShort(t *testing.T) {
	c := NewClient("127.0.0.1", 8899, 12345, 1)
	_, err := c.unwrapFrame([]byte{startMarker, 0x01, endMarker})
	assert.Error(t, err)
}
