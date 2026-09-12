package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/bms/interpack"
)

func TestBankPageShowsReadOnlyPackView(t *testing.T) {
	ws := NewWebServer(nil, nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/bank", nil))
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()
	for _, want := range []string{"Battery Bank", "bank-packs", "Cell delta", "pack-to-pack delta", "api/bms", "Unmonitored connected pack"} {
		if !strings.Contains(page, want) {
			t.Fatalf("bank page missing %q", want)
		}
	}
}

func TestAPIBMS_Disabled(t *testing.T) {
	ws := NewWebServer(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var resp BMSResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.False(t, resp.Configured)
	assert.Empty(t, resp.SerialDevice)
	assert.False(t, resp.Observed)
	assert.False(t, resp.AllObservedFresh)
	assert.False(t, resp.AllObservedSettingsFresh)
	assert.False(t, resp.AllObservedAlarmsClear)
	assert.Equal(t, "disabled", resp.Status)
	assert.Equal(t, 60, resp.FreshnessSeconds)
	assert.Equal(t, "60s", resp.FreshnessDuration)
	assert.Equal(t, 0, resp.TotalPacks)
	assert.Equal(t, 0, resp.FreshPacks)
	assert.Equal(t, 0, resp.StalePacks)
	assert.Equal(t, 0, resp.UnconfirmedPacks)
	assert.Equal(t, 0, resp.UncertainPacks)
	assert.Empty(t, resp.Packs)
}

func TestAPIBMS_Unseen(t *testing.T) {
	ws := NewWebServer(nil, nil)
	store := &interpack.Store{}
	cfg := &BMSConfig{SerialDevice: "/dev/ttyUSB0"}
	ws.SetBMS(store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp BMSResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Configured)
	assert.Equal(t, "/dev/ttyUSB0", resp.SerialDevice)
	assert.False(t, resp.Observed)
	assert.False(t, resp.AllObservedFresh, "unseen telemetry must not report all_observed_fresh")
	assert.False(t, resp.AllObservedSettingsFresh)
	assert.False(t, resp.AllObservedAlarmsClear)
	assert.Equal(t, "unseen", resp.Status)
	assert.Equal(t, 60, resp.FreshnessSeconds)
	assert.Equal(t, "60s", resp.FreshnessDuration)
	assert.Equal(t, 0, resp.TotalPacks)
	assert.Equal(t, 0, resp.FreshPacks)
	assert.Equal(t, 0, resp.StalePacks)
	assert.Equal(t, 0, resp.UnconfirmedPacks)
	assert.Equal(t, 0, resp.UncertainPacks)
	assert.Empty(t, resp.Packs)
}

func TestAPIBMS_Fresh(t *testing.T) {
	store := &interpack.Store{}
	now := time.Now()
	store.Record(interpack.Frame{
		Address:               1,
		PackVoltageCentivolts: 5350,
		SOCPercent:            95,
	}, now)
	store.Record(interpack.Frame{
		Address:               2,
		PackVoltageCentivolts: 5348,
		SOCPercent:            94,
	}, now)

	ws := NewWebServer(nil, nil)
	ws.SetBMS(store, &BMSConfig{SerialDevice: "/dev/ttyUSB0"})

	req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp BMSResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Configured)
	assert.Equal(t, "/dev/ttyUSB0", resp.SerialDevice)
	assert.True(t, resp.Observed)
	assert.True(t, resp.AllObservedFresh)
	assert.Equal(t, "fresh", resp.Status)
	assert.Equal(t, 2, resp.TotalPacks)
	assert.Equal(t, 2, resp.FreshPacks)
	assert.Equal(t, 0, resp.StalePacks)
	assert.Equal(t, 0, resp.UnconfirmedPacks)
	assert.Equal(t, 0, resp.UncertainPacks)
	require.Len(t, resp.Packs, 2)

	assert.Equal(t, uint8(1), resp.Packs[0].Frame.Address)
	assert.True(t, resp.Packs[0].Fresh)
	assert.True(t, resp.Packs[0].Confirmed)
	assert.False(t, resp.Packs[0].IdentityUncertain)

	assert.Equal(t, uint8(2), resp.Packs[1].Frame.Address)
	assert.True(t, resp.Packs[1].Fresh)
	assert.True(t, resp.Packs[1].Confirmed)
	assert.False(t, resp.Packs[1].IdentityUncertain)
}

func TestAPIBMS_Stale(t *testing.T) {
	store := &interpack.Store{}
	now := time.Now()
	// Pack 1 is fresh (seen 5s ago)
	store.Record(interpack.Frame{Address: 1, PackVoltageCentivolts: 5350}, now.Add(-5*time.Second))
	// Pack 2 is stale (seen 90s ago, exceeding 60s default freshness)
	store.Record(interpack.Frame{Address: 2, PackVoltageCentivolts: 5345}, now.Add(-90*time.Second))

	ws := NewWebServer(nil, nil)
	ws.SetBMS(store, &BMSConfig{SerialDevice: "/dev/ttyUSB0"})

	req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp BMSResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Configured)
	assert.True(t, resp.Observed)
	assert.False(t, resp.AllObservedFresh, "stale pack must prevent all_observed_fresh")
	assert.Equal(t, "stale", resp.Status)
	assert.Equal(t, 2, resp.TotalPacks)
	assert.Equal(t, 1, resp.FreshPacks)
	assert.Equal(t, 1, resp.StalePacks)
	assert.Equal(t, 0, resp.UnconfirmedPacks)
	require.Len(t, resp.Packs, 2)

	assert.True(t, resp.Packs[0].Fresh)
	assert.False(t, resp.Packs[1].Fresh, "pack 2 must be marked stale")
}

func TestAPIBMS_Unconfirmed(t *testing.T) {
	store := &interpack.Store{}
	now := time.Now()
	// A single report for address 0 does not confirm identity
	store.Record(interpack.Frame{Address: 0, CycleCount: 100}, now)

	ws := NewWebServer(nil, nil)
	ws.SetBMS(store, &BMSConfig{SerialDevice: "/dev/ttyUSB0"})

	req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp BMSResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Configured)
	assert.True(t, resp.Observed)
	assert.False(t, resp.AllObservedFresh, "unconfirmed address 0 must prevent all_observed_fresh")
	assert.Equal(t, "unconfirmed", resp.Status)
	assert.Equal(t, 1, resp.TotalPacks)
	assert.Equal(t, 1, resp.FreshPacks)
	assert.Equal(t, 0, resp.StalePacks)
	assert.Equal(t, 1, resp.UnconfirmedPacks)
	assert.Equal(t, 1, resp.UncertainPacks)
	require.Len(t, resp.Packs, 1)

	assert.False(t, resp.Packs[0].Confirmed)
	assert.True(t, resp.Packs[0].IdentityUncertain)
	assert.True(t, resp.Packs[0].Fresh)
}

func TestAPIBMS_AddressZeroConfirmed(t *testing.T) {
	store := &interpack.Store{}
	now := time.Now()
	f0 := interpack.Frame{
		Address:             0,
		CycleCount:          100,
		FullCapacityCentiAh: 10000,
		BatchRaw:            [12]byte{1, 2, 3},
	}
	// Two consistent consecutive reports confirm address 0
	store.Record(f0, now.Add(-2*time.Second))
	store.Record(f0, now)

	ws := NewWebServer(nil, nil)
	ws.SetBMS(store, &BMSConfig{SerialDevice: "/dev/ttyUSB0"})

	req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp BMSResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Configured)
	assert.True(t, resp.Observed)
	assert.True(t, resp.AllObservedFresh)
	assert.Equal(t, "fresh", resp.Status)
	assert.Equal(t, 1, resp.TotalPacks)
	assert.Equal(t, 1, resp.FreshPacks)
	assert.Equal(t, 0, resp.StalePacks)
	assert.Equal(t, 0, resp.UnconfirmedPacks)
	assert.Equal(t, 1, resp.UncertainPacks, "address 0 must preserve identity uncertainty even when confirmed")
	require.Len(t, resp.Packs, 1)

	assert.True(t, resp.Packs[0].Confirmed)
	assert.True(t, resp.Packs[0].IdentityUncertain)
	assert.True(t, resp.Packs[0].Fresh)
}

func TestAPIBMS_ConcurrentAccess(t *testing.T) {
	store := &interpack.Store{}
	ws := NewWebServer(nil, nil)
	ws.SetBMS(store, &BMSConfig{SerialDevice: "/dev/ttyUSB0"})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writers simulating interpack.Run callback
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(addr uint8) {
			defer wg.Done()
			ticker := time.NewTicker(2 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case now := <-ticker.C:
					store.Record(interpack.Frame{Address: addr, CycleCount: 50}, now)
				}
			}
		}(uint8(i + 1))
	}

	// Readers querying /api/bms
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(2 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
					w := httptest.NewRecorder()
					ws.Handler().ServeHTTP(w, req)
					if w.Code != http.StatusOK {
						t.Errorf("unexpected status code: %d", w.Code)
					}
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestAPIBMS_SettingsExposedWithUnits(t *testing.T) {
	store := &interpack.Store{}
	now := time.Now()

	store.Record(interpack.Frame{
		Address:               1,
		PackVoltageCentivolts: 5350,
		Protection: interpack.ProtectionInfo{
			AlarmsClear: true,
		},
	}, now)

	store.RecordSettings(interpack.SettingsFrame{
		Address:   1,
		BlockType: interpack.BlockTypeBalance,
		Balance: &interpack.BalanceSettings{
			BalanceStartMillivolts: 3400,
			BalanceDeltaMillivolts: 30,
			FullChargeCentivolts:   5680,
			FullChargeVolts:        56.80,
		},
	}, now)

	store.RecordSettings(interpack.SettingsFrame{
		Address:   1,
		BlockType: interpack.BlockTypeProtection,
		Protection: &interpack.ProtectionSettings{
			CellOVPAlarmMillivolts:      3600,
			CellOVPProtectionMillivolts: 3650,
			PackOVPAlarmCentivolts:      5760,
			PackOVPProtectionCentivolts: 5840,
			PackOVPAlarmVolts:           57.60,
			PackOVPProtectionVolts:      58.40,
		},
	}, now)

	ws := NewWebServer(nil, nil)
	ws.SetBMS(store, &BMSConfig{SerialDevice: "/dev/ttyUSB0"})

	req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp BMSResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.AllObservedSettingsFresh)
	assert.True(t, resp.AllObservedAlarmsClear)
	require.Len(t, resp.Packs, 1)
	pack := resp.Packs[0]
	require.NotNil(t, pack.Settings)
	assert.Equal(t, interpack.SettingsFresh, pack.Settings.Status)
	assert.True(t, pack.Settings.Fresh)

	// Verify Balance settings and units
	require.NotNil(t, pack.Settings.Balance)
	assert.Equal(t, uint16(3400), pack.Settings.Balance.BalanceStartMillivolts)
	assert.Equal(t, uint16(30), pack.Settings.Balance.BalanceDeltaMillivolts)
	assert.Equal(t, uint16(5680), pack.Settings.Balance.FullChargeCentivolts)
	assert.InDelta(t, 56.80, pack.Settings.Balance.FullChargeVolts, 0.001)

	// Verify Protection settings and units
	require.NotNil(t, pack.Settings.Protection)
	assert.Equal(t, uint16(3600), pack.Settings.Protection.CellOVPAlarmMillivolts)
	assert.Equal(t, uint16(3650), pack.Settings.Protection.CellOVPProtectionMillivolts)
	assert.Equal(t, uint16(5760), pack.Settings.Protection.PackOVPAlarmCentivolts)
	assert.Equal(t, uint16(5840), pack.Settings.Protection.PackOVPProtectionCentivolts)
	assert.InDelta(t, 57.60, pack.Settings.Protection.PackOVPAlarmVolts, 0.001)
	assert.InDelta(t, 58.40, pack.Settings.Protection.PackOVPProtectionVolts, 0.001)
}

func TestAPIBMS_NineOfTenNotMisrepresentedAsComplete(t *testing.T) {
	store := &interpack.Store{}
	now := time.Now()

	// Populate addresses 0..7 and 9 (9 packs total, reflecting live bus where addr 8 did not reply)
	addrs := []uint8{0, 1, 2, 3, 4, 5, 6, 7, 9}
	for _, addr := range addrs {
		f := interpack.Frame{
			Address:               addr,
			PackVoltageCentivolts: 5350,
			CycleCount:            100,
			FullCapacityCentiAh:   10000,
			Protection: interpack.ProtectionInfo{
				AlarmsClear: true,
			},
		}
		if addr == 0 {
			// Address 0 requires 2 consecutive samples to confirm
			store.Record(f, now.Add(-time.Second))
		}
		store.Record(f, now)

		// Record fresh settings
		store.RecordSettings(interpack.SettingsFrame{
			Address:   addr,
			BlockType: interpack.BlockTypeBalance,
			Balance: &interpack.BalanceSettings{
				BalanceStartMillivolts: 3400,
				BalanceDeltaMillivolts: 30,
				FullChargeCentivolts:   5680,
			},
		}, now)
		store.RecordSettings(interpack.SettingsFrame{
			Address:   addr,
			BlockType: interpack.BlockTypeProtection,
			Protection: &interpack.ProtectionSettings{
				CellOVPAlarmMillivolts:      3600,
				CellOVPProtectionMillivolts: 3650,
				PackOVPAlarmCentivolts:      5760,
				PackOVPProtectionCentivolts: 5840,
			},
		}, now)
	}

	ws := NewWebServer(nil, nil)
	ws.SetBMS(store, &BMSConfig{SerialDevice: "/dev/ttyUSB0"})

	req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	var resp BMSResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Verify factual telemetry counts: exactly 9 packs reported, never misrepresented as 10
	assert.Equal(t, 9, resp.TotalPacks, "total_packs must strictly reflect observed packs (9), not full 10-pack bank")
	assert.Equal(t, 9, resp.FreshPacks)
	assert.Equal(t, 0, resp.StalePacks)
	assert.Equal(t, 0, resp.UnconfirmedPacks)
	assert.Equal(t, 9, resp.SettingsFreshPacks)
	assert.Equal(t, 0, resp.SettingsStalePacks)
	assert.True(t, resp.AllObservedSettingsFresh, "all 9 observed packs have fresh settings")
	assert.True(t, resp.AllObservedAlarmsClear, "all 9 observed packs have clear alarm masks")
	assert.True(t, resp.AllObservedFresh, "all observed telemetry is fresh")
	assert.Equal(t, "fresh", resp.Status)
}

func TestAPIBMS_SettingsStaleOrUnavailable(t *testing.T) {
	store := &interpack.Store{}
	now := time.Now()

	// Pack 1: Fresh settings
	store.Record(interpack.Frame{
		Address: 1,
		Protection: interpack.ProtectionInfo{
			AlarmsClear: true,
		},
	}, now)
	store.RecordSettings(interpack.SettingsFrame{
		Address:   1,
		BlockType: interpack.BlockTypeBalance,
		Balance: &interpack.BalanceSettings{
			BalanceStartMillivolts: 3400,
			BalanceDeltaMillivolts: 30,
			FullChargeCentivolts:   5680,
		},
	}, now)
	store.RecordSettings(interpack.SettingsFrame{
		Address:   1,
		BlockType: interpack.BlockTypeProtection,
		Protection: &interpack.ProtectionSettings{
			CellOVPAlarmMillivolts:      3600,
			CellOVPProtectionMillivolts: 3650,
			PackOVPAlarmCentivolts:      5760,
			PackOVPProtectionCentivolts: 5840,
		},
	}, now)

	// Pack 2: Missing settings (unavailable)
	store.Record(interpack.Frame{
		Address: 2,
		Protection: interpack.ProtectionInfo{
			AlarmsClear: true,
		},
	}, now)

	ws := NewWebServer(nil, nil)
	ws.SetBMS(store, &BMSConfig{SerialDevice: "/dev/ttyUSB0"})

	req := httptest.NewRequest(http.MethodGet, "/api/bms", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	var resp BMSResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, 2, resp.TotalPacks)
	assert.Equal(t, 1, resp.SettingsFreshPacks)
	assert.Equal(t, 0, resp.SettingsStalePacks)
	assert.False(t, resp.AllObservedSettingsFresh, "all_observed_settings_fresh must be false when pack 2 is missing settings")
}
