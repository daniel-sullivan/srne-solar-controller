package conditioning

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/bms/interpack"
)

// helper to build 9 valid packs meeting all operating contract requirements.
func buildMockValidPacks(now time.Time, baseCellMv uint16, spreadMv uint16) []interpack.Pack {
	packs := make([]interpack.Pack, len(ExpectedAddresses))
	for i, addr := range ExpectedAddresses {
		var cellMvs [16]uint16
		for c := 0; c < 16; c++ {
			cellMvs[c] = baseCellMv
		}
		if spreadMv > 0 {
			cellMvs[15] = baseCellMv + spreadMv
		}

		packs[i] = interpack.Pack{
			Frame: interpack.Frame{
				Address:        addr,
				CellMillivolts: cellMvs,
				Protection: interpack.ProtectionInfo{
					FaultMask:   0,
					WarningMask: 0,
					AlarmsClear: true,
				},
			},
			SeenAt:            now,
			Fresh:             true,
			Confirmed:         true, // Address 0 confirmed
			IdentityUncertain: addr == 0,
			Settings: &interpack.PackSettings{
				Address:          addr,
				Status:           interpack.SettingsFresh,
				Fresh:            true,
				SeenAt:           now,
				BalanceSeenAt:    now,
				ProtectionSeenAt: now,
				Balance: &interpack.BalanceSettings{
					BalanceStartMillivolts: ExpectedBalanceStartMillivolts,
					BalanceDeltaMillivolts: ExpectedBalanceDeltaMillivolts,
					FullChargeCentivolts:   5680,
					BMSID:                  fmt.Sprintf("UP16S-JBD-SERIAL-%02d", addr),
				},
				Protection: &interpack.ProtectionSettings{
					CellOVPAlarmMillivolts:      ExpectedCellOVPAlarmMillivolts,
					CellOVPProtectionMillivolts: ExpectedCellOVPProtectionMillivolts,
					PackOVPAlarmCentivolts:      ExpectedPackOVPAlarmCentivolts,
					PackOVPProtectionCentivolts: ExpectedPackOVPProtectionCentivolts,
				},
			},
		}
	}
	return packs
}

// helper to build a valid inverter telemetry packet with current and bank voltage.
func buildMockInverter(now time.Time, currentA, voltageV float64) InverterTelemetry {
	return InverterTelemetry{
		TotalChargeCurrentAmps: currentA,
		BankVoltageVolts:       voltageV,
		Timestamp:              now,
		Valid:                  true,
	}
}

// TestEngine_HappyPathFullCycle verifies progression through Stage 1 -> Stage 2 -> Stage 3 -> Completion.
func TestEngine_HappyPathFullCycle(t *testing.T) {
	engine := NewEngine()
	start := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)

	// Stage 1 Start: base cell 3350mV, spread 25mV (<= 30mV), inverter 15A at 54.40V
	packs := buildMockValidPacks(start, 3350, 25)
	inv := buildMockInverter(start, 15.0, 54.40)

	status := engine.Start(start, Input{Packs: packs, Inverter: inv})
	require.True(t, status.IsRunning())
	assert.Equal(t, Stage1, status.Stage)
	assert.Equal(t, Stage1VoltageVolts, status.TargetVoltageVolts)
	assert.Equal(t, TotalBankChargingCapAmps, status.TotalCurrentCapAmps)
	assert.Equal(t, PerInverterChargingCapAmps, status.PerInverterCurrentCapAmps)
	assert.Equal(t, 10, status.TotalConnectedPacks)
	assert.Equal(t, 9, status.ExpectedMonitoredPacks)
	assert.Equal(t, 9, status.ObservedMonitoredPacks)
	assert.Equal(t, 1, status.UnmonitoredPacks)
	assert.Equal(t, 54.40, status.BankVoltageVolts)
	assert.False(t, status.Terminal)

	// Hold in Stage 1 at t+15min (dwell 15min < 30min)
	t15 := start.Add(15 * time.Minute)
	packs15 := buildMockValidPacks(t15, 3360, 20)
	status = engine.Update(t15, Input{Packs: packs15, Inverter: buildMockInverter(t15, 14.0, 54.40)})
	assert.True(t, status.IsRunning())
	assert.Equal(t, Stage1, status.Stage)
	assert.Equal(t, Stage1VoltageVolts, status.TargetVoltageVolts)
	assert.Equal(t, 15*time.Minute, status.DwellDuration)

	// Advance to Stage 2 at t+30min (dwell >= 30min at 54.40V and spread <= 30mV)
	t30 := start.Add(30 * time.Minute)
	packs30 := buildMockValidPacks(t30, 3380, 22)
	status = engine.Update(t30, Input{Packs: packs30, Inverter: buildMockInverter(t30, 12.0, 54.40)})
	assert.True(t, status.IsRunning())
	assert.Equal(t, Stage2, status.Stage)
	assert.Equal(t, Stage2VoltageVolts, status.TargetVoltageVolts)

	// Ramp to 54.80V at t+31m in Stage 2 (dwell begins)
	t31 := start.Add(31 * time.Minute)
	status = engine.Update(t31, Input{Packs: packs30, Inverter: buildMockInverter(t31, 12.0, 54.80)})
	assert.Equal(t, Stage2, status.Stage)

	// Hold in Stage 2 at t+46min (15min dwell in Stage 2 at 54.80V)
	t46 := start.Add(46 * time.Minute)
	packs45 := buildMockValidPacks(t46, 3390, 18)
	status = engine.Update(t46, Input{Packs: packs45, Inverter: buildMockInverter(t46, 11.0, 54.80)})
	assert.Equal(t, Stage2, status.Stage)
	assert.Equal(t, Stage2VoltageVolts, status.TargetVoltageVolts)
	assert.Equal(t, 15*time.Minute, status.DwellDuration)

	// Advance to Stage 3 at t+61min (30min dwell in Stage 2 at 54.80V and spread <= 30mV)
	t61 := start.Add(61 * time.Minute)
	packs60 := buildMockValidPacks(t61, 3405, 15) // all cells >= 3400mV, spread 15mV <= 20mV
	status = engine.Update(t61, Input{Packs: packs60, Inverter: buildMockInverter(t61, 8.0, 54.80)})
	assert.True(t, status.IsRunning())
	assert.Equal(t, Stage3, status.Stage)
	assert.Equal(t, Stage3VoltageVolts, status.TargetVoltageVolts)

	// Ramp to 55.20V at t+62m in Stage 3 (tail qualification begins)
	t62 := start.Add(62 * time.Minute)
	status = engine.Update(t62, Input{Packs: packs60, Inverter: buildMockInverter(t62, 8.0, 55.20)})
	assert.Equal(t, Stage3, status.Stage)
	assert.Equal(t, time.Duration(0), status.TailDuration)

	// Stage 3 Tail qualifying at t+77min (15min of continuous tail qualification)
	t77 := start.Add(77 * time.Minute)
	packs75 := buildMockValidPacks(t77, 3410, 14)
	status = engine.Update(t77, Input{Packs: packs75, Inverter: buildMockInverter(t77, 7.5, 55.20)})
	assert.True(t, status.IsRunning())
	assert.Equal(t, Stage3, status.Stage)
	assert.Equal(t, 15*time.Minute, status.TailDuration)

	// Stage 3 Completion at t+92min (30 continuous minutes of tail qualification)
	t92 := start.Add(92 * time.Minute)
	packs90 := buildMockValidPacks(t92, 3415, 12)
	status = engine.Update(t92, Input{Packs: packs90, Inverter: buildMockInverter(t92, 6.0, 55.20)})
	assert.True(t, status.IsCompleted())
	assert.True(t, status.IsTerminal())
	assert.Equal(t, StateCompleted, status.State)
	assert.Equal(t, 0.0, status.TargetVoltageVolts)
	assert.Equal(t, 0.0, status.TotalCurrentCapAmps)
	assert.Equal(t, 0.0, status.PerInverterCurrentCapAmps)
	assert.Contains(t, status.Reason, "conditioning completed")
	assert.NoError(t, status.Err())
}

// TestEngine_StartIdempotent verifies that repeated Start calls while running do not fault or reset.
func TestEngine_StartIdempotent(t *testing.T) {
	engine := NewEngine()
	start := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)

	packs := buildMockValidPacks(start, 3350, 15)
	inv := buildMockInverter(start, 15.0, 54.40)

	status1 := engine.Start(start, Input{Packs: packs, Inverter: inv})
	require.True(t, status1.IsRunning())

	// Advance time 10 minutes and send another Start command (e.g. repeated HA ON action)
	t10 := start.Add(10 * time.Minute)
	packs10 := buildMockValidPacks(t10, 3360, 15)
	inv10 := buildMockInverter(t10, 14.0, 54.40)

	// Update first to establish state at t10
	engine.Update(t10, Input{Packs: packs10, Inverter: inv10})

	// Now send duplicate Start
	status2 := engine.Start(t10, Input{Packs: packs10, Inverter: inv10})
	assert.True(t, status2.IsRunning(), "duplicate Start must not fault")
	assert.Equal(t, Stage1, status2.Stage)
	assert.Equal(t, Stage1VoltageVolts, status2.TargetVoltageVolts)
	assert.Equal(t, 10*time.Minute, status2.Elapsed)
	assert.Equal(t, 10*time.Minute, status2.DwellDuration)
	assert.NoError(t, status2.Err())
}

// TestEngine_9vs8_Packs verifies failure when only 8 packs report at Start or during Update.
func TestEngine_9vs8_Packs(t *testing.T) {
	now := time.Now()

	// 1. Missing pack at Start (8 packs instead of 9)
	t.Run("MissingPackAtStart", func(t *testing.T) {
		engine := NewEngine()
		all9 := buildMockValidPacks(now, 3350, 10)
		only8 := all9[:8] // drop pack at address 9

		status := engine.Start(now, Input{Packs: only8, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.True(t, status.IsTerminal())
		assert.Equal(t, StateFaulted, status.State)
		assert.Equal(t, 0.0, status.TargetVoltageVolts)
		assert.Contains(t, status.Reason, "expected 9 packs, got 8")
		assert.ErrorIs(t, status.Err(), ErrMissingPack)
	})

	// 2. Pack dropping off during Update (9 reporting initially, then 8)
	t.Run("PackDropsOffDuringUpdate", func(t *testing.T) {
		engine := NewEngine()
		all9 := buildMockValidPacks(now, 3350, 10)
		status := engine.Start(now, Input{Packs: all9, Inverter: buildMockInverter(now, 10.0, 54.40)})
		require.True(t, status.IsRunning())

		t10 := now.Add(10 * time.Minute)
		packs8 := buildMockValidPacks(t10, 3360, 10)[:8]
		status = engine.Update(t10, Input{Packs: packs8, Inverter: buildMockInverter(t10, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.True(t, status.IsTerminal())
		assert.Equal(t, 0.0, status.TargetVoltageVolts)
		assert.Contains(t, status.Reason, "expected 9 packs, got 8")
	})
}

// TestEngine_UnexpectedAddress verifies failure when an unexpected address (such as 8 or 15) is present.
func TestEngine_UnexpectedAddress(t *testing.T) {
	now := time.Now()
	engine := NewEngine()
	packs := buildMockValidPacks(now, 3350, 10)
	// Replace pack 9 with address 8 (the unmonitored broken pack)
	packs[8].Frame.Address = 8
	packs[8].Settings.Address = 8

	status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
	assert.True(t, status.IsFaulted())
	assert.Contains(t, status.Reason, "address 8 is not in expected set")
	assert.ErrorIs(t, status.Err(), ErrExtraPack)
}

// TestEngine_DuplicateAddresses verifies failure when duplicate addresses exist in input.
func TestEngine_DuplicateAddresses(t *testing.T) {
	now := time.Now()
	engine := NewEngine()
	packs := buildMockValidPacks(now, 3350, 10)
	// Set pack 8's address to 1 (duplicate of pack 1)
	packs[8].Frame.Address = 1
	packs[8].Settings.Address = 1

	status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
	assert.True(t, status.IsFaulted())
	assert.Contains(t, status.Reason, "address 1 reported multiple times")
	assert.ErrorIs(t, status.Err(), ErrDuplicateAddress)
}

// TestEngine_DuplicateBMSIDs verifies failure without leaking the redacted raw identifier in Reason.
func TestEngine_DuplicateBMSIDs(t *testing.T) {
	now := time.Now()
	engine := NewEngine()
	packs := buildMockValidPacks(now, 3350, 10)

	secretID := "CONFIDENTIAL-BMS-SERIAL-998877"
	packs[1].Settings.Balance.BMSID = secretID
	packs[2].Settings.Balance.BMSID = secretID

	status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
	assert.True(t, status.IsFaulted())
	assert.ErrorIs(t, status.Err(), ErrDuplicateBMSID)

	// Invariant: Raw BMS ID must NOT be leaked into public reason or logs!
	assert.NotContains(t, status.Reason, secretID, "status.Reason must not leak raw BMS identifier")
	assert.Contains(t, status.Reason, "duplicate physical BMS identifier")
	assert.Contains(t, status.Reason, "1")
	assert.Contains(t, status.Reason, "2")
}

// TestEngine_MissingBMSID verifies failure when a pack has an empty BMS ID string or nil settings.
func TestEngine_MissingBMSID(t *testing.T) {
	now := time.Now()
	engine := NewEngine()
	packs := buildMockValidPacks(now, 3350, 10)
	packs[3].Settings.Balance.BMSID = ""

	status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
	assert.True(t, status.IsFaulted())
	assert.Contains(t, status.Reason, "empty BMS ID")
	assert.ErrorIs(t, status.Err(), ErrMissingBMSID)
}

// TestEngine_StaleTelemetry verifies failure when telemetry is stale, missing, or unconfirmed.
func TestEngine_StaleTelemetry(t *testing.T) {
	now := time.Now()

	t.Run("FlagNotFresh", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 10)
		packs[2].Fresh = false

		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Contains(t, status.Reason, "telemetry marked stale")
		assert.ErrorIs(t, status.Err(), ErrStaleTelemetry)
	})

	t.Run("AgeExceeds60s", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 10)
		packs[4].SeenAt = now.Add(-65 * time.Second)

		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Contains(t, status.Reason, "telemetry age")
		assert.ErrorIs(t, status.Err(), ErrStaleTelemetry)
	})

	t.Run("Address0Unconfirmed", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 10)
		packs[0].Confirmed = false

		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Contains(t, status.Reason, "address 0 telemetry is not confirmed")
		assert.ErrorIs(t, status.Err(), ErrUnconfirmedAddressZero)
	})
}

// TestEngine_StaleSettings verifies failure when BMS settings are stale or unavailable.
func TestEngine_StaleSettings(t *testing.T) {
	now := time.Now()

	t.Run("StatusStale", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 10)
		packs[1].Settings.Status = interpack.SettingsStale

		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Contains(t, status.Reason, "settings status stale")
		assert.ErrorIs(t, status.Err(), ErrStaleSettings)
	})

	t.Run("BalanceTimestampStale", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 10)
		packs[2].Settings.BalanceSeenAt = now.Add(-4 * time.Minute)

		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Contains(t, status.Reason, "settings block age exceeds limit")
		assert.ErrorIs(t, status.Err(), ErrStaleSettings)
	})

	t.Run("ProtectionTimestampStale", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 10)
		packs[3].Settings.ProtectionSeenAt = now.Add(-4 * time.Minute)

		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Contains(t, status.Reason, "settings block age exceeds limit")
		assert.ErrorIs(t, status.Err(), ErrStaleSettings)
	})
}

// TestEngine_ChangedInstalledSafetySettings verifies failure when any BMS safety setting deviates.
func TestEngine_ChangedInstalledSafetySettings(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name   string
		modify func(p *interpack.Pack)
		substr string
	}{
		{
			name:   "BalanceStartMismatch",
			modify: func(p *interpack.Pack) { p.Settings.Balance.BalanceStartMillivolts = 3300 },
			substr: "balance start 3300mV != 3400mV",
		},
		{
			name:   "BalanceDeltaMismatch",
			modify: func(p *interpack.Pack) { p.Settings.Balance.BalanceDeltaMillivolts = 20 },
			substr: "balance delta 20mV != 30mV",
		},
		{
			name:   "CellOVPAlarmMismatch",
			modify: func(p *interpack.Pack) { p.Settings.Protection.CellOVPAlarmMillivolts = 3550 },
			substr: "cell OVP alarm 3550mV != 3600mV",
		},
		{
			name:   "CellOVPProtectMismatch",
			modify: func(p *interpack.Pack) { p.Settings.Protection.CellOVPProtectionMillivolts = 3600 },
			substr: "cell OVP protect 3600mV != 3650mV",
		},
		{
			name:   "PackOVPAlarmMismatch",
			modify: func(p *interpack.Pack) { p.Settings.Protection.PackOVPAlarmCentivolts = 5700 },
			substr: "pack OVP alarm 5700cV != 5760cV",
		},
		{
			name:   "PackOVPProtectMismatch",
			modify: func(p *interpack.Pack) { p.Settings.Protection.PackOVPProtectionCentivolts = 5800 },
			substr: "pack OVP protect 5800cV != 5840cV",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := NewEngine()
			packs := buildMockValidPacks(now, 3350, 10)
			tc.modify(&packs[1])

			status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
			assert.True(t, status.IsFaulted())
			assert.Contains(t, status.Reason, tc.substr)
			assert.ErrorIs(t, status.Err(), ErrSettingsMismatch)
		})
	}
}

// TestEngine_UnknownAlarms verifies failure when any unmapped or named alarm bits are non-zero.
func TestEngine_UnknownAlarms(t *testing.T) {
	now := time.Now()

	t.Run("UnknownFaultMaskBit", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 10)
		packs[2].Frame.Protection.FaultMask = 0x80000000 // unmapped high bit
		packs[2].Frame.Protection.AlarmsClear = false

		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Contains(t, status.Reason, "fault mask non-zero: 0x80000000")
		assert.ErrorIs(t, status.Err(), ErrActiveAlarm)
	})

	t.Run("UnknownWarningMaskBit", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 10)
		packs[4].Frame.Protection.WarningMask = 0x0080 // unmapped warning bit
		packs[4].Frame.Protection.AlarmsClear = false

		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Contains(t, status.Reason, "warning mask non-zero: 0x0080")
		assert.ErrorIs(t, status.Err(), ErrActiveAlarm)
	})
}

// TestEngine_3550mVCutoffThreshold verifies immediate safety trip at exactly 3550mV or above.
func TestEngine_3550mVCutoffThreshold(t *testing.T) {
	now := time.Now()

	t.Run("3549mV_Passes", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3540, 9) // max cell = 3549 mV
		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsRunning())
		assert.Equal(t, uint16(3549), status.MaxCellMillivolts)
	})

	t.Run("3550mV_ImmediateShutdown", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3540, 10) // max cell = 3550 mV
		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Equal(t, 0.0, status.TargetVoltageVolts)
		assert.Contains(t, status.Reason, "voltage 3550mV >= 3550mV cutoff")
		assert.ErrorIs(t, status.Err(), ErrCellOvervoltageCutoff)
	})

	t.Run("3551mV_ImmediateShutdown", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3540, 11) // max cell = 3551 mV
		status := engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})
		assert.True(t, status.IsFaulted())
		assert.Contains(t, status.Reason, "voltage 3551mV >= 3550mV cutoff")
		assert.ErrorIs(t, status.Err(), ErrCellOvervoltageCutoff)
	})
}

// TestEngine_InverterTelemetryDataLoss verifies that loss of inverter telemetry at any stage fails closed.
func TestEngine_InverterTelemetryDataLoss(t *testing.T) {
	now := time.Now()

	t.Run("Stage1_ValidFalse", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 15)
		inv := InverterTelemetry{Valid: false, Timestamp: now, TotalChargeCurrentAmps: 10.0, BankVoltageVolts: 54.40}
		status := engine.Start(now, Input{Packs: packs, Inverter: inv})
		assert.True(t, status.IsFaulted())
		assert.ErrorIs(t, status.Err(), ErrInverterTelemetry)
	})

	t.Run("Stage1_StaleTimestamp", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 15)
		inv := InverterTelemetry{Valid: true, Timestamp: now.Add(-90 * time.Second), TotalChargeCurrentAmps: 10.0, BankVoltageVolts: 54.40}
		status := engine.Start(now, Input{Packs: packs, Inverter: inv})
		assert.True(t, status.IsFaulted())
		assert.ErrorIs(t, status.Err(), ErrInverterTelemetry)
	})

	t.Run("Stage1_ZeroTimestamp", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 15)
		inv := InverterTelemetry{Valid: true, Timestamp: time.Time{}, TotalChargeCurrentAmps: 10.0, BankVoltageVolts: 54.40}
		status := engine.Start(now, Input{Packs: packs, Inverter: inv})
		assert.True(t, status.IsFaulted())
		assert.ErrorIs(t, status.Err(), ErrInverterTelemetry)
	})

	t.Run("Stage2_DataLossMidRun", func(t *testing.T) {
		engine := NewEngine()
		packs := buildMockValidPacks(now, 3350, 15)
		engine.Start(now, Input{Packs: packs, Inverter: buildMockInverter(now, 10.0, 54.40)})

		// Advance to Stage 2 after 30min dwell at 54.40V
		t31 := now.Add(31 * time.Minute)
		status := engine.Update(t31, Input{Packs: buildMockValidPacks(t31, 3360, 15), Inverter: buildMockInverter(t31, 10.0, 54.40)})
		require.Equal(t, Stage2, status.Stage)

		// Inverter data loss during Stage 2
		t35 := now.Add(35 * time.Minute)
		invLost := InverterTelemetry{Valid: false, Timestamp: t35}
		status = engine.Update(t35, Input{Packs: buildMockValidPacks(t35, 3360, 15), Inverter: invLost})
		assert.True(t, status.IsFaulted())
		assert.Equal(t, 0.0, status.TargetVoltageVolts)
		assert.ErrorIs(t, status.Err(), ErrInverterTelemetry)
	})
}

// TestEngine_Dwell_NoChargeNeverReached verifies that a pre-balanced bank never advances if CV voltage is not reached.
func TestEngine_Dwell_NoChargeNeverReached(t *testing.T) {
	engine := NewEngine()
	start := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)

	// Bank is pre-balanced (spread 15mV <= 30mV), but charger is off (53.0V << 54.4V)
	packs := buildMockValidPacks(start, 3350, 15)
	inv := buildMockInverter(start, 0.0, 53.00)

	status := engine.Start(start, Input{Packs: packs, Inverter: inv})
	assert.Equal(t, Stage1, status.Stage)
	assert.Equal(t, time.Duration(0), status.DwellDuration)

	// At t+35m (time elapsed > 30m, spread <= 30mV, but bank voltage 53.0V not at target): MUST NOT ADVANCE
	t35 := start.Add(35 * time.Minute)
	status = engine.Update(t35, Input{Packs: buildMockValidPacks(t35, 3350, 15), Inverter: buildMockInverter(t35, 0.0, 53.00)})
	assert.Equal(t, Stage1, status.Stage)
	assert.Equal(t, time.Duration(0), status.DwellDuration)
	assert.Contains(t, status.Reason, "not within ±0.10V of 54.40V")

	// Holds all the way to 8h timeout without false success
	t8h := start.Add(8 * time.Hour)
	status = engine.Update(t8h, Input{Packs: buildMockValidPacks(t8h, 3350, 15), Inverter: buildMockInverter(t8h, 0.0, 53.00)})
	assert.True(t, status.IsTimedOut())
	assert.Equal(t, StateTimedOut, status.State)
	assert.Equal(t, 0.0, status.TargetVoltageVolts)
}

// TestEngine_Dwell_TransientReach verifies that reaching voltage briefly does not satisfy 30-min dwell.
func TestEngine_Dwell_TransientReach(t *testing.T) {
	engine := NewEngine()
	start := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)

	// Starts below target at 53.5V
	engine.Start(start, Input{Packs: buildMockValidPacks(start, 3350, 15), Inverter: buildMockInverter(start, 10.0, 53.50)})

	// Reaches 54.40V at t+10m
	t10 := start.Add(10 * time.Minute)
	status := engine.Update(t10, Input{Packs: buildMockValidPacks(t10, 3360, 15), Inverter: buildMockInverter(t10, 10.0, 54.40)})
	assert.Equal(t, time.Duration(0), status.DwellDuration)

	// Dwells until t+15m (5m dwell)
	t15 := start.Add(15 * time.Minute)
	status = engine.Update(t15, Input{Packs: buildMockValidPacks(t15, 3360, 15), Inverter: buildMockInverter(t15, 10.0, 54.40)})
	assert.Equal(t, 5*time.Minute, status.DwellDuration)

	// Voltage falls away to 53.8V at t+16m (transient reach lost)
	t16 := start.Add(16 * time.Minute)
	status = engine.Update(t16, Input{Packs: buildMockValidPacks(t16, 3360, 15), Inverter: buildMockInverter(t16, 5.0, 53.80)})
	assert.Equal(t, time.Duration(0), status.DwellDuration, "dwell timer must reset when voltage falls away")

	// At t+35m from start, even though total session elapsed is 35m, Stage 1 must NOT advance
	t35 := start.Add(35 * time.Minute)
	status = engine.Update(t35, Input{Packs: buildMockValidPacks(t35, 3360, 15), Inverter: buildMockInverter(t35, 5.0, 53.80)})
	assert.Equal(t, Stage1, status.Stage)
}

// TestEngine_Dwell_FallAwayReset verifies that dwell and tail timers reset whenever voltage drops away.
func TestEngine_Dwell_FallAwayReset(t *testing.T) {
	engine := NewEngine()
	start := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)

	// Stage 1 starts at 54.40V
	engine.Start(start, Input{Packs: buildMockValidPacks(start, 3350, 15), Inverter: buildMockInverter(start, 15.0, 54.40)})

	// Dwell for 25 minutes at 54.40V
	t25 := start.Add(25 * time.Minute)
	status := engine.Update(t25, Input{Packs: buildMockValidPacks(t25, 3360, 15), Inverter: buildMockInverter(t25, 12.0, 54.40)})
	assert.Equal(t, 25*time.Minute, status.DwellDuration)

	// Voltage drops to 54.10V (deviation 0.30V > 0.10V tolerance)
	t26 := start.Add(26 * time.Minute)
	status = engine.Update(t26, Input{Packs: buildMockValidPacks(t26, 3360, 15), Inverter: buildMockInverter(t26, 10.0, 54.10)})
	assert.Equal(t, time.Duration(0), status.DwellDuration, "dwell timer must reset when voltage falls away")
	assert.Equal(t, Stage1, status.Stage)

	// Voltage returns to 54.40V at t+30m
	t30 := start.Add(30 * time.Minute)
	status = engine.Update(t30, Input{Packs: buildMockValidPacks(t30, 3360, 15), Inverter: buildMockInverter(t30, 10.0, 54.40)})
	assert.Equal(t, time.Duration(0), status.DwellDuration)

	// Must dwell for full 30 continuous minutes from t30 (i.e. until t60)
	t59 := start.Add(59 * time.Minute)
	status = engine.Update(t59, Input{Packs: buildMockValidPacks(t59, 3370, 15), Inverter: buildMockInverter(t59, 10.0, 54.40)})
	assert.Equal(t, Stage1, status.Stage)
	assert.Equal(t, 29*time.Minute, status.DwellDuration)

	t60 := start.Add(60 * time.Minute)
	status = engine.Update(t60, Input{Packs: buildMockValidPacks(t60, 3370, 15), Inverter: buildMockInverter(t60, 10.0, 54.40)})
	assert.Equal(t, Stage2, status.Stage, "must advance to Stage 2 after 30 continuous minutes of dwell")

	// Ramps to 54.80V at t+61m
	t61 := start.Add(61 * time.Minute)
	engine.Update(t61, Input{Packs: buildMockValidPacks(t61, 3370, 15), Inverter: buildMockInverter(t61, 10.0, 54.80)})

	// Advance through Stage 2 with 30min dwell at 54.80V (from t61 to t91)
	t91 := start.Add(91 * time.Minute)
	status = engine.Update(t91, Input{Packs: buildMockValidPacks(t91, 3410, 15), Inverter: buildMockInverter(t91, 8.0, 54.80)})
	assert.Equal(t, Stage3, status.Stage)

	// Ramps to 55.20V in Stage 3 at t+92m
	t92 := start.Add(92 * time.Minute)
	engine.Update(t92, Input{Packs: buildMockValidPacks(t92, 3410, 15), Inverter: buildMockInverter(t92, 8.0, 55.20)})

	// In Stage 3, qualify tail for 15 minutes at 55.20V (until t107m)
	t107 := start.Add(107 * time.Minute)
	status = engine.Update(t107, Input{Packs: buildMockValidPacks(t107, 3410, 15), Inverter: buildMockInverter(t107, 8.0, 55.20)})
	assert.Equal(t, 15*time.Minute, status.TailDuration)

	// Stage 3 Voltage falls away to 54.90V (< 55.10V) -> TAIL RESETS
	t108 := start.Add(108 * time.Minute)
	status = engine.Update(t108, Input{Packs: buildMockValidPacks(t108, 3410, 15), Inverter: buildMockInverter(t108, 8.0, 54.90)})
	assert.Equal(t, time.Duration(0), status.TailDuration, "tail timer must reset when voltage falls away")
	assert.Contains(t, status.Reason, "tail timer reset")
}

// TestEngine_RampHoldAndAdvanceTiming verifies stage advance timing and spread gating.
func TestEngine_RampHoldAndAdvanceTiming(t *testing.T) {
	engine := NewEngine()
	start := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

	// Start Stage 1 at 54.40V
	status := engine.Start(start, Input{Packs: buildMockValidPacks(start, 3350, 15), Inverter: buildMockInverter(start, 15.0, 54.40)})
	assert.Equal(t, Stage1, status.Stage)

	// At t+35min (dwell 35m >= 30m, but spread = 35mV > 30mV): MUST HOLD
	t35 := start.Add(35 * time.Minute)
	status = engine.Update(t35, Input{Packs: buildMockValidPacks(t35, 3360, 35), Inverter: buildMockInverter(t35, 12.0, 54.40)})
	assert.Equal(t, Stage1, status.Stage)
	assert.Contains(t, status.Reason, "stage 1 holding")
	assert.Contains(t, status.Reason, "max spread 35mV > 30mV")

	// At t+40min: spread drops to 25mV (<= 30mV): ADVANCES to Stage 2
	t40 := start.Add(40 * time.Minute)
	status = engine.Update(t40, Input{Packs: buildMockValidPacks(t40, 3370, 25), Inverter: buildMockInverter(t40, 12.0, 54.40)})
	assert.Equal(t, Stage2, status.Stage)
	assert.Equal(t, Stage2VoltageVolts, status.TargetVoltageVolts)

	// Ramps to 54.80V at t+41m in Stage 2
	t41 := start.Add(41 * time.Minute)
	engine.Update(t41, Input{Packs: buildMockValidPacks(t41, 3370, 25), Inverter: buildMockInverter(t41, 12.0, 54.80)})

	// In Stage 2 at t+51min (10min dwell in Stage 2 at 54.80V, spread = 15mV <= 30mV): MUST HOLD due to < 30min
	t51 := start.Add(51 * time.Minute)
	status = engine.Update(t51, Input{Packs: buildMockValidPacks(t51, 3380, 15), Inverter: buildMockInverter(t51, 10.0, 54.80)})
	assert.Equal(t, Stage2, status.Stage)
	assert.Contains(t, status.Reason, "stage 2 holding")
	assert.Contains(t, status.Reason, "dwell 10m0s < 30m0s")

	// In Stage 2 at t+72min (31min dwell in Stage 2 at 54.80V, spread = 20mV <= 30mV): ADVANCES to Stage 3
	t72 := start.Add(72 * time.Minute)
	status = engine.Update(t72, Input{Packs: buildMockValidPacks(t72, 3390, 20), Inverter: buildMockInverter(t72, 9.0, 54.80)})
	assert.Equal(t, Stage3, status.Stage)
	assert.Equal(t, Stage3VoltageVolts, status.TargetVoltageVolts)
}

// TestEngine_TailReset verifies tail timer resets on current spike, invalid telemetry, or spread increase.
func TestEngine_TailReset(t *testing.T) {
	engine := NewEngine()
	start := time.Date(2026, 9, 12, 14, 0, 0, 0, time.UTC)

	// Advance through Stage 1 and 2 directly to Stage 3
	engine.Start(start, Input{Packs: buildMockValidPacks(start, 3350, 20), Inverter: buildMockInverter(start, 15.0, 54.40)})
	engine.Update(start.Add(31*time.Minute), Input{Packs: buildMockValidPacks(start.Add(31*time.Minute), 3360, 20), Inverter: buildMockInverter(start.Add(31*time.Minute), 12.0, 54.40)})
	engine.Update(start.Add(32*time.Minute), Input{Packs: buildMockValidPacks(start.Add(32*time.Minute), 3360, 20), Inverter: buildMockInverter(start.Add(32*time.Minute), 12.0, 54.80)})
	engine.Update(start.Add(63*time.Minute), Input{Packs: buildMockValidPacks(start.Add(63*time.Minute), 3410, 15), Inverter: buildMockInverter(start.Add(63*time.Minute), 8.0, 54.80)})
	status := engine.Update(start.Add(64*time.Minute), Input{Packs: buildMockValidPacks(start.Add(64*time.Minute), 3410, 15), Inverter: buildMockInverter(start.Add(64*time.Minute), 8.0, 55.20)})
	require.Equal(t, Stage3, status.Stage)

	// t+74m: 10 minutes of continuous tail qualification
	t74 := start.Add(74 * time.Minute)
	status = engine.Update(t74, Input{Packs: buildMockValidPacks(t74, 3415, 15), Inverter: buildMockInverter(t74, 8.0, 55.20)})
	assert.Equal(t, 10*time.Minute, status.TailDuration)

	// Case 1: Current spikes above 10A -> TAIL RESETS
	t75 := start.Add(75 * time.Minute)
	status = engine.Update(t75, Input{Packs: buildMockValidPacks(t75, 3415, 15), Inverter: buildMockInverter(t75, 11.5, 55.20)})
	assert.Equal(t, time.Duration(0), status.TailDuration)
	assert.Contains(t, status.Reason, "inverter current 11.50A > 10.0A")
	assert.Contains(t, status.Reason, "tail timer reset")

	// Case 2: Current returns to 8A, but one cell drops to 3390mV (< 3400mV) -> TAIL REMAINS ZERO
	t76 := start.Add(76 * time.Minute)
	status = engine.Update(t76, Input{Packs: buildMockValidPacks(t76, 3390, 15), Inverter: buildMockInverter(t76, 8.0, 55.20)})
	assert.Equal(t, time.Duration(0), status.TailDuration)
	assert.Contains(t, status.Reason, "min cell 3390mV < 3400mV")

	// Case 3: Cells recover to 3410mV, but pack spread is 25mV (> 20mV) -> TAIL REMAINS ZERO
	t77 := start.Add(77 * time.Minute)
	status = engine.Update(t77, Input{Packs: buildMockValidPacks(t77, 3410, 25), Inverter: buildMockInverter(t77, 8.0, 55.20)})
	assert.Equal(t, time.Duration(0), status.TailDuration)
	assert.Contains(t, status.Reason, "max spread 25mV > 20mV")

	// Case 4: Inverter telemetry invalid -> fails closed immediately
	t78 := start.Add(78 * time.Minute)
	invInvalid := InverterTelemetry{TotalChargeCurrentAmps: 8.0, BankVoltageVolts: 55.20, Timestamp: t78, Valid: false}
	status = engine.Update(t78, Input{Packs: buildMockValidPacks(t78, 3410, 15), Inverter: invInvalid})
	assert.True(t, status.IsFaulted())
	assert.ErrorIs(t, status.Err(), ErrInverterTelemetry)
}

// TestEngine_OverallTimeout8h verifies that failing to settle within 8 hours halts the engine.
func TestEngine_OverallTimeout8h(t *testing.T) {
	engine := NewEngine()
	start := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)

	// Start Stage 1 with spread = 35mV (> 30mV) at 54.40V
	status := engine.Start(start, Input{Packs: buildMockValidPacks(start, 3350, 35), Inverter: buildMockInverter(start, 15.0, 54.40)})
	assert.True(t, status.IsRunning())

	// Advance time to 7h 59m: still holding
	t7h59m := start.Add(7*time.Hour + 59*time.Minute)
	status = engine.Update(t7h59m, Input{Packs: buildMockValidPacks(t7h59m, 3360, 35), Inverter: buildMockInverter(t7h59m, 12.0, 54.40)})
	assert.True(t, status.IsRunning())
	assert.False(t, status.Terminal)

	// Advance time to 8h 00m: overall timeout triggers
	t8h := start.Add(8 * time.Hour)
	status = engine.Update(t8h, Input{Packs: buildMockValidPacks(t8h, 3360, 35), Inverter: buildMockInverter(t8h, 12.0, 54.40)})
	assert.True(t, status.IsTimedOut())
	assert.True(t, status.IsTerminal())
	assert.Equal(t, StateTimedOut, status.State)
	assert.Equal(t, 0.0, status.TargetVoltageVolts)
	assert.Contains(t, status.Reason, "overall conditioning timeout exceeded")
	assert.ErrorIs(t, status.Err(), ErrTimeout)
}

// TestEngine_ManualStop verifies that Stop() halts the engine and zeros targets.
func TestEngine_ManualStop(t *testing.T) {
	engine := NewEngine()
	now := time.Now()

	engine.Start(now, Input{Packs: buildMockValidPacks(now, 3350, 20), Inverter: buildMockInverter(now, 15.0, 54.40)})
	status := engine.Stop(now.Add(5 * time.Minute))

	assert.True(t, status.IsStopped())
	assert.True(t, status.IsTerminal())
	assert.Equal(t, StateStopped, status.State)
	assert.Equal(t, 0.0, status.TargetVoltageVolts)
	assert.Contains(t, status.Reason, "manual stop requested")
	assert.ErrorIs(t, status.Err(), ErrManualStop)

	// Subsequent Update returns stopped status
	subsequent := engine.Update(now.Add(10*time.Minute), Input{Packs: buildMockValidPacks(now.Add(10*time.Minute), 3350, 20), Inverter: buildMockInverter(now.Add(10*time.Minute), 15.0, 54.40)})
	assert.Equal(t, StateStopped, subsequent.State)
}

// TestEngine_UnmonitoredPackExposure verifies topology exposure of the single unmonitored pack.
func TestEngine_UnmonitoredPackExposure(t *testing.T) {
	engine := NewEngine()
	now := time.Now()

	status := engine.Start(now, Input{Packs: buildMockValidPacks(now, 3350, 20), Inverter: buildMockInverter(now, 15.0, 54.40)})
	assert.Equal(t, 10, status.TotalConnectedPacks)
	assert.Equal(t, 9, status.ExpectedMonitoredPacks)
	assert.Equal(t, 9, status.ObservedMonitoredPacks)
	assert.Equal(t, 1, status.UnmonitoredPacks)
}

// TestEngine_InputFromStore verifies integration helper InputFromStore with interpack.Store.
func TestEngine_InputFromStore(t *testing.T) {
	now := time.Now()
	store := &interpack.Store{}

	// Record 9 valid packs into the store
	mockPacks := buildMockValidPacks(now, 3350, 15)
	for _, p := range mockPacks {
		store.Record(p.Frame, now)
		// Record address 0 again to confirm it
		if p.Frame.Address == 0 {
			store.Record(p.Frame, now)
		}
		// Record balance settings
		store.RecordSettings(interpack.SettingsFrame{
			Address:   p.Frame.Address,
			BlockType: interpack.BlockTypeBalance,
			Balance:   p.Settings.Balance,
		}, now)
		// Record protection settings
		store.RecordSettings(interpack.SettingsFrame{
			Address:    p.Frame.Address,
			BlockType:  interpack.BlockTypeProtection,
			Protection: p.Settings.Protection,
		}, now)
	}

	inv := buildMockInverter(now, 12.0, 54.40)
	input := InputFromStore(store, now, inv, DefaultMaxTelemetryAge, DefaultMaxSettingsAge)
	assert.Len(t, input.Packs, 9)

	engine := NewEngine()
	status := engine.Start(now, input)
	assert.True(t, status.IsRunning())
	assert.Equal(t, Stage1, status.Stage)
}

// TestEngine_NonMonotonicTime verifies rejection of backwards time steps.
func TestEngine_NonMonotonicTime(t *testing.T) {
	engine := NewEngine()
	now := time.Now()

	engine.Start(now, Input{Packs: buildMockValidPacks(now, 3350, 15), Inverter: buildMockInverter(now, 12.0, 54.40)})
	past := now.Add(-10 * time.Second)
	status := engine.Update(past, Input{Packs: buildMockValidPacks(now, 3350, 15), Inverter: buildMockInverter(now, 12.0, 54.40)})

	assert.True(t, status.IsFaulted())
	assert.Contains(t, status.Reason, "non-monotonic time")
	assert.ErrorIs(t, status.Err(), ErrNonMonotonicTime)
}

// TestEngine_NaNAndInfCurrent verifies rejection of non-numeric inverter currents or voltages.
func TestEngine_NaNAndInfCurrent(t *testing.T) {
	engine := NewEngine()
	now := time.Now()

	t.Run("NaNCurrentFailsClosed", func(t *testing.T) {
		invNaN := InverterTelemetry{TotalChargeCurrentAmps: math.NaN(), BankVoltageVolts: 54.40, Timestamp: now, Valid: true}
		status := engine.Start(now, Input{Packs: buildMockValidPacks(now, 3350, 15), Inverter: invNaN})
		assert.True(t, status.IsFaulted())
		assert.ErrorIs(t, status.Err(), ErrInverterTelemetry)
	})

	t.Run("NaNVoltageFailsClosed", func(t *testing.T) {
		invNaN := InverterTelemetry{TotalChargeCurrentAmps: 10.0, BankVoltageVolts: math.NaN(), Timestamp: now, Valid: true}
		status := engine.Start(now, Input{Packs: buildMockValidPacks(now, 3350, 15), Inverter: invNaN})
		assert.True(t, status.IsFaulted())
		assert.ErrorIs(t, status.Err(), ErrInverterTelemetry)
	})

	t.Run("NegativeVoltageFailsClosed", func(t *testing.T) {
		invNeg := InverterTelemetry{TotalChargeCurrentAmps: 10.0, BankVoltageVolts: -54.0, Timestamp: now, Valid: true}
		status := engine.Start(now, Input{Packs: buildMockValidPacks(now, 3350, 15), Inverter: invNeg})
		assert.True(t, status.IsFaulted())
		assert.ErrorIs(t, status.Err(), ErrInverterTelemetry)
	})
}
