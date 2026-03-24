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
type Sim struct {
	*Inverter

	mu      sync.Mutex
	ticker  *time.Ticker
	done    chan struct{}
	simTime time.Time // virtual clock (allows test control)
	soc     float64   // fractional SOC for smooth drift
	loadW   float64   // current load in watts
	faultQ  []uint16  // queued fault codes to inject
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

// SetSOC directly sets the battery state of charge (0-100).
func (s *Sim) SetSOC(pct float64) {
	s.mu.Lock()
	s.soc = clamp(pct, 0, 100)
	s.mu.Unlock()
	s.syncRegisters()
}

// SetLoad sets the current AC load in watts.
func (s *Sim) SetLoad(watts float64) {
	s.mu.Lock()
	s.loadW = watts
	s.mu.Unlock()
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

func (s *Sim) step() {
	s.mu.Lock()
	hour := s.simTime.Hour()
	load := s.loadW
	faults := s.faultQ
	s.faultQ = nil
	s.mu.Unlock()

	// PV generation follows a bell curve peaking at solar noon (13:00)
	pvW := pvCurve(hour) + jitter(50)
	pv1W := pvW * 0.6
	pv2W := pvW * 0.4

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

func (s *Sim) syncRegisters() {
	s.mu.Lock()
	soc := s.soc
	s.mu.Unlock()

	battV := 44.0 + (soc/100.0)*10.4
	s.SetRegister(register.AddrBatterySOC, uint16(soc))
	s.SetRegister(register.AddrBatteryVoltage, uint16(battV*10))
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

		// Inverter data
		register.AddrFaultBits:           0,
		register.AddrFaultBits + 1:       0,
		register.AddrFaultBitsExt:        0,
		register.AddrFaultBitsExt + 1:    0,
		register.AddrFaultCode1:          0,
		register.AddrFaultCode2:          0,
		register.AddrFaultCode3:          0,
		register.AddrFaultCode4:          0,
		register.AddrMachineState:        5,    // inverter mode
		register.AddrBusVoltage:          3800, // 380.0V
		register.AddrGridVoltageL1:       0,    // off-grid
		register.AddrGridCurrentL1:       0,
		register.AddrGridFrequency:       0,
		register.AddrInverterVoltageL1:   1200, // 120.0V
		register.AddrInverterCurrentL1:   42,   // 4.2A
		register.AddrInverterFrequency:   6000, // 60.00Hz
		register.AddrLoadCurrentL1:       42,
		register.AddrLoadPowerFactor:     980,
		register.AddrLoadPowerL1:         500,
		register.AddrLoadApparentPowerL1: 510,
		register.AddrMainsChargeCurrent:  0,
		register.AddrLoadRatioL1:         5,
		register.AddrHeatsinkATemp:       350, // 35.0°C
		register.AddrHeatsinkBTemp:       380, // 38.0°C
		register.AddrHeatsinkCTemp:       300, // 30.0°C
		register.AddrHeatsinkDTemp:       250, // 25.0°C
		register.AddrPVChargeCurrentBatt: 200,
		register.AddrInverterVoltageL2:   1200,
		register.AddrInverterCurrentL2:   42,
		register.AddrLoadCurrentL2:       42,
		register.AddrLoadPowerL2:         500,
		register.AddrLoadApparentPowerL2: 510,
		register.AddrLoadRatioL2:         5,

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

		// Inverter settings
		register.AddrOutputPriority:        0,    // SOL
		register.AddrMainsChargeCurrentLim: 250,  // 25.0A
		register.AddrOutputVoltage:         1200, // 120.0V
		register.AddrOutputFrequency:       6000, // 60.00Hz
		register.AddrMaxChargeCurrent:      800,  // 80.0A
		register.AddrChargerPriority:       0,    // CSO
		register.AddrBMSProtocol:           1,
		register.AddrBMSCommunicationEn:    1,
		register.AddrBMSErrorStopEnable:    1,

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

	// Fill serial number ASCII (20 registers)
	serial := "MOCK000012345678    "
	for i := 0; i < 20; i++ {
		if i*2+1 < len(serial) {
			regs[register.AddrSerialNumber+uint16(i)] = uint16(serial[i*2])<<8 | uint16(serial[i*2+1])
		}
	}

	return regs
}
