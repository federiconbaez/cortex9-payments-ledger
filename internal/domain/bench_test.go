package domain

import (
	"fmt"
	"testing"
	"time"
)

func benchNow() time.Time { return time.Date(2026, 11, 19, 11, 30, 0, 0, time.UTC) }

func BenchmarkIntraTransfer(b *testing.B) {
	l, ids, now := NewLedger(), &Seq{}, benchNow()
	_, _ = l.ApplyOpen(OpenAccount{ID: "alice", Shard: "s1", Currency: "ARS", IdemKey: "oa"}, now, ids)
	_, _ = l.ApplyOpen(OpenAccount{ID: "bob", Shard: "s1", Currency: "ARS", IdemKey: "ob"}, now, ids)
	_, _ = l.ApplyFund(Fund{Account: "alice", Amount: Money{Currency: "ARS", Amount: 1_000_000_000}, IdemKey: "f"}, now, ids)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := l.ApplyTransfer(Transfer{
			From: "alice", To: "bob",
			Amount:  Money{Currency: "ARS", Amount: 1},
			IdemKey: IdemKey(fmt.Sprintf("t-%d", i)),
		}, now, ids)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHash(b *testing.B) {
	e := Entry{ID: "e1", Shard: "s1", Ts: benchNow(), Kind: KindTransfer, Lines: []Posting{
		{Account: "a", Side: Debit, Amount: 42, Currency: "ARS"},
		{Account: "b", Side: Credit, Amount: 42, Currency: "ARS"},
	}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ComputeHash(Hash{}, e)
	}
}

func BenchmarkVerify(b *testing.B) {
	l, ids, now := NewLedger(), &Seq{}, benchNow()
	_, _ = l.ApplyOpen(OpenAccount{ID: "alice", Shard: "s1", Currency: "ARS", IdemKey: "oa"}, now, ids)
	_, _ = l.ApplyOpen(OpenAccount{ID: "bob", Shard: "s1", Currency: "ARS", IdemKey: "ob"}, now, ids)
	_, _ = l.ApplyFund(Fund{Account: "alice", Amount: Money{Currency: "ARS", Amount: 1_000_000}, IdemKey: "f"}, now, ids)
	for i := 0; i < 128; i++ {
		_, _ = l.ApplyTransfer(Transfer{From: "alice", To: "bob", Amount: Money{Currency: "ARS", Amount: 1}, IdemKey: IdemKey(fmt.Sprintf("t-%d", i))}, now, ids)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if vs := l.Verify(); len(vs) > 0 {
			b.Fatal(vs)
		}
	}
}
