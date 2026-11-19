package filestore

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"

	"github.com/federiconbaez/cortex9-payments-ledger/internal/domain"
)

const recordMagic uint32 = 0x43395053 // "C9PS"

func init() {
	gob.Register(domain.Hash{})
}

// Store es un mayor append-only con fsync y recuperacion de cola rota.
type Store struct {
	mu   sync.Mutex
	path string
	f    *os.File
	led  *domain.Ledger
	gen  uint64
	sync bool
}

// Open reabre un archivo, descarta un record final incompleto o con CRC
// invalido, y reconstruye el estado desde el ultimo snapshot bueno.
func Open(path string) (*Store, error) {
	return OpenWith(path, true)
}

// OpenWith controla si cada commit espera fsync. El harness de fallos
// puede desactivarlo para recorrer miles de semillas; el formato del
// record y la recuperacion de cola rota no cambian.
func OpenWith(path string, fsyncOn bool) (*Store, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	led, n, err := replay(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Truncate(n); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.Seek(n, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Store{path: path, f: f, led: led, gen: 0, sync: fsyncOn}, nil
}

// Close cierra el archivo.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

// Update aplica fn sobre un clone y persiste un snapshot con fsync.
func (s *Store) Update(ctx context.Context, fn func(*domain.Ledger) error) error {
	s.mu.Lock()
	gen := s.gen
	start := s.led.Clone()
	s.mu.Unlock()

	if err := fn(start); err != nil {
		return err
	}
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return domain.ErrConflict
	}
	if err := s.appendSnapshot(start); err != nil {
		return err
	}
	s.led = start
	s.gen++
	return nil
}

// View ejecuta fn sobre un clone.
func (s *Store) View(_ context.Context, fn func(*domain.Ledger) error) error {
	s.mu.Lock()
	snap := s.led.Clone()
	s.mu.Unlock()
	return fn(snap)
}

func (s *Store) appendSnapshot(led *domain.Ledger) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(snapshotFrom(led)); err != nil {
		return err
	}
	payload := buf.Bytes()
	crc := crc32.ChecksumIEEE(payload)
	var hdr [12]byte
	binary.LittleEndian.PutUint32(hdr[0:4], recordMagic)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	binary.LittleEndian.PutUint32(hdr[8:12], crc)
	if _, err := s.f.Write(hdr[:]); err != nil {
		return err
	}
	if _, err := s.f.Write(payload); err != nil {
		return err
	}
	if !s.sync {
		return nil
	}
	return s.f.Sync()
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

func snapshotFrom(l *domain.Ledger) snapshot {
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
	return s
}

func (s snapshot) into() *domain.Ledger {
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
	return l
}

func replay(f *os.File) (*domain.Ledger, int64, error) {
	led := domain.NewLedger()
	var good int64
	for {
		off, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, 0, err
		}
		var hdr [12]byte
		n, err := io.ReadFull(f, hdr[:])
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			if n == 0 && err == io.EOF {
				return led, good, nil
			}
			return led, good, nil // cola rota: nos quedamos en el ultimo bueno
		}
		if err != nil {
			return nil, 0, err
		}
		magic := binary.LittleEndian.Uint32(hdr[0:4])
		size := binary.LittleEndian.Uint32(hdr[4:8])
		crc := binary.LittleEndian.Uint32(hdr[8:12])
		if magic != recordMagic || size > 64<<20 {
			return led, good, nil
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(f, payload); err != nil {
			return led, good, nil
		}
		if crc32.ChecksumIEEE(payload) != crc {
			return led, good, nil
		}
		var snap snapshot
		if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&snap); err != nil {
			return nil, 0, fmt.Errorf("snapshot corrupto en offset %d: %w", off, err)
		}
		led = snap.into()
		good, err = f.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, 0, err
		}
	}
}
