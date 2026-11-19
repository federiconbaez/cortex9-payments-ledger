package app

import (
	"context"
	"errors"

	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

const maxRetries = 4096

func (s *Service) update(ctx context.Context, fn func(*domain.Ledger) error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = s.store.Update(ctx, fn)
		if err == nil || !errors.Is(err, domain.ErrConflict) {
			return err
		}
	}
	return err
}

// Open crea un wallet.
func (s *Service) Open(ctx context.Context, cmd domain.OpenAccount) (domain.Result, error) {
	var res domain.Result
	err := s.update(ctx, func(l *domain.Ledger) error {
		var err error
		res, err = l.ApplyOpen(cmd, s.clock.Now(), s.ids)
		return err
	})
	return res, err
}

// Fund acredita un wallet.
func (s *Service) Fund(ctx context.Context, cmd domain.Fund) (domain.Result, error) {
	var res domain.Result
	err := s.update(ctx, func(l *domain.Ledger) error {
		var err error
		res, err = l.ApplyFund(cmd, s.clock.Now(), s.ids)
		return err
	})
	return res, err
}

// Transfer mueve fondos o abre una saga.
func (s *Service) Transfer(ctx context.Context, cmd domain.Transfer) (domain.Result, error) {
	var res domain.Result
	err := s.update(ctx, func(l *domain.Ledger) error {
		var err error
		res, err = l.ApplyTransfer(cmd, s.clock.Now(), s.ids)
		return err
	})
	return res, err
}

// Reverse publica un asiento compensatorio.
func (s *Service) Reverse(ctx context.Context, cmd domain.Reverse) (domain.Result, error) {
	var res domain.Result
	err := s.update(ctx, func(l *domain.Ledger) error {
		var err error
		res, err = l.ApplyReverse(cmd, s.clock.Now(), s.ids)
		return err
	})
	return res, err
}

// Freeze congela un wallet.
func (s *Service) Freeze(ctx context.Context, cmd domain.Freeze) (domain.Result, error) {
	var res domain.Result
	err := s.update(ctx, func(l *domain.Ledger) error {
		var err error
		res, err = l.ApplyFreeze(cmd, s.clock.Now(), s.ids)
		return err
	})
	return res, err
}

// Flush entrega hasta max mensajes del outbox. max<=0 significa todos los
// pendientes al comenzar, mas los que se generen en la misma corrida, con
// un tope interno para no ciclar.
func (s *Service) Flush(ctx context.Context, max int) error {
	if max <= 0 {
		max = 1024
	}
	for i := 0; i < max; i++ {
		var empty bool
		err := s.update(ctx, func(l *domain.Ledger) error {
			if len(l.Outbox) == 0 {
				empty = true
				return nil
			}
			return l.Deliver(l.Outbox[0], s.clock.Now(), s.ids)
		})
		if err != nil {
			return err
		}
		if empty {
			return nil
		}
	}
	return nil
}

// FlushAllWith entrega los mensajes pendientes permitiendo al caller
// reordenar o duplicar el lote. Cada entrega es un commit separado.
func (s *Service) FlushAllWith(ctx context.Context, mutate func([]domain.Message) []domain.Message) error {
	var batch []domain.Message
	err := s.store.View(ctx, func(l *domain.Ledger) error {
		batch = append([]domain.Message(nil), l.Outbox...)
		return nil
	})
	if err != nil {
		return err
	}
	if mutate != nil {
		batch = mutate(batch)
	}
	for _, msg := range batch {
		m := msg
		if err := s.update(ctx, func(l *domain.Ledger) error {
			return l.Deliver(m, s.clock.Now(), s.ids)
		}); err != nil {
			return err
		}
	}
	return nil
}

// Account devuelve una copia de la cuenta.
func (s *Service) Account(ctx context.Context, id domain.AccountID) (domain.Account, error) {
	var a domain.Account
	err := s.store.View(ctx, func(l *domain.Ledger) error {
		acc, ok := l.Accounts[id]
		if !ok {
			return domain.ErrAccountNotFound
		}
		a = acc
		return nil
	})
	return a, err
}

// Statement devuelve el extracto.
func (s *Service) Statement(ctx context.Context, id domain.AccountID) ([]domain.StatementLine, error) {
	var lines []domain.StatementLine
	err := s.store.View(ctx, func(l *domain.Ledger) error {
		var err error
		lines, err = l.Statement(id)
		return err
	})
	return lines, err
}

// Saga devuelve una saga.
func (s *Service) Saga(ctx context.Context, id domain.SagaID) (domain.Saga, error) {
	var sg domain.Saga
	err := s.store.View(ctx, func(l *domain.Ledger) error {
		s0, ok := l.Sagas[id]
		if !ok {
			return domain.ErrSagaState
		}
		sg = s0
		return nil
	})
	return sg, err
}

// Verify corre el verificador sobre una vista consistente.
func (s *Service) Verify(ctx context.Context, closed bool) ([]domain.Violation, error) {
	var vs []domain.Violation
	err := s.store.View(ctx, func(l *domain.Ledger) error {
		if closed {
			vs = l.ClosedVerify()
		} else {
			vs = l.Verify()
		}
		return nil
	})
	return vs, err
}
