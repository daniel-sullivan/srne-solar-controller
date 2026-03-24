package modbus

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
)

// Client is the interface for communicating with a MODBUS device.
// Implementations include Solarman V5 (TCP via wifi dongle) and
// direct RS485 serial connections.
type Client interface {
	Connect() error
	Close() error
	ReadRegisters(startAddr uint16, count uint16) ([]uint16, error)
	WriteSingleRegister(addr uint16, value uint16) error
	WriteMultipleRegisters(startAddr uint16, values []uint16) error
}

// Lookup reads a single register value, returning a cached result if available.
// This is the function signature passed to ScaleFuncs so they can
// resolve dependent registers (e.g., system voltage) on demand.
type Lookup func(addr uint16) (uint16, error)

// Session wraps a Client with a per-connection register cache and
// automatic retry with exponential backoff on I/O timeouts
// (common when the MODBUS bus is contended by other services).
type Session struct {
	Client Client
	mu     sync.Mutex
	cache  map[uint16]uint16
}

// NewSession creates a session with caching and retries over the given client.
func NewSession(client Client) *Session {
	return &Session{
		Client: client,
		cache:  make(map[uint16]uint16),
	}
}

func (s *Session) retryOpts() []backoff.RetryOption {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 1 * time.Second
	b.MaxInterval = 5 * time.Second
	return []backoff.RetryOption{
		backoff.WithBackOff(b),
		backoff.WithMaxTries(4),
	}
}

// ReadRegisters reads registers with automatic retry on timeout.
func (s *Session) ReadRegisters(startAddr uint16, count uint16) ([]uint16, error) {
	return retryOnTimeout(s.retryOpts(), func() ([]uint16, error) {
		return s.Client.ReadRegisters(startAddr, count)
	})
}

// Lookup returns a cached register value or reads it from the device.
func (s *Session) Lookup(addr uint16) (uint16, error) {
	s.mu.Lock()
	if v, ok := s.cache[addr]; ok {
		s.mu.Unlock()
		return v, nil
	}
	s.mu.Unlock()

	vals, err := s.ReadRegisters(addr, 1)
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	s.cache[addr] = vals[0]
	s.mu.Unlock()
	return vals[0], nil
}

// Store caches register values (e.g., from a bulk read) so that
// subsequent Lookup calls don't hit the device.
func (s *Session) Store(startAddr uint16, values []uint16) {
	s.mu.Lock()
	for i, v := range values {
		s.cache[startAddr+uint16(i)] = v
	}
	s.mu.Unlock()
}

// WriteSingleRegister writes a register with retry and invalidates the cache.
func (s *Session) WriteSingleRegister(addr uint16, value uint16) error {
	_, err := retryOnTimeout(s.retryOpts(), func() (struct{}, error) {
		return struct{}{}, s.Client.WriteSingleRegister(addr, value)
	})
	s.invalidate()
	return err
}

// WriteMultipleRegisters writes registers with retry and invalidates the cache.
func (s *Session) WriteMultipleRegisters(startAddr uint16, values []uint16) error {
	_, err := retryOnTimeout(s.retryOpts(), func() (struct{}, error) {
		return struct{}{}, s.Client.WriteMultipleRegisters(startAddr, values)
	})
	s.invalidate()
	return err
}

func (s *Session) invalidate() {
	s.mu.Lock()
	clear(s.cache)
	s.mu.Unlock()
}

// retryOnTimeout retries fn with exponential backoff, but only for timeout errors.
// Non-timeout errors (e.g., illegal address) are returned immediately via Permanent().
func retryOnTimeout[T any](opts []backoff.RetryOption, fn func() (T, error)) (T, error) {
	return backoff.Retry(context.Background(), func() (T, error) {
		result, err := fn()
		if err != nil && !isTimeout(err) {
			return result, backoff.Permanent(err)
		}
		return result, err
	}, opts...)
}

func isTimeout(err error) bool {
	return err != nil && strings.Contains(err.Error(), "timeout")
}
