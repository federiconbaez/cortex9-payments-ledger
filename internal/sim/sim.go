package sim

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	sysclock "github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/clock"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/filestore"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/app"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

// Config parametriza una corrida determinista.
type Config struct {
	Seed   int64
	Steps  int
	Dir    string
	Clock0 time.Time
}

// Resultado resume una corrida que no violó invariantes.
type Resultado struct {
	Seed       int64
	Steps      int
	Transfers  int
	Flushes    int
	Crashes    int
	Duplicates int
}

// Run ejecuta una semilla. Si falla, el error menciona la semilla para
// reproducirla con la misma Config.Seed.
func Run(cfg Config) (Resultado, error) {
	if cfg.Steps <= 0 {
		cfg.Steps = 80
	}
	if cfg.Clock0.IsZero() {
		cfg.Clock0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if cfg.Dir == "" {
		return Resultado{}, fmt.Errorf("semilla %d: falta Dir", cfg.Seed)
	}
	path := filepath.Join(cfg.Dir, "ledger.bin")
	rng := rand.New(rand.NewSource(cfg.Seed))
	clk := &sysclock.Fixed{T: cfg.Clock0}
	ids := &domain.Seq{ /* unique per seed */ }
	// Offset the sequence by seed so ids no chocan de forma interesante entre
	// corridas, pero son deterministas.
	for i := int64(0); i < cfg.Seed%97; i++ {
		ids.NewID("warm")
	}

	st, err := filestore.OpenWith(path, false)
	if err != nil {
		return Resultado{}, fmt.Errorf("semilla %d: open: %w", cfg.Seed, err)
	}
	svc := app.New(st, clk, ids)
	ctx := context.Background()
	if err := bootstrap(ctx, svc); err != nil {
		_ = st.Close()
		return Resultado{}, fmt.Errorf("semilla %d: bootstrap: %w", cfg.Seed, err)
	}

	var out Resultado
	out.Seed = cfg.Seed
	out.Steps = cfg.Steps

	for step := 0; step < cfg.Steps; step++ {
		clk.Advance(time.Second)
		op := rng.Intn(100)
		switch {
		case op < 45:
			if err := doTransfer(ctx, svc, rng, step); err != nil {
				_ = st.Close()
				return out, fmt.Errorf("semilla %d paso %d transfer: %w", cfg.Seed, step, err)
			}
			out.Transfers++
		case op < 55:
			if err := doFund(ctx, svc, rng, step); err != nil {
				_ = st.Close()
				return out, fmt.Errorf("semilla %d paso %d fund: %w", cfg.Seed, step, err)
			}
		case op < 80:
			dups := rng.Intn(3)
			if err := flushChaos(ctx, svc, rng, dups); err != nil {
				_ = st.Close()
				return out, fmt.Errorf("semilla %d paso %d flush: %w", cfg.Seed, step, err)
			}
			out.Flushes++
			if dups > 0 {
				out.Duplicates++
			}
		default:
			if err := st.Close(); err != nil {
				return out, fmt.Errorf("semilla %d paso %d close: %w", cfg.Seed, step, err)
			}
			st, err = filestore.OpenWith(path, false)
			if err != nil {
				return out, fmt.Errorf("semilla %d paso %d reopen: %w", cfg.Seed, step, err)
			}
			svc = app.New(st, clk, ids)
			out.Crashes++
		}
	}
	// Cierre: entregar todo el outbox residual, con reintentos.
	for i := 0; i < 8; i++ {
		if err := svc.Flush(ctx, 0); err != nil {
			_ = st.Close()
			return out, fmt.Errorf("semilla %d flush final: %w", cfg.Seed, err)
		}
	}
	vs, err := svc.Verify(ctx, true)
	if err != nil {
		_ = st.Close()
		return out, fmt.Errorf("semilla %d verify: %w", cfg.Seed, err)
	}
	if len(vs) > 0 {
		_ = st.Close()
		return out, fmt.Errorf("semilla %d invariantes: %v", cfg.Seed, vs)
	}
	if err := st.Close(); err != nil {
		return out, err
	}
	return out, nil
}

func bootstrap(ctx context.Context, svc *app.Service) error {
	accounts := []struct {
		id, shard string
	}{
		{"alice", "s1"}, {"cara", "s1"}, {"bob", "s2"}, {"dani", "s2"},
	}
	for _, a := range accounts {
		_, err := svc.Open(ctx, domain.OpenAccount{
			ID: domain.AccountID(a.id), Shard: domain.Shard(a.shard),
			Currency: "ARS", CreditLimit: 50, IdemKey: domain.IdemKey("open-" + a.id),
		})
		if err != nil && !errors.Is(err, domain.ErrAccountExists) {
			// idemp hit after crash/reopen during bootstrap is fine
			if _, accErr := svc.Account(ctx, domain.AccountID(a.id)); accErr != nil {
				return err
			}
		}
		_, err = svc.Fund(ctx, domain.Fund{
			Account: domain.AccountID(a.id),
			Amount:  domain.Money{Currency: "ARS", Amount: 1_000},
			IdemKey: domain.IdemKey("fund-" + a.id),
		})
		if err != nil {
			if _, accErr := svc.Account(ctx, domain.AccountID(a.id)); accErr != nil {
				return err
			}
		}
	}
	return nil
}

var names = []domain.AccountID{"alice", "cara", "bob", "dani"}

func doTransfer(ctx context.Context, svc *app.Service, rng *rand.Rand, step int) error {
	from := names[rng.Intn(len(names))]
	to := names[rng.Intn(len(names))]
	if from == to {
		to = names[(rng.Intn(len(names)-1)+1)%len(names)]
		if to == from {
			return nil
		}
	}
	amt := int64(rng.Intn(30) + 1)
	_, err := svc.Transfer(ctx, domain.Transfer{
		From: from, To: to,
		Amount:  domain.Money{Currency: "ARS", Amount: amt},
		IdemKey: domain.IdemKey(fmt.Sprintf("t-%d-%d", step, rng.Intn(1_000_000))),
	})
	if err == nil || errors.Is(err, domain.ErrInsufficientFunds) || errors.Is(err, domain.ErrIdempotencyConflict) {
		return nil
	}
	return err
}

func doFund(ctx context.Context, svc *app.Service, rng *rand.Rand, step int) error {
	id := names[rng.Intn(len(names))]
	_, err := svc.Fund(ctx, domain.Fund{
		Account: id,
		Amount:  domain.Money{Currency: "ARS", Amount: int64(rng.Intn(20) + 1)},
		IdemKey: domain.IdemKey(fmt.Sprintf("f-%d-%d", step, rng.Intn(1_000_000))),
	})
	return err
}

func flushChaos(ctx context.Context, svc *app.Service, rng *rand.Rand, extraDups int) error {
	mutate := func(in []domain.Message) []domain.Message {
		out := append([]domain.Message(nil), in...)
		rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		for i := 0; i < extraDups && len(in) > 0; i++ {
			out = append(out, in[rng.Intn(len(in))])
		}
		return out
	}
	return svc.FlushAllWith(ctx, mutate)
}

// RunDir crea un directorio temporal bajo root para una semilla.
func RunDir(root string, seed int64) (string, func(), error) {
	dir := filepath.Join(root, fmt.Sprintf("s-%d", seed))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}
