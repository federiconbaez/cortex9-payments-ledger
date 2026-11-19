package domain

import "errors"

var (
	ErrInvalidMoney        = errors.New("monto invalido")
	ErrInvalidCurrency     = errors.New("moneda invalida")
	ErrCurrencyMismatch    = errors.New("monedas distintas, no hay FX implicito")
	ErrInsufficientFunds   = errors.New("fondos insuficientes")
	ErrAccountExists       = errors.New("la cuenta ya existe")
	ErrAccountNotFound     = errors.New("cuenta inexistente")
	ErrIdempotencyConflict = errors.New("conflicto de idempotencia")
	ErrConflict            = errors.New("conflicto de version")
	ErrAlreadyReversed     = errors.New("el asiento ya fue revertido")
	ErrEntryNotFound       = errors.New("asiento inexistente")
	ErrInvalidCommand      = errors.New("comando invalido")
	ErrSagaState           = errors.New("estado de saga invalido")
	ErrUnknownMessage      = errors.New("mensaje desconocido")
	ErrTamperDetected      = errors.New("cadena de hash invalida")
	ErrUnbalancedEntry     = errors.New("asiento desbalanceado")
	ErrUnknownAccountType  = errors.New("tipo de cuenta desconocido")
	ErrCreditLimitNegative = errors.New("limite de descubierto negativo")
	ErrAccountFrozen       = errors.New("cuenta congelada")
)
