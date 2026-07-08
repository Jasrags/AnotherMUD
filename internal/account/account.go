// Package account owns the account identity model and its on-disk store.
//
// Spec: docs/specs/persistence.md §5. Accounts carry an immutable id, a
// normalized email, a bcrypt password hash, and a list of character names
// the account owns. The store lays files out under
// <root>/accounts/<id>/account.yaml plus an index.yaml that maps
// lowercased email to account id.
//
// M3 scope: Create / AuthenticateByEmail / AuthenticateByID / LoadByID /
// AddCharacter / RemoveCharacter. Password change, soft delete, online-
// entity tracking, and verification-state workflows are out of scope and
// land when the features that need them appear.
package account

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	"github.com/Jasrags/AnotherMUD/internal/persistence"
)

// MinBcryptCostForTests keeps unit tests under 100ms; production code
// uses bcrypt.DefaultCost (10).
const MinBcryptCostForTests = bcrypt.MinCost

// Sentinel errors. Authenticate returns ErrAuthFailed for both wrong
// password and unknown email so callers cannot distinguish (spec §5.2,
// "without revealing which condition failed").
var (
	ErrEmailTaken      = errors.New("account: email already in use")
	ErrUsernameTaken   = errors.New("account: username already in use")
	ErrAuthFailed      = errors.New("account: authentication failed")
	ErrNotFound        = errors.New("account: not found")
	ErrInvalidEmail    = errors.New("account: invalid email")
	ErrInvalidUsername = errors.New("account: invalid username")
)

// Account is the on-disk record for a single account.
type Account struct {
	ID string `yaml:"id"`
	// Username is the account's login identifier (character-select §2.1):
	// unique case-insensitively, distinct from any character name. Added
	// alongside the username index; existing accounts are backfilled from
	// the email local part at load. Stored in NORMALIZED (lowercased) form —
	// the login key and the display form are the same; unlike character
	// names, the typed casing is not preserved.
	Username string `yaml:"username,omitempty"`
	// Email is now an optional recovery/contact field, no longer the login
	// key (character-select §2.1 — on a deprecation path).
	Email        string    `yaml:"email"`
	PasswordHash string    `yaml:"password_hash"`
	Characters   []string  `yaml:"characters,omitempty"`
	CreatedAt    time.Time `yaml:"created_at"`
	Verified     bool      `yaml:"verified,omitempty"`
	VerifiedAt   time.Time `yaml:"verified_at,omitempty"`
}

// Service wraps a file-backed account store with the domain operations
// from persistence spec §5. It is safe for concurrent use.
type Service struct {
	root       string
	bcryptCost int

	// dummyHash is a valid bcrypt hash of a random sentinel. Compared
	// against during AuthenticateByEmail's "no such email" branch so
	// the failure path takes roughly the same wall-clock time as a
	// real password check — closes the trivial timing oracle for
	// account enumeration. Generated once at NewService.
	dummyHash []byte

	mu        sync.Mutex
	index     map[string]string // normalized email -> account id
	userIndex map[string]string // normalized username -> account id (character-select §2.1)
}

// Option configures NewService.
type Option func(*Service)

// WithBcryptCost overrides the bcrypt cost used for new password hashes.
// Defaults to bcrypt.DefaultCost.
func WithBcryptCost(cost int) Option {
	return func(s *Service) { s.bcryptCost = cost }
}

// NewService opens (or initializes) an account store rooted at
// <root>/accounts. The email index is loaded into memory; the directory
// is created if missing.
func NewService(root string, opts ...Option) (*Service, error) {
	s := &Service{
		root:       filepath.Join(root, "accounts"),
		bcryptCost: bcrypt.DefaultCost,
		index:      make(map[string]string),
		userIndex:  make(map[string]string),
	}
	for _, opt := range opts {
		opt(s)
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return nil, fmt.Errorf("account: mkdir root: %w", err)
	}
	if err := s.loadIndex(); err != nil {
		return nil, fmt.Errorf("account: load index: %w", err)
	}
	if err := s.backfillUsernames(); err != nil {
		return nil, fmt.Errorf("account: backfill usernames: %w", err)
	}
	// Pre-compute a valid bcrypt hash for the timing-equalization path
	// in AuthenticateByEmail. The input value is irrelevant — only the
	// hash structure and cost matter.
	var sentinel [16]byte
	if _, err := rand.Read(sentinel[:]); err != nil {
		return nil, fmt.Errorf("account: dummy hash entropy: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword(sentinel[:], s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("account: dummy hash: %w", err)
	}
	s.dummyHash = hash
	return s, nil
}

// EmailExists reports whether an account is registered for the
// normalized form of email. Used by the login flow to choose between
// "existing account" and "new account" branches without leaking the
// distinction through password-failure messages.
func (s *Service) EmailExists(email string) bool {
	norm := NormalizeEmail(email)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.index[norm]
	return ok
}

func (s *Service) indexPath() string {
	return filepath.Join(s.root, "index.yaml")
}

func (s *Service) accountPath(id string) (string, error) {
	dir, err := persistence.SafeJoin(s.root, id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "account.yaml"), nil
}

type indexFile struct {
	Entries map[string]string `yaml:"entries"`
	// Usernames is the username→id index (character-select §2.1). Absent in
	// pre-username index files; rebuilt by the load-time backfill.
	Usernames map[string]string `yaml:"usernames,omitempty"`
}

func (s *Service) loadIndex() error {
	data, err := os.ReadFile(s.indexPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var f indexFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("decode index: %w", err)
	}
	if f.Entries != nil {
		s.index = f.Entries
	}
	if f.Usernames != nil {
		s.userIndex = f.Usernames
	}
	return nil
}

func (s *Service) saveIndexLocked() error {
	data, err := yaml.Marshal(indexFile{Entries: s.index, Usernames: s.userIndex})
	if err != nil {
		return fmt.Errorf("encode index: %w", err)
	}
	return persistence.AtomicWrite(s.indexPath(), data)
}

// NormalizeEmail lowercases and trims an email. Exposed so login (which
// also displays the email back to the user) can canonicalize once.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidEmail is a minimal sanity check used by the login flow; full RFC
// 5322 parsing is overkill for a MUD account and is policy that can
// tighten later (login spec §7 configuration surface).
func ValidEmail(email string) bool {
	if email == "" {
		return false
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	if strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	return true
}

// NormalizeUsername lowercases and trims an account username for indexing
// and uniqueness (character-select §2.1). Display preserves what the user
// typed; the index keys on the normalized form.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// ValidUsername reports whether a (normalized) username is acceptable:
// 3–32 ASCII letters/digits/underscore, not empty. Policy that can tighten
// later (character-select configuration surface).
func ValidUsername(username string) bool {
	if len(username) < 3 || len(username) > 32 {
		return false
	}
	for _, r := range username {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

// UsernameExists reports whether an account is registered for the
// normalized form of username (character-select §2.2 new-vs-existing branch).
func (s *Service) UsernameExists(username string) bool {
	norm := NormalizeUsername(username)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.userIndex[norm]
	return ok
}

// deriveUsernameLocked produces a unique normalized username for an account
// that has none — used by the load-time backfill and by Create (the
// email-based legacy path). It takes the email local part (before '@'),
// sanitizes it to the ValidUsername charset, pads if too short, and appends
// a numeric suffix until it is unique in userIndex. Caller holds s.mu.
func (s *Service) deriveUsernameLocked(email string) string {
	local := email
	if at := strings.IndexByte(email, '@'); at > 0 {
		local = email[:at]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(local) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	base := b.String()
	for len(base) < 3 {
		base += "x"
	}
	if len(base) > 32 {
		base = base[:32]
	}
	candidate := base
	for i := 2; ; i++ {
		if _, taken := s.userIndex[candidate]; !taken {
			return candidate
		}
		suffix := fmt.Sprintf("%d", i)
		trimTo := min(32-len(suffix), len(base))
		candidate = base[:trimTo] + suffix
	}
}

// backfillUsernames assigns a username to every indexed account that lacks
// one (character-select §2.1 migration) and (re)builds userIndex. Runs once
// at NewService after loadIndex; deterministic, no operator input. An
// account already carrying a username is indexed as-is.
func (s *Service) backfillUsernames() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dirty := false
	// First pass: index accounts that already carry a username so the
	// derive step de-duplicates against them.
	for _, id := range s.index {
		acc, err := s.loadByIDLocked(id)
		if err != nil {
			continue // a dangling index entry; leave it for the email path to surface
		}
		if u := NormalizeUsername(acc.Username); u != "" {
			s.userIndex[u] = id
		}
	}
	// Second pass: derive + persist a username for accounts without one.
	for _, id := range s.index {
		acc, err := s.loadByIDLocked(id)
		if err != nil || NormalizeUsername(acc.Username) != "" {
			continue
		}
		u := s.deriveUsernameLocked(acc.Email)
		acc.Username = u
		if err := s.writeAccountLocked(acc); err != nil {
			return fmt.Errorf("backfill write %q: %w", id, err)
		}
		s.userIndex[u] = id
		dirty = true
	}
	if dirty {
		return s.saveIndexLocked()
	}
	return nil
}

// Create registers a new account. Email is normalized; password is
// hashed with bcrypt. Returns ErrEmailTaken if the normalized email is
// already in the index.
//
// bcrypt hashing runs outside s.mu so a burst of new-account creates
// doesn't serialize every other account operation behind the
// key-stretching CPU work. The taken-email check is racy across
// concurrent Creates with the same email, but a re-check after hashing
// closes the window and we accept the wasted hash on losing races as
// the cost of a non-blocking design.
func (s *Service) Create(ctx context.Context, email, password string) (*Account, error) {
	// Legacy/back-compat entry: derive the account username from the email
	// (the username-first flow uses CreateWithUsername with an explicit one).
	return s.create(ctx, "", email, password)
}

// CreateWithUsername registers an account with an explicit, player-chosen
// username (character-select §2.2). Email is optional (may be ""), reflecting
// its demotion to a recovery field. Returns ErrUsernameTaken / ErrEmailTaken
// on a collision, ErrInvalidUsername / ErrInvalidEmail on a bad value.
func (s *Service) CreateWithUsername(ctx context.Context, username, email, password string) (*Account, error) {
	return s.create(ctx, username, email, password)
}

// create is the shared registration path. username == "" means "derive from
// email" (the legacy Create). email == "" is allowed only with an explicit
// username (email is optional, character-select §2.1).
func (s *Service) create(ctx context.Context, username, email, password string) (*Account, error) {
	normEmail := NormalizeEmail(email)
	if normEmail != "" && !ValidEmail(normEmail) {
		return nil, fmt.Errorf("account.Create: %w", ErrInvalidEmail)
	}
	if username == "" && normEmail == "" {
		return nil, fmt.Errorf("account.Create: %w: need a username or an email", ErrInvalidUsername)
	}
	normUser := NormalizeUsername(username)
	if username != "" && !ValidUsername(normUser) {
		return nil, fmt.Errorf("account.Create: %w", ErrInvalidUsername)
	}

	// Pre-check collisions before the expensive hash (re-checked under lock
	// after hashing to close the race window).
	s.mu.Lock()
	if normEmail != "" {
		if _, taken := s.index[normEmail]; taken {
			s.mu.Unlock()
			return nil, fmt.Errorf("account.Create %q: %w", normEmail, ErrEmailTaken)
		}
	}
	if normUser != "" {
		if _, taken := s.userIndex[normUser]; taken {
			s.mu.Unlock()
			return nil, fmt.Errorf("account.Create %q: %w", normUser, ErrUsernameTaken)
		}
	}
	s.mu.Unlock()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("account.Create: hash: %w", err)
	}
	id, err := newID()
	if err != nil {
		return nil, fmt.Errorf("account.Create: id: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-check after hashing: a concurrent Create may have won the race.
	if normEmail != "" {
		if _, taken := s.index[normEmail]; taken {
			return nil, fmt.Errorf("account.Create %q: %w", normEmail, ErrEmailTaken)
		}
	}
	// Derive the username now (under lock) when none was supplied, so it is
	// unique against the live index.
	if normUser == "" {
		normUser = s.deriveUsernameLocked(normEmail)
	} else if _, taken := s.userIndex[normUser]; taken {
		return nil, fmt.Errorf("account.Create %q: %w", normUser, ErrUsernameTaken)
	}

	acc := &Account{
		ID:           id,
		Username:     normUser,
		Email:        normEmail,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.commitAccountLocked(acc, normEmail, normUser); err != nil {
		return nil, err
	}
	return acc, nil
}

// commitAccountLocked writes the account file, updates the in-memory email +
// username indexes, and persists them. The caller holds s.mu. On an index-save
// failure the in-memory index entries are ROLLED BACK so a retry (or a
// concurrent create) is not wrongly refused with ErrUsernameTaken/ErrEmailTaken
// for a registration that did not durably complete.
func (s *Service) commitAccountLocked(acc *Account, normEmail, normUser string) error {
	if err := s.writeAccountLocked(acc); err != nil {
		return fmt.Errorf("account.Create: write: %w", err)
	}
	if normEmail != "" {
		s.index[normEmail] = acc.ID
	}
	s.userIndex[normUser] = acc.ID
	if err := s.saveIndexLocked(); err != nil {
		delete(s.userIndex, normUser)
		if normEmail != "" {
			delete(s.index, normEmail)
		}
		// The account file is left on disk (orphaned until a future re-create
		// or cleanup); the in-memory index is consistent again so a retry works.
		return fmt.Errorf("account.Create: save index: %w", err)
	}
	return nil
}

// AuthenticateByEmail returns the account on success, ErrAuthFailed on
// any mismatch — including missing account — per spec §5.2.
func (s *Service) AuthenticateByEmail(ctx context.Context, email, password string) (*Account, error) {
	norm := NormalizeEmail(email)
	s.mu.Lock()
	id, ok := s.index[norm]
	s.mu.Unlock()
	if !ok {
		// Compare against a real bcrypt hash so the unknown-email path
		// takes roughly the same wall-clock time as a real check.
		// CompareHashAndPassword on a malformed hash short-circuits
		// before doing key-stretching work, which would leak the
		// distinction; using a valid hash from NewService avoids that.
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		return nil, ErrAuthFailed
	}
	return s.AuthenticateByID(ctx, id, password)
}

// AuthenticateByUsername returns the account on success, ErrAuthFailed on
// any mismatch — including unknown username — per the same no-distinction
// rule as AuthenticateByEmail (character-select §2.2). This is the
// account-first login front door.
func (s *Service) AuthenticateByUsername(ctx context.Context, username, password string) (*Account, error) {
	norm := NormalizeUsername(username)
	s.mu.Lock()
	id, ok := s.userIndex[norm]
	s.mu.Unlock()
	if !ok {
		// Timing-equalize the unknown-username path (see AuthenticateByEmail).
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		return nil, ErrAuthFailed
	}
	return s.AuthenticateByID(ctx, id, password)
}

// AuthenticateByID is the same as AuthenticateByEmail but starts from a
// known id. Used when the login flow already loaded a character record
// that carries the owning account id.
func (s *Service) AuthenticateByID(ctx context.Context, id, password string) (*Account, error) {
	acc, err := s.LoadByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrAuthFailed
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(password)); err != nil {
		return nil, ErrAuthFailed
	}
	return acc, nil
}

// LoadByID reads an account file by id.
func (s *Service) LoadByID(ctx context.Context, id string) (*Account, error) {
	path, err := s.accountPath(id)
	if err != nil {
		return nil, fmt.Errorf("account.LoadByID: %w", err)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("account.LoadByID %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("account.LoadByID %q: %w", id, err)
	}
	var acc Account
	if err := yaml.Unmarshal(data, &acc); err != nil {
		return nil, fmt.Errorf("account.LoadByID %q: decode: %w", id, err)
	}
	return &acc, nil
}

// AddCharacter appends name to the account's character list, idempotent
// on case-insensitive comparison (spec §5.2 AC).
func (s *Service) AddCharacter(ctx context.Context, id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, err := s.loadByIDLocked(id)
	if err != nil {
		return err
	}
	for _, existing := range acc.Characters {
		if strings.EqualFold(existing, name) {
			return nil
		}
	}
	acc.Characters = append(acc.Characters, name)
	return s.writeAccountLocked(acc)
}

// RemoveCharacter drops name from the account's character list, matching
// case-insensitively. Idempotent: a name not on the list is a no-op (no
// error), so a double-delete or a stale roster entry can't fail the caller.
// The character save itself is removed separately by the player store
// (character-select §8 roster operations).
func (s *Service) RemoveCharacter(ctx context.Context, id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, err := s.loadByIDLocked(id)
	if err != nil {
		return err
	}
	kept := acc.Characters[:0:0] // new backing array — never mutate in place
	removed := false
	for _, existing := range acc.Characters {
		if strings.EqualFold(existing, name) {
			removed = true
			continue
		}
		kept = append(kept, existing)
	}
	if !removed {
		return nil
	}
	acc.Characters = kept
	return s.writeAccountLocked(acc)
}

// ChangePassword replaces the account's password hash after verifying the
// current password (character-select §8 roster operations). The caller is
// expected to have collected both; verifying here keeps the credential
// check and the rehash atomic under the account lock. Returns ErrAuthFailed
// when current does not match, so the caller can reprompt without leaking
// which field was wrong.
func (s *Service) ChangePassword(ctx context.Context, id, current, next string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, err := s.loadByIDLocked(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrAuthFailed
		}
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(current)); err != nil {
		return ErrAuthFailed
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("account.ChangePassword: hash: %w", err)
	}
	acc.PasswordHash = string(hash)
	return s.writeAccountLocked(acc)
}

func (s *Service) loadByIDLocked(id string) (*Account, error) {
	// Same as LoadByID but assumes the caller holds s.mu so callers
	// composing read+modify+write don't race with each other.
	path, err := s.accountPath(id)
	if err != nil {
		return nil, fmt.Errorf("account.load %q: %w", id, err)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("account.load %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	var acc Account
	if err := yaml.Unmarshal(data, &acc); err != nil {
		return nil, fmt.Errorf("account.load %q: decode: %w", id, err)
	}
	return &acc, nil
}

func (s *Service) writeAccountLocked(acc *Account) error {
	path, err := s.accountPath(acc.ID)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(acc)
	if err != nil {
		return fmt.Errorf("encode account: %w", err)
	}
	return persistence.AtomicWrite(path, data)
}

// newID generates a 128-bit random hex id. Not a formal UUID — we don't
// need v4 layout, just collision-resistant opacity that's safe in
// filesystem paths.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
