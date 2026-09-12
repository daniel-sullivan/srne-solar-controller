package interpack

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestBalanceFrame(address uint8, startMV, deltaMV, fullChargeCV uint16) []byte {
	payloadLen := BalancePayloadLength
	totalLen := 8 + payloadLen + 2
	data := make([]byte, totalLen)
	data[0] = address
	data[1] = FunctionCodeSettings
	binary.BigEndian.PutUint16(data[2:4], RegBalanceStart)
	binary.BigEndian.PutUint16(data[4:6], RegBalanceEnd)
	binary.BigEndian.PutUint16(data[6:8], uint16(payloadLen))

	// Payload
	binary.BigEndian.PutUint16(data[8+4:8+6], startMV)
	binary.BigEndian.PutUint16(data[8+6:8+8], deltaMV)
	binary.BigEndian.PutUint16(data[8+12:8+14], fullChargeCV)

	// CRC-16 Modbus LE
	crc := CRC16(data[:totalLen-2])
	binary.LittleEndian.PutUint16(data[totalLen-2:], crc)
	return data
}

func makeTestProtectionFrame(address uint8, cellAlarmMV, cellProtMV, packAlarmCV, packProtCV uint16) []byte {
	payloadLen := ProtectionPayloadLength
	totalLen := 8 + payloadLen + 2
	data := make([]byte, totalLen)
	data[0] = address
	data[1] = FunctionCodeSettings
	binary.BigEndian.PutUint16(data[2:4], RegProtectionStart)
	binary.BigEndian.PutUint16(data[4:6], RegProtectionEnd)
	binary.BigEndian.PutUint16(data[6:8], uint16(payloadLen))

	// Payload
	binary.BigEndian.PutUint16(data[8+0:8+2], cellAlarmMV)
	binary.BigEndian.PutUint16(data[8+6:8+8], cellProtMV)
	binary.BigEndian.PutUint16(data[8+24:8+26], packAlarmCV)
	binary.BigEndian.PutUint16(data[8+30:8+32], packProtCV)

	// CRC-16 Modbus LE
	crc := CRC16(data[:totalLen-2])
	binary.LittleEndian.PutUint16(data[totalLen-2:], crc)
	return data
}

func TestBuildSettingsRequest(t *testing.T) {
	reqBal := BalanceSettingsRequest(2)
	assert.Equal(t, uint8(2), reqBal[0])
	assert.Equal(t, uint8(0x78), reqBal[1])
	assert.Equal(t, uint16(0x1C00), binary.BigEndian.Uint16(reqBal[2:4]))
	assert.Equal(t, uint16(0x1CA0), binary.BigEndian.Uint16(reqBal[4:6]))
	assert.Equal(t, uint16(0), binary.BigEndian.Uint16(reqBal[6:8]))
	expectedBalCRC := CRC16(reqBal[:8])
	assert.Equal(t, expectedBalCRC, binary.LittleEndian.Uint16(reqBal[8:10]))

	reqProt := ProtectionSettingsRequest(7)
	assert.Equal(t, uint8(7), reqProt[0])
	assert.Equal(t, uint8(0x78), reqProt[1])
	assert.Equal(t, uint16(0x1800), binary.BigEndian.Uint16(reqProt[2:4]))
	assert.Equal(t, uint16(0x1900), binary.BigEndian.Uint16(reqProt[4:6]))
	expectedProtCRC := CRC16(reqProt[:8])
	assert.Equal(t, expectedProtCRC, binary.LittleEndian.Uint16(reqProt[8:10]))
}

func TestParseSettingsFrame_Balance(t *testing.T) {
	raw := makeTestBalanceFrame(3, 3400, 30, 5680)
	frame, err := ParseSettingsFrame(raw)
	require.NoError(t, err)

	assert.Equal(t, uint8(3), frame.Address)
	assert.Equal(t, BlockTypeBalance, frame.BlockType)
	assert.Equal(t, uint16(RegBalanceStart), frame.Start)
	assert.Equal(t, uint16(RegBalanceEnd), frame.End)
	assert.Equal(t, uint16(BalancePayloadLength), frame.PayloadLen)

	require.NotNil(t, frame.Balance)
	assert.Equal(t, uint16(3400), frame.Balance.BalanceStartMillivolts)
	assert.Equal(t, uint16(30), frame.Balance.BalanceDeltaMillivolts)
	assert.Equal(t, uint16(5680), frame.Balance.FullChargeCentivolts)
	assert.InDelta(t, 56.80, frame.Balance.FullChargeVolts, 0.001)
	assert.Nil(t, frame.Protection)
}

func TestParseSettingsFrame_Protection(t *testing.T) {
	raw := makeTestProtectionFrame(5, 3600, 3650, 5760, 5840)
	frame, err := ParseSettingsFrame(raw)
	require.NoError(t, err)

	assert.Equal(t, uint8(5), frame.Address)
	assert.Equal(t, BlockTypeProtection, frame.BlockType)
	assert.Equal(t, uint16(RegProtectionStart), frame.Start)
	assert.Equal(t, uint16(RegProtectionEnd), frame.End)
	assert.Equal(t, uint16(ProtectionPayloadLength), frame.PayloadLen)

	require.NotNil(t, frame.Protection)
	assert.Equal(t, uint16(3600), frame.Protection.CellOVPAlarmMillivolts)
	assert.Equal(t, uint16(3650), frame.Protection.CellOVPProtectionMillivolts)
	assert.Equal(t, uint16(5760), frame.Protection.PackOVPAlarmCentivolts)
	assert.Equal(t, uint16(5840), frame.Protection.PackOVPProtectionCentivolts)
	assert.InDelta(t, 57.60, frame.Protection.PackOVPAlarmVolts, 0.001)
	assert.InDelta(t, 58.40, frame.Protection.PackOVPProtectionVolts, 0.001)
	assert.Nil(t, frame.Balance)
}

func TestParseSettingsFrame_RejectsCorruption(t *testing.T) {
	raw := makeTestBalanceFrame(1, 3400, 30, 5680)

	// Short frame
	_, err := ParseSettingsFrame(raw[:9])
	assert.ErrorIs(t, err, ErrSettingsFrameSize)

	// Corrupted command code
	badCmd := append([]byte(nil), raw...)
	badCmd[1] = 0x45
	_, err = ParseSettingsFrame(badCmd)
	assert.ErrorIs(t, err, ErrSettingsHeader)

	// Truncated payload vs declared length
	badLen := append([]byte(nil), raw...)
	_, err = ParseSettingsFrame(badLen[:len(badLen)-5])
	assert.ErrorIs(t, err, ErrSettingsFrameSize)

	// Corrupted CRC
	badCRC := append([]byte(nil), raw...)
	badCRC[len(badCRC)-2] ^= 0x55
	_, err = ParseSettingsFrame(badCRC)
	assert.ErrorIs(t, err, ErrCRC)

	// Unsupported register block
	badBlock := append([]byte(nil), raw...)
	binary.BigEndian.PutUint16(badBlock[2:4], 0x2000)
	binary.BigEndian.PutUint16(badBlock[4:6], 0x2100)
	binary.LittleEndian.PutUint16(badBlock[len(badBlock)-2:], CRC16(badBlock[:len(badBlock)-2]))
	_, err = ParseSettingsFrame(badBlock)
	assert.ErrorIs(t, err, ErrSettingsBlock)

	// Out of range balance start voltage
	badVal := append([]byte(nil), raw...)
	binary.BigEndian.PutUint16(badVal[8+4:8+6], 100) // 100 mV is invalid for LiFePO4
	binary.LittleEndian.PutUint16(badVal[len(badVal)-2:], CRC16(badVal[:len(badVal)-2]))
	_, err = ParseSettingsFrame(badVal)
	assert.ErrorIs(t, err, ErrSettingsRange)
}

func TestDecoder_MixedTelemetryAndSettings(t *testing.T) {
	var decoder Decoder

	fTele1 := testFrame(1)
	fBal := makeTestBalanceFrame(1, 3400, 30, 5680)
	fProt := makeTestProtectionFrame(1, 3600, 3650, 5760, 5840)
	fTele2 := testFrame(2)

	// Stream with arbitrary noise interspersed
	stream := append([]byte{0xDE, 0xAD}, fTele1...)
	stream = append(stream, []byte{0x00, 0xFF}...)
	stream = append(stream, fBal...)
	stream = append(stream, []byte{0x45}...) // lone byte resembling 0x45
	stream = append(stream, fProt...)
	stream = append(stream, []byte{0x78, 0x01}...) // partial header noise
	stream = append(stream, fTele2...)

	// Feed one byte at a time to test fragmentation across chunks
	var allTele []Frame
	var allSettings []SettingsFrame
	for _, b := range stream {
		tFrames, sFrames := decoder.FeedAll([]byte{b})
		allTele = append(allTele, tFrames...)
		allSettings = append(allSettings, sFrames...)
	}

	require.Len(t, allTele, 2)
	assert.Equal(t, uint8(1), allTele[0].Address)
	assert.Equal(t, uint8(2), allTele[1].Address)

	require.Len(t, allSettings, 2)
	assert.Equal(t, BlockTypeBalance, allSettings[0].BlockType)
	assert.Equal(t, uint16(3400), allSettings[0].Balance.BalanceStartMillivolts)
	assert.Equal(t, BlockTypeProtection, allSettings[1].BlockType)
	assert.Equal(t, uint16(3600), allSettings[1].Protection.CellOVPAlarmMillivolts)
}

func TestDecoder_ResynchronizationAfterCorruptedSettings(t *testing.T) {
	var decoder Decoder

	badSettings := makeTestBalanceFrame(3, 3400, 30, 5680)
	badSettings[len(badSettings)-2] ^= 0xFF // corrupt CRC
	goodSettings := makeTestProtectionFrame(3, 3600, 3650, 5760, 5840)
	goodTele := testFrame(3)
	stream := append([]byte(nil), badSettings...)
	stream = append(stream, goodSettings...)
	stream = append(stream, goodTele...)

	tele, settings := decoder.FeedAll(stream)
	require.Len(t, settings, 1)
	assert.Equal(t, BlockTypeProtection, settings[0].BlockType)
	require.Len(t, tele, 1)
	assert.Equal(t, uint8(3), tele[0].Address)
}

func TestStoreSettings_FreshUnavailableStale(t *testing.T) {
	store := &Store{}
	now := time.Now()

	// 1. Pack with telemetry only: settings should be unavailable
	store.Record(Frame{Address: 1, PackVoltageCentivolts: 5350}, now)
	snaps := store.Snapshot(now, 60*time.Second)
	require.Len(t, snaps, 1)
	require.NotNil(t, snaps[0].Settings)
	assert.Equal(t, SettingsUnavailable, snaps[0].Settings.Status)
	assert.False(t, snaps[0].Settings.Fresh)

	// 2. Record balance settings only: still unavailable (incomplete)
	balFrame, err := ParseSettingsFrame(makeTestBalanceFrame(1, 3400, 30, 5680))
	require.NoError(t, err)
	store.RecordSettings(balFrame, now)
	snaps = store.Snapshot(now, 60*time.Second)
	require.Len(t, snaps, 1)
	assert.Equal(t, SettingsUnavailable, snaps[0].Settings.Status, "partial settings block must be reported as unavailable")
	assert.False(t, snaps[0].Settings.Fresh)

	// 3. Record protection settings: both blocks present, within age -> fresh
	protFrame, err := ParseSettingsFrame(makeTestProtectionFrame(1, 3600, 3650, 5760, 5840))
	require.NoError(t, err)
	store.RecordSettings(protFrame, now)
	snaps = store.Snapshot(now, 60*time.Second)
	require.Len(t, snaps, 1)
	assert.Equal(t, SettingsFresh, snaps[0].Settings.Status)
	assert.True(t, snaps[0].Settings.Fresh)
	assert.Equal(t, uint16(3400), snaps[0].Settings.Balance.BalanceStartMillivolts)
	assert.Equal(t, uint16(3600), snaps[0].Settings.Protection.CellOVPAlarmMillivolts)

	// 4. Advance time past settingsMaxAge (e.g. 4 minutes later with default 3m settings freshness): becomes stale
	later := now.Add(4 * time.Minute)
	snapsLater := store.Snapshot(later, 60*time.Second)
	require.Len(t, snapsLater, 1)
	assert.Equal(t, SettingsStale, snapsLater[0].Settings.Status)
	assert.False(t, snapsLater[0].Settings.Fresh)

	// 5. Re-recording settings restores freshness
	store.RecordSettings(balFrame, later)
	store.RecordSettings(protFrame, later)
	snapsRestored := store.Snapshot(later, 60*time.Second)
	require.Len(t, snapsRestored, 1)
	assert.Equal(t, SettingsFresh, snapsRestored[0].Settings.Status)
	assert.True(t, snapsRestored[0].Settings.Fresh)
}

func TestStoreSettings_DistinguishUnavailableFromStale(t *testing.T) {
	store := &Store{}
	now := time.Now()

	// Pack 1: Fresh settings
	store.Record(Frame{Address: 1}, now)
	bal1, _ := ParseSettingsFrame(makeTestBalanceFrame(1, 3400, 30, 5680))
	prot1, _ := ParseSettingsFrame(makeTestProtectionFrame(1, 3600, 3650, 5760, 5840))
	store.RecordSettings(bal1, now)
	store.RecordSettings(prot1, now)

	// Pack 2: Stale settings (recorded 4 minutes ago, exceeding 3m settings freshness)
	pastTime := now.Add(-4 * time.Minute)
	store.Record(Frame{Address: 2}, now)
	bal2, _ := ParseSettingsFrame(makeTestBalanceFrame(2, 3400, 30, 5680))
	prot2, _ := ParseSettingsFrame(makeTestProtectionFrame(2, 3600, 3650, 5760, 5840))
	store.RecordSettings(bal2, pastTime)
	store.RecordSettings(prot2, pastTime)

	// Pack 3: Unavailable settings (never fetched)
	store.Record(Frame{Address: 3}, now)

	snaps := store.Snapshot(now, 60*time.Second)
	require.Len(t, snaps, 3)

	assert.Equal(t, SettingsFresh, snaps[0].Settings.Status)
	assert.True(t, snaps[0].Settings.Fresh)

	assert.Equal(t, SettingsStale, snaps[1].Settings.Status)
	assert.False(t, snaps[1].Settings.Fresh)

	assert.Equal(t, SettingsUnavailable, snaps[2].Settings.Status)
	assert.False(t, snaps[2].Settings.Fresh)
}

func TestParseSettingsFrame_RejectsArbitraryShortLengths(t *testing.T) {
	// Construct a frame with declared payload length 50 (shorter than verified 134/136)
	totalLen := 8 + 50 + 2
	shortBal := make([]byte, totalLen)
	shortBal[0] = 1
	shortBal[1] = FunctionCodeSettings
	binary.BigEndian.PutUint16(shortBal[2:4], RegBalanceStart)
	binary.BigEndian.PutUint16(shortBal[4:6], RegBalanceEnd)
	binary.BigEndian.PutUint16(shortBal[6:8], 50)
	binary.BigEndian.PutUint16(shortBal[8+4:8+6], 3400)
	binary.BigEndian.PutUint16(shortBal[8+6:8+8], 30)
	binary.BigEndian.PutUint16(shortBal[8+12:8+14], 5680)
	binary.LittleEndian.PutUint16(shortBal[totalLen-2:], CRC16(shortBal[:totalLen-2]))

	_, err := ParseSettingsFrame(shortBal)
	assert.ErrorIs(t, err, ErrSettingsFrameSize, "must reject arbitrary shorter payload lengths")

	// Construct protection frame with declared payload length 100 (shorter than verified 208)
	totalProtLen := 8 + 100 + 2
	shortProt := make([]byte, totalProtLen)
	shortProt[0] = 1
	shortProt[1] = FunctionCodeSettings
	binary.BigEndian.PutUint16(shortProt[2:4], RegProtectionStart)
	binary.BigEndian.PutUint16(shortProt[4:6], RegProtectionEnd)
	binary.BigEndian.PutUint16(shortProt[6:8], 100)
	binary.LittleEndian.PutUint16(shortProt[totalProtLen-2:], CRC16(shortProt[:totalProtLen-2]))

	_, err = ParseSettingsFrame(shortProt)
	assert.ErrorIs(t, err, ErrSettingsFrameSize, "must reject arbitrary shorter protection payload lengths")

	// Verify valid 136-byte reference variant is accepted and offsets are stable
	total136Len := 8 + 136 + 2
	var136 := make([]byte, total136Len)
	var136[0] = 2
	var136[1] = FunctionCodeSettings
	binary.BigEndian.PutUint16(var136[2:4], RegBalanceStart)
	binary.BigEndian.PutUint16(var136[4:6], RegBalanceEnd)
	binary.BigEndian.PutUint16(var136[6:8], 136)
	binary.BigEndian.PutUint16(var136[8+4:8+6], 3400)
	binary.BigEndian.PutUint16(var136[8+6:8+8], 30)
	binary.BigEndian.PutUint16(var136[8+12:8+14], 5680)
	binary.LittleEndian.PutUint16(var136[total136Len-2:], CRC16(var136[:total136Len-2]))

	frame136, err := ParseSettingsFrame(var136)
	require.NoError(t, err)
	assert.Equal(t, uint16(3400), frame136.Balance.BalanceStartMillivolts)
	assert.Equal(t, uint16(30), frame136.Balance.BalanceDeltaMillivolts)
	assert.Equal(t, uint16(5680), frame136.Balance.FullChargeCentivolts)
}

func TestStoreSettings_IndependentBlockFreshness_OneOldOneNew(t *testing.T) {
	store := &Store{}
	now := time.Now()

	store.Record(Frame{Address: 1}, now)

	balFrame, err := ParseSettingsFrame(makeTestBalanceFrame(1, 3400, 30, 5680))
	require.NoError(t, err)
	protFrame, err := ParseSettingsFrame(makeTestProtectionFrame(1, 3600, 3650, 5760, 5840))
	require.NoError(t, err)

	// Case A: Balance is fresh (30s old), Protection is stale (4 minutes old > 3m settings bound)
	store.RecordSettings(balFrame, now.Add(-30*time.Second))
	store.RecordSettings(protFrame, now.Add(-4*time.Minute))

	snaps := store.Snapshot(now, 60*time.Second) // 60s telemetry, default 3m settings
	require.Len(t, snaps, 1)
	assert.Equal(t, SettingsStale, snaps[0].Settings.Status, "stale protection block must not be masked by fresh balance block")
	assert.False(t, snaps[0].Settings.Fresh)

	// Case B: Protection is fresh (30s old), Balance is stale (4 minutes old)
	store.RecordSettings(balFrame, now.Add(-4*time.Minute))
	store.RecordSettings(protFrame, now.Add(-30*time.Second))

	snaps = store.Snapshot(now, 60*time.Second)
	require.Len(t, snaps, 1)
	assert.Equal(t, SettingsStale, snaps[0].Settings.Status, "stale balance block must not be masked by fresh protection block")
	assert.False(t, snaps[0].Settings.Fresh)

	// Case C: Both blocks fresh (< 3 minutes)
	store.RecordSettings(balFrame, now.Add(-30*time.Second))
	store.RecordSettings(protFrame, now.Add(-30*time.Second))

	snaps = store.Snapshot(now, 60*time.Second)
	require.Len(t, snaps, 1)
	assert.Equal(t, SettingsFresh, snaps[0].Settings.Status)
	assert.True(t, snaps[0].Settings.Fresh)
}

func TestStoreSettings_SettingsBeforeTelemetryDoesNotCreatePack(t *testing.T) {
	store := &Store{}
	now := time.Now()

	// Settings arrive for address 5 before any telemetry frame has arrived
	balFrame, err := ParseSettingsFrame(makeTestBalanceFrame(5, 3400, 30, 5680))
	require.NoError(t, err)
	protFrame, err := ParseSettingsFrame(makeTestProtectionFrame(5, 3600, 3650, 5760, 5840))
	require.NoError(t, err)

	store.RecordSettings(balFrame, now)
	store.RecordSettings(protFrame, now)

	// Snapshot must NOT include address 5 because no CRC-valid 0x45 telemetry report was received
	snaps := store.Snapshot(now, 60*time.Second)
	assert.Empty(t, snaps, "snapshot must only contain real telemetry packs, never synthetic settings placeholders")

	// Verify settings are preserved independently in store
	ps, ok := store.GetSettings(5)
	require.True(t, ok)
	assert.Equal(t, uint16(3400), ps.Balance.BalanceStartMillivolts)

	// Now a real telemetry frame arrives for address 5
	store.Record(Frame{Address: 5, PackVoltageCentivolts: 5350}, now)
	snapsAfter := store.Snapshot(now, 60*time.Second)
	require.Len(t, snapsAfter, 1)
	assert.Equal(t, uint8(5), snapsAfter[0].Frame.Address)
	assert.Equal(t, SettingsFresh, snapsAfter[0].Settings.Status)
}

func TestStore_DistinctBMSIdentities_AntiAliasing(t *testing.T) {
	store := &Store{}
	now := time.Now()

	// Pack 1 with ID "TEST-PACK-1"
	store.Record(Frame{Address: 1}, now)
	rawBal1 := makeTestBalanceFrame(1, 3400, 30, 5680)
	copy(rawBal1[8+16:8+46], []byte("TEST-PACK-1"))
	binary.LittleEndian.PutUint16(rawBal1[len(rawBal1)-2:], CRC16(rawBal1[:len(rawBal1)-2]))
	bal1, err := ParseSettingsFrame(rawBal1)
	require.NoError(t, err)
	store.RecordSettings(bal1, now)

	// Pack 2 with ID "TEST-PACK-2"
	store.Record(Frame{Address: 2}, now)
	rawBal2 := makeTestBalanceFrame(2, 3400, 30, 5680)
	copy(rawBal2[8+16:8+46], []byte("TEST-PACK-2"))
	binary.LittleEndian.PutUint16(rawBal2[len(rawBal2)-2:], CRC16(rawBal2[:len(rawBal2)-2]))
	bal2, err := ParseSettingsFrame(rawBal2)
	require.NoError(t, err)
	store.RecordSettings(bal2, now)

	count, hasDups := store.DistinctBMSIdentities(now, 60*time.Second)
	assert.Equal(t, 2, count)
	assert.False(t, hasDups)

	// Address 0 reports same ID as Pack 1 (duplicate / alias)
	f0 := Frame{Address: 0}
	store.Record(f0, now.Add(-time.Second))
	store.Record(f0, now) // confirmed address 0
	bal0 := bal1
	bal0.Address = 0
	store.RecordSettings(bal0, now)

	countWithDup, hasDupsNow := store.DistinctBMSIdentities(now, 60*time.Second)
	assert.Equal(t, 2, countWithDup, "duplicate BMS ID across different addresses must not count twice")
	assert.True(t, hasDupsNow, "duplicate BMS ID must be detected")
}
