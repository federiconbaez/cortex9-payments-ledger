package memory

import (
	"context"
	"sync"

	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

// Store es un mayor en memoria con commit optimista por generacion.
// Un commit intermedio hace fallar al que computo sobre un snapshot viejo
// (ErrConflict) para que el servicio reintente el mismo comando.
type Store struct {
	mu  sync.Mutex
	led *domain.Ledger
	gen uint64
}

// New construye un almacen vacio.
func New() *Store { return &Store{led: domain.NewLedger()} }

// Update clona el mayor, aplica fn sin retener el lock, y commitea solo
// si la generacion no cambio.
func (s *Store) Update(_ context.Context, fn func(*domain.Ledger) error) error {
	s.mu.Lock()
	gen := s.gen
	start := s.led.Clone()
	s.mu.Unlock()

	if err := fn(start); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return domain.ErrConflict
	}
	s.led = start
	s.gen++
	return nil
}

// View ejecuta fn sobre un clone de solo lectura.
func (s *Store) View(_ context.Context, fn func(*domain.Ledger) error) error {
	s.mu.Lock()
	snap := s.led.Clone()
	s.mu.Unlock()
	return fn(snap)
}
