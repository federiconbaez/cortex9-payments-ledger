package domain

import "fmt"

// AccountType distingue la formula del saldo.
type AccountType uint8

const (
	// Liability: wallets, transito, due-to. Saldo = creditos - debitos.
	Liability AccountType = iota + 1
	// Asset: clearing, incoming. Saldo = debitos - creditos.
	Asset
)

func (t AccountType) String() string {
	switch t {
	case Liability:
		return "liability"
	case Asset:
		return "asset"
	default:
		return "unknown"
	}
}

// AccountID identifica una cuenta.
type AccountID string

// Shard es la particion de concurrencia y de diario.
type Shard string

// Account es el agregado de una cuenta. Debits y Credits son acumulados
// del diario, no un cache independiente.
type Account struct {
	ID          AccountID
	Shard       Shard
	Currency    Currency
	Type        AccountType
	CreditLimit int64
	System      bool
	Frozen      bool
	Version     uint64
	Debits      int64
	Credits     int64
}

// Balance devuelve el saldo en unidades menores, segun el tipo.
func (a Account) Balance() int64 {
	switch a.Type {
	case Liability:
		return a.Credits - a.Debits
	case Asset:
		return a.Debits - a.Credits
	default:
		return 0
	}
}

// CanPost informa si aplicar el lado no viola el limite de un wallet.
func (a Account) CanPost(side Side, amount int64) error {
	if amount <= 0 {
		return ErrInvalidMoney
	}
	if a.System {
		return nil
	}
	next := a
	next.apply(side, amount)
	if next.Balance() < -a.CreditLimit {
		return fmt.Errorf("%w: cuenta %s saldo=%d limite=%d", ErrInsufficientFunds, a.ID, a.Balance(), a.CreditLimit)
	}
	return nil
}

func (a *Account) apply(side Side, amount int64) {
	switch side {
	case Debit:
		a.Debits += amount
	case Credit:
		a.Credits += amount
	}
	a.Version++
}

func (a Account) validateNew() error {
	if a.ID == "" || a.Shard == "" {
		return ErrInvalidCommand
	}
	if _, err := ParseCurrency(string(a.Currency)); err != nil {
		return err
	}
	if a.Type != Liability && a.Type != Asset {
		return ErrUnknownAccountType
	}
	if a.CreditLimit < 0 {
		return ErrCreditLimitNegative
	}
	return nil
}
