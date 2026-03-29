package inverter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

// MaxStaleSnapshots is the number of consecutive read failures before a unit's
// snapshot is marked stale (Stale=true). Stale data is still returned so
// consumers can decide how to handle it.
const MaxStaleSnapshots = 3

// UnitInfo holds static device information read once at connect time.
type UnitInfo struct {
	Host            string `json:"host"`
	Serial          string `json:"serial"`
	SlaveID         uint8  `json:"slave_id"`
	Model           string `json:"model"`
	ProtocolVersion uint16 `json:"protocol_version"`
	SoftwareVersion uint16 `json:"software_version"`
	ParallelMode    uint16 `json:"parallel_mode"`
}

// unit is an internal handle for a single inverter connection.
type unit struct {
	info         UnitInfo
	session      *modbus.Session
	lastSnapshot UnitSnapshot // retained for stale fallback
	failCount    int          // consecutive snapshot failures
}

// System manages connections to one or more inverters and provides
// aggregated snapshots of the entire system.
type System struct {
	mu       sync.RWMutex
	units    []*unit
	parallel bool
}

// NewSystem creates a system from pre-established modbus.Client connections.
// Each client should already be connected. The caller is responsible for
// creating and connecting the clients (e.g. via solarman.NewClient or mock).
func NewSystem(clients []modbus.Client) *System {
	s := &System{}
	for _, c := range clients {
		session := modbus.NewSession(c)
		s.units = append(s.units, &unit{
			session: session,
		})
	}
	return s
}

// Init reads product info and settings from all units and detects parallel mode.
// Call this once after creating the system.
func (s *System) Init(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make([]error, len(s.units))

	for i, u := range s.units {
		wg.Add(1)
		go func(idx int, u *unit) {
			defer wg.Done()
			u.session.Ctx = ctx
			if err := readUnitInfo(u.session, &u.info); err != nil {
				errs[idx] = fmt.Errorf("unit %d: read info: %w", idx, err)
			}
		}(i, u)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	// Detect parallel mode from any unit reporting it
	for _, u := range s.units {
		if u.info.ParallelMode > 0 {
			s.parallel = true
			break
		}
	}

	return nil
}

// Units returns static info for all connected inverters.
func (s *System) Units() []UnitInfo {
	infos := make([]UnitInfo, len(s.units))
	for i, u := range s.units {
		infos[i] = u.info
	}
	return infos
}

// IsParallel returns whether the system detected parallel operation.
func (s *System) IsParallel() bool {
	return s.parallel
}

// Snapshot reads all units concurrently and returns an aggregated system snapshot.
// Units that fail to read use their previous snapshot data. After MaxStaleSnapshots
// consecutive failures, the unit's data is marked Stale.
func (s *System) Snapshot(ctx context.Context) *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	unitSnaps := make([]UnitSnapshot, len(s.units))
	var wg sync.WaitGroup

	for i, u := range s.units {
		wg.Add(1)
		go func(idx int, u *unit) {
			defer wg.Done()
			u.session.Ctx = ctx

			snap, err := readUnitSnapshot(u.session)
			if err != nil || len(snap.Errors) > 0 {
				u.failCount++
				// Fall back to last good snapshot
				fallback := u.lastSnapshot
				fallback.Stale = u.failCount >= MaxStaleSnapshots
				if err != nil {
					fallback.Errors = append(fallback.Errors, err.Error())
				} else {
					fallback.Errors = append(fallback.Errors, snap.Errors...)
				}
				unitSnaps[idx] = fallback
				return
			}

			u.failCount = 0
			snap.Host = u.info.Host
			snap.Serial = u.info.Serial
			snap.SlaveID = u.info.SlaveID
			u.lastSnapshot = snap
			unitSnaps[idx] = snap
		}(i, u)
	}
	wg.Wait()

	result := aggregate(unitSnaps)
	result.Time = time.Now()
	result.Parallel = s.parallel
	result.Units = unitSnaps
	return &result
}

// RefreshSettings re-reads settings from all units. Call this after changing
// inverter configuration or periodically for long-running processes.
func (s *System) RefreshSettings(ctx context.Context) error {
	for _, u := range s.units {
		u.session.Ctx = ctx
		if err := readUnitInfo(u.session, &u.info); err != nil {
			return fmt.Errorf("unit %s: refresh settings: %w", u.info.Host, err)
		}
	}
	// Re-check parallel mode
	s.parallel = false
	for _, u := range s.units {
		if u.info.ParallelMode > 0 {
			s.parallel = true
			break
		}
	}
	return nil
}

// readUnitInfo reads static product info and key settings from a single unit.
func readUnitInfo(session *modbus.Session, info *UnitInfo) error {
	if err := bulkRead(session, register.ProductInfo); err != nil {
		return err
	}

	// Serial number (ASCIILoByte, 20 registers)
	serialRegs := make([]uint16, 20)
	for i := 0; i < 20; i++ {
		v, err := session.Lookup(register.AddrSerialNumber + uint16(i))
		if err != nil {
			break
		}
		serialRegs[i] = v
	}
	info.Serial = decodeASCIILoByte(serialRegs)

	// Model (ASCII, 8 registers)
	modelRegs := make([]uint16, 8)
	for i := 0; i < 8; i++ {
		v, err := session.Lookup(register.AddrProductModel + uint16(i))
		if err != nil {
			break
		}
		modelRegs[i] = v
	}
	info.Model = decodeASCII(modelRegs)

	info.ProtocolVersion, _ = session.Lookup(register.AddrProtocolVersion)
	info.SoftwareVersion, _ = session.Lookup(register.AddrSoftwareVersionCPU1)

	// Read parallel mode from inverter settings
	if err := bulkRead(session, register.InverterSettings); err == nil {
		info.ParallelMode, _ = session.Lookup(register.AddrParallelMode)
	}

	return nil
}

// decodeASCII decodes big-endian 2-char-per-register ASCII.
func decodeASCII(values []uint16) string {
	buf := make([]byte, len(values)*2)
	for i, v := range values {
		buf[i*2] = byte(v >> 8)
		buf[i*2+1] = byte(v & 0xFF)
	}
	// Trim nulls and spaces
	end := len(buf)
	for end > 0 && (buf[end-1] == 0 || buf[end-1] == ' ') {
		end--
	}
	return string(buf[:end])
}

// decodeASCIILoByte decodes one ASCII char per register (low byte).
func decodeASCIILoByte(values []uint16) string {
	buf := make([]byte, len(values))
	for i, v := range values {
		buf[i] = byte(v & 0xFF)
	}
	end := len(buf)
	for end > 0 && (buf[end-1] == 0 || buf[end-1] == ' ') {
		end--
	}
	return string(buf[:end])
}
