package solarman

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
)

func TestSendAndReceive_CompactsBufferWhileSkippingInterleavedFrames(t *testing.T) {
	client := NewClient("127.0.0.1", 8899, 12345, 1)
	client.timeout = 200 * time.Millisecond

	noiseClient := NewClient("127.0.0.1", 8899, 12345, 2)
	interleaved := buildResponseFrame(t, noiseClient, modbus.BuildWriteSingleRegister(2, 0x0200, 0x0001))

	respClient := NewClient("127.0.0.1", 8899, 12345, 1)
	responseFrame := buildResponseFrame(t, respClient, modbus.AppendCRC([]byte{0x01, 0x03, 0x02, 0x12, 0x34}))

	chunks := make([][]byte, 0, 131)
	for i := 0; i < 130; i++ {
		frameCopy := append([]byte(nil), interleaved...)
		chunks = append(chunks, frameCopy)
	}
	chunks = append(chunks, append([]byte(nil), responseFrame...))

	client.conn = &scriptedConn{reads: chunks}

	got, err := client.sendAndReceive(modbus.BuildReadHoldingRegisters(1, 0x0100, 1))
	require.NoError(t, err)

	want := modbus.AppendCRC([]byte{0x01, 0x03, 0x02, 0x12, 0x34})
	assert.Equal(t, want, got)
}

type scriptedConn struct {
	reads [][]byte
}

func buildResponseFrame(t *testing.T, c *Client, modbusFrame []byte) []byte {
	t.Helper()

	header := []byte{
		startMarker,
		0x00, 0x00,
		0x10, controlResponse,
		0x01, 0x00,
		0x00, 0x00, 0x00, 0x00,
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
	return frame
}

func (c *scriptedConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errors.New("read called with zero-length buffer")
	}
	if len(c.reads) == 0 {
		return 0, io.EOF
	}

	chunk := c.reads[0]
	n := copy(p, chunk)
	if n == len(chunk) {
		c.reads = c.reads[1:]
	} else {
		c.reads[0] = c.reads[0][n:]
	}
	return n, nil
}

func (c *scriptedConn) Write(p []byte) (int, error) {
	return len(p), nil
}

func (c *scriptedConn) Close() error {
	return nil
}

func (c *scriptedConn) LocalAddr() net.Addr {
	return dummyAddr("local")
}

func (c *scriptedConn) RemoteAddr() net.Addr {
	return dummyAddr("remote")
}

func (c *scriptedConn) SetDeadline(time.Time) error {
	return nil
}

func (c *scriptedConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *scriptedConn) SetWriteDeadline(time.Time) error {
	return nil
}

type dummyAddr string

func (a dummyAddr) Network() string {
	return "tcp"
}

func (a dummyAddr) String() string {
	return string(a)
}
