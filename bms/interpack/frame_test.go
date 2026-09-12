package interpack

import (
	"encoding/binary"
	"errors"
	"testing"
)

func testFrame(address byte) []byte {
	data := make([]byte, FrameSize)
	data[0] = address
	copy(data[1:], telemetryHeader[:])
	for i := 0; i < 16; i++ {
		binary.BigEndian.PutUint16(data[10+2*i:], uint16(3400+i))
	}
	binary.BigEndian.PutUint16(data[42:], 3415)
	binary.BigEndian.PutUint16(data[44:], 3400)
	binary.BigEndian.PutUint16(data[46:], 5460)
	copy(data[48:], []byte{0, 0, 4, 210})
	binary.BigEndian.PutUint16(data[52:], 3000)
	binary.BigEndian.PutUint16(data[54:], 2990)
	binary.BigEndian.PutUint16(data[56:], 10000)
	binary.BigEndian.PutUint16(data[58:], 9750)
	binary.BigEndian.PutUint16(data[60:], 149)
	binary.BigEndian.PutUint16(data[62:], 97)
	binary.BigEndian.PutUint16(data[64:], 100)
	copy(data[66:], []byte{0, 0, 0, 0, 0, 0x0b, 0, 0})
	binary.BigEndian.PutUint16(data[104:], 500)
	binary.BigEndian.PutUint16(data[106:], 1000)
	binary.LittleEndian.PutUint16(data[124:], CRC16(data[:124]))
	return data
}

func TestCRC16KnownVector(t *testing.T) {
	if got := CRC16([]byte("123456789")); got != 0x4b37 {
		t.Fatalf("CRC16 = 0x%04x, want 0x4b37", got)
	}
}

func TestParseFrame(t *testing.T) {
	frame, err := ParseFrame(testFrame(8))
	if err != nil {
		t.Fatal(err)
	}
	if frame.Address != 8 || frame.CellMillivolts[0] != 3400 || frame.CellMillivolts[15] != 3415 {
		t.Fatalf("wrong address or cell byte order: %+v", frame)
	}
	if frame.PackVoltageCentivolts != 5460 || frame.FullCapacityCentiAh != 10000 || frame.CycleCount != 149 || frame.SOCPercent != 97 {
		t.Fatalf("wrong word byte order: %+v", frame)
	}
	if frame.Unknown104 != 500 || frame.Unknown106 != 1000 || frame.StatusRaw[5] != 0x0b {
		t.Fatalf("raw fields were not preserved: %+v", frame)
	}
}

func TestParseFrameRejectsCorruption(t *testing.T) {
	data := testFrame(1)
	if _, err := ParseFrame(data[:125]); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("short frame error = %v", err)
	}
	badHeader := append([]byte(nil), data...)
	badHeader[7] = 0
	if _, err := ParseFrame(badHeader); !errors.Is(err, ErrHeader) {
		t.Fatalf("bad header error = %v", err)
	}
	badCRC := append([]byte(nil), data...)
	badCRC[20] ^= 0x01
	if _, err := ParseFrame(badCRC); !errors.Is(err, ErrCRC) {
		t.Fatalf("bad CRC error = %v", err)
	}
	badCell := append([]byte(nil), data...)
	binary.BigEndian.PutUint16(badCell[10:], 0)
	binary.LittleEndian.PutUint16(badCell[124:], CRC16(badCell[:124]))
	if _, err := ParseFrame(badCell); !errors.Is(err, ErrCells) {
		t.Fatalf("bad cell error = %v", err)
	}
}

func TestDecoderResynchronizesAcrossChunks(t *testing.T) {
	bad := testFrame(3)
	bad[30] ^= 1
	good := testFrame(9)
	stream := append([]byte{0x77, 0x99, 0}, bad...)
	stream = append(stream, 0x45, 0x01)
	stream = append(stream, good...)
	var decoder Decoder
	var frames []Frame
	for _, b := range stream {
		frames = append(frames, decoder.Feed([]byte{b})...)
	}
	if len(frames) != 1 || frames[0].Address != 9 {
		t.Fatalf("decoded frames = %+v, want only address 9", frames)
	}
	if got := decoder.Feed(testFrame(2)); len(got) != 1 || got[0].Address != 2 {
		t.Fatalf("decoder did not recover: %+v", got)
	}
}

func TestDecoderBoundsMemoryOnNoisyStream(t *testing.T) {
	var decoder Decoder
	// Feed a large amount of noise
	hugeNoise := make([]byte, 12288)
	for i := range hugeNoise {
		hugeNoise[i] = byte(i % 251)
	}
	frames := decoder.Feed(hugeNoise)
	if len(frames) != 0 {
		t.Fatalf("expected 0 frames from noise, got %d", len(frames))
	}
	if len(decoder.buffer) > 9 {
		t.Fatalf("retained buffer length = %d, want <= 9", len(decoder.buffer))
	}
	if cap(decoder.buffer) > FrameSize {
		t.Fatalf("retained buffer capacity = %d, want <= FrameSize (%d)", cap(decoder.buffer), FrameSize)
	}

	// Ensure decoder can still parse a valid frame after noise
	valid := testFrame(4)
	frames = decoder.Feed(valid)
	if len(frames) != 1 || frames[0].Address != 4 {
		t.Fatalf("decoder failed to parse valid frame after noise: %+v", frames)
	}
}

func TestDecoderLargeChunkPreservesFramesAtEnds(t *testing.T) {
	var decoder Decoder

	frameStart := testFrame(2)
	frameEnd := testFrame(9)

	// Construct a chunk significantly larger than 4096 bytes (8192 bytes)
	chunkSize := 8192
	largeChunk := make([]byte, chunkSize)

	// Place valid frame at the very beginning
	copy(largeChunk[0:], frameStart)

	// Fill middle with arbitrary noise (no headers)
	for i := len(frameStart); i < chunkSize-len(frameEnd); i++ {
		largeChunk[i] = 0xAA
	}

	// Place valid frame at the very end
	copy(largeChunk[chunkSize-len(frameEnd):], frameEnd)

	// Feed the entire large chunk in a single call
	frames := decoder.Feed(largeChunk)

	if len(frames) != 2 {
		t.Fatalf("expected 2 frames from large chunk, got %d", len(frames))
	}
	if frames[0].Address != 2 {
		t.Fatalf("first frame address = %d, want 2 (frame at beginning lost)", frames[0].Address)
	}
	if frames[1].Address != 9 {
		t.Fatalf("second frame address = %d, want 9 (frame at end lost)", frames[1].Address)
	}

	// Verify retained memory is bounded (at most a partial header since end frame was complete)
	if len(decoder.buffer) > 9 {
		t.Fatalf("retained buffer length %d exceeds partial header bound (9)", len(decoder.buffer))
	}
	if cap(decoder.buffer) > FrameSize {
		t.Fatalf("retained buffer capacity %d exceeds FrameSize bound (%d)", cap(decoder.buffer), FrameSize)
	}

	// Verify decoder can continue to process subsequent frames
	nextFrames := decoder.Feed(testFrame(3))
	if len(nextFrames) != 1 || nextFrames[0].Address != 3 {
		t.Fatalf("subsequent frame failed to decode: %+v", nextFrames)
	}
}

func TestParseFrameAllFields(t *testing.T) {
	raw := testFrame(5)
	// Populate specific fields
	binary.BigEndian.PutUint16(raw[108:], 0x1234)
	binary.BigEndian.PutUint16(raw[110:], 2951)
	binary.BigEndian.PutUint16(raw[112:], 2952)
	binary.BigEndian.PutUint16(raw[114:], 0x0402)
	binary.BigEndian.PutUint16(raw[116:], 2961)
	binary.BigEndian.PutUint16(raw[118:], 2962)
	binary.BigEndian.PutUint16(raw[120:], 2963)
	binary.BigEndian.PutUint16(raw[122:], 2964)
	copy(raw[74:86], []byte("BATCH1234567"))
	binary.LittleEndian.PutUint16(raw[124:], CRC16(raw[:124]))

	frame, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame failed: %v", err)
	}
	if frame.Address != 5 {
		t.Fatalf("frame.Address = %d, want 5", frame.Address)
	}
	if frame.Unknown108 != 0x1234 {
		t.Fatalf("Unknown108 = 0x%04x, want 0x1234", frame.Unknown108)
	}
	if frame.Temperatures2Decikelvin[0] != 2951 || frame.Temperatures2Decikelvin[1] != 2952 {
		t.Fatalf("Temperatures2 = %+v, want [2951 2952]", frame.Temperatures2Decikelvin)
	}
	if frame.Unknown114 != 0x0402 {
		t.Fatalf("Unknown114 = 0x%04x, want 0x0402", frame.Unknown114)
	}
	if frame.Temperatures4Decikelvin != [4]uint16{2961, 2962, 2963, 2964} {
		t.Fatalf("Temperatures4 = %+v, want [2961 2962 2963 2964]", frame.Temperatures4Decikelvin)
	}
	if string(frame.BatchRaw[:]) != "BATCH1234567" {
		t.Fatalf("BatchRaw = %q, want %q", string(frame.BatchRaw[:]), "BATCH1234567")
	}
}
