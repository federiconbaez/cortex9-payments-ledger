package sim

import (
	"flag"
	"fmt"
	"os"
	"testing"
	"time"
)

var seedFlag = flag.Int64("seed", 0, "reproduce una semilla puntual del harness de fallos")

// SeedsCI es el numero de semillas que corre la CI. Se elige para terminar
// en un tiempo acotado; ver el README para la medicion real.
const SeedsCI = 2000

func TestFaultInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("harness de fallos omitido en -short")
	}
	root := t.TempDir()
	if *seedFlag != 0 {
		runOne(t, root, *seedFlag, 80)
		return
	}
	n := SeedsCI
	if os.Getenv("LEDGER_SIM_SEEDS") != "" {
		fmt.Sscanf(os.Getenv("LEDGER_SIM_SEEDS"), "%d", &n)
	}
	start := time.Now()
	for s := int64(1); s <= int64(n); s++ {
		s := s
		runOne(t, root, s, 80)
		if t.Failed() {
			return
		}
	}
	t.Logf("semillas=%d duracion=%s", n, time.Since(start).Round(time.Millisecond))
}

func runOne(t *testing.T, root string, seed int64, steps int) {
	t.Helper()
	dir, cleanup, err := RunDir(root, seed)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	_, err = Run(Config{Seed: seed, Steps: steps, Dir: dir})
	if err != nil {
		t.Fatalf("reproducir con: go test ./internal/sim -count=1 -args -seed=%d\n%v", seed, err)
	}
}

func TestFaultInjectionSingleSeedReproduces(t *testing.T) {
	root := t.TempDir()
	runOne(t, root, 42, 40)
}
