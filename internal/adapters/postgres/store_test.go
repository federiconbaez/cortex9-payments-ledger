package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	sysclock "github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/clock"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/app"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

func TestPostgresTransferAndIdempotency(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL no definido")
	}
	ctx := context.Background()
	st, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	// Aislar este test sobre una fila nueva (unica en el corte).
	_, _ = st.pool.Exec(ctx, `UPDATE ledger_snap SET gen=0, blob=$1 WHERE id=1`, encode(domain.NewLedger()))

	svc := app.New(st, &sysclock.Fixed{T: time.Date(2026, 11, 19, 11, 30, 0, 0, time.UTC)}, &domain.Seq{})
	if _, err := svc.Open(ctx, domain.OpenAccount{ID: "alice", Shard: "s1", Currency: "ARS", IdemKey: "o1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(ctx, domain.OpenAccount{ID: "bob", Shard: "s2", Currency: "ARS", IdemKey: "o2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Fund(ctx, domain.Fund{Account: "alice", Amount: domain.Money{Currency: "ARS", Amount: 300}, IdemKey: "f"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Transfer(ctx, domain.Transfer{From: "alice", To: "bob", Amount: domain.Money{Currency: "ARS", Amount: 90}, IdemKey: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if res.SagaID == "" {
		t.Fatal("se esperaba saga cross-shard")
	}
	if err := svc.Flush(ctx, 0); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Transfer(ctx, domain.Transfer{From: "alice", To: "bob", Amount: domain.Money{Currency: "ARS", Amount: 90}, IdemKey: "t"})
	if err != nil || again.SagaID != res.SagaID {
		t.Fatalf("idemp %v %+v", err, again)
	}
	alice, _ := svc.Account(ctx, "alice")
	bob, _ := svc.Account(ctx, "bob")
	if alice.Balance() != 210 || bob.Balance() != 90 {
		t.Fatalf("A=%d B=%d", alice.Balance(), bob.Balance())
	}
	vs, err := svc.Verify(ctx, true)
	if err != nil || len(vs) > 0 {
		t.Fatalf("verify %v %v", vs, err)
	}
}
