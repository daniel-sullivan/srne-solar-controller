package solarman

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"strconv"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
)

// errNoInnerData indicates a valid V5 frame was received but the inner MODBUS
// payload was too short to be a real response. The Solarman dongle sends these
// both as an ACK (immediately) and as a "device did not respond" notification
// (after its internal timeout of ~5 seconds).
var errNoInnerData = errors.New("no MODBUS data in V5 frame")

const (
	startMarker = 0xA5
	endMarker   = 0x15

	controlRequest   = 0x45
	controlResponse  = 0x15 // REQUEST - 0x30
	controlHandshake = 0x41
	controlHeartbeat = 0x47

	frameType = 0x02

	// Inner MODBUS frame starts at byte 25 in the V5 response,
	// and ends 2 bytes before the frame end (checksum + end marker).
	innerFrameOffset = 25
)

// Client communicates with an inverter through a Solarman V5 wifi dongle.
type Client struct {
	host       string
	port       int
	serial     uint32 // configured serial (0 = auto-detect)
	connSerial uint32 // learned serial for this connection
	slaveID    byte
	seq        byte
	conn       net.Conn
	timeout    time.Duration
	Debug      bool
}

// NewClient creates a new Solarman V5 client.
func NewClient(host string, port int, serial uint32, slaveID byte) *Client {
	return &Client{
		host:    host,
		port:    port,
		serial:  serial,
		slaveID: slaveID,
		seq:     byte(rand.IntN(255) + 1),
		timeout: 10 * time.Second,
	}
}

// Connect opens a TCP connection to the dongle.
func (c *Client) Connect() error {
	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	conn, err := net.DialTimeout("tcp", addr, c.timeout)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	c.conn = conn
	c.connSerial = c.serial
	return nil
}

// activeSerial returns the serial to use in frames for this connection.
func (c *Client) activeSerial() uint32 {
	return c.connSerial
}

// Close closes the TCP connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ProbeSlaveID discovers the correct slave ID for the device behind this dongle.
// It tries the currently configured slave ID first, then scans IDs 1 through maxID.
// A shorter timeout is used for probing. On success, the client's slave ID is set
// to the discovered ID and returned.
func (c *Client) ProbeSlaveID(maxID byte) (byte, error) {
	if c.conn == nil {
		return 0, fmt.Errorf("not connected")
	}

	origTimeout := c.timeout
	origSlaveID := c.slaveID
	c.timeout = 6 * time.Second
	defer func() { c.timeout = origTimeout }()

	// Try current slave ID first.
	if id, ok := c.trySlaveID(c.slaveID); ok {
		return id, nil
	}

	// Scan remaining IDs.
	for id := byte(1); id <= maxID; id++ {
		if id == origSlaveID {
			continue
		}
		if found, ok := c.trySlaveID(id); ok {
			return found, nil
		}
	}

	c.slaveID = origSlaveID
	return 0, fmt.Errorf("no device responded on slave IDs 1-%d", maxID)
}

// trySlaveID attempts a single register read with the given slave ID.
// Returns the ID and true if the device responded (even with a MODBUS error).
func (c *Client) trySlaveID(id byte) (byte, bool) {
	c.slaveID = id
	_, err := c.ReadRegisters(0x000A, 1)
	if err == nil {
		return id, true
	}
	// A MODBUS error (illegal address, etc.) still means the device responded.
	var modbusErr *modbus.ModbusError
	if errors.As(err, &modbusErr) {
		return id, true
	}
	return 0, false
}

// ReadRegisters reads holding registers from the inverter.
func (c *Client) ReadRegisters(startAddr uint16, count uint16) ([]uint16, error) {
	modbusFrame := modbus.BuildReadHoldingRegisters(c.slaveID, startAddr, count)
	resp, err := c.sendRequest(modbusFrame)
	if err != nil {
		return nil, err
	}
	return modbus.ParseReadResponse(resp)
}

// WriteSingleRegister writes a single register on the inverter.
func (c *Client) WriteSingleRegister(addr uint16, value uint16) error {
	modbusFrame := modbus.BuildWriteSingleRegister(c.slaveID, addr, value)
	resp, err := c.sendRequest(modbusFrame)
	if err != nil {
		return err
	}
	_, _, err = modbus.ParseWriteResponse(resp)
	return err
}

// WriteMultipleRegisters writes multiple contiguous registers.
func (c *Client) WriteMultipleRegisters(startAddr uint16, values []uint16) error {
	modbusFrame := modbus.BuildWriteMultipleRegisters(c.slaveID, startAddr, values)
	resp, err := c.sendRequest(modbusFrame)
	if err != nil {
		return err
	}
	_, _, err = modbus.ParseWriteResponse(resp)
	return err
}

func (c *Client) sendRequest(modbusFrame []byte) ([]byte, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	// If we don't have a serial yet, probe the dongle to learn it.
	if c.connSerial == 0 {
		if err := c.probeSerial(modbusFrame); err != nil {
			return nil, fmt.Errorf("serial auto-detect: %w", err)
		}
	}

	return c.sendAndReceive(modbusFrame)
}

// probeSerial sends a request with serial=0, reads the dongle's ack to
// learn the serial number, then sets connSerial for subsequent requests.
func (c *Client) probeSerial(modbusFrame []byte) error {
	frame := c.wrapFrame(modbusFrame)
	if c.Debug {
		fmt.Printf("PROBE TX (%d bytes): % X\n", len(frame), frame)
	}

	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.conn.Write(frame); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	// Read the ack frame to extract the dongle serial
	buf := make([]byte, 1024)
	deadline := time.Now().Add(c.timeout)
	total := 0
	for time.Now().Before(deadline) {
		_ = c.conn.SetReadDeadline(deadline)
		n, err := c.conn.Read(buf[total:])
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		total += n

		for _, raw := range extractAllFrames(buf[:total]) {
			if len(raw) >= 11 && raw[0] == startMarker {
				serial := binary.LittleEndian.Uint32(raw[7:11])
				if serial != 0 {
					c.connSerial = serial
					if c.Debug {
						fmt.Printf("  auto-detected serial: %d\n", serial)
					}
					return nil
				}
			}
		}
	}
	return fmt.Errorf("no response from dongle")
}

func (c *Client) sendAndReceive(modbusFrame []byte) ([]byte, error) {
	frame := c.wrapFrame(modbusFrame)
	if c.Debug {
		fmt.Printf("TX (%d bytes): % X\n", len(frame), frame)
	}

	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.conn.Write(frame); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	// Read frames until we get one with valid MODBUS data.
	// The dongle first sends a short ack frame, then later the actual
	// data frame once the inverter responds on the RS485 bus. If the
	// inverter doesn't respond, the dongle sends a second short frame
	// after its internal timeout (~5s).
	buf := make([]byte, 4096)
	total := 0
	consumed := 0
	emptyFrames := 0
	deadline := time.Now().Add(c.timeout)

	for time.Now().Before(deadline) {
		_ = c.conn.SetReadDeadline(deadline)
		n, err := c.conn.Read(buf[total:])
		if err != nil {
			if total > consumed {
				break
			}
			return nil, fmt.Errorf("read: %w", err)
		}
		total += n

		frames := extractAllFrames(buf[consumed:total])
		for _, raw := range frames {
			inner, err := c.unwrapFrame(raw)
			if err != nil {
				if c.Debug {
					fmt.Printf("  skip: %v\n", err)
				}
				if errors.Is(err, errNoInnerData) {
					emptyFrames++
					if emptyFrames >= 2 {
						return nil, fmt.Errorf("device did not respond")
					}
				}
				continue
			}
			if c.Debug {
				fmt.Printf("  modbus (%d bytes): % X\n", len(inner), inner)
			}
			return inner, nil
		}

		advance := 0
		for _, f := range frames {
			advance += len(f)
		}
		consumed += advance
	}
	return nil, fmt.Errorf("no MODBUS response within timeout")
}

// wrapFrame builds a Solarman V5 request frame wrapping an inner MODBUS RTU frame.
//
// Frame layout (ha-solarman compatible):
//
//	[0]     0xA5 start
//	[1:3]   payload length (LE u16) = 15 + len(modbus)
//	[3]     0x10 control suffix
//	[4]     0x45 control code (REQUEST)
//	[5:7]   sequence number (LE u16, only low byte significant)
//	[7:11]  dongle serial (LE u32)
//	[11]    0x02 frame type
//	[12:14] 0x0000 sensor type placeholder
//	[14:26] 0x00*12 timestamp placeholders
//	[26:N]  inner MODBUS RTU frame
//	[N]     checksum: sum(bytes[1:N]) & 0xFF
//	[N+1]   0x15 end
func (c *Client) wrapFrame(modbusFrame []byte) []byte {
	bodyLen := 15 + len(modbusFrame) // frame_type(1) + sensor(2) + timestamps(12) + modbus
	frame := make([]byte, 0, bodyLen+13)

	// Header (11 bytes)
	frame = append(frame, startMarker)
	frame = append(frame, byte(bodyLen), byte(bodyLen>>8)) // length LE
	frame = append(frame, 0x10, controlRequest)            // control
	seq := c.seq
	c.seq++
	frame = append(frame, seq, 0x00) // sequence LE

	serialBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(serialBytes, c.activeSerial())
	frame = append(frame, serialBytes...) // serial LE

	// Body
	frame = append(frame, frameType)           // [11]
	frame = append(frame, 0x00, 0x00)          // [12:14] sensor type
	frame = append(frame, make([]byte, 12)...) // [14:26] timestamps
	frame = append(frame, modbusFrame...)      // [26:N]

	// Trailer
	var checksum byte
	for _, b := range frame[1:] {
		checksum += b
	}
	frame = append(frame, checksum, endMarker)

	return frame
}

// unwrapFrame extracts the inner MODBUS RTU frame from a V5 response.
func (c *Client) unwrapFrame(data []byte) ([]byte, error) {
	if len(data) < innerFrameOffset+2 {
		return nil, fmt.Errorf("frame too short for MODBUS data (%d bytes)", len(data))
	}
	if data[0] != startMarker || data[len(data)-1] != endMarker {
		return nil, fmt.Errorf("invalid frame markers")
	}

	// Validate V5 checksum: sum of bytes[1 : len-2] & 0xFF
	var checksum byte
	for _, b := range data[1 : len(data)-2] {
		checksum += b
	}
	if checksum != data[len(data)-2] {
		return nil, fmt.Errorf("V5 checksum mismatch: got 0x%02X want 0x%02X", data[len(data)-2], checksum)
	}

	// Inner MODBUS RTU frame is at data[25:-2]
	inner := data[innerFrameOffset : len(data)-2]

	// Must be at least 5 bytes for a minimal MODBUS response (slave + func + 1 byte + CRC)
	if len(inner) < 5 {
		return nil, fmt.Errorf("%w (%d bytes)", errNoInnerData, len(inner))
	}

	// Validate MODBUS CRC
	if !modbus.ValidateCRC(inner) {
		// Handle double-CRC quirk: trailing 0x0000
		if len(inner) > 4 && inner[len(inner)-1] == 0x00 && inner[len(inner)-2] == 0x00 {
			trimmed := inner[:len(inner)-2]
			if modbus.ValidateCRC(trimmed) {
				return trimmed, nil
			}
		}
		return nil, fmt.Errorf("MODBUS CRC invalid")
	}

	return inner, nil
}

// ScanFrames reads V5 frames from the dongle stream and calls the callback
// for each valid MODBUS frame found. Returns when callback returns false or on error.
func (c *Client) ScanFrames(callback func(modbusFrame []byte) bool) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	buf := make([]byte, 4096)
	total := 0
	consumed := 0

	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.timeout))
		n, err := c.conn.Read(buf[total:])
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		total += n

		frames := extractAllFrames(buf[consumed:total])
		for _, raw := range frames {
			inner, err := c.unwrapFrame(raw)
			if err != nil {
				continue
			}
			if !callback(inner) {
				return nil
			}
		}

		advance := 0
		for _, f := range frames {
			advance += len(f)
		}
		consumed += advance

		if consumed > len(buf)/2 {
			copy(buf, buf[consumed:total])
			total -= consumed
			consumed = 0
		}
	}
}

// extractAllFrames splits a byte buffer into individual Solarman V5 frames.
// Frame total length = payload_length + 13 (11 header + 1 checksum + 1 end).
func extractAllFrames(data []byte) [][]byte {
	var frames [][]byte
	for i := 0; i < len(data); {
		if data[i] != startMarker {
			i++
			continue
		}
		remaining := data[i:]
		if len(remaining) < 5 {
			break
		}
		payloadLen := int(remaining[1]) | int(remaining[2])<<8
		frameLen := payloadLen + 13
		if frameLen > len(remaining) {
			break
		}
		if remaining[frameLen-1] == endMarker {
			frames = append(frames, remaining[:frameLen])
		}
		i += frameLen
	}
	return frames
}
