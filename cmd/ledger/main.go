package main

import (
	"context"
	"fmt"
	"os"

	"github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/filestore"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "verify" {
		fmt.Fprintf(os.Stderr, "uso: ledger verify <archivo>\n")
		os.Exit(2)
	}
	st, err := filestore.Open(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer st.Close()
	var vs []domain.Violation
	err = st.View(context.Background(), func(l *domain.Ledger) error {
		vs = l.ClosedVerify()
		fmt.Printf("cuentas=%d asientos=%d sagas=%d outbox=%d\n",
			len(l.Accounts), countEntries(l), len(l.Sagas), len(l.Outbox))
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(vs) == 0 {
		fmt.Println("ok: invariantes en verde")
		return
	}
	for _, v := range vs {
		fmt.Fprintln(os.Stderr, v)
	}
	os.Exit(1)
}

func countEntries(l *domain.Ledger) int {
	n := 0
	for _, log := range l.Log {
		n += len(log)
	}
	return n
}
