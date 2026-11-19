package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// Side es el lado de una linea.
type Side uint8

const (
	Debit Side = iota + 1
	Credit
)

func (s Side) String() string {
	switch s {
	case Debit:
		return "D"
	case Credit:
		return "C"
	default:
		return "?"
	}
}

// EntryKind clasifica el asiento.
type EntryKind string

const (
	KindOpen     EntryKind = "open"
	KindFund     EntryKind = "fund"
	KindTransfer EntryKind = "transfer"
	KindReserve  EntryKind = "reserve"
	KindCapture  EntryKind = "capture"
	KindRelease  EntryKind = "release"
	KindReversal EntryKind = "reversal"
)

// EntryID, Hash e IdemKey son identificadores y huellas del diario.
type EntryID string
type IdemKey string
type Hash [32]byte

func (h Hash) String() string { return hex.EncodeToString(h[:]) }

func ParseHash(s string) (Hash, error) {
	var h Hash
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		return h, fmt.Errorf("hash invalido")
	}
	copy(h[:], b)
	return h, nil
}

// Posting es una linea de asiento. Amount > 0.
type Posting struct {
	Account  AccountID
	Side     Side
	Amount   int64
	Currency Currency
}

// Entry es un asiento inmutable, hash-encadenado al previo del shard.
type Entry struct {
	ID       EntryID
	Shard    Shard
	Ts       time.Time
	Kind     EntryKind
	Lines    []Posting
	IdemKey  IdemKey
	Reverses EntryID
	SagaID   SagaID
	PrevHash Hash
	Hash     Hash
}

// Balanced comprueba debitos = creditos por moneda.
func (e Entry) Balanced() bool {
	type acc struct{ d, c int64 }
	by := map[Currency]acc{}
	for _, ln := range e.Lines {
		if ln.Amount <= 0 || ln.Account == "" || (ln.Side != Debit && ln.Side != Credit) {
			return false
		}
		a := by[ln.Currency]
		if ln.Side == Debit {
			a.d += ln.Amount
		} else {
			a.c += ln.Amount
		}
		by[ln.Currency] = a
	}
	if len(by) == 0 {
		return false
	}
	for _, a := range by {
		if a.d != a.c {
			return false
		}
	}
	return true
}

func (e Entry) reversedLines() []Posting {
	out := make([]Posting, len(e.Lines))
	for i, ln := range e.Lines {
		side := Credit
		if ln.Side == Credit {
			side = Debit
		}
		out[i] = Posting{Account: ln.Account, Side: side, Amount: ln.Amount, Currency: ln.Currency}
	}
	return out
}

// ComputeHash devuelve SHA-256(prev || canonico(asiento sin Hash)).
func ComputeHash(prev Hash, e Entry) Hash {
	h := sha256.New()
	h.Write([]byte("entry_v1"))
	writeString(h, string(e.ID))
	writeString(h, string(e.Shard))
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(e.Ts.UTC().UnixNano()))
	h.Write(ts[:])
	writeString(h, string(e.Kind))
	writeString(h, string(e.IdemKey))
	writeString(h, string(e.Reverses))
	writeString(h, string(e.SagaID))
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(e.Lines)))
	h.Write(n[:])
	for _, ln := range e.Lines {
		writeString(h, string(ln.Account))
		writeString(h, ln.Side.String())
		var amt [8]byte
		binary.BigEndian.PutUint64(amt[:], uint64(ln.Amount))
		h.Write(amt[:])
		writeString(h, string(ln.Currency))
	}
	h.Write(prev[:])
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

func writeString(h interface{ Write([]byte) (int, error) }, s string) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	_, _ = h.Write(n[:])
	_, _ = h.Write([]byte(s))
}

func writeFingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		writeString(h, p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
