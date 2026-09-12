package interpack

// Decoder accepts arbitrary serial chunks and emits complete, CRC-checked
// telemetry and settings frames. Invalid bytes are skipped so later valid frames
// can be read. Retained buffer memory after Feed or FeedAll returns is naturally
// bounded to at most a partial frame.
type Decoder struct {
	buffer []byte
}

// Feed appends bytes from the serial port and returns every complete telemetry frame.
func (d *Decoder) Feed(chunk []byte) []Frame {
	telemetry, _ := d.FeedAll(chunk)
	return telemetry
}

// FeedAll appends bytes from the serial port and returns all complete telemetry
// and settings frames decoded from the stream.
func (d *Decoder) FeedAll(chunk []byte) ([]Frame, []SettingsFrame) {
	d.buffer = append(d.buffer, chunk...)
	var telemetry []Frame
	var settings []SettingsFrame

	for len(d.buffer) > 0 {
		start := -1
		isSettings := false
		targetSize := 0

		// Scan for next candidate header
		for i := 0; i < len(d.buffer); i++ {
			if hasTelemetryHeader(d.buffer[i:]) {
				start = i
				isSettings = false
				targetSize = FrameSize
				break
			}
			if totalLen, ok := matchSettingsHeader(d.buffer[i:]); ok {
				start = i
				isSettings = true
				targetSize = totalLen
				break
			}
		}

		if start < 0 {
			// No header candidate found. Keep at most 9 bytes across serial reads.
			if len(d.buffer) > 9 {
				d.buffer = d.buffer[len(d.buffer)-9:]
			}
			if len(d.buffer) > 0 {
				d.buffer = append([]byte(nil), d.buffer...)
			} else {
				d.buffer = nil
			}
			return telemetry, settings
		}

		// Discard preceding unaligned bytes
		d.buffer = d.buffer[start:]

		if len(d.buffer) < targetSize {
			// Partial frame: preserve buffer and await more bytes
			d.buffer = append([]byte(nil), d.buffer...)
			return telemetry, settings
		}

		if !isSettings {
			frame, err := ParseFrame(d.buffer[:targetSize])
			if err != nil {
				d.buffer = d.buffer[1:]
				continue
			}
			telemetry = append(telemetry, frame)
			d.buffer = d.buffer[targetSize:]
		} else {
			sframe, err := ParseSettingsFrame(d.buffer[:targetSize])
			if err != nil {
				d.buffer = d.buffer[1:]
				continue
			}
			settings = append(settings, sframe)
			d.buffer = d.buffer[targetSize:]
		}
	}

	return telemetry, settings
}
