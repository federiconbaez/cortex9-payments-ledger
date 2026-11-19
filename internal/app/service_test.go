package app_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sysclock "github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/clock"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/memory"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/app"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

func ready(t *testing.T) (*app.Service, context.Context) {
	t.Helper()
	svc := app.New(memory.New(), &sysclock.Fixed{T: time.Date(2026, 11, 19, 11, 30, 0, 0, time.UTC)}, &domain.Seq{})
	ctx := context.Background()
	for _, id := range []string{"alice", "bob"} {
		if _, err := svc.Open(ctx, domain.OpenAccount{ID: domain.AccountID(id), Shard: "s1", Currency: "ARS", IdemKey: domain.IdemKey("o-" + id)}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Fund(ctx, domain.Fund{Account: domain.AccountID(id), Amount: domain.Money{Currency: "ARS", Amount: 10_000}, IdemKey: domain.IdemKey("f-" + id)}); err != nil {
			t.Fatal(err)
		}
	}
	return svc, ctx
}

func TestConcurrentTransfersNoLostUpdates(t *testing.T) {
	svc, ctx := ready(t)
	const workers = 32
	const each = 20
	var wg sync.WaitGroup
	var fails atomic.Int64
	for w := 0; w < workers; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				from, to := domain.AccountID("alice"), domain.AccountID("bob")
				if (w+i)%2 == 1 {
					from, to = to, from
				}
				_, err := svc.Transfer(ctx, domain.Transfer{
					From: from, To: to,
					Amount:  domain.Money{Currency: "ARS", Amount: 1},
					IdemKey: domain.IdemKey(fmt.Sprintf("t-%d-%d", w, i)),
				})
				if err != nil {
					fails.Add(1)
					t.Errorf("transfer: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if fails.Load() != 0 {
		t.Fatalf("fallos: %d", fails.Load())
	}
	alice, err := svc.Account(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := svc.Account(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if alice.Balance()+bob.Balance() != 20_000 {
		t.Fatalf("conservacion rota A=%d B=%d", alice.Balance(), bob.Balance())
	}
	vs, err := svc.Verify(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) > 0 {
		t.Fatalf("invariantes: %v", vs)
	}
}
