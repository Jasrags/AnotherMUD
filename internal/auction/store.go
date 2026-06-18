package auction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/persistence"
	"gopkg.in/yaml.v3"
)

// FileName is the persisted listing store, written at the save-dir root next
// to accounts/, players/, and trade-audit.yaml.
const FileName = "auctions.yaml"

// CurrentListingVersion is the on-disk schema version of a single listing
// record. CurrentFileVersion is the version of the container. Bump the
// matching constant and add a migration when either shape changes; never
// edit a shipped record in place (§4 — records outlive schema changes).
const (
	CurrentListingVersion = 1
	CurrentFileVersion    = 1
)

// Sentinel errors surfaced to the Manager / verb layer.
var (
	// ErrNotFound — no listing with that id.
	ErrNotFound = errors.New("auction: listing not found")
	// ErrNotActive — an operation that needs an active listing hit a
	// sold/expired/cancelled one (e.g. a buyout racing another buyer).
	ErrNotActive = errors.New("auction: listing is no longer active")
	// ErrTemplateGone — a listing's item template no longer exists (a
	// content edit removed it); the escrowed value needs operator handling.
	ErrTemplateGone = errors.New("auction: item template no longer exists")
	// ErrCannotRefund — a sale cannot be auto-reversed: it is not sold, the
	// buyer already collected the item, or the seller already collected the
	// proceeds (no safe clawback). The operator handles it manually via the
	// audit log.
	ErrCannotRefund = errors.New("auction: sale cannot be refunded")
)

// pendingEntry is one player's uncollected coin proceeds, in the persisted
// container's list form (a map does not round-trip deterministically).
type pendingEntry struct {
	Player string `yaml:"player"`
	Amount int    `yaml:"amount"`
}

// auctionFile is the on-disk container: a version header, the monotonic id
// counter, the listing records, and the pending-coin ledger.
type auctionFile struct {
	Version  int            `yaml:"version"`
	Seq      uint64         `yaml:"seq"`
	Listings []Listing      `yaml:"listings"`
	Pending  []pendingEntry `yaml:"pending,omitempty"`
}

// Store is the persisted, in-memory-authoritative listing store. A single
// mutex guards both maps; every mutation persists the whole file atomically
// under the lock (the store is small — global market, bounded listings — so
// a full rewrite per mutation mirrors the gameclock/channel stores rather
// than the audit log's load-append-write).
type Store struct {
	mu       sync.Mutex
	path     string
	clk      clock.Clock
	listings map[string]*Listing // id -> listing
	pending  map[string]int      // playerID -> uncollected coin
	seq      uint64              // monotonic id counter, persisted (survives prune + reboot)
}

// NewStore builds a store rooted at saveDir (artifact saveDir/auctions.yaml),
// stamping times from clk. Call Load before use to populate from disk.
func NewStore(saveDir string, clk clock.Clock) *Store {
	return &Store{
		path:     filepath.Join(saveDir, FileName),
		clk:      clk,
		listings: map[string]*Listing{},
		pending:  map[string]int{},
	}
}

// Load reads the store from disk, migrating each record forward, and
// populates the in-memory maps. A missing file is an empty store, not an
// error. A corrupt file is a hard error — the store holds real player value
// and must not silently start empty over a parse failure.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("auction load: read: %w", err)
	}
	var f auctionFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("auction load: parse: %w", err)
	}
	f = migrateFile(f)

	s.seq = f.Seq
	s.listings = make(map[string]*Listing, len(f.Listings))
	for i := range f.Listings {
		l := f.Listings[i]
		lp := migrateListing(&l)
		s.listings[lp.ID] = lp
	}
	s.pending = make(map[string]int, len(f.Pending))
	for _, p := range f.Pending {
		if p.Amount > 0 {
			s.pending[p.Player] += p.Amount
		}
	}
	return nil
}

// persistLocked writes the whole store atomically. Caller holds s.mu.
func (s *Store) persistLocked() error {
	f := auctionFile{Version: CurrentFileVersion, Seq: s.seq}
	f.Listings = make([]Listing, 0, len(s.listings))
	for _, l := range s.listings {
		f.Listings = append(f.Listings, *l)
	}
	// Deterministic order so the file diffs cleanly and tests are stable.
	sort.Slice(f.Listings, func(i, j int) bool { return f.Listings[i].ID < f.Listings[j].ID })

	f.Pending = make([]pendingEntry, 0, len(s.pending))
	for player, amount := range s.pending {
		if amount > 0 {
			f.Pending = append(f.Pending, pendingEntry{Player: player, Amount: amount})
		}
	}
	sort.Slice(f.Pending, func(i, j int) bool { return f.Pending[i].Player < f.Pending[j].Player })

	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("auction persist: marshal: %w", err)
	}
	if err := persistence.AtomicWrite(s.path, data); err != nil {
		return fmt.Errorf("auction persist: write: %w", err)
	}
	return nil
}

// NextID allocates a globally-unique listing id, incrementing and persisting
// the monotonic counter so an id is never reused — even after the listing it
// named is pruned, and even across a reboot (the counter is on disk). Audit
// records reference these ids, so reuse would corrupt the trail.
func (s *Store) NextID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	if err := s.persistLocked(); err != nil {
		s.seq-- // not allocated — roll the counter back.
		return "", err
	}
	return fmt.Sprintf("au-%d", s.seq), nil
}

// Add registers a new active listing and persists. The caller fills every
// field except Version, which Add stamps to the current schema version.
func (s *Store) Add(l *Listing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l.Version = CurrentListingVersion
	cp := *l
	s.listings[l.ID] = &cp
	return s.persistLocked()
}

// Get returns a COPY of the listing with id, or (zero, false). A copy keeps
// callers from mutating store state outside the lock.
func (s *Store) Get(id string) (Listing, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.listings[id]
	if !ok {
		return Listing{}, false
	}
	return *l, true
}

// ActiveCountBySeller counts a seller's active listings (for the per-player
// cap, §3).
func (s *Store) ActiveCountBySeller(playerID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, l := range s.listings {
		if l.Status == StatusActive && l.Seller == playerID {
			n++
		}
	}
	return n
}

// ActiveListings returns copies of every active listing, id-sorted for
// stable browse ordering (the verb layer re-sorts by the requested key).
func (s *Store) ActiveListings() []Listing {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Listing, 0, len(s.listings))
	for _, l := range s.listings {
		if l.Status == StatusActive {
			out = append(out, *l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LapsedActive returns copies of every active listing whose expiry is at or
// before now — the set the expire path processes, on the tick and on boot
// (one path serves both, so a deadline that passed during downtime is not
// skipped, §4/§8).
func (s *Store) LapsedActive(now time.Time) []Listing {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Listing, 0)
	for _, l := range s.listings {
		if l.Status == StatusActive && !l.ExpiresAt.After(now) {
			out = append(out, *l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// HeldForPickup returns copies of every listing still holding an uncollected
// item for playerID (expired/cancelled returns, won items). Drives collect.
func (s *Store) HeldForPickup(playerID string) []Listing {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Listing, 0)
	for _, l := range s.listings {
		if l.Collector == playerID && l.HeldForPickup() {
			out = append(out, *l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// MarkSold transitions an active listing to sold: the item is earmarked for
// the buyer and the seller's proceeds are credited to the pending ledger.
// Idempotent against a race: returns ErrNotActive if the listing already
// left the active set (another buyer won a tick earlier). The actual coin
// move runs through escrow in the Manager; this records the outcome.
func (s *Store) MarkSold(id, buyerID, buyerName string, proceeds int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.listings[id]
	if !ok {
		return ErrNotFound
	}
	if l.Status != StatusActive {
		return ErrNotActive
	}
	l.Status = StatusSold
	l.Buyer = buyerID
	l.BuyerName = buyerName
	l.Collector = buyerID
	if proceeds > 0 {
		s.pending[l.Seller] += proceeds
	}
	return s.persistLocked()
}

// RefundSale reverses a sold-but-uncollected listing for admin moderation
// (§11): it claws the seller's proceeds back out of the pending ledger,
// credits the buyer's refund to the buyer's pending ledger (the buyer may be
// offline), and re-earmarks the item for the seller to collect. It is
// all-or-nothing and refuses (ErrCannotRefund) unless the sale can be cleanly
// reversed — still sold, item not yet collected, and the seller's proceeds
// still in the ledger (not yet collected) — so no balance ever goes negative
// and no item is duplicated. Returns the updated listing.
func (s *Store) RefundSale(id string, refundToBuyer, proceedsBack int) (Listing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.listings[id]
	if !ok {
		return Listing{}, ErrNotFound
	}
	if l.Status != StatusSold || l.ItemCollected {
		return Listing{}, ErrCannotRefund
	}
	if s.pending[l.Seller] < proceedsBack {
		return Listing{}, ErrCannotRefund // seller already collected — no safe clawback.
	}
	s.pending[l.Seller] -= proceedsBack
	if s.pending[l.Seller] == 0 {
		delete(s.pending, l.Seller)
	}
	if refundToBuyer > 0 {
		s.pending[l.Buyer] += refundToBuyer
	}
	l.Status = StatusCancelled
	l.Collector = l.Seller // item goes back to the seller's pickup.
	if err := s.persistLocked(); err != nil {
		return Listing{}, err
	}
	return *l, nil
}

// MarkExpired transitions an active listing to expired and earmarks the item
// for the seller. Returns ErrNotActive if it already left the active set, so
// the shared expire path (tick + boot) is exactly-once (§8).
func (s *Store) MarkExpired(id string) error {
	return s.markReturned(id, StatusExpired)
}

// MarkCancelled transitions an active listing to cancelled and earmarks the
// item for the seller (§3). Returns ErrNotActive if not active.
func (s *Store) MarkCancelled(id string) error {
	return s.markReturned(id, StatusCancelled)
}

// markReturned is the shared expire/cancel transition: active -> status,
// item earmarked for the seller. Caller-distinct status.
func (s *Store) markReturned(id string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.listings[id]
	if !ok {
		return ErrNotFound
	}
	if l.Status != StatusActive {
		return ErrNotActive
	}
	l.Status = status
	l.Collector = l.Seller
	return s.persistLocked()
}

// MarkItemCollected records that the held item has been claimed and prunes
// the now-finished listing from the store (its proceeds, if any, live in the
// separate pending ledger). Returns ErrNotFound if absent.
func (s *Store) MarkItemCollected(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.listings[id]
	if !ok {
		return ErrNotFound
	}
	l.ItemCollected = true
	if l.Status != StatusActive {
		delete(s.listings, id) // terminal + collected — done.
	}
	// An active listing is never collected (the verb only collects
	// HeldForPickup, i.e. non-active, listings), so the not-deleted branch is
	// unreachable in practice; if a future path reaches it the flag is set
	// defensively without pruning a still-live listing.
	return s.persistLocked()
}

// PendingCoin reports a player's uncollected proceeds.
func (s *Store) PendingCoin(playerID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending[playerID]
}

// ClaimCoin zeroes and returns a player's pending proceeds in one step — the
// single-winner primitive so a double collect cannot pay twice. Persists.
func (s *Store) ClaimCoin(playerID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	amt := s.pending[playerID]
	if amt == 0 {
		return 0, nil
	}
	delete(s.pending, playerID)
	if err := s.persistLocked(); err != nil {
		s.pending[playerID] = amt // restore on write failure — coin not claimed.
		return 0, err
	}
	return amt, nil
}

// migrateFile brings the container forward. v1 is current; future versions
// add cases. Append-only — never rewrite an existing case.
func migrateFile(f auctionFile) auctionFile {
	switch f.Version {
	case 0, CurrentFileVersion:
		f.Version = CurrentFileVersion
	}
	return f
}

// migrateListing brings one record forward. v1 is current. Append-only.
func migrateListing(l *Listing) *Listing {
	switch l.Version {
	case 0, CurrentListingVersion:
		l.Version = CurrentListingVersion
	}
	return l
}
