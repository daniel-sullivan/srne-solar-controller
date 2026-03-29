package mock

import (
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/register"
)

// Sim is a live-simulated inverter that ticks internally, updating
// registers to mimic a real ASP48100U200-H. Embeds Inverter for
// the modbus.Client interface.
//
// In addition to the time-based background simulation (Start/Tick),
// Sim supports deterministic "poke" methods (SetSOC, SetPV, SetLoad, etc.)
// that immediately update all related registers without jitter. This makes
// Sim usable both as a live simulator and as a deterministic test fixture.
type Sim struct {
	*Inverter

	mu      sync.Mutex
	ticker  *time.Ticker
	done    chan struct{}
	simTime time.Time // virtual clock (allows test control)

	// Internal state — mutated by poke methods and step()
	soc          float64  // fractional SOC for smooth drift
	loadW        float64  // current load in watts
	pv1W         float64  // PV1 power (negative = auto from curve)
	pv2W         float64  // PV2 power (negative = auto from curve)
	pvManual     bool     // true when PV set explicitly via SetPV
	gridV        float64  // grid voltage (0 = off-grid)
	parallelMode uint16   // parallel mode register value
	faultQ       []uint16 // queued fault codes to inject
}

// NewSim creates a Sim pre-loaded with realistic register values for
// an ASP48100U200-H (48V LiFePO4, split-phase 120/240V).
func NewSim() *Sim {
	regs := defaultRegisters()
	s := &Sim{
		Inverter: NewInverter(regs),
		simTime:  time.Now(),
		soc:      85,
		loadW:    500,
		pv1W:     1800,
		pv2W:     1200,
	}
	return s
}

// Start begins the background simulation tick at the given interval.
// Call Stop() to clean up.
func (s *Sim) Start(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ticker != nil {
		return
	}
	s.done = make(chan struct{})
	s.ticker = time.NewTicker(interval)
	ticker := s.ticker
	done := s.done
	go s.loop(ticker, done)
}

// Stop halts the background simulation.
func (s *Sim) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ticker != nil {
		s.ticker.Stop()
		close(s.done)
		s.ticker = nil
	}
}

// Tick advances the simulation by one step manually (for deterministic tests).
func (s *Sim) Tick() {
	s.mu.Lock()
	s.simTime = s.simTime.Add(1 * time.Second)
	s.mu.Unlock()
	s.step()
}

// --- Poke methods: deterministic, immediate register updates ---

// SetSOC directly sets the battery state of charge (0-100).
func (s *Sim) SetSOC(pct float64) {
	s.mu.Lock()
	s.soc = clamp(pct, 0, 100)
	s.mu.Unlock()
	s.syncRegisters()
}

// SetLoad sets the current AC load in watts and updates all related registers.
func (s *Sim) SetLoad(watts float64) {
	s.mu.Lock()
	s.loadW = watts
	s.mu.Unlock()
	s.syncRegisters()
}

// SetPV sets PV1 and PV2 power in watts and updates all related registers.
// This overrides the time-based PV curve until the next Tick/Start cycle.
func (s *Sim) SetPV(pv1Watts, pv2Watts float64) {
	s.mu.Lock()
	s.pv1W = pv1Watts
	s.pv2W = pv2Watts
	s.pvManual = true
	s.mu.Unlock()
	s.syncRegisters()
}

// SetGridVoltage sets the grid input voltage (0 = off-grid) and updates registers.
func (s *Sim) SetGridVoltage(volts float64) {
	s.mu.Lock()
	s.gridV = volts
	s.mu.Unlock()
	s.syncRegisters()
}

// SetParallelMode sets the parallel mode register value.
func (s *Sim) SetParallelMode(mode uint16) {
	s.mu.Lock()
	s.parallelMode = mode
	s.mu.Unlock()
	s.SetRegister(register.AddrParallelMode, mode)
}

// TriggerFault queues a fault code to be applied on the next tick.
func (s *Sim) TriggerFault(code uint16) {
	s.mu.Lock()
	s.faultQ = append(s.faultQ, code)
	s.mu.Unlock()
}

// ClearFaults resets all fault registers to zero.
func (s *Sim) ClearFaults() {
	s.SetRegister(register.AddrFaultBits, 0)
	s.SetRegister(register.AddrFaultBits+1, 0)
	s.SetRegister(register.AddrFaultBitsExt, 0)
	s.SetRegister(register.AddrFaultBitsExt+1, 0)
	s.SetRegister(register.AddrFaultCode1, 0)
	s.SetRegister(register.AddrFaultCode2, 0)
	s.SetRegister(register.AddrFaultCode3, 0)
	s.SetRegister(register.AddrFaultCode4, 0)
	s.SetRegister(register.AddrMachineState, 5) // back to inverter mode
}

// --- Internal simulation ---

func (s *Sim) loop(ticker *time.Ticker, done chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s.mu.Lock()
			s.simTime = s.simTime.Add(1 * time.Second)
			s.mu.Unlock()
			s.step()
		}
	}
}

// step advances the simulation one tick with jitter (for live simulation).
func (s *Sim) step() {
	s.mu.Lock()
	hour := s.simTime.Hour()
	load := s.loadW
	faults := s.faultQ
	s.faultQ = nil
	pvManual := s.pvManual
	pv1 := s.pv1W
	pv2 := s.pv2W
	s.mu.Unlock()

	// PV generation: use manual values or bell curve
	var pv1W, pv2W float64
	if pvManual {
		pv1W = pv1
		pv2W = pv2
	} else {
		pvW := pvCurve(hour) + jitter(50)
		pv1W = pvW * 0.6
		pv2W = pvW * 0.4
	}
	pvW := pv1W + pv2W

	// Net power: PV charging minus load discharging
	netW := pvW - load
	// SOC drift: netW / (48V * 100Ah) * 100 per second (simplified)
	socDelta := netW / (48.0 * 100.0) * 100.0 / 3600.0

	s.mu.Lock()
	s.soc = clamp(s.soc+socDelta, 0, 100)
	soc := s.soc
	s.mu.Unlock()

	// Battery voltage: LiFePO4 48V curve (44V empty → 54.4V full)
	battV := 44.0 + (soc/100.0)*10.4
	battI := netW / battV // positive = charging

	// Temperatures: ambient + load-dependent heating
	ambient := 25.0 + jitter(2)
	heatsinkA := ambient + load/200.0 + jitter(1)
	heatsinkB := ambient + load/150.0 + jitter(1)

	// Write all registers
	s.SetRegister(register.AddrBatterySOC, uint16(soc))
	s.SetRegister(register.AddrBatteryVoltage, uint16(battV*10))
	s.SetRegister(register.AddrBatteryCurrent, uint16(int16(battI*10)))
	s.SetRegister(register.AddrTemps, packTemp(int8(ambient), int8(ambient+2)))

	s.SetRegister(register.AddrPV1Voltage, uint16((90+jitter(5))*10))
	s.SetRegister(register.AddrPV1Current, uint16(max(0, pv1W/(90+jitter(5)))*10))
	s.SetRegister(register.AddrPV1Power, uint16(max(0, pv1W)))
	s.SetRegister(register.AddrPV2Voltage, uint16((85+jitter(5))*10))
	s.SetRegister(register.AddrPV2Current, uint16(max(0, pv2W/(85+jitter(5)))*10))
	s.SetRegister(register.AddrPV2Power, uint16(max(0, pv2W)))
	s.SetRegister(register.AddrTotalChargePower, uint16(max(0, pvW)))

	s.SetRegister(register.AddrInverterVoltageL1, uint16(120.0*10+jitter(5)))
	s.SetRegister(register.AddrInverterVoltageL2, uint16(120.0*10+jitter(5)))
	s.SetRegister(register.AddrInverterFrequency, uint16(60.0*100+jitter(10)))
	s.SetRegister(register.AddrLoadPowerL1, uint16(load*0.5))
	s.SetRegister(register.AddrLoadPowerL2, uint16(load*0.5))
	s.SetRegister(register.AddrBusVoltage, uint16(380.0*10+jitter(20)))

	s.SetRegister(register.AddrHeatsinkATemp, uint16(int16(heatsinkA*10)))
	s.SetRegister(register.AddrHeatsinkBTemp, uint16(int16(heatsinkB*10)))
	s.SetRegister(register.AddrHeatsinkCTemp, uint16(int16((ambient+5)*10)))
	s.SetRegister(register.AddrHeatsinkDTemp, uint16(int16(ambient*10)))

	// Charge status
	switch {
	case battI > 0.5:
		s.SetRegister(register.AddrChargeStatus, 1) // quick charge
	case battI > 0:
		s.SetRegister(register.AddrChargeStatus, 2) // constant voltage
	default:
		s.SetRegister(register.AddrChargeStatus, 0) // off
	}

	// Inject queued faults
	for _, code := range faults {
		s.SetRegister(register.AddrFaultCode1, code)
		s.SetRegister(register.AddrMachineState, 10) // fault state
	}
}

// syncRegisters recomputes all derived register values from the current
// internal state, without jitter. Used by the poke methods for deterministic output.
func (s *Sim) syncRegisters() {
	s.mu.Lock()
	soc := s.soc
	load := s.loadW
	pv1W := s.pv1W
	pv2W := s.pv2W
	gridV := s.gridV
	s.mu.Unlock()

	pvW := pv1W + pv2W

	// Battery voltage: LiFePO4 48V curve (44V empty → 54.4V full)
	battV := 44.0 + (soc/100.0)*10.4
	netW := pvW - load
	battI := netW / battV

	s.SetRegister(register.AddrBatterySOC, uint16(soc))
	s.SetRegister(register.AddrBatteryVoltage, uint16(battV*10))
	s.SetRegister(register.AddrBatteryCurrent, uint16(int16(battI*10)))

	// PV — derive voltage/current from power at typical string voltages
	const pv1Vnom = 90.0
	const pv2Vnom = 85.0
	s.SetRegister(register.AddrPV1Voltage, uint16(pv1Vnom*10))
	s.SetRegister(register.AddrPV1Power, uint16(max(0, pv1W)))
	if pv1W > 0 {
		s.SetRegister(register.AddrPV1Current, uint16(pv1W/pv1Vnom*10))
	} else {
		s.SetRegister(register.AddrPV1Current, 0)
	}
	s.SetRegister(register.AddrPV2Voltage, uint16(pv2Vnom*10))
	s.SetRegister(register.AddrPV2Power, uint16(max(0, pv2W)))
	if pv2W > 0 {
		s.SetRegister(register.AddrPV2Current, uint16(pv2W/pv2Vnom*10))
	} else {
		s.SetRegister(register.AddrPV2Current, 0)
	}
	s.SetRegister(register.AddrTotalChargePower, uint16(max(0, pvW)))

	// Load split evenly across L1/L2
	halfLoad := load * 0.5
	s.SetRegister(register.AddrLoadPowerL1, uint16(halfLoad))
	s.SetRegister(register.AddrLoadPowerL2, uint16(halfLoad))
	s.SetRegister(register.AddrLoadApparentPowerL1, uint16(halfLoad*1.02))
	s.SetRegister(register.AddrLoadApparentPowerL2, uint16(halfLoad*1.02))
	if halfLoad > 0 {
		s.SetRegister(register.AddrLoadCurrentL1, uint16(halfLoad/120.0*10))
		s.SetRegister(register.AddrLoadCurrentL2, uint16(halfLoad/120.0*10))
	} else {
		s.SetRegister(register.AddrLoadCurrentL1, 0)
		s.SetRegister(register.AddrLoadCurrentL2, 0)
	}

	// Inverter output
	s.SetRegister(register.AddrInverterVoltageL1, 1200) // 120.0V
	s.SetRegister(register.AddrInverterVoltageL2, 1200)
	s.SetRegister(register.AddrInverterFrequency, 6000) // 60.00Hz
	s.SetRegister(register.AddrBusVoltage, 3800)        // 380.0V

	// Heatsink temps: deterministic from load
	ambient := 25.0
	s.SetRegister(register.AddrTemps, packTemp(int8(ambient), int8(ambient+2)))
	s.SetRegister(register.AddrHeatsinkATemp, uint16(int16((ambient+load/200.0)*10)))
	s.SetRegister(register.AddrHeatsinkBTemp, uint16(int16((ambient+load/150.0)*10)))
	s.SetRegister(register.AddrHeatsinkCTemp, uint16(int16((ambient+5)*10)))
	s.SetRegister(register.AddrHeatsinkDTemp, uint16(int16(ambient*10)))

	// Grid
	s.SetRegister(register.AddrGridVoltageL1, uint16(gridV*10))
	s.SetRegister(register.AddrGridVoltageL2, uint16(gridV*10))

	// Charge status
	switch {
	case battI > 0.5:
		s.SetRegister(register.AddrChargeStatus, 1) // quick charge
	case battI > 0:
		s.SetRegister(register.AddrChargeStatus, 2) // constant voltage
	default:
		s.SetRegister(register.AddrChargeStatus, 0) // off
	}

	// Machine state
	if gridV > 0 {
		s.SetRegister(register.AddrMachineState, 4) // mains mode
	} else {
		s.SetRegister(register.AddrMachineState, 5) // inverter mode
	}
}

// pvCurve returns simulated PV output in watts for a given hour (0-23).
// Bell curve centered at 13:00, zero before 6am and after 20:00.
func pvCurve(hour int) float64 {
	if hour < 6 || hour > 19 {
		return 0
	}
	// Gaussian centered at 13, sigma=3
	x := float64(hour) - 13.0
	return 4000 * math.Exp(-(x*x)/(2*3*3))
}

func jitter(scale float64) float64 {
	return (rand.Float64() - 0.5) * 2 * scale
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func packTemp(hi, lo int8) uint16 {
	return uint16(byte(hi))<<8 | uint16(byte(lo))
}

func defaultRegisters() map[uint16]uint16 {
	regs := map[uint16]uint16{
		// Product info
		register.AddrMaxVoltageRatedCurrent: 0x3064, // 48V/100A
		register.AddrProductType:            4,      // Integrated Inverter Controller
		register.AddrSoftwareVersionCPU1:    818,    // V8.18
		register.AddrSoftwareVersionCPU2:    100,
		register.AddrHardwareVersionControl: 100,
		register.AddrHardwareVersionPower:   100,
		register.AddrRS485Address:           1,
		register.AddrModelCode:              0,
		register.AddrProtocolVersion:        107, // V1.07

		// Battery / PV data
		register.AddrBatterySOC:         85,
		register.AddrBatteryVoltage:     532, // 53.2V
		register.AddrBatteryCurrent:     100, // 10.0A charging
		register.AddrTemps:              packTemp(30, 25),
		register.AddrDCLoadVoltage:      0,
		register.AddrDCLoadCurrent:      0,
		register.AddrDCLoadPower:        0,
		register.AddrPV1Voltage:         900, // 90.0V
		register.AddrPV1Current:         200, // 20.0A
		register.AddrPV1Power:           1800,
		register.AddrChargeStatus:       1, // quick charge
		register.AddrFaultAlarmBits:     0,
		register.AddrFaultAlarmBits + 1: 0,
		register.AddrTotalChargePower:   3000,
		register.AddrPV2Voltage:         850,
		register.AddrPV2Current:         140,
		register.AddrPV2Power:           1200,

		// Battery settings
		register.AddrPVChargeCurrentLimit:  800, // 80.0A
		register.AddrNominalBatteryCapAH:   100, // 100AH
		register.AddrSystemVoltage:         48,
		register.AddrBatteryType:           6,   // LiFePO4 (BMS)
		register.AddrOverVoltageProtection: 150, // 12V-base
		register.AddrLimitedChargeVoltage:  148,
		register.AddrBoostChargeVoltage:    144,
		register.AddrFloatChargeVoltage:    136,
		register.AddrBoostReturnVoltage:    130,
		register.AddrOverDischargeReturnV:  120,
		register.AddrUnderVoltageWarning:   118,
		register.AddrOverDischargeVoltage:  115,
		register.AddrLimitedDischargeVolt:  112,
		register.AddrCutoffSOC:             0x6414, // charge 100% / discharge 20%
		register.AddrStopChargeSOC:         100,
		register.AddrLowSOCAlarm:           20,
		register.AddrSOCSwitchToMains:      15,
		register.AddrSOCSwitchToBattery:    25,

		// Statistics
		register.AddrPVGenerationToday:     150, // 15.0kWh
		register.AddrLoadConsumptionToday:  120, // 12.0kWh
		register.AddrBatteryChargeToday:    80,  // 80AH
		register.AddrBatteryDischargeToday: 60,  // 60AH
		register.AddrTotalRunningDays:      365,
	}

	// Fill product model ASCII "ASP48100U200-H" (8 registers)
	model := "ASP48100U200-H\x00\x00"
	for i := 0; i < 8; i++ {
		regs[register.AddrProductModel+uint16(i)] = uint16(model[i*2])<<8 | uint16(model[i*2+1])
	}

	// Fill serial number (ASCIILoByte — one char in low byte per register)
	serial := "MOCK000012345678XXXX"
	for i := 0; i < 20; i++ {
		regs[register.AddrSerialNumber+uint16(i)] = uint16(serial[i])
	}

	// Fill product info gaps for bulk reader (0x000A-0x0048)
	for addr := uint16(0x000A); addr <= 0x0048; addr++ {
		if _, ok := regs[addr]; !ok {
			regs[addr] = 0
		}
	}

	// Inverter data — fill full range 0x0200-0x0239 for bulk reader gap bridging
	for addr := uint16(0x0200); addr <= 0x0239; addr++ {
		regs[addr] = 0
	}
	regs[register.AddrMachineState] = 5  // inverter mode
	regs[register.AddrBusVoltage] = 3800 // 380.0V
	regs[register.AddrInverterVoltageL1] = 1200
	regs[register.AddrInverterCurrentL1] = 42
	regs[register.AddrInverterFrequency] = 6000
	regs[register.AddrLoadCurrentL1] = 42
	regs[register.AddrLoadPowerFactor] = 980
	regs[register.AddrLoadPowerL1] = 500
	regs[register.AddrLoadApparentPowerL1] = 510
	regs[register.AddrLoadRatioL1] = 5
	regs[register.AddrHeatsinkATemp] = 350
	regs[register.AddrHeatsinkBTemp] = 380
	regs[register.AddrHeatsinkCTemp] = 300
	regs[register.AddrHeatsinkDTemp] = 250
	regs[register.AddrPVChargeCurrentBatt] = 200
	regs[register.AddrInverterVoltageL2] = 1200
	regs[register.AddrInverterCurrentL2] = 42
	regs[register.AddrLoadCurrentL2] = 42
	regs[register.AddrLoadPowerL2] = 500
	regs[register.AddrLoadApparentPowerL2] = 510
	regs[register.AddrLoadRatioL2] = 5

	// Inverter settings — fill full range 0xE200-0xE221
	for addr := uint16(0xE200); addr <= 0xE221; addr++ {
		regs[addr] = 0
	}
	regs[register.AddrInvRS485Address] = 1
	regs[register.AddrOutputPriority] = 0
	regs[register.AddrMainsChargeCurrentLim] = 250
	regs[register.AddrOutputVoltage] = 1200
	regs[register.AddrOutputFrequency] = 6000
	regs[register.AddrMaxChargeCurrent] = 800
	regs[register.AddrChargerPriority] = 0
	regs[register.AddrBMSProtocol] = 1
	regs[register.AddrBMSCommunicationEn] = 1
	regs[register.AddrBMSErrorStopEnable] = 1

	// Statistics — fill full range 0xF000-0xF04B
	for addr := uint16(0xF000); addr <= 0xF04B; addr++ {
		if _, ok := regs[addr]; !ok {
			regs[addr] = 0
		}
	}
	// Accumulated stats (U32 pairs)
	regs[register.AddrAccumBatteryCharge] = 5000
	regs[register.AddrAccumBatteryCharge+1] = 0
	regs[register.AddrAccumBatteryDischarge] = 4500
	regs[register.AddrAccumBatteryDischarge+1] = 0
	regs[register.AddrAccumPVGeneration] = 10000
	regs[register.AddrAccumPVGeneration+1] = 0
	regs[register.AddrAccumLoadConsumption] = 8000
	regs[register.AddrAccumLoadConsumption+1] = 0
	regs[register.AddrAccumMainsCharge] = 0
	regs[register.AddrAccumMainsCharge+1] = 0
	regs[register.AddrAccumInverterHours] = 8000
	regs[register.AddrAccumBypassHours] = 100

	return regs
}
