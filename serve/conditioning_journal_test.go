package serve

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/register"
	"github.com/stretchr/testify/require"
)

func testJournalUnits() []inverter.UnitChargeSettings {
	regs := map[uint16]uint16{
		register.AddrSystemVoltage:        48,
		register.AddrBatteryType:          6,
		register.AddrLimitedChargeVoltage: 136,
		register.AddrEqualizingChargeVolt: 146,
		register.AddrBoostChargeVoltage:   146,
		register.AddrFloatChargeVoltage:   136,
		register.AddrEqualizingChargeEn:   0,
		register.AddrMaxChargeCurrent:     100,
		register.AddrBMSCommunicationEn:   2,
		register.AddrStopChargeSOC:        90,
	}
	return []inverter.UnitChargeSettings{
		{UnitIndex: 0, Host: "inverter-a", RawRegisters: regs},
		{UnitIndex: 1, Serial: "serial-b", RawRegisters: regs},
	}
}

func TestConditioningJournalRoundTripAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conditioning.json")
	want := testJournalUnits()
	require.NoError(t, SaveConditioningJournal(path, want))
	want[0].RawRegisters[register.AddrMaxChargeCurrent] = 999
	got, err := LoadConditioningJournal(path)
	require.NoError(t, err)
	require.Equal(t, uint16(100), got.Units[0].RawRegisters[register.AddrMaxChargeCurrent])
	require.Equal(t, uint16(2), got.Units[1].RawRegisters[register.AddrBMSCommunicationEn])
	require.Equal(t, os.FileMode(0600), fileMode(t, path))
	require.NoError(t, ClearConditioningJournal(path))
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestConditioningJournalRejectsUnsafeInputAndExistingRecord(t *testing.T) {
	require.Error(t, SaveConditioningJournal("relative.json", testJournalUnits()))
	path := filepath.Join(t.TempDir(), "conditioning.json")
	units := testJournalUnits()
	require.NoError(t, SaveConditioningJournal(path, units))
	require.Error(t, SaveConditioningJournal(path, units))
	emptyPath := filepath.Join(t.TempDir(), "empty.json")
	require.NoError(t, os.WriteFile(emptyPath, nil, 0600))
	require.Error(t, SaveConditioningJournal(emptyPath, units))
	units[1].Host, units[1].Serial = units[0].Host, units[0].Serial
	require.Error(t, SaveConditioningJournal(filepath.Join(t.TempDir(), "duplicate.json"), units))
	units = testJournalUnits()
	units[1].UnitIndex = 2
	require.Error(t, SaveConditioningJournal(filepath.Join(t.TempDir(), "invalid-index.json"), units))
	units = testJournalUnits()
	units[1].UnitIndex = 0
	require.Error(t, SaveConditioningJournal(filepath.Join(t.TempDir(), "duplicate-index.json"), units))
	units = testJournalUnits()
	delete(units[0].RawRegisters, register.AddrStopChargeSOC)
	require.Error(t, SaveConditioningJournal(filepath.Join(t.TempDir(), "missing.json"), units))
}

func TestConditioningJournalRejectsCorruptionAndConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":99,"units":[]}`), 0600))
	_, err := LoadConditioningJournal(path)
	require.Error(t, err)

	path = filepath.Join(dir, "concurrent.json")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- SaveConditioningJournal(path, testJournalUnits()) }()
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}
