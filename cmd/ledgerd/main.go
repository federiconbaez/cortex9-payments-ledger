package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	sysclock "github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/clock"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/filestore"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/httpapi"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/memory"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/app"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

func main() {
	addr := flag.String("addr", ":8080", "direccion HTTP")
	file := flag.String("file", "", "ruta del almacen append-only (vacio = memoria)")
	flag.Parse()

	var store app.Store
	if *file == "" {
		store = memory.New()
		log.Printf("almacen: memoria")
	} else {
		fs, err := filestore.Open(*file)
		if err != nil {
			log.Fatal(err)
		}
		defer fs.Close()
		store = fs
		log.Printf("almacen: archivo %s", *file)
	}

	svc := app.New(store, sysclock.System{}, domain.RandomIDs{})
	h := httpapi.New(svc).Handler()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := svc.Flush(ctx, 32); err != nil && ctx.Err() == nil {
					log.Printf("flush: %v", err)
				}
			}
		}
	}()

	srv := &http.Server{Addr: *addr, Handler: h}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	log.Printf("escuchando %s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
