package domain

import (
	"fmt"
	"time"
)

// Violation es un invariante roto.
type Violation struct {
	Code   string
	Detail string
}

func (v Violation) String() string { return v.Code + ": " + v.Detail }

// Verify recorre el mayor y reporta todas las violaciones encontradas.
func (l *Ledger) Verify() []Violation {
	var out []Violation
	out = append(out, l.verifyChains()...)
	out = append(out, l.verifyBalances()...)
	out = append(out, l.verifyEquation()...)
	out = append(out, l.verifyLimits()...)
	out = append(out, l.verifySagas()...)
	return out
}

// ClosedVerify exige, ademas, que no queden sagas no terminales ni outbox.
func (l *Ledger) ClosedVerify() []Violation {
	v := l.Verify()
	if len(l.Outbox) > 0 {
		v = append(v, Violation{Code: "open_outbox", Detail: fmt.Sprintf("%d mensajes", len(l.Outbox))})
	}
	for _, s := range l.Sagas {
		if !s.State.Terminal() {
			v = append(v, Violation{Code: "open_saga", Detail: fmt.Sprintf("%s %s", s.ID, s.State)})
		}
	}
	return v
}

func (l *Ledger) verifyChains() []Violation {
	var vs []Violation
	for shard, log := range l.Log {
		var prev Hash
		for i, e := range log {
			if !e.Balanced() {
				vs = append(vs, Violation{Code: "unbalanced", Detail: fmt.Sprintf("%s/%s", shard, e.ID)})
			}
			if e.PrevHash != prev {
				vs = append(vs, Violation{Code: "prev_hash", Detail: fmt.Sprintf("%s idx=%d", shard, i)})
			}
			want := ComputeHash(prev, e)
			if e.Hash != want {
				vs = append(vs, Violation{Code: "hash", Detail: fmt.Sprintf("%s/%s", shard, e.ID)})
			}
			prev = e.Hash
		}
		if l.Heads[shard] != prev {
			vs = append(vs, Violation{Code: "head", Detail: string(shard)})
		}
	}
	return vs
}

func (l *Ledger) verifyBalances() []Violation {
	type acc struct{ d, c int64 }
	sums := map[AccountID]acc{}
	for _, log := range l.Log {
		for _, e := range log {
			for _, ln := range e.Lines {
				a := sums[ln.Account]
				if ln.Side == Debit {
					a.d += ln.Amount
				} else {
					a.c += ln.Amount
				}
				sums[ln.Account] = a
			}
		}
	}
	var vs []Violation
	for id, a := range l.Accounts {
		got := sums[id]
		if got.d != a.Debits || got.c != a.Credits {
			vs = append(vs, Violation{Code: "drift", Detail: fmt.Sprintf("%s diario=(%d,%d) cuenta=(%d,%d)", id, got.d, got.c, a.Debits, a.Credits)})
		}
	}
	for id, got := range sums {
		if _, ok := l.Accounts[id]; !ok {
			vs = append(vs, Violation{Code: "orphan_posting", Detail: string(id)})
			_ = got
		}
	}
	return vs
}

func (l *Ledger) verifyEquation() []Violation {
	assets := map[Currency]int64{}
	liabs := map[Currency]int64{}
	ccy := map[Currency]struct{}{}
	for _, a := range l.Accounts {
		ccy[a.Currency] = struct{}{}
		switch a.Type {
		case Asset:
			assets[a.Currency] += a.Balance()
		case Liability:
			liabs[a.Currency] += a.Balance()
		}
	}
	var vs []Violation
	for c := range ccy {
		if assets[c] != liabs[c] {
			vs = append(vs, Violation{Code: "equation", Detail: fmt.Sprintf("%s activos=%d pasivos=%d", c, assets[c], liabs[c])})
		}
	}
	debits := map[Currency]int64{}
	credits := map[Currency]int64{}
	for _, log := range l.Log {
		for _, e := range log {
			for _, ln := range e.Lines {
				if ln.Side == Debit {
					debits[ln.Currency] += ln.Amount
				} else {
					credits[ln.Currency] += ln.Amount
				}
			}
		}
	}
	for c, d := range debits {
		if credits[c] != d {
			vs = append(vs, Violation{Code: "global_dc", Detail: fmt.Sprintf("%s D=%d C=%d", c, d, credits[c])})
		}
	}
	return vs
}

func (l *Ledger) verifyLimits() []Violation {
	var vs []Violation
	for _, a := range l.Accounts {
		if a.System {
			continue
		}
		if a.Balance() < -a.CreditLimit {
			vs = append(vs, Violation{Code: "overdraft", Detail: fmt.Sprintf("%s saldo=%d limite=%d", a.ID, a.Balance(), a.CreditLimit)})
		}
	}
	return vs
}

func (l *Ledger) verifySagas() []Violation {
	var vs []Violation
	for _, s := range l.Sagas {
		if s.Amount.Validate() != nil {
			vs = append(vs, Violation{Code: "saga_amount", Detail: string(s.ID)})
		}
		if s.State.Terminal() && s.State == SagaSettled && s.ReserveEntry == "" {
			vs = append(vs, Violation{Code: "saga_settled_no_reserve", Detail: string(s.ID)})
		}
	}
	return vs
}

// Statement arma un extracto de un wallet con saldo corrido.
func (l *Ledger) Statement(id AccountID) ([]StatementLine, error) {
	a, err := l.get(id)
	if err != nil {
		return nil, err
	}
	var lines []StatementLine
	var debits, credits int64
	for _, e := range l.Log[a.Shard] {
		for _, ln := range e.Lines {
			if ln.Account != id {
				continue
			}
			if ln.Side == Debit {
				debits += ln.Amount
			} else {
				credits += ln.Amount
			}
			bal := credits - debits
			if a.Type == Asset {
				bal = debits - credits
			}
			lines = append(lines, StatementLine{
				EntryID: e.ID, Ts: e.Ts, Kind: e.Kind, Side: ln.Side,
				Amount: ln.Amount, Currency: ln.Currency, Balance: bal,
			})
		}
	}
	return lines, nil
}

// StatementLine es una linea de extracto.
type StatementLine struct {
	EntryID  EntryID
	Ts       time.Time
	Kind     EntryKind
	Side     Side
	Amount   int64
	Currency Currency
	Balance  int64
}
