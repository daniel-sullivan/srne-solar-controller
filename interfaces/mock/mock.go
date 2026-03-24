package mock

import (
	"fmt"
	"sync"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
)

// Inverter is an in-memory MODBUS device for testing.
// It implements the modbus.Client interface with a configurable register map.
type Inverter struct {
	mu        sync.Mutex
	registers map[uint16]uint16
	connected bool

	// ReadCount tracks total ReadRegisters calls for test assertions.
	ReadCount int
	// WriteCount tracks total write calls for test assertions.
	WriteCount int

	// ReadHook is called on every ReadRegisters call before returning data.
	// Return a non-nil error to simulate a device error for that range.
	ReadHook func(startAddr, count uint16) error
	// WriteHook is called on every write call before committing.
	WriteHook func(addr, value uint16) error
}

// NewInverter creates a mock Inverter with the given initial register values.
func NewInverter(initial map[uint16]uint16) *Inverter {
	regs := make(map[uint16]uint16, len(initial))
	for k, v := range initial {
		regs[k] = v
	}
	return &Inverter{
		registers: regs,
	}
}

func (m *Inverter) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

func (m *Inverter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}

func (m *Inverter) ReadRegisters(startAddr uint16, count uint16) ([]uint16, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, fmt.Errorf("not connected")
	}
	m.ReadCount++

	if m.ReadHook != nil {
		if err := m.ReadHook(startAddr, count); err != nil {
			return nil, err
		}
	}

	values := make([]uint16, count)
	for i := uint16(0); i < count; i++ {
		v, ok := m.registers[startAddr+i]
		if !ok {
			return nil, &modbus.ModbusError{
				FunctionCode:  modbus.FuncReadHoldingRegisters,
				ExceptionCode: 0x02, // illegal address
			}
		}
		values[i] = v
	}
	return values, nil
}

func (m *Inverter) WriteSingleRegister(addr uint16, value uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return fmt.Errorf("not connected")
	}
	m.WriteCount++

	if m.WriteHook != nil {
		if err := m.WriteHook(addr, value); err != nil {
			return err
		}
	}

	if _, ok := m.registers[addr]; !ok {
		return &modbus.ModbusError{
			FunctionCode:  modbus.FuncWriteSingleRegister,
			ExceptionCode: 0x02,
		}
	}
	m.registers[addr] = value
	return nil
}

func (m *Inverter) WriteMultipleRegisters(startAddr uint16, values []uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return fmt.Errorf("not connected")
	}
	m.WriteCount++

	// Validate all addresses exist before writing any
	for i := range values {
		addr := startAddr + uint16(i)
		if _, ok := m.registers[addr]; !ok {
			return &modbus.ModbusError{
				FunctionCode:  modbus.FuncWriteMultipleRegisters,
				ExceptionCode: 0x02,
			}
		}
	}
	for i, v := range values {
		m.registers[startAddr+uint16(i)] = v
	}
	return nil
}

// SetRegister sets a register value directly (bypasses hooks).
func (m *Inverter) SetRegister(addr, value uint16) {
	m.mu.Lock()
	m.registers[addr] = value
	m.mu.Unlock()
}

// GetRegister reads a register value directly.
func (m *Inverter) GetRegister(addr uint16) (uint16, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.registers[addr]
	return v, ok
}
