package serve

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

const conditioningJournalVersion = 1

var conditioningJournalMu sync.Mutex

// ConditioningJournal is the durable record of the settings that must be
// restored when a conditioning session ends or the service restarts.
type ConditioningJournal struct {
	Version   int                           `json:"version"`
	CreatedAt time.Time                     `json:"created_at"`
	Units     []inverter.UnitChargeSettings `json:"units"`
}

var conditioningJournalRegisters = [...]uint16{
	register.AddrSystemVoltage,
	register.AddrBatteryType,
	register.AddrLimitedChargeVoltage,
	register.AddrEqualizingChargeVolt,
	register.AddrBoostChargeVoltage,
	register.AddrFloatChargeVoltage,
	register.AddrEqualizingChargeEn,
	register.AddrMaxChargeCurrent,
	register.AddrBMSCommunicationEn,
	register.AddrStopChargeSOC,
}

// Save records the exact per-unit raw charge registers before any conditioning
// write. It refuses to replace any existing journal.
func SaveConditioningJournal(path string, units []inverter.UnitChargeSettings) error {
	conditioningJournalMu.Lock()
	defer conditioningJournalMu.Unlock()
	if err := validateJournalPath(path); err != nil {
		return err
	}
	if err := validateJournalUnits(units); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("conditioning journal %q already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect conditioning journal: %w", err)
	}

	journal := ConditioningJournal{Version: conditioningJournalVersion, CreatedAt: time.Now().UTC(), Units: cloneUnits(units)}
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("encode conditioning journal: %w", err)
	}
	data = append(data, '\n')
	if err := atomicJournalWrite(path, data); err != nil {
		return err
	}
	return nil
}

// Load reads and validates a saved conditioning journal. The caller must keep
// it until every saved register has been restored and verified.
func LoadConditioningJournal(path string) (*ConditioningJournal, error) {
	if err := validateJournalPath(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open conditioning journal: %w", err)
	}
	defer func() { _ = f.Close() }()
	var journal ConditioningJournal
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&journal); err != nil {
		return nil, fmt.Errorf("decode conditioning journal: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("conditioning journal contains trailing data")
		}
		return nil, fmt.Errorf("read conditioning journal: %w", err)
	}
	if journal.Version != conditioningJournalVersion {
		return nil, fmt.Errorf("unsupported conditioning journal version %d", journal.Version)
	}
	if err := validateJournalUnits(journal.Units); err != nil {
		return nil, err
	}
	journal.Units = cloneUnits(journal.Units)
	return &journal, nil
}

// Clear removes a journal after the service has verified complete restoration.
// It leaves the record in place if the pre-removal durability step fails.
func ClearConditioningJournal(path string) error {
	conditioningJournalMu.Lock()
	defer conditioningJournalMu.Unlock()
	if err := validateJournalPath(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read conditioning journal before clear: %w", err)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open conditioning journal directory: %w", err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("sync conditioning journal directory: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close conditioning journal directory: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("clear conditioning journal: %w", err)
	}
	dir, err = os.Open(filepath.Dir(path))
	if err == nil {
		err = dir.Sync()
		_ = dir.Close()
	}
	if err != nil {
		// Best effort recovery keeps the safety record available after a failed
		// durability operation.
		_ = atomicJournalWrite(path, data)
		return fmt.Errorf("sync cleared conditioning journal: %w", err)
	}
	return nil
}

func validateJournalPath(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("conditioning journal path must be absolute and non-empty")
	}
	return nil
}

func validateJournalUnits(units []inverter.UnitChargeSettings) error {
	if len(units) != 2 {
		return fmt.Errorf("conditioning journal requires exactly two inverter units, got %d", len(units))
	}
	seen := make(map[string]struct{}, len(units))
	seenIndex := make(map[int]struct{}, len(units))
	for _, unit := range units {
		if unit.UnitIndex != 0 && unit.UnitIndex != 1 {
			return fmt.Errorf("invalid inverter unit index %d", unit.UnitIndex)
		}
		if _, ok := seenIndex[unit.UnitIndex]; ok {
			return fmt.Errorf("duplicate inverter unit index %d", unit.UnitIndex)
		}
		seenIndex[unit.UnitIndex] = struct{}{}
		if unit.Host == "" && unit.Serial == "" {
			return fmt.Errorf("unit %d has no stable host or serial identity", unit.UnitIndex)
		}
		identity := unit.Host + "\x00" + unit.Serial
		if _, ok := seen[identity]; ok {
			return fmt.Errorf("duplicate inverter identity for unit %d", unit.UnitIndex)
		}
		seen[identity] = struct{}{}
		if len(unit.RawRegisters) == 0 {
			return fmt.Errorf("unit %d has no raw register snapshot", unit.UnitIndex)
		}
		for _, addr := range conditioningJournalRegisters {
			if _, ok := unit.RawRegisters[addr]; !ok {
				return fmt.Errorf("unit %d snapshot is missing required register 0x%04X", unit.UnitIndex, addr)
			}
		}
	}
	return nil
}

func cloneUnits(units []inverter.UnitChargeSettings) []inverter.UnitChargeSettings {
	result := make([]inverter.UnitChargeSettings, len(units))
	for i, unit := range units {
		result[i] = unit
		result[i].RawRegisters = make(map[uint16]uint16, len(unit.RawRegisters))
		for addr, value := range unit.RawRegisters {
			result[i].RawRegisters[addr] = value
		}
	}
	return result
}

func atomicJournalWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".conditioning-journal-*")
	if err != nil {
		return fmt.Errorf("create conditioning journal temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write conditioning journal: %w", err)
	}
	// Link is atomic and fails with EEXIST, so a second process can never
	// replace a journal after the safety record has been created.
	if err := os.Link(tmpName, path); err != nil {
		return fmt.Errorf("install conditioning journal: %w", err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open conditioning journal directory: %w", err)
	}
	err = d.Sync()
	_ = d.Close()
	if err != nil {
		return fmt.Errorf("sync conditioning journal directory: %w", err)
	}
	return nil
}
