package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sysclock "github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/clock"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/adapters/memory"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/app"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

func TestHTTPTransferAndProblemJSON(t *testing.T) {
	svc := app.New(memory.New(), &sysclock.Fixed{T: time.Date(2026, 11, 19, 11, 30, 0, 0, time.UTC)}, &domain.Seq{})
	h := New(svc).Handler()

	req := func(method, path, key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	w := req(http.MethodPost, "/accounts", "o1", `{"id":"alice","shard":"s1","currency":"ARS"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("open alice %d %s", w.Code, w.Body.String())
	}
	if req(http.MethodPost, "/accounts", "o2", `{"id":"bob","shard":"s1","currency":"ARS"}`).Code != http.StatusCreated {
		t.Fatal("open bob")
	}
	if req(http.MethodPost, "/accounts/alice/fund", "f1", `{"amount":1000,"currency":"ARS"}`).Code != http.StatusCreated {
		t.Fatal("fund")
	}
	w = req(http.MethodPost, "/transfers", "t1", `{"from":"alice","to":"bob","amount":200,"currency":"ARS"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("transfer %d %s", w.Code, w.Body.String())
	}
	w = req(http.MethodGet, "/accounts/alice", "", "")
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	var acc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &acc); err != nil {
		t.Fatal(err)
	}
	if acc["balance"].(float64) != 800 {
		t.Fatalf("balance=%v", acc["balance"])
	}
	w = req(http.MethodPost, "/transfers", "t1", `{"from":"alice","to":"bob","amount":1,"currency":"ARS"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("conflicto esperado %d %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type %s", ct)
	}
	w = req(http.MethodPost, "/transfers", "t2", `{"from":"alice","to":"bob","amount":9000,"currency":"ARS"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("fondos %d %s", w.Code, w.Body.String())
	}
}
