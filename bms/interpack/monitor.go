package interpack

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"go.bug.st/serial"
)

const (
	serialBaudRate   = 19200
	readTimeout      = 50 * time.Millisecond
	pollInterval     = 15 * time.Second
	probeGap         = 150 * time.Millisecond
	probeTimeout     = 250 * time.Millisecond
	quietDuration    = 30 * time.Millisecond
	initialQuiet     = 500 * time.Millisecond
	maxProbeRetry    = 2
	maxSettingsRetry = 1
	settingsInterval = 60 * time.Second
)

var reconnectDelay = 5 * time.Second

// DefaultCandidateAddresses lists candidate addresses in a 10-pack bank.
var DefaultCandidateAddresses = []uint8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

type serialPort interface {
	io.ReadWriteCloser
	SetReadTimeout(time.Duration) error
}

func writeFull(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
		b = b[n:]
	}
	return nil
}

// PollFrame builds the read-only inter-pack telemetry request for an address.
// The master polls slave addresses 2-15 itself; callers only need to request
// addresses 1 and 0, which are absent from the normal polling cycle.
func PollFrame(address uint8) [10]byte {
	request := [10]byte{address, 0x45, 0, 0, 0, 0x54, 0, 0}
	binary.LittleEndian.PutUint16(request[8:], CRC16(request[:8]))
	return request
}

// Monitor reads the inter-pack serial bus and calls onFrame for each valid
// telemetry reply, and optional onSettings for verified settings replies.
// It periodically polls addresses 1 and 0 for telemetry, and schedules read-only
// 0x78 settings queries with collision-conscious quiet gaps.
func Monitor(ctx context.Context, path string, onFrame func(Frame, time.Time), onSettings ...func(SettingsFrame, time.Time)) error {
	port, err := serial.Open(path, &serial.Mode{
		BaudRate: serialBaudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return fmt.Errorf("open inter-pack serial port %q: %w", path, err)
	}
	defer func() { _ = port.Close() }()

	return monitorPort(ctx, port, onFrame, onSettings...)
}

type probeState int

const (
	probeStateIdle probeState = iota
	probeStateAddr1
	probeStateWaitAddr1
	probeStateGap
	probeStateAddr0
	probeStateWaitAddr0
	probeStateSettingsGap
	probeStateSettingsTx
	probeStateSettingsWait
)

type settingsTask struct {
	address   uint8
	start     uint16
	end       uint16
	blockType SettingsBlockType
}

func enqueueSettingsForAddress(queue []settingsTask, addr uint8) []settingsTask {
	// Add balance block query
	queue = append(queue, settingsTask{
		address:   addr,
		start:     RegBalanceStart,
		end:       RegBalanceEnd,
		blockType: BlockTypeBalance,
	})
	// Add protection block query
	queue = append(queue, settingsTask{
		address:   addr,
		start:     RegProtectionStart,
		end:       RegProtectionEnd,
		blockType: BlockTypeProtection,
	})
	return queue
}

func monitorPort(ctx context.Context, port serialPort, onFrame func(Frame, time.Time), onSettings ...func(SettingsFrame, time.Time)) error {
	if err := port.SetReadTimeout(readTimeout); err != nil {
		return fmt.Errorf("set inter-pack serial read timeout: %w", err)
	}

	stop := context.AfterFunc(ctx, func() {
		_ = port.Close()
	})
	defer stop()

	var handleSettings func(SettingsFrame, time.Time)
	if len(onSettings) > 0 && onSettings[0] != nil {
		handleSettings = onSettings[0]
	}

	var decoder Decoder
	buffer := make([]byte, 512)

	state := probeStateIdle
	now := time.Now()
	nextTelemetryCycle := now.Add(initialQuiet)
	nextSettingsCycle := now.Add(initialQuiet + 2*time.Second)
	telemetryPolled := false

	var lastRxTime time.Time
	var probeSentAt time.Time
	var gapUntil time.Time
	addr1Replied := false
	addr0Replied := false
	addr0Retries := 0

	var settingsQueue []settingsTask
	var currentTask settingsTask
	settingsRetries := 0
	settingsReplied := false

	// Initialize settings queue with default candidate addresses
	for _, addr := range DefaultCandidateAddresses {
		settingsQueue = enqueueSettingsForAddress(settingsQueue, addr)
	}

	observedAddresses := make(map[uint8]bool)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		now = time.Now()

		// Periodic re-enqueuing of settings refresh
		if now.After(nextSettingsCycle) && len(settingsQueue) == 0 {
			for _, addr := range DefaultCandidateAddresses {
				settingsQueue = enqueueSettingsForAddress(settingsQueue, addr)
			}
			for addr := range observedAddresses {
				found := false
				for _, cand := range DefaultCandidateAddresses {
					if cand == addr {
						found = true
						break
					}
				}
				if !found {
					settingsQueue = enqueueSettingsForAddress(settingsQueue, addr)
				}
			}
			nextSettingsCycle = now.Add(settingsInterval)
		}

		switch state {
		case probeStateIdle:
			if !now.Before(nextTelemetryCycle) {
				state = probeStateAddr1
				addr1Replied = false
			} else if telemetryPolled && len(settingsQueue) > 0 && !now.Before(gapUntil) {
				state = probeStateSettingsGap
			}

		case probeStateAddr1:
			if now.Sub(lastRxTime) >= quietDuration {
				req := PollFrame(1)
				if err := writeFull(port, req[:]); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					return fmt.Errorf("poll inter-pack address 1: %w", err)
				}
				probeSentAt = now
				state = probeStateWaitAddr1
			}

		case probeStateWaitAddr1:
			if addr1Replied || now.Sub(probeSentAt) >= probeTimeout {
				gapUntil = now.Add(probeGap)
				state = probeStateGap
			}

		case probeStateGap:
			if !now.Before(gapUntil) {
				state = probeStateAddr0
				addr0Replied = false
				addr0Retries = 0
			}

		case probeStateAddr0:
			if now.Sub(lastRxTime) >= quietDuration {
				req := PollFrame(0)
				if err := writeFull(port, req[:]); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					return fmt.Errorf("poll inter-pack address 0: %w", err)
				}
				probeSentAt = now
				state = probeStateWaitAddr0
			}

		case probeStateWaitAddr0:
			if addr0Replied {
				telemetryPolled = true
				nextTelemetryCycle = now.Add(pollInterval)
				gapUntil = now.Add(probeGap)
				state = probeStateSettingsGap
			} else if now.Sub(probeSentAt) >= probeTimeout {
				if addr0Retries < maxProbeRetry {
					addr0Retries++
					state = probeStateAddr0
				} else {
					telemetryPolled = true
					nextTelemetryCycle = now.Add(pollInterval)
					gapUntil = now.Add(probeGap)
					state = probeStateSettingsGap
				}
			}

		case probeStateSettingsGap:
			if !now.Before(nextTelemetryCycle) {
				state = probeStateAddr1
				addr1Replied = false
			} else if !now.Before(gapUntil) {
				if len(settingsQueue) > 0 {
					currentTask = settingsQueue[0]
					settingsRetries = 0
					settingsReplied = false
					state = probeStateSettingsTx
				} else {
					state = probeStateIdle
				}
			}

		case probeStateSettingsTx:
			if !now.Before(nextTelemetryCycle) {
				// Telemetry poll takes precedence
				state = probeStateAddr1
				addr1Replied = false
			} else if now.Sub(lastRxTime) >= quietDuration {
				req := BuildSettingsRequest(currentTask.address, currentTask.start, currentTask.end)
				if err := writeFull(port, req[:]); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					return fmt.Errorf("query settings addr %d (0x%04x): %w", currentTask.address, currentTask.start, err)
				}
				probeSentAt = now
				state = probeStateSettingsWait
			}

		case probeStateSettingsWait:
			if settingsReplied {
				settingsQueue = settingsQueue[1:]
				gapUntil = now.Add(probeGap)
				state = probeStateSettingsGap
			} else if now.Sub(probeSentAt) >= probeTimeout {
				if settingsRetries < maxSettingsRetry {
					settingsRetries++
					state = probeStateSettingsTx
				} else {
					// Drop task after bounded retries and advance
					settingsQueue = settingsQueue[1:]
					gapUntil = now.Add(probeGap)
					state = probeStateSettingsGap
				}
			}
		}

		n, err := port.Read(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("inter-pack serial port closed")
			}
			return fmt.Errorf("read inter-pack serial port: %w", err)
		}
		if n > 0 {
			lastRxTime = time.Now()
			telemetryFrames, settingsFrames := decoder.FeedAll(buffer[:n])

			for _, frame := range telemetryFrames {
				if frame.Address == 1 && state == probeStateWaitAddr1 {
					addr1Replied = true
				}
				if frame.Address == 0 && state == probeStateWaitAddr0 {
					addr0Replied = true
				}
				if !observedAddresses[frame.Address] {
					observedAddresses[frame.Address] = true
					// Queue settings query for newly observed address if not already in candidates
					found := false
					for _, cand := range DefaultCandidateAddresses {
						if cand == frame.Address {
							found = true
							break
						}
					}
					if !found {
						settingsQueue = enqueueSettingsForAddress(settingsQueue, frame.Address)
					}
				}
				if onFrame != nil {
					onFrame(frame, time.Now())
				}
			}

			for _, sframe := range settingsFrames {
				if state == probeStateSettingsWait &&
					sframe.Address == currentTask.address &&
					sframe.Start == currentTask.start &&
					sframe.End == currentTask.end {
					settingsReplied = true
				}
				if handleSettings != nil {
					handleSettings(sframe, time.Now())
				}
			}
		}
	}
}

// Run reconnects to a serial adapter that may be absent during startup or
// temporarily unplugged. Loss of telemetry is visible through snapshot age.
func Run(ctx context.Context, path string, onFrame func(Frame, time.Time), onSettings ...func(SettingsFrame, time.Time)) {
	for ctx.Err() == nil {
		if err := Monitor(ctx, path, onFrame, onSettings...); err != nil && ctx.Err() == nil {
			slog.Warn("inter-pack monitor disconnected", "path", path, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}
