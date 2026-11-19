package domain

import (
	"errors"
	"testing"
	"time"
)

func setup(t *testing.T) (*Ledger, *Seq, time.Time) {
	t.Helper()
	return NewLedger(), &Seq{}, time.Date(2026, 11, 19, 11, 30, 0, 0, time.UTC)
}

func mustOpen(t *testing.T, l *Ledger, ids *Seq, now time.Time, id, shard, ccy string, limit int64) {
	t.Helper()
	_, err := l.ApplyOpen(OpenAccount{ID: AccountID(id), Shard: Shard(shard), Currency: Currency(ccy), CreditLimit: limit, IdemKey: IdemKey("open-" + id)}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
}

func mustFund(t *testing.T, l *Ledger, ids *Seq, now time.Time, id string, amount int64, ccy Currency, key string) {
	t.Helper()
	_, err := l.ApplyFund(Fund{Account: AccountID(id), Amount: Money{Currency: ccy, Amount: amount}, IdemKey: IdemKey(key)}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
}

func flush(t *testing.T, l *Ledger, ids *Seq, now time.Time) {
	t.Helper()
	for i := 0; i < 32 && len(l.Outbox) > 0; i++ {
		msg := l.Outbox[0]
		if err := l.Deliver(msg, now, ids); err != nil {
			t.Fatal(err)
		}
	}
	if len(l.Outbox) > 0 {
		t.Fatalf("outbox no vacio: %+v", l.Outbox)
	}
}

func mustClean(t *testing.T, l *Ledger) {
	t.Helper()
	if vs := l.ClosedVerify(); len(vs) > 0 {
		t.Fatalf("invariantes: %v", vs)
	}
}

func TestOpenFundAndIntraTransfer(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 0)
	mustOpen(t, l, ids, now, "bob", "s1", "ARS", 0)
	mustFund(t, l, ids, now, "alice", 1000, "ARS", "fund-1")
	res, err := l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 400}, IdemKey: "t1"}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "settled" || len(res.EntryIDs) != 1 {
		t.Fatalf("%+v", res)
	}
	if l.Accounts["alice"].Balance() != 600 || l.Accounts["bob"].Balance() != 400 {
		t.Fatalf("saldos A=%d B=%d", l.Accounts["alice"].Balance(), l.Accounts["bob"].Balance())
	}
	mustClean(t, l)
}

func TestIdempotentReplayAndConflict(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 0)
	mustOpen(t, l, ids, now, "bob", "s1", "ARS", 0)
	mustFund(t, l, ids, now, "alice", 500, "ARS", "f")
	cmd := Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 100}, IdemKey: "same"}
	a, err := l.ApplyTransfer(cmd, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	b, err := l.ApplyTransfer(cmd, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	if a.EntryIDs[0] != b.EntryIDs[0] {
		t.Fatal("el reintento no devolvio el mismo asiento")
	}
	if l.Accounts["alice"].Balance() != 400 {
		t.Fatalf("doble efecto: %d", l.Accounts["alice"].Balance())
	}
	cmd2 := cmd
	cmd2.Amount.Amount = 50
	if _, err := l.ApplyTransfer(cmd2, now, ids); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicto: %v", err)
	}
	mustClean(t, l)
}

func TestNoImplicitFXAndInsufficientFunds(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 0)
	mustOpen(t, l, ids, now, "bob", "s1", "USD", 0)
	mustFund(t, l, ids, now, "alice", 100, "ARS", "f")
	_, err := l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 10}, IdemKey: "fx"}, now, ids)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("fx: %v", err)
	}
	mustOpen(t, l, ids, now, "cara", "s1", "ARS", 0)
	_, err = l.ApplyTransfer(Transfer{From: "alice", To: "cara", Amount: Money{Currency: "ARS", Amount: 1000}, IdemKey: "sf"}, now, ids)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("fondos: %v", err)
	}
	mustClean(t, l)
}

func TestCreditLimitAllowsOverdraftUpToLimit(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 200)
	mustOpen(t, l, ids, now, "bob", "s1", "ARS", 0)
	_, err := l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 150}, IdemKey: "od"}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	if l.Accounts["alice"].Balance() != -150 {
		t.Fatalf("saldo=%d", l.Accounts["alice"].Balance())
	}
	_, err = l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 100}, IdemKey: "od2"}, now, ids)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("topeak: %v", err)
	}
	mustClean(t, l)
}

func TestCrossShardSagaSettles(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 0)
	mustOpen(t, l, ids, now, "bob", "s2", "ARS", 0)
	mustFund(t, l, ids, now, "alice", 1000, "ARS", "f")
	res, err := l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 250}, IdemKey: "x"}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(SagaReserved) || res.SagaID == "" {
		t.Fatalf("%+v", res)
	}
	if l.Accounts["alice"].Balance() != 750 {
		t.Fatalf("alice=%d", l.Accounts["alice"].Balance())
	}
	flush(t, l, ids, now)
	if l.Sagas[res.SagaID].State != SagaSettled {
		t.Fatalf("saga %s", l.Sagas[res.SagaID].State)
	}
	if l.Accounts["bob"].Balance() != 250 {
		t.Fatalf("bob=%d", l.Accounts["bob"].Balance())
	}
	mustClean(t, l)
}

func TestCrossShardReleaseWhenPayeeFrozen(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 0)
	mustOpen(t, l, ids, now, "bob", "s2", "ARS", 0)
	mustFund(t, l, ids, now, "alice", 1000, "ARS", "f")
	res, err := l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 250}, IdemKey: "x"}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.ApplyFreeze(Freeze{Account: "bob", IdemKey: "fr"}, now, ids); err != nil {
		t.Fatal(err)
	}
	flush(t, l, ids, now)
	if l.Sagas[res.SagaID].State != SagaReleased {
		t.Fatalf("saga %s", l.Sagas[res.SagaID].State)
	}
	if l.Accounts["alice"].Balance() != 1000 || l.Accounts["bob"].Balance() != 0 {
		t.Fatalf("A=%d B=%d", l.Accounts["alice"].Balance(), l.Accounts["bob"].Balance())
	}
	if tr := l.Accounts[transitID(res.SagaID)]; tr.Balance() != 0 {
		t.Fatalf("transit=%d", tr.Balance())
	}
	mustClean(t, l)
}

func TestMessageDuplicationIsIdempotent(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 0)
	mustOpen(t, l, ids, now, "bob", "s2", "ARS", 0)
	mustFund(t, l, ids, now, "alice", 100, "ARS", "f")
	res, err := l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 40}, IdemKey: "x"}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	msg := l.Outbox[0]
	if err := l.Deliver(msg, now, ids); err != nil {
		t.Fatal(err)
	}
	if err := l.Deliver(msg, now, ids); err != nil {
		t.Fatal(err)
	}
	flush(t, l, ids, now)
	if l.Accounts["bob"].Balance() != 40 {
		t.Fatalf("doble capture: %d", l.Accounts["bob"].Balance())
	}
	if l.Sagas[res.SagaID].State != SagaSettled {
		t.Fatal(l.Sagas[res.SagaID].State)
	}
	mustClean(t, l)
}

func TestReversalCompensatesAndCannotRepeat(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 0)
	mustOpen(t, l, ids, now, "bob", "s1", "ARS", 0)
	mustFund(t, l, ids, now, "alice", 100, "ARS", "f")
	tr, err := l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 30}, IdemKey: "t"}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := l.ApplyReverse(Reverse{EntryID: tr.EntryIDs[0], IdemKey: "r"}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	if l.Accounts["alice"].Balance() != 100 || l.Accounts["bob"].Balance() != 0 {
		t.Fatalf("A=%d B=%d", l.Accounts["alice"].Balance(), l.Accounts["bob"].Balance())
	}
	if _, err := l.ApplyReverse(Reverse{EntryID: tr.EntryIDs[0], IdemKey: "r2"}, now, ids); !errors.Is(err, ErrAlreadyReversed) {
		t.Fatalf("segunda reversa: %v", err)
	}
	again, err := l.ApplyReverse(Reverse{EntryID: tr.EntryIDs[0], IdemKey: "r"}, now, ids)
	if err != nil || again.EntryIDs[0] != rev.EntryIDs[0] {
		t.Fatalf("idemp reversa %v %+v", err, again)
	}
	mustClean(t, l)
}

func TestTamperIsDetected(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 0)
	mustFund(t, l, ids, now, "alice", 50, "ARS", "f")
	e := l.Log["s1"][0]
	e.Lines[0].Amount = 99
	l.Log["s1"][0] = e
	vs := l.Verify()
	found := false
	for _, v := range vs {
		if v.Code == "hash" || v.Code == "unbalanced" || v.Code == "drift" || v.Code == "equation" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no se detecto adulteracion: %v", vs)
	}
}

func TestStatementRunningBalance(t *testing.T) {
	l, ids, now := setup(t)
	mustOpen(t, l, ids, now, "alice", "s1", "ARS", 0)
	mustOpen(t, l, ids, now, "bob", "s1", "ARS", 0)
	mustFund(t, l, ids, now, "alice", 100, "ARS", "f")
	_, err := l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 20}, IdemKey: "t"}, now, ids)
	if err != nil {
		t.Fatal(err)
	}
	st, err := l.Statement("alice")
	if err != nil || len(st) != 2 {
		t.Fatalf("%v %+v", err, st)
	}
	if st[0].Balance != 100 || st[1].Balance != 80 {
		t.Fatalf("saldos corridos %+v", st)
	}
}
