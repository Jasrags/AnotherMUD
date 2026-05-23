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
	ErrEmailTaken   = errors.New("account: email already in use")
	ErrAuthFailed   = errors.New("account: authentication failed")
	ErrNotFound     = errors.New("account: not found")
	ErrInvalidEmail = errors.New("account: invalid email")
)

// Account is the on-disk record for a single account.
type Account struct {
	ID           string    `yaml:"id"`
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

	mu    sync.Mutex
	index map[string]string // normalized email -> account id
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
	return nil
}

func (s *Service) saveIndexLocked() error {
	data, err := yaml.Marshal(indexFile{Entries: s.index})
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
	norm := NormalizeEmail(email)
	if !ValidEmail(norm) {
		return nil, fmt.Errorf("account.Create: %w", ErrInvalidEmail)
	}

	s.mu.Lock()
	if _, taken := s.index[norm]; taken {
		s.mu.Unlock()
		return nil, fmt.Errorf("account.Create %q: %w", norm, ErrEmailTaken)
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

	acc := &Account{
		ID:           id,
		Email:        norm,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-check after hashing: a concurrent Create with the same email
	// may have won the race while we were hashing.
	if _, taken := s.index[norm]; taken {
		return nil, fmt.Errorf("account.Create %q: %w", norm, ErrEmailTaken)
	}
	if err := s.writeAccountLocked(acc); err != nil {
		return nil, fmt.Errorf("account.Create: write: %w", err)
	}
	s.index[norm] = id
	if err := s.saveIndexLocked(); err != nil {
		// Best-effort: the account file exists but the index won't see
		// it on restart. Surface the error so the caller can decide.
		return nil, fmt.Errorf("account.Create: save index: %w", err)
	}
	return acc, nil
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
