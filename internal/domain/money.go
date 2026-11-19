package domain

import (
	"fmt"
	"unicode"
)

// Currency es un codigo ISO-like de exactamente 3 letras A-Z.
type Currency string

// Money es un monto en unidades menores enteras. Amount debe ser > 0 en
// postings y comandos; nunca se usa un flotante para representarlo.
type Money struct {
	Currency Currency
	Amount   int64
}

// ParseCurrency valida y normaliza a mayusculas.
func ParseCurrency(s string) (Currency, error) {
	if len(s) != 3 {
		return "", fmt.Errorf("%w: %q", ErrInvalidCurrency, s)
	}
	b := make([]byte, 3)
	for i := 0; i < 3; i++ {
		r := rune(s[i])
		if r > unicode.MaxASCII || !unicode.IsLetter(r) {
			return "", fmt.Errorf("%w: %q", ErrInvalidCurrency, s)
		}
		b[i] = byte(unicode.ToUpper(r))
	}
	return Currency(b), nil
}

// Validate comprueba que el monto sea usable en un posting o comando.
func (m Money) Validate() error {
	if _, err := ParseCurrency(string(m.Currency)); err != nil {
		return err
	}
	if m.Amount <= 0 {
		return fmt.Errorf("%w: %d", ErrInvalidMoney, m.Amount)
	}
	return nil
}

func (m Money) sameCurrency(c Currency) error {
	if m.Currency != c {
		return fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.Currency, c)
	}
	return nil
}
