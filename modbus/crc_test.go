package modbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCRC16(t *testing.T) {
	frame := []byte{0x01, 0x03, 0x01, 0x00, 0x00, 0x01}
	withCRC := AppendCRC(frame)
	require.Len(t, withCRC, 8)
	assert.True(t, ValidateCRC(withCRC))
}

func TestCRC16_KnownValue(t *testing.T) {
	data := []byte("123456789")
	assert.Equal(t, uint16(0x4B37), CRC16(data))
}

func TestValidateCRC_TooShort(t *testing.T) {
	assert.False(t, ValidateCRC([]byte{0x01, 0x02}))
}

func TestValidateCRC_Invalid(t *testing.T) {
	frame := AppendCRC([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01})
	frame[len(frame)-1] ^= 0xFF
	assert.False(t, ValidateCRC(frame))
}
