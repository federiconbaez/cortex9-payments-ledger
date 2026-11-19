package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	sysclock "github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/clock"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/app"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

func TestFileStoreRecoversAndSurvivesTornTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.bin")
	ctx := context.Background()
	clk := &sysclock.Fixed{T: time.Date(2026, 11, 19, 11, 30, 0, 0, time.UTC)}
	ids := &domain.Seq{}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := app.New(s, clk, ids)
	if _, err := svc.Open(ctx, domain.OpenAccount{ID: "alice", Shard: "s1", Currency: "ARS", IdemKey: "o"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Fund(ctx, domain.Fund{Account: "alice", Amount: domain.Money{Currency: "ARS", Amount: 500}, IdemKey: "f"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	svc = app.New(s, clk, ids)
	acc, err := svc.Account(ctx, "alice")
	if err != nil || acc.Balance() != 500 {
		t.Fatalf("reopen: %+v %v", acc, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	good := info.Size()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("cola-rota-incompleta\x00\x01")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	svc = app.New(s, clk, ids)
	acc, err = svc.Account(ctx, "alice")
	if err != nil || acc.Balance() != 500 {
		t.Fatalf("tras cola rota: %+v %v", acc, err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != good {
		t.Fatalf("no se trunco la cola: size=%d good=%d", info.Size(), good)
	}
	if _, err := svc.Fund(ctx, domain.Fund{Account: "alice", Amount: domain.Money{Currency: "ARS", Amount: 7}, IdemKey: "f2"}); err != nil {
		t.Fatal(err)
	}
	acc, err = svc.Account(ctx, "alice")
	if err != nil || acc.Balance() != 507 {
		t.Fatalf("seguir escribiendo: %+v %v", acc, err)
	}
	vs, err := svc.Verify(ctx, true)
	if err != nil || len(vs) > 0 {
		t.Fatalf("verify %v %v", vs, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
