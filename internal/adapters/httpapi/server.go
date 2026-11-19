package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/federiconbaez/cortex9-payments-ledger/internal/app"
	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

// Server es la API HTTP del ledger. Los errores de negocio salen como RFC 9457.
type Server struct {
	svc *app.Service
}

// New envuelve el servicio.
func New(svc *app.Service) *Server { return &Server{svc: svc} }

// Handler registra las rutas.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /accounts", s.postAccount)
	mux.HandleFunc("POST /accounts/{id}/fund", s.postFund)
	mux.HandleFunc("POST /accounts/{id}/freeze", s.postFreeze)
	mux.HandleFunc("GET /accounts/{id}", s.getAccount)
	mux.HandleFunc("GET /accounts/{id}/statement", s.getStatement)
	mux.HandleFunc("POST /transfers", s.postTransfer)
	mux.HandleFunc("POST /reversals", s.postReverse)
	mux.HandleFunc("GET /sagas/{id}", s.getSaga)
	mux.HandleFunc("POST /flush", s.postFlush)
	mux.HandleFunc("GET /verify", s.getVerify)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func (s *Server) writeProblem(w http.ResponseWriter, err error) {
	st, typ, title := mapError(err)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(st)
	_ = json.NewEncoder(w).Encode(problem{Type: typ, Title: title, Status: st, Detail: err.Error()})
}

func mapError(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return http.StatusConflict, "https://payments.cortex9.dev/problems/idempotency-conflict", "Conflicto de idempotencia"
	case errors.Is(err, domain.ErrInsufficientFunds):
		return http.StatusConflict, "https://payments.cortex9.dev/problems/insufficient-funds", "Fondos insuficientes"
	case errors.Is(err, domain.ErrAccountNotFound), errors.Is(err, domain.ErrEntryNotFound), errors.Is(err, domain.ErrSagaState):
		return http.StatusNotFound, "https://payments.cortex9.dev/problems/not-found", "No encontrado"
	case errors.Is(err, domain.ErrAccountExists):
		return http.StatusConflict, "https://payments.cortex9.dev/problems/account-exists", "La cuenta ya existe"
	case errors.Is(err, domain.ErrCurrencyMismatch):
		return http.StatusUnprocessableEntity, "https://payments.cortex9.dev/problems/currency-mismatch", "Monedas distintas"
	case errors.Is(err, domain.ErrAccountFrozen):
		return http.StatusConflict, "https://payments.cortex9.dev/problems/frozen", "Cuenta congelada"
	case errors.Is(err, domain.ErrAlreadyReversed):
		return http.StatusConflict, "https://payments.cortex9.dev/problems/already-reversed", "Asiento ya revertido"
	case errors.Is(err, domain.ErrInvalidMoney), errors.Is(err, domain.ErrInvalidCurrency), errors.Is(err, domain.ErrInvalidCommand), errors.Is(err, domain.ErrCreditLimitNegative):
		return http.StatusBadRequest, "https://payments.cortex9.dev/problems/invalid", "Pedido invalido"
	default:
		return http.StatusInternalServerError, "https://payments.cortex9.dev/problems/internal", "Error interno"
	}
}

func idemKey(r *http.Request) (domain.IdemKey, error) {
	k := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if k == "" {
		return "", domain.ErrInvalidCommand
	}
	return domain.IdemKey(k), nil
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// DecodeOpen decodifica el cuerpo de apertura. Esta exportado para fuzzing.
func DecodeOpen(body []byte) (domain.OpenAccount, error) {
	var in struct {
		ID          string `json:"id"`
		Shard       string `json:"shard"`
		Currency    string `json:"currency"`
		CreditLimit int64  `json:"credit_limit"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return domain.OpenAccount{}, err
	}
	ccy, err := domain.ParseCurrency(in.Currency)
	if err != nil {
		return domain.OpenAccount{}, err
	}
	if in.ID == "" || in.Shard == "" {
		return domain.OpenAccount{}, domain.ErrInvalidCommand
	}
	return domain.OpenAccount{ID: domain.AccountID(in.ID), Shard: domain.Shard(in.Shard), Currency: ccy, CreditLimit: in.CreditLimit}, nil
}

func (s *Server) postAccount(w http.ResponseWriter, r *http.Request) {
	key, err := idemKey(r)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	cmd, err := DecodeOpen(body)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	cmd.IdemKey = key
	res, err := s.svc.Open(r.Context(), cmd)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

type fundBody struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func (s *Server) postFund(w http.ResponseWriter, r *http.Request) {
	key, err := idemKey(r)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	var in fundBody
	if err := readJSON(r, &in); err != nil {
		s.writeProblem(w, err)
		return
	}
	ccy, err := domain.ParseCurrency(in.Currency)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	res, err := s.svc.Fund(r.Context(), domain.Fund{
		Account: domain.AccountID(r.PathValue("id")),
		Amount:  domain.Money{Currency: ccy, Amount: in.Amount},
		IdemKey: key,
	})
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) postFreeze(w http.ResponseWriter, r *http.Request) {
	key, err := idemKey(r)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	res, err := s.svc.Freeze(r.Context(), domain.Freeze{Account: domain.AccountID(r.PathValue("id")), IdemKey: key})
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := s.svc.Account(r.Context(), domain.AccountID(r.PathValue("id")))
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": acc.ID, "shard": acc.Shard, "currency": acc.Currency,
		"type": acc.Type.String(), "credit_limit": acc.CreditLimit,
		"system": acc.System, "frozen": acc.Frozen, "version": acc.Version,
		"balance": acc.Balance(),
	})
}

func (s *Server) getStatement(w http.ResponseWriter, r *http.Request) {
	lines, err := s.svc.Statement(r.Context(), domain.AccountID(r.PathValue("id")))
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

type transferBody struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// DecodeTransfer decodifica una transferencia. Exportado para fuzzing.
func DecodeTransfer(body []byte) (domain.Transfer, error) {
	var in transferBody
	if err := json.Unmarshal(body, &in); err != nil {
		return domain.Transfer{}, err
	}
	ccy, err := domain.ParseCurrency(in.Currency)
	if err != nil {
		return domain.Transfer{}, err
	}
	if in.From == "" || in.To == "" {
		return domain.Transfer{}, domain.ErrInvalidCommand
	}
	return domain.Transfer{From: domain.AccountID(in.From), To: domain.AccountID(in.To), Amount: domain.Money{Currency: ccy, Amount: in.Amount}}, nil
}

func (s *Server) postTransfer(w http.ResponseWriter, r *http.Request) {
	key, err := idemKey(r)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	cmd, err := DecodeTransfer(body)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	cmd.IdemKey = key
	res, err := s.svc.Transfer(r.Context(), cmd)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

type reverseBody struct {
	EntryID string `json:"entry_id"`
}

func (s *Server) postReverse(w http.ResponseWriter, r *http.Request) {
	key, err := idemKey(r)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	var in reverseBody
	if err := readJSON(r, &in); err != nil {
		s.writeProblem(w, err)
		return
	}
	res, err := s.svc.Reverse(r.Context(), domain.Reverse{EntryID: domain.EntryID(in.EntryID), IdemKey: key})
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) getSaga(w http.ResponseWriter, r *http.Request) {
	sg, err := s.svc.Saga(r.Context(), domain.SagaID(r.PathValue("id")))
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sg)
}

func (s *Server) postFlush(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Flush(r.Context(), 0); err != nil {
		s.writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "flushed"})
}

func (s *Server) getVerify(w http.ResponseWriter, r *http.Request) {
	closed := r.URL.Query().Get("closed") == "1"
	vs, err := s.svc.Verify(r.Context(), closed)
	if err != nil {
		s.writeProblem(w, err)
		return
	}
	st := http.StatusOK
	if len(vs) > 0 {
		st = http.StatusConflict
	}
	writeJSON(w, st, vs)
}
