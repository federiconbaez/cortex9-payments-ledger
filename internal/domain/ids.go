package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
)

// Seq es un generador determinista, para tests y para el harness de fallos.
type Seq struct {
	n uint64
}

// NewID implementa IDGen.
func (s *Seq) NewID(prefix string) string {
	v := atomic.AddUint64(&s.n, 1)
	return fmt.Sprintf("%s%016x", prefix, v)
}

// RandomIDs usa la caja aleatoria del sistema. No es para el harness.
type RandomIDs struct{}

// NewID implementa IDGen.
func (RandomIDs) NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b[:])
}
