package app

import (
	"context"
	"time"

	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

// Clock es el puerto de tiempo.
type Clock interface {
	Now() time.Time
}

// Store es el puerto de persistencia. Update aplica fn sobre un clone y
// commitea solo si las versiones de las cuentas no cambiaron.
type Store interface {
	Update(ctx context.Context, fn func(*domain.Ledger) error) error
	View(ctx context.Context, fn func(*domain.Ledger) error) error
}

// Service orquesta el dominio contra el almacen y el reloj.
type Service struct {
	store Store
	clock Clock
	ids   domain.IDGen
}

// New construye el servicio.
func New(store Store, clock Clock, ids domain.IDGen) *Service {
	if ids == nil {
		ids = domain.RandomIDs{}
	}
	return &Service{store: store, clock: clock, ids: ids}
}
