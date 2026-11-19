package clock

import "time"

// System es el reloj de pared.
type System struct{}

// Now implementa el puerto de reloj.
func (System) Now() time.Time { return time.Now().UTC() }

// Fixed es un reloj inyectable, para tests y el harness.
type Fixed struct{ T time.Time }

// Now devuelve el instante configurado.
func (f *Fixed) Now() time.Time { return f.T }

// Advance mueve el reloj.
func (f *Fixed) Advance(d time.Duration) { f.T = f.T.Add(d) }
