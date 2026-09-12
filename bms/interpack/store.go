package interpack

import (
	"sort"
	"sync"
	"time"
)

const maxConsecutiveGap = 2 * time.Minute

// DefaultSettingsFreshnessDuration is the default maximum age for BMS settings blocks.
// Settings change rarely; 3 minutes allows several 60-second query cycles while
// promptly identifying communication loss.
const DefaultSettingsFreshnessDuration = 3 * time.Minute

// SettingsState represents the availability and freshness of a pack's settings.
type SettingsState string

const (
	SettingsUnavailable SettingsState = "unavailable"
	SettingsFresh       SettingsState = "fresh"
	SettingsStale       SettingsState = "stale"
)

// PackSettings contains verified settings for a pack and tracks their freshness.
type PackSettings struct {
	Address          uint8               `json:"address"`
	Status           SettingsState       `json:"status"`
	Fresh            bool                `json:"fresh"`
	SeenAt           time.Time           `json:"seen_at"`
	BalanceSeenAt    time.Time           `json:"balance_seen_at,omitempty"`
	ProtectionSeenAt time.Time           `json:"protection_seen_at,omitempty"`
	Balance          *BalanceSettings    `json:"balance,omitempty"`
	Protection       *ProtectionSettings `json:"protection,omitempty"`
}

// BMSID returns the unique UP16S BMS identifier if available.
func (ps *PackSettings) BMSID() string {
	if ps == nil || ps.Balance == nil {
		return ""
	}
	return ps.Balance.BMSID
}

// Pack is the last validated report for a bus address. Address zero has no
// reliable physical identity, so it is confirmed only after two consistent
// consecutive reports; this does not prove which physical battery answered.
// IdentityUncertain is set to true for address 0.
type Pack struct {
	Frame             Frame         `json:"frame"`
	SeenAt            time.Time     `json:"seen_at"`
	Fresh             bool          `json:"fresh"`
	Confirmed         bool          `json:"confirmed"`
	IdentityUncertain bool          `json:"identity_uncertain"`
	Settings          *PackSettings `json:"settings,omitempty"`
}

// Ambiguous reports whether the pack's physical identity cannot be uniquely established.
func (p Pack) Ambiguous() bool {
	return p.IdentityUncertain
}

// Active reports whether the pack is both fresh and confirmed.
func (p Pack) Active() bool {
	return p.Fresh && p.Confirmed
}

type trackedPack struct {
	pack    Pack
	samples uint8
}

// Store tracks responders and settings dynamically without assuming a fixed battery count.
type Store struct {
	mu       sync.RWMutex
	packs    map[uint8]trackedPack
	settings map[uint8]*PackSettings
}

// Record replaces an address's last report with a CRC-validated telemetry frame.
func (s *Store) Record(frame Frame, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.packs == nil {
		s.packs = make(map[uint8]trackedPack)
	}
	previous := s.packs[frame.Address]
	samples := uint8(1)
	if frame.Address != 0 {
		samples = 2
	} else if previous.samples > 0 &&
		at.Sub(previous.pack.SeenAt) >= 0 &&
		at.Sub(previous.pack.SeenAt) <= maxConsecutiveGap &&
		previous.pack.Frame.CycleCount == frame.CycleCount &&
		previous.pack.Frame.FullCapacityCentiAh == frame.FullCapacityCentiAh &&
		previous.pack.Frame.BatchRaw == frame.BatchRaw {
		samples = 2
	}
	s.packs[frame.Address] = trackedPack{
		pack: Pack{
			Frame:             frame,
			SeenAt:            at,
			Confirmed:         samples == 2,
			IdentityUncertain: frame.Address == 0,
		},
		samples: samples,
	}
}

// RecordSettings updates an address's verified settings with a decoded settings frame.
// It preserves settings independently and does NOT fabricate a synthetic telemetry Pack;
// snapshot inclusion strictly requires real CRC-valid 0x45 telemetry reports.
func (s *Store) RecordSettings(frame SettingsFrame, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settings == nil {
		s.settings = make(map[uint8]*PackSettings)
	}
	ps := s.settings[frame.Address]
	if ps == nil {
		ps = &PackSettings{Address: frame.Address}
		s.settings[frame.Address] = ps
	}
	ps.SeenAt = at
	switch frame.BlockType {
	case BlockTypeBalance:
		ps.Balance = frame.Balance
		ps.BalanceSeenAt = at
	case BlockTypeProtection:
		ps.Protection = frame.Protection
		ps.ProtectionSeenAt = at
	}
}

// GetSettings retrieves the recorded settings for a given address.
func (s *Store) GetSettings(address uint8) (*PackSettings, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.settings == nil {
		return nil, false
	}
	ps, ok := s.settings[address]
	if !ok || ps == nil {
		return nil, false
	}
	copyPS := *ps
	return &copyPS, true
}

// DistinctBMSIdentities counts unique physical UP16S BMS identifiers parsed from
// balance config offset 16:46 across active telemetry packs. It flags whether any
// duplicate identifier was detected across different bus addresses.
func (s *Store) DistinctBMSIdentities(now time.Time, telemetryMaxAge time.Duration) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seenIDs := make(map[string]uint8)
	hasDuplicates := false

	for addr, tracked := range s.packs {
		age := now.Sub(tracked.pack.SeenAt)
		if telemetryMaxAge > 0 && (age < 0 || age > telemetryMaxAge) {
			continue
		}
		if !tracked.pack.Confirmed {
			continue
		}

		ps := s.settings[addr]
		if ps == nil || ps.Balance == nil || ps.Balance.BMSID == "" {
			continue
		}
		bmsID := ps.Balance.BMSID
		if existingAddr, exists := seenIDs[bmsID]; exists {
			if existingAddr != addr {
				hasDuplicates = true
			}
		} else {
			seenIDs[bmsID] = addr
		}
	}
	return len(seenIDs), hasDuplicates
}

// Snapshot returns every observed address with evaluated telemetry and settings freshness.
// Telemetry freshness is evaluated against maxAge; settings freshness is evaluated
// against settingsMaxAge (defaulting to DefaultSettingsFreshnessDuration).
// Both BalanceSeenAt and ProtectionSeenAt are evaluated independently so one fresh read
// cannot mask a stale block.
func (s *Store) Snapshot(now time.Time, maxAge time.Duration, settingsMaxAge ...time.Duration) []Pack {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sLimit := DefaultSettingsFreshnessDuration
	if len(settingsMaxAge) > 0 && settingsMaxAge[0] > 0 {
		sLimit = settingsMaxAge[0]
	}

	packs := make([]Pack, 0, len(s.packs))
	for _, tracked := range s.packs {
		pack := tracked.pack
		age := now.Sub(pack.SeenAt)
		pack.Fresh = maxAge > 0 && age >= 0 && age <= maxAge

		// Attach settings with independent block evaluation
		var ps *PackSettings
		if s.settings != nil {
			ps = s.settings[pack.Frame.Address]
		}
		if ps == nil || (ps.Balance == nil && ps.Protection == nil) {
			pack.Settings = &PackSettings{
				Address: pack.Frame.Address,
				Status:  SettingsUnavailable,
				Fresh:   false,
			}
		} else {
			psCopy := *ps

			balPresent := ps.Balance != nil && !ps.BalanceSeenAt.IsZero()
			balAge := now.Sub(ps.BalanceSeenAt)
			balFresh := balPresent && balAge >= 0 && balAge <= sLimit
			balStale := balPresent && balAge > sLimit

			protPresent := ps.Protection != nil && !ps.ProtectionSeenAt.IsZero()
			protAge := now.Sub(ps.ProtectionSeenAt)
			protFresh := protPresent && protAge >= 0 && protAge <= sLimit
			protStale := protPresent && protAge > sLimit

			// Both blocks must be independently fresh for overall settings to be fresh
			switch {
			case balFresh && protFresh:
				psCopy.Status = SettingsFresh
				psCopy.Fresh = true
			case balStale || protStale:
				// If either block is stale, overall status is stale
				psCopy.Status = SettingsStale
				psCopy.Fresh = false
			default:
				// Incomplete blocks (e.g. one fresh, other never seen)
				psCopy.Status = SettingsUnavailable
				psCopy.Fresh = false
			}

			pack.Settings = &psCopy
		}

		packs = append(packs, pack)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].Frame.Address < packs[j].Frame.Address })
	return packs
}
