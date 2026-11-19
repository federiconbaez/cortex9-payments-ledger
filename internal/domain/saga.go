package domain

// SagaID identifica una transferencia cross-shard.
type SagaID string

// MessageID es estable por (saga, tipo) para que un reenvio sea el mismo mensaje.
type MessageID string

// SagaState es el ciclo de vida de una saga.
type SagaState string

const (
	SagaReserved SagaState = "reserved"
	SagaSettled  SagaState = "settled"
	SagaReleased SagaState = "released"
	SagaFailed   SagaState = "failed"
)

// Terminal informa si la saga ya no emitira mas efectos.
func (s SagaState) Terminal() bool {
	return s == SagaSettled || s == SagaReleased || s == SagaFailed
}

// Saga registra una transferencia entre shards.
type Saga struct {
	ID           SagaID
	From         AccountID
	To           AccountID
	Amount       Money
	State        SagaState
	ReserveEntry EntryID
	CaptureEntry EntryID
	ReleaseEntry EntryID
}

// MessageKind es el tipo de mensaje de outbox.
type MessageKind string

const (
	MsgCapturePayee MessageKind = "capture_payee"
	MsgAckCaptured  MessageKind = "ack_captured"
	MsgReleasePayer MessageKind = "release_payer"
)

// Message es un comando asincronico entre shards.
type Message struct {
	ID     MessageID
	Kind   MessageKind
	SagaID SagaID
	From   AccountID
	To     AccountID
	Amount Money
}

func captureMsgID(id SagaID) MessageID { return MessageID("capture:" + string(id)) }
func ackMsgID(id SagaID) MessageID     { return MessageID("ack:" + string(id)) }
func releaseMsgID(id SagaID) MessageID { return MessageID("release:" + string(id)) }
