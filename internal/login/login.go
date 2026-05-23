// Package login implements the connection-to-character handoff
// described in docs/specs/login.md.
//
// M3 scope (thin slice):
//
//   - Name → (returning: Password) | (new: Email → Password)
//   - Echo suppression via raw telnet IAC WILL/WONT ECHO (no full IAC
//     parser yet)
//   - Per-phase failed-attempt cap
//   - Hands off a Loaded record to the session layer on success
//
// Deferred to M4:
//
//   - Session takeover, link-dead reconnect, per-account concurrency cap
//   - Name-gates (pluggable allow/reject policy)
//   - Per-phase idle timeouts (needs a Clock-aware Conn.Read deadline)
//   - Structured GMCP phase events
package login

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/account"
	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// Telnet IAC bytes for echo control. The server tells the client "I
// will echo" so the client stops local-echoing the password, then
// "I won't echo" once the password is captured. This is a partial
// implementation of RFC 857 sufficient for password masking; a real
// IAC parser arrives with the networking-protocols milestone.
var (
	iacWillEcho = []byte{0xFF, 0xFB, 0x01}
	iacWontEcho = []byte{0xFF, 0xFC, 0x01}
)

// Default policy values. The login spec calls these "configuration
// surface" (§7); we expose them on Config so cmd/anothermud can plumb
// env vars when the operator needs to tune them.
const (
	DefaultMaxPasswordAttempts = 3
	DefaultMaxEmailAttempts    = 3
	DefaultMinPasswordLength   = 6
	DefaultMinNameLength       = 2
	DefaultMaxNameLength       = 16
)

// Sentinel errors returned from Run.
var (
	ErrAborted      = errors.New("login: connection closed before login")
	ErrPasswordCap  = errors.New("login: too many password attempts")
	ErrEmailCap     = errors.New("login: too many email attempts")
	ErrNameRejected = errors.New("login: name policy rejected")
)

// Loaded is what Run hands back to the session layer on success: an
// authenticated account paired with either a freshly created player
// save (for new players) or a loaded one (for returning players).
type Loaded struct {
	Account *account.Account
	Player  *player.Save
	New     bool // true if this login flow created the character
}

// Config wires the login flow to its dependencies and policy knobs.
type Config struct {
	Accounts *account.Service
	Players  *player.Store

	// DefaultLocation is the starting room for newly created characters.
	DefaultLocation string

	// Policy knobs — zero values fall back to package defaults so
	// callers can leave them blank.
	MaxPasswordAttempts int
	MaxEmailAttempts    int
	MinPasswordLength   int
	MinNameLength       int
	MaxNameLength       int
}

func (c Config) maxPwAttempts() int {
	if c.MaxPasswordAttempts > 0 {
		return c.MaxPasswordAttempts
	}
	return DefaultMaxPasswordAttempts
}
func (c Config) maxEmailAttempts() int {
	if c.MaxEmailAttempts > 0 {
		return c.MaxEmailAttempts
	}
	return DefaultMaxEmailAttempts
}
func (c Config) minPwLen() int {
	if c.MinPasswordLength > 0 {
		return c.MinPasswordLength
	}
	return DefaultMinPasswordLength
}
func (c Config) minNameLen() int {
	if c.MinNameLength > 0 {
		return c.MinNameLength
	}
	return DefaultMinNameLength
}
func (c Config) maxNameLen() int {
	if c.MaxNameLength > 0 {
		return c.MaxNameLength
	}
	return DefaultMaxNameLength
}

// Run drives the login state machine over conn until either a Loaded
// record is produced or the connection is closed / the context is
// cancelled.
func Run(ctx context.Context, c conn.Connection, cfg Config) (*Loaded, error) {
	lio := &lineIO{c: c}
	if err := lio.writeln(ctx, "Welcome to AnotherMUD."); err != nil {
		return nil, err
	}

	for {
		name, err := promptName(ctx, lio, cfg)
		if err != nil {
			return nil, err
		}

		if cfg.Players.Exists(name) {
			res, err := returningPlayer(ctx, lio, cfg, name)
			if err != nil {
				if errors.Is(err, errBackToName) {
					continue
				}
				return nil, err
			}
			return res, nil
		}

		res, err := newPlayer(ctx, lio, cfg, name)
		if err != nil {
			if errors.Is(err, errBackToName) {
				continue
			}
			return nil, err
		}
		return res, nil
	}
}

// errBackToName is an internal signal that a sub-phase wants to bounce
// the user back to the Name prompt without aborting the connection.
var errBackToName = errors.New("login: back to name prompt")

func promptName(ctx context.Context, lio *lineIO, cfg Config) (string, error) {
	for {
		if err := lio.writeln(ctx, "By what name shall we know you?"); err != nil {
			return "", err
		}
		raw, err := lio.readln(ctx)
		if err != nil {
			return "", err
		}
		name := strings.TrimSpace(raw)
		if msg := validateName(name, cfg); msg != "" {
			if err := lio.writeln(ctx, msg); err != nil {
				return "", err
			}
			continue
		}
		// Return the typed-as-typed name. The store lowercases for
		// path computation (spec §3.2); display preserves what the
		// user typed so MUD-traditional PascalCase names render right.
		return name, nil
	}
}

func validateName(name string, cfg Config) string {
	if len(name) < cfg.minNameLen() {
		return fmt.Sprintf("Names must be at least %d characters.", cfg.minNameLen())
	}
	if len(name) > cfg.maxNameLen() {
		return fmt.Sprintf("Names must be at most %d characters.", cfg.maxNameLen())
	}
	for _, r := range name {
		// ASCII letters only for M3. Punctuation, digits, and non-ASCII
		// land when name-gates do (M4-ish).
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return "Names must use ASCII letters only."
		}
	}
	return ""
}

func returningPlayer(ctx context.Context, lio *lineIO, cfg Config, name string) (*Loaded, error) {
	save, err := cfg.Players.Load(ctx, name)
	if err != nil {
		// Player file disappeared between Exists and Load, or version
		// drift. Surface a generic message and bounce.
		logging.From(ctx).Warn("returning player load failed",
			slog.String("name", name), slog.Any("err", err))
		_ = lio.writeln(ctx, "Sorry, your character file could not be loaded right now.")
		return nil, errBackToName
	}

	for attempts := 0; attempts < cfg.maxPwAttempts(); attempts++ {
		pw, err := promptPassword(ctx, lio, "Password: ")
		if err != nil {
			return nil, err
		}
		acc, err := cfg.Accounts.AuthenticateByID(ctx, save.AccountID, pw)
		if err != nil {
			if errors.Is(err, account.ErrAuthFailed) {
				if err := lio.writeln(ctx, "Incorrect password."); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("returning auth: %w", err)
		}
		return &Loaded{Account: acc, Player: save, New: false}, nil
	}
	_ = lio.writeln(ctx, "Too many failed attempts. Goodbye.")
	return nil, ErrPasswordCap
}

func newPlayer(ctx context.Context, lio *lineIO, cfg Config, name string) (*Loaded, error) {
	if err := lio.writeln(ctx, fmt.Sprintf("No character named %q exists. Creating a new one.", name)); err != nil {
		return nil, err
	}

	email, err := promptEmail(ctx, lio, cfg)
	if err != nil {
		return nil, err
	}

	// Branch on EmailExists rather than try-and-create. This avoids
	// the "created with typoed password" trap and lets us validate +
	// confirm before any irreversible work. The check leaks no more
	// than the eventual "incorrect password" message already does, and
	// matches login spec §5.1's explicit existing-vs-new lookup.
	if cfg.Accounts.EmailExists(email) {
		return newCharacterOnExistingAccount(ctx, lio, cfg, email, name)
	}
	return newCharacterOnNewAccount(ctx, lio, cfg, email, name)
}

// newCharacterOnExistingAccount asks for the email's password and, on
// success, attaches the chosen character name to the existing account.
// One failed attempt bounces back to the name prompt (login spec §5.2).
func newCharacterOnExistingAccount(ctx context.Context, lio *lineIO, cfg Config, email, name string) (*Loaded, error) {
	pw, err := promptPassword(ctx, lio, fmt.Sprintf("Password for %s: ", email))
	if err != nil {
		return nil, err
	}
	acc, err := cfg.Accounts.AuthenticateByEmail(ctx, email, pw)
	if err != nil {
		if errors.Is(err, account.ErrAuthFailed) {
			_ = lio.writeln(ctx, "Incorrect password for that email.")
			return nil, errBackToName
		}
		return nil, fmt.Errorf("auth by email: %w", err)
	}
	return commitNewCharacter(ctx, lio, cfg, acc, name)
}

// newCharacterOnNewAccount runs the password policy + confirmation
// dance, then creates the account and binds the character. No account
// is created until both the policy check and the confirmation succeed,
// so a mistyped or short password leaves no trace on disk.
func newCharacterOnNewAccount(ctx context.Context, lio *lineIO, cfg Config, email, name string) (*Loaded, error) {
	pw, err := promptPassword(ctx, lio, fmt.Sprintf("Choose a password for %s: ", email))
	if err != nil {
		return nil, err
	}
	if msg := validateNewPassword(pw, cfg); msg != "" {
		_ = lio.writeln(ctx, msg)
		return nil, errBackToName
	}
	confirm, err := promptPassword(ctx, lio, "Confirm password: ")
	if err != nil {
		return nil, err
	}
	if confirm != pw {
		_ = lio.writeln(ctx, "Passwords did not match. Returning to name prompt.")
		return nil, errBackToName
	}

	acc, err := cfg.Accounts.Create(ctx, email, pw)
	if err != nil {
		// Treat a lost race against a concurrent create the same way
		// we'd treat a returning visitor at the name prompt — bounce.
		if errors.Is(err, account.ErrEmailTaken) {
			_ = lio.writeln(ctx, "That email is already registered.")
			return nil, errBackToName
		}
		return nil, fmt.Errorf("create account: %w", err)
	}
	return commitNewCharacter(ctx, lio, cfg, acc, name)
}

func validateNewPassword(pw string, cfg Config) string {
	if len(pw) < cfg.minPwLen() {
		return fmt.Sprintf("Passwords must be at least %d characters.", cfg.minPwLen())
	}
	return ""
}

func commitNewCharacter(ctx context.Context, lio *lineIO, cfg Config, acc *account.Account, name string) (*Loaded, error) {
	id, err := newPlayerID()
	if err != nil {
		return nil, fmt.Errorf("new player id: %w", err)
	}
	save := &player.Save{
		Version:   player.CurrentVersion,
		ID:        id,
		AccountID: acc.ID,
		Name:      name,
		Location:  cfg.DefaultLocation,
	}
	if err := cfg.Players.Save(ctx, save); err != nil {
		return nil, fmt.Errorf("save new player: %w", err)
	}
	if err := cfg.Accounts.AddCharacter(ctx, acc.ID, name); err != nil {
		return nil, fmt.Errorf("link character to account: %w", err)
	}
	logging.From(ctx).Info("character created",
		slog.String("name", name),
		slog.String("account_id", acc.ID))
	if err := lio.writeln(ctx, fmt.Sprintf("Welcome, %s.", name)); err != nil {
		return nil, err
	}
	return &Loaded{Account: acc, Player: save, New: true}, nil
}

func promptEmail(ctx context.Context, lio *lineIO, cfg Config) (string, error) {
	for attempts := 0; attempts < cfg.maxEmailAttempts(); attempts++ {
		if err := lio.writeln(ctx, "Email address: "); err != nil {
			return "", err
		}
		raw, err := lio.readln(ctx)
		if err != nil {
			return "", err
		}
		email := account.NormalizeEmail(raw)
		if !account.ValidEmail(email) {
			if err := lio.writeln(ctx, "That doesn't look like an email address."); err != nil {
				return "", err
			}
			continue
		}
		return email, nil
	}
	_ = lio.writeln(ctx, "Too many invalid attempts. Goodbye.")
	return "", ErrEmailCap
}

func promptPassword(ctx context.Context, lio *lineIO, prompt string) (string, error) {
	if err := lio.writeRaw(ctx, iacWillEcho); err != nil {
		return "", err
	}
	if err := lio.writeRaw(ctx, []byte(prompt)); err != nil {
		return "", err
	}
	pw, readErr := lio.readln(ctx)
	// Restore echo before doing anything else so the next prompt is
	// visible — even if Read errored.
	if err := lio.writeRaw(ctx, iacWontEcho); err != nil {
		if readErr == nil {
			return "", err
		}
	}
	// A friendly newline so the next message doesn't run into the
	// echoed nothing.
	_ = lio.writeln(ctx, "")
	if readErr != nil {
		return "", readErr
	}
	return strings.TrimSpace(pw), nil
}

// lineIO bundles ctx-aware line read + write helpers around conn.Connection.
// Centralized so the EOF/closed translation lives in one place.
type lineIO struct {
	c conn.Connection
}

func (l *lineIO) readln(ctx context.Context) (string, error) {
	line, err := l.c.Read(ctx)
	if err == nil {
		return line, nil
	}
	if errors.Is(err, io.EOF) || errors.Is(err, conn.ErrClosed) {
		return "", ErrAborted
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", ErrAborted
	}
	return "", err
}

func (l *lineIO) writeln(ctx context.Context, s string) error {
	_, err := l.c.Write(ctx, []byte(s+"\r\n"))
	return err
}

func (l *lineIO) writeRaw(ctx context.Context, b []byte) error {
	_, err := l.c.Write(ctx, b)
	return err
}

// newPlayerID generates an opaque 128-bit hex id for a fresh character.
func newPlayerID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
