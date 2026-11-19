package domain

import (
	"fmt"
	"time"
)

// IDGen genera identificadores de asiento y de saga.
type IDGen interface {
	NewID(prefix string) string
}

// Ledger es la maquina de estados pura del mayor. No hace I/O.
type Ledger struct {
	Accounts map[AccountID]Account
	Log      map[Shard][]Entry
	Heads    map[Shard]Hash
	Sagas    map[SagaID]Saga
	Idem     map[IdemKey]IdemRecord
	Outbox   []Message
	Inbox    map[MessageID]struct{}
	Reversal map[EntryID]EntryID
}

// IdemRecord asocia una clave al fingerprint y al resultado aceptado.
type IdemRecord struct {
	Key         IdemKey
	Fingerprint string
	Result      Result
}

// NewLedger construye un mayor vacio.
func NewLedger() *Ledger {
	return &Ledger{
		Accounts: make(map[AccountID]Account),
		Log:      make(map[Shard][]Entry),
		Heads:    make(map[Shard]Hash),
		Sagas:    make(map[SagaID]Saga),
		Idem:     make(map[IdemKey]IdemRecord),
		Inbox:    make(map[MessageID]struct{}),
		Reversal: make(map[EntryID]EntryID),
	}
}

// Clone es una copia profunda suficiente para aislar un intento de commit.
func (l *Ledger) Clone() *Ledger {
	if l == nil {
		return NewLedger()
	}
	out := &Ledger{
		Accounts: make(map[AccountID]Account, len(l.Accounts)),
		Log:      make(map[Shard][]Entry, len(l.Log)),
		Heads:    make(map[Shard]Hash, len(l.Heads)),
		Sagas:    make(map[SagaID]Saga, len(l.Sagas)),
		Idem:     make(map[IdemKey]IdemRecord, len(l.Idem)),
		Outbox:   append([]Message(nil), l.Outbox...),
		Inbox:    make(map[MessageID]struct{}, len(l.Inbox)),
		Reversal: make(map[EntryID]EntryID, len(l.Reversal)),
	}
	for k, v := range l.Accounts {
		out.Accounts[k] = v
	}
	for k, v := range l.Log {
		out.Log[k] = append([]Entry(nil), v...)
	}
	for k, v := range l.Heads {
		out.Heads[k] = v
	}
	for k, v := range l.Sagas {
		out.Sagas[k] = v
	}
	for k, v := range l.Idem {
		out.Idem[k] = v
	}
	for k := range l.Inbox {
		out.Inbox[k] = struct{}{}
	}
	for k, v := range l.Reversal {
		out.Reversal[k] = v
	}
	return out
}

func (l *Ledger) remember(key IdemKey, fp string, res Result) {
	if key == "" {
		return
	}
	l.Idem[key] = IdemRecord{Key: key, Fingerprint: fp, Result: res}
}

func (l *Ledger) idem(key IdemKey, fp string) (Result, bool, error) {
	if key == "" {
		return Result{}, false, fmt.Errorf("%w: falta clave de idempotencia", ErrInvalidCommand)
	}
	rec, ok := l.Idem[key]
	if !ok {
		return Result{}, false, nil
	}
	if rec.Fingerprint != fp {
		return Result{}, false, ErrIdempotencyConflict
	}
	return rec.Result, true, nil
}

func (l *Ledger) get(id AccountID) (Account, error) {
	a, ok := l.Accounts[id]
	if !ok {
		return Account{}, fmt.Errorf("%w: %s", ErrAccountNotFound, id)
	}
	return a, nil
}

func (l *Ledger) put(a Account) { l.Accounts[a.ID] = a }

func (l *Ledger) appendEntry(e Entry, accs []Account) (Entry, error) {
	if !e.Balanced() {
		return Entry{}, ErrUnbalancedEntry
	}
	e.PrevHash = l.Heads[e.Shard]
	e.Hash = ComputeHash(e.PrevHash, e)
	l.Log[e.Shard] = append(l.Log[e.Shard], e)
	l.Heads[e.Shard] = e.Hash
	for _, a := range accs {
		l.put(a)
	}
	return e, nil
}

func (l *Ledger) post(now time.Time, ids IDGen, shard Shard, kind EntryKind, key IdemKey, saga SagaID, reverses EntryID, lines []Posting) (Entry, []Account, error) {
	if ids == nil {
		return Entry{}, nil, ErrInvalidCommand
	}
	touched := make([]Account, 0, len(lines))
	seen := map[AccountID]int{}
	for _, ln := range lines {
		idx, ok := seen[ln.Account]
		var a Account
		if ok {
			a = touched[idx]
		} else {
			var err error
			a, err = l.get(ln.Account)
			if err != nil {
				return Entry{}, nil, err
			}
		}
		if a.Currency != ln.Currency {
			return Entry{}, nil, fmt.Errorf("%w: cuenta %s", ErrCurrencyMismatch, a.ID)
		}
		if err := a.CanPost(ln.Side, ln.Amount); err != nil {
			return Entry{}, nil, err
		}
		a.apply(ln.Side, ln.Amount)
		if ok {
			touched[idx] = a
		} else {
			seen[ln.Account] = len(touched)
			touched = append(touched, a)
		}
	}
	e := Entry{
		ID:       EntryID(ids.NewID("e")),
		Shard:    shard,
		Ts:       now.UTC(),
		Kind:     kind,
		Lines:    append([]Posting(nil), lines...),
		IdemKey:  key,
		Reverses: reverses,
		SagaID:   saga,
	}
	e, err := l.appendEntry(e, touched)
	return e, touched, err
}

func clearingID(shard Shard, ccy Currency) AccountID {
	return AccountID("sys.clearing." + string(shard) + "." + string(ccy))
}

func transitID(id SagaID) AccountID  { return AccountID("sys.transit." + string(id)) }
func incomingID(id SagaID) AccountID { return AccountID("sys.incoming." + string(id)) }

func (l *Ledger) ensureSystem(id AccountID, shard Shard, ccy Currency, typ AccountType) Account {
	if a, ok := l.Accounts[id]; ok {
		return a
	}
	a := Account{ID: id, Shard: shard, Currency: ccy, Type: typ, System: true, Version: 0}
	l.put(a)
	return a
}

// ApplyOpen crea un wallet de pasivo.
func (l *Ledger) ApplyOpen(cmd OpenAccount, now time.Time, ids IDGen) (Result, error) {
	fp := cmd.fingerprint()
	if res, hit, err := l.idem(cmd.IdemKey, fp); err != nil || hit {
		return res, err
	}
	ccy, err := ParseCurrency(string(cmd.Currency))
	if err != nil {
		return Result{}, err
	}
	cmd.Currency = ccy
	a := Account{
		ID:          cmd.ID,
		Shard:       cmd.Shard,
		Currency:    cmd.Currency,
		Type:        Liability,
		CreditLimit: cmd.CreditLimit,
		Version:     0,
	}
	if err := a.validateNew(); err != nil {
		return Result{}, err
	}
	if _, exists := l.Accounts[a.ID]; exists {
		return Result{}, ErrAccountExists
	}
	a.Version = 1
	l.put(a)
	res := Result{Status: "opened"}
	l.remember(cmd.IdemKey, fp, res)
	return res, nil
}

// ApplyFreeze marca un wallet como congelado (no recibe abonos).
func (l *Ledger) ApplyFreeze(cmd Freeze, now time.Time, ids IDGen) (Result, error) {
	fp := cmd.fingerprint()
	if res, hit, err := l.idem(cmd.IdemKey, fp); err != nil || hit {
		return res, err
	}
	a, err := l.get(cmd.Account)
	if err != nil {
		return Result{}, err
	}
	if a.System {
		return Result{}, ErrInvalidCommand
	}
	a.Frozen = true
	a.Version++
	l.put(a)
	res := Result{Status: "frozen"}
	l.remember(cmd.IdemKey, fp, res)
	return res, nil
}

// ApplyFund acredita un wallet contra clearing.
func (l *Ledger) ApplyFund(cmd Fund, now time.Time, ids IDGen) (Result, error) {
	fp := cmd.fingerprint()
	if res, hit, err := l.idem(cmd.IdemKey, fp); err != nil || hit {
		return res, err
	}
	if err := cmd.Amount.Validate(); err != nil {
		return Result{}, err
	}
	wallet, err := l.get(cmd.Account)
	if err != nil {
		return Result{}, err
	}
	if err := cmd.Amount.sameCurrency(wallet.Currency); err != nil {
		return Result{}, err
	}
	if wallet.Frozen {
		return Result{}, ErrAccountFrozen
	}
	clr := l.ensureSystem(clearingID(wallet.Shard, wallet.Currency), wallet.Shard, wallet.Currency, Asset)
	_ = clr
	e, _, err := l.post(now, ids, wallet.Shard, KindFund, cmd.IdemKey, "", "", []Posting{
		{Account: clearingID(wallet.Shard, wallet.Currency), Side: Debit, Amount: cmd.Amount.Amount, Currency: wallet.Currency},
		{Account: wallet.ID, Side: Credit, Amount: cmd.Amount.Amount, Currency: wallet.Currency},
	})
	if err != nil {
		return Result{}, err
	}
	res := Result{EntryIDs: []EntryID{e.ID}, Status: "funded"}
	l.remember(cmd.IdemKey, fp, res)
	return res, nil
}

// ApplyTransfer ejecuta una transferencia intra-shard o abre una saga.
func (l *Ledger) ApplyTransfer(cmd Transfer, now time.Time, ids IDGen) (Result, error) {
	fp := cmd.fingerprint()
	if res, hit, err := l.idem(cmd.IdemKey, fp); err != nil || hit {
		return res, err
	}
	if err := cmd.Amount.Validate(); err != nil {
		return Result{}, err
	}
	if cmd.From == cmd.To {
		return Result{}, fmt.Errorf("%w: origen y destino iguales", ErrInvalidCommand)
	}
	from, err := l.get(cmd.From)
	if err != nil {
		return Result{}, err
	}
	to, err := l.get(cmd.To)
	if err != nil {
		return Result{}, err
	}
	if err := cmd.Amount.sameCurrency(from.Currency); err != nil {
		return Result{}, err
	}
	if err := cmd.Amount.sameCurrency(to.Currency); err != nil {
		return Result{}, err
	}
	if to.Frozen {
		return Result{}, ErrAccountFrozen
	}
	if from.Shard == to.Shard {
		e, _, err := l.post(now, ids, from.Shard, KindTransfer, cmd.IdemKey, "", "", []Posting{
			{Account: from.ID, Side: Debit, Amount: cmd.Amount.Amount, Currency: from.Currency},
			{Account: to.ID, Side: Credit, Amount: cmd.Amount.Amount, Currency: to.Currency},
		})
		if err != nil {
			return Result{}, err
		}
		res := Result{EntryIDs: []EntryID{e.ID}, Status: "settled"}
		l.remember(cmd.IdemKey, fp, res)
		return res, nil
	}
	return l.startSaga(cmd, from, to, now, ids, fp)
}

func (l *Ledger) startSaga(cmd Transfer, from, to Account, now time.Time, ids IDGen, fp string) (Result, error) {
	sid := SagaID(ids.NewID("s"))
	tr := transitID(sid)
	l.ensureSystem(tr, from.Shard, from.Currency, Liability)
	e, _, err := l.post(now, ids, from.Shard, KindReserve, cmd.IdemKey, sid, "", []Posting{
		{Account: from.ID, Side: Debit, Amount: cmd.Amount.Amount, Currency: from.Currency},
		{Account: tr, Side: Credit, Amount: cmd.Amount.Amount, Currency: from.Currency},
	})
	if err != nil {
		return Result{}, err
	}
	l.Sagas[sid] = Saga{
		ID:           sid,
		From:         from.ID,
		To:           to.ID,
		Amount:       cmd.Amount,
		State:        SagaReserved,
		ReserveEntry: e.ID,
	}
	l.Outbox = append(l.Outbox, Message{
		ID:     captureMsgID(sid),
		Kind:   MsgCapturePayee,
		SagaID: sid,
		From:   from.ID,
		To:     to.ID,
		Amount: cmd.Amount,
	})
	res := Result{EntryIDs: []EntryID{e.ID}, SagaID: sid, Status: string(SagaReserved)}
	l.remember(cmd.IdemKey, fp, res)
	return res, nil
}

// ApplyReverse publica un asiento compensatorio.
func (l *Ledger) ApplyReverse(cmd Reverse, now time.Time, ids IDGen) (Result, error) {
	fp := cmd.fingerprint()
	if res, hit, err := l.idem(cmd.IdemKey, fp); err != nil || hit {
		return res, err
	}
	if cmd.EntryID == "" {
		return Result{}, ErrInvalidCommand
	}
	if _, ok := l.Reversal[cmd.EntryID]; ok {
		return Result{}, ErrAlreadyReversed
	}
	orig, err := l.findEntry(cmd.EntryID)
	if err != nil {
		return Result{}, err
	}
	e, _, err := l.post(now, ids, orig.Shard, KindReversal, cmd.IdemKey, orig.SagaID, orig.ID, orig.reversedLines())
	if err != nil {
		return Result{}, err
	}
	l.Reversal[orig.ID] = e.ID
	res := Result{EntryIDs: []EntryID{e.ID}, SagaID: orig.SagaID, Status: "reversed"}
	l.remember(cmd.IdemKey, fp, res)
	return res, nil
}

func (l *Ledger) findEntry(id EntryID) (Entry, error) {
	for _, log := range l.Log {
		for _, e := range log {
			if e.ID == id {
				return e, nil
			}
		}
	}
	return Entry{}, ErrEntryNotFound
}

// Pending devuelve una copia de los mensajes de outbox aun no retirados.
func (l *Ledger) Pending() []Message {
	return append([]Message(nil), l.Outbox...)
}

// Deliver aplica un mensaje de outbox. Es idempotente por Message.ID (inbox).
func (l *Ledger) Deliver(msg Message, now time.Time, ids IDGen) error {
	if _, seen := l.Inbox[msg.ID]; seen {
		return nil
	}
	var err error
	switch msg.Kind {
	case MsgCapturePayee:
		err = l.deliverCapture(msg, now, ids)
	case MsgAckCaptured:
		err = l.deliverAck(msg)
	case MsgReleasePayer:
		err = l.deliverRelease(msg, now, ids)
	default:
		return ErrUnknownMessage
	}
	if err != nil {
		return err
	}
	l.Inbox[msg.ID] = struct{}{}
	l.dropOutbox(msg.ID)
	return nil
}

func (l *Ledger) dropOutbox(id MessageID) {
	out := l.Outbox[:0]
	for _, m := range l.Outbox {
		if m.ID != id {
			out = append(out, m)
		}
	}
	l.Outbox = out
}

func (l *Ledger) deliverCapture(msg Message, now time.Time, ids IDGen) error {
	saga, ok := l.Sagas[msg.SagaID]
	if ok && saga.State != SagaReserved {
		return fmt.Errorf("%w: capture sobre %s", ErrSagaState, saga.State)
	}
	to, err := l.get(msg.To)
	if err != nil || msg.Amount.sameCurrency(to.Currency) != nil || to.Frozen {
		l.Outbox = append(l.Outbox, Message{
			ID:     releaseMsgID(msg.SagaID),
			Kind:   MsgReleasePayer,
			SagaID: msg.SagaID,
			From:   msg.From,
			To:     msg.To,
			Amount: msg.Amount,
		})
		return nil
	}
	inc := incomingID(msg.SagaID)
	l.ensureSystem(inc, to.Shard, to.Currency, Asset)
	e, _, err := l.post(now, ids, to.Shard, KindCapture, "", msg.SagaID, "", []Posting{
		{Account: inc, Side: Debit, Amount: msg.Amount.Amount, Currency: to.Currency},
		{Account: to.ID, Side: Credit, Amount: msg.Amount.Amount, Currency: to.Currency},
	})
	if err != nil {
		return err
	}
	if ok {
		saga.State = SagaReserved
		saga.CaptureEntry = e.ID
		l.Sagas[msg.SagaID] = saga
	}
	l.Outbox = append(l.Outbox, Message{
		ID:     ackMsgID(msg.SagaID),
		Kind:   MsgAckCaptured,
		SagaID: msg.SagaID,
		From:   msg.From,
		To:     msg.To,
		Amount: msg.Amount,
	})
	return nil
}

func (l *Ledger) deliverAck(msg Message) error {
	saga, ok := l.Sagas[msg.SagaID]
	if !ok {
		return fmt.Errorf("%w: saga %s", ErrSagaState, msg.SagaID)
	}
	if saga.State == SagaSettled {
		return nil
	}
	if saga.State != SagaReserved {
		return fmt.Errorf("%w: ack sobre %s", ErrSagaState, saga.State)
	}
	saga.State = SagaSettled
	l.Sagas[msg.SagaID] = saga
	return nil
}

func (l *Ledger) deliverRelease(msg Message, now time.Time, ids IDGen) error {
	saga, ok := l.Sagas[msg.SagaID]
	if !ok {
		return fmt.Errorf("%w: saga %s", ErrSagaState, msg.SagaID)
	}
	if saga.State == SagaReleased {
		return nil
	}
	if saga.State != SagaReserved {
		return fmt.Errorf("%w: release sobre %s", ErrSagaState, saga.State)
	}
	tr := transitID(saga.ID)
	e, _, err := l.post(now, ids, l.Accounts[saga.From].Shard, KindRelease, "", saga.ID, "", []Posting{
		{Account: tr, Side: Debit, Amount: saga.Amount.Amount, Currency: saga.Amount.Currency},
		{Account: saga.From, Side: Credit, Amount: saga.Amount.Amount, Currency: saga.Amount.Currency},
	})
	if err != nil {
		return err
	}
	saga.State = SagaReleased
	saga.ReleaseEntry = e.ID
	l.Sagas[msg.SagaID] = saga
	return nil
}
