package domain

import (
	"testing"
	"testing/quick"
	"time"
)

func TestHashChainDependsOnPrevAndPayload(t *testing.T) {
	now := time.Unix(0, 1).UTC()
	e := Entry{ID: "e1", Shard: "s1", Ts: now, Kind: KindFund, Lines: []Posting{
		{Account: "a", Side: Debit, Amount: 1, Currency: "ARS"},
		{Account: "b", Side: Credit, Amount: 1, Currency: "ARS"},
	}}
	h1 := ComputeHash(Hash{}, e)
	h2 := ComputeHash(Hash{1}, e)
	if h1 == h2 {
		t.Fatal("el prev hash no participa")
	}
	e.Lines[0].Amount = 2
	h3 := ComputeHash(Hash{}, e)
	if h1 == h3 {
		t.Fatal("el payload no participa")
	}
}

func TestHashIsDeterministic(t *testing.T) {
	fn := func(id string, amount uint32) bool {
		if id == "" {
			id = "e"
		}
		e := Entry{ID: EntryID(id), Shard: "s", Ts: time.Unix(0, 0).UTC(), Kind: KindTransfer, Lines: []Posting{
			{Account: "a", Side: Debit, Amount: int64(amount) + 1, Currency: "USD"},
			{Account: "b", Side: Credit, Amount: int64(amount) + 1, Currency: "USD"},
		}}
		a := ComputeHash(Hash{}, e)
		b := ComputeHash(Hash{}, e)
		return a == b
	}
	if err := quick.Check(fn, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}
