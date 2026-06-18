package escrow

// The trade audit log (docs/specs/trade-escrow.md §5). Every committed
// transaction appends one record sufficient to reconstruct and reverse it:
// the parties, every item instance and coin amount moved, the source
// consumer, and a timestamp. The log is append-only and tamper-evident —
// records are added, never edited or deleted in place — so a later reading
// reflects the true history (support rollback + dupe/RMT tracing).
//
// It is long-lived world data, written with the engine's atomic file
// discipline (persistence.AtomicWrite). It is versioned like player saves
// so old records load via migration rather than being dropped — their
// item references represent real player value.

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/persistence"
	"gopkg.in/yaml.v3"
)

// AuditFileName is the global trade-audit artifact, written at the save-dir
// root next to the accounts/ and players/ subtrees.
const AuditFileName = "trade-audit.yaml"

// CurrentAuditVersion is the on-disk schema version of an audit record.
// Bump it and add a migration when the record shape changes; never edit a
// shipped record in place.
const CurrentAuditVersion = 1

// AuditLeg is one moved unit of value in a committed transaction.
type AuditLeg struct {
	Party  string `yaml:"party"`
	Dest   string `yaml:"dest"`
	Kind   string `yaml:"kind"`             // "item" | "coin"
	Item   string `yaml:"item,omitempty"`   // item leg
	Amount int    `yaml:"amount,omitempty"` // coin leg
}

// AuditRecord is one committed transaction. Time is stamped by the store at
// append time (engine packages read time through a Clock, not time.Now).
type AuditRecord struct {
	Version int        `yaml:"version"`
	TxnID   string     `yaml:"txn_id"`
	Source  string     `yaml:"source"`
	Time    time.Time  `yaml:"time"`
	Legs    []AuditLeg `yaml:"legs"`
}

// auditFile is the on-disk container — a version header plus the append-only
// record list. The per-file version lets the loader migrate the whole log
// forward if the container shape ever changes.
type auditFile struct {
	Version int           `yaml:"version"`
	Records []AuditRecord `yaml:"records"`
}

// AuditStore appends and reads the global trade-audit log. It serializes
// appends under a mutex and stamps each record's time from its clock.
type AuditStore struct {
	mu   sync.Mutex
	path string
	clk  clock.Clock
}

// NewAuditStore builds a store rooted at saveDir (artifact
// saveDir/trade-audit.yaml), stamping record times from clk.
func NewAuditStore(saveDir string, clk clock.Clock) *AuditStore {
	return &AuditStore{
		path: filepath.Join(saveDir, AuditFileName),
		clk:  clk,
	}
}

// Append stamps rec with the current time and the current version, then
// appends it to the log via load → append → atomic rewrite. The whole
// operation is serialized so concurrent commits cannot interleave a
// read-modify-write and drop a record.
func (s *AuditStore) Append(rec AuditRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec.Version = CurrentAuditVersion
	if s.clk != nil {
		rec.Time = s.clk.Now()
	}

	f, err := s.loadLocked()
	if err != nil {
		return err
	}
	f.Version = CurrentAuditVersion
	f.Records = append(f.Records, rec)

	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("trade-audit append: marshal: %w", err)
	}
	if err := persistence.AtomicWrite(s.path, data); err != nil {
		return fmt.Errorf("trade-audit append: write: %w", err)
	}
	return nil
}

// Load returns all audit records in append order. A missing file is an
// empty log, not an error.
func (s *AuditStore) Load() ([]AuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	return f.Records, nil
}

// loadLocked reads the current file. Caller holds s.mu. A missing file
// yields an empty container; a corrupt file is a hard error (the audit log
// must not silently lose history).
func (s *AuditStore) loadLocked() (auditFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return auditFile{Version: CurrentAuditVersion}, nil
		}
		return auditFile{}, fmt.Errorf("trade-audit load: read: %w", err)
	}
	var f auditFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return auditFile{}, fmt.Errorf("trade-audit load: parse: %w", err)
	}
	return f, nil
}

// auditRecord builds the (un-timestamped) record for a committed
// transaction from its bus legs. Append stamps the time + version.
func (t *Transaction) auditRecord(busLegs []eventbus.TradeLeg) AuditRecord {
	legs := make([]AuditLeg, 0, len(busLegs))
	for _, bl := range busLegs {
		legs = append(legs, AuditLeg{
			Party:  bl.PartyID,
			Dest:   bl.DestPartyID,
			Kind:   bl.Kind,
			Item:   bl.ItemID,
			Amount: bl.Amount,
		})
	}
	return AuditRecord{TxnID: t.id, Source: t.source, Legs: legs}
}
