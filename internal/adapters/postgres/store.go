package postgres

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"

	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store guarda un snapshot transaccional del mayor con CAS de generacion.
// No es un esquema normalizado de asientos: es un adaptador durable para
// probar el mismo dominio contra Postgres. El modelo relacional fino queda
// fuera de este corte.
type Store struct {
	pool *pgxpool.Pool
}

const schema = `
CREATE TABLE IF NOT EXISTS ledger_snap (
    id   smallint PRIMARY KEY CHECK (id = 1),
    gen  bigint  NOT NULL,
    blob bytea   NOT NULL
);
`

// Open conecta, asegura el esquema y la fila inicial.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, err
	}
	_, err = pool.Exec(ctx, `INSERT INTO ledger_snap (id, gen, blob) VALUES (1, 0, $1)
		ON CONFLICT (id) DO NOTHING`, encode(domain.NewLedger()))
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// Close cierra el pool.
func (s *Store) Close() { s.pool.Close() }

// Update aplica fn sobre un clone y commitea con CAS de gen.
func (s *Store) Update(ctx context.Context, fn func(*domain.Ledger) error) error {
	var gen int64
	var blob []byte
	err := s.pool.QueryRow(ctx, `SELECT gen, blob FROM ledger_snap WHERE id = 1`).Scan(&gen, &blob)
	if err != nil {
		return err
	}
	led, err := decode(blob)
	if err != nil {
		return err
	}
	if err := fn(led); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE ledger_snap SET gen = $1, blob = $2 WHERE id = 1 AND gen = $3`,
		gen+1, encode(led), gen)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}

// View lee un snapshot consistente.
func (s *Store) View(ctx context.Context, fn func(*domain.Ledger) error) error {
	var blob []byte
	if err := s.pool.QueryRow(ctx, `SELECT blob FROM ledger_snap WHERE id = 1`).Scan(&blob); err != nil {
		return err
	}
	led, err := decode(blob)
	if err != nil {
		return err
	}
	return fn(led)
}

type snapshot struct {
	Accounts []domain.Account
	Logs     map[domain.Shard][]domain.Entry
	Heads    map[domain.Shard]domain.Hash
	Sagas    []domain.Saga
	Idem     []domain.IdemRecord
	Outbox   []domain.Message
	Inbox    []domain.MessageID
	Reversal map[domain.EntryID]domain.EntryID
}

func encode(l *domain.Ledger) []byte {
	s := snapshot{
		Logs:     make(map[domain.Shard][]domain.Entry, len(l.Log)),
		Heads:    make(map[domain.Shard]domain.Hash, len(l.Heads)),
		Reversal: make(map[domain.EntryID]domain.EntryID, len(l.Reversal)),
	}
	for _, a := range l.Accounts {
		s.Accounts = append(s.Accounts, a)
	}
	for k, v := range l.Log {
		s.Logs[k] = append([]domain.Entry(nil), v...)
	}
	for k, v := range l.Heads {
		s.Heads[k] = v
	}
	for _, v := range l.Sagas {
		s.Sagas = append(s.Sagas, v)
	}
	for _, v := range l.Idem {
		s.Idem = append(s.Idem, v)
	}
	s.Outbox = append([]domain.Message(nil), l.Outbox...)
	for id := range l.Inbox {
		s.Inbox = append(s.Inbox, id)
	}
	for k, v := range l.Reversal {
		s.Reversal[k] = v
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&s); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func decode(b []byte) (*domain.Ledger, error) {
	if len(b) == 0 {
		return domain.NewLedger(), nil
	}
	var s snapshot
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&s); err != nil {
		return nil, fmt.Errorf("snapshot postgres: %w", err)
	}
	l := domain.NewLedger()
	for _, a := range s.Accounts {
		l.Accounts[a.ID] = a
	}
	for k, v := range s.Logs {
		l.Log[k] = append([]domain.Entry(nil), v...)
	}
	for k, v := range s.Heads {
		l.Heads[k] = v
	}
	for _, v := range s.Sagas {
		l.Sagas[v.ID] = v
	}
	for _, v := range s.Idem {
		l.Idem[v.Key] = v
	}
	l.Outbox = append([]domain.Message(nil), s.Outbox...)
	for _, id := range s.Inbox {
		l.Inbox[id] = struct{}{}
	}
	for k, v := range s.Reversal {
		l.Reversal[k] = v
	}
	return l, nil
}
