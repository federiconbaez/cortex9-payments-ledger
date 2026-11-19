package domain

import "strconv"

// Result es lo que se guarda bajo la clave de idempotencia de un comando aceptado.
type Result struct {
	EntryIDs []EntryID
	SagaID   SagaID
	Status   string
}

// OpenAccount crea un wallet de pasivo.
type OpenAccount struct {
	ID          AccountID
	Shard       Shard
	Currency    Currency
	CreditLimit int64
	IdemKey     IdemKey
}

func (c OpenAccount) fingerprint() string {
	return writeFingerprint("open", string(c.ID), string(c.Shard), string(c.Currency), strconv.FormatInt(c.CreditLimit, 10))
}

// Fund acredita un wallet contra la cuenta de clearing del shard.
type Fund struct {
	Account AccountID
	Amount  Money
	IdemKey IdemKey
}

func (c Fund) fingerprint() string {
	return writeFingerprint("fund", string(c.Account), string(c.Amount.Currency), strconv.FormatInt(c.Amount.Amount, 10))
}

// Transfer mueve fondos entre dos wallets. Si estan en distinta shard, abre una saga.
type Transfer struct {
	From    AccountID
	To      AccountID
	Amount  Money
	IdemKey IdemKey
}

func (c Transfer) fingerprint() string {
	return writeFingerprint("transfer", string(c.From), string(c.To), string(c.Amount.Currency), strconv.FormatInt(c.Amount.Amount, 10))
}

// Reverse publica un asiento compensatorio de uno existente.
type Reverse struct {
	EntryID EntryID
	IdemKey IdemKey
}

func (c Reverse) fingerprint() string {
	return writeFingerprint("reverse", string(c.EntryID))
}

// Freeze bloquea un wallet para que no reciba captures ni fondos.
type Freeze struct {
	Account AccountID
	IdemKey IdemKey
}

func (c Freeze) fingerprint() string {
	return writeFingerprint("freeze", string(c.Account))
}
