package domain

import (
	"testing"
	"time"
)

func FuzzComputeHash(f *testing.F) {
	f.Add("e1", int64(1), "ARS")
	f.Add("", int64(0), "USD")
	f.Add("entrada larga con unicode ñoño", int64(-3), "xxx")
	f.Fuzz(func(t *testing.T, id string, amount int64, ccy string) {
		if amount < 0 {
			amount = -amount
		}
		if amount == 0 {
			amount = 1
		}
		cur, err := ParseCurrency(ccy)
		if err != nil {
			cur = "ARS"
		}
		e := Entry{
			ID:    EntryID(id),
			Shard: "s1",
			Ts:    time.Unix(0, 0).UTC(),
			Kind:  KindTransfer,
			Lines: []Posting{
				{Account: "a", Side: Debit, Amount: amount, Currency: cur},
				{Account: "b", Side: Credit, Amount: amount, Currency: cur},
			},
		}
		h1 := ComputeHash(Hash{}, e)
		h2 := ComputeHash(Hash{}, e)
		if h1 != h2 {
			t.Fatal("hash no determinista")
		}
		var other Hash
		other[0] = 1
		if ComputeHash(other, e) == h1 && amount > 0 {
			t.Fatal("el prev no cambia el hash")
		}
	})
}
