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
// Idle timeout (spec §6.1): every interactive read is bounded by a
// Clock-driven idle timeout. Config.IdleTimeout is the global fallback;
// Config.PhaseIdleTimeouts overrides it per interactive phase (Name,
// Email, Password — the phases this package owns; the SessionTakeover
// and Creating-wizard phases are bounded by the session/wizard layer).
// Each read passes its phase's resolved timeout to the read primitive,
// so a fresh timer with the right window is created per read.
//
// Deferred:
//
//   - Name-gates (pluggable allow/reject policy)
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
	"time"

	"github.com/Jasrags/AnotherMUD/internal/account"
	"github.com/Jasrags/AnotherMUD/internal/clock"
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

// Phase names the interactive login phases this package owns and bounds
// with an idle timeout (spec §6.1, §2). Used as the key for per-phase
// timeout overrides. SessionTakeover and Creating are spec phases too,
// but their idle bounding lives in the session/wizard layer, not here.
type Phase string

const (
	PhaseName     Phase = "name"
	PhaseEmail    Phase = "email"
	PhasePassword Phase = "password"
)

// Sentinel errors returned from Run.
var (
	ErrAborted      = errors.New("login: connection closed before login")
	ErrIdleTimeout  = errors.New("login: idle timeout")
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

	// ActiveWorlds is the server's active world set (character-identity §5):
	// the namespaces of the loaded `kind: world` packs. A returning
	// character whose WorldID is not in this set is refused login (its world
	// isn't running here). Empty disables the gate entirely — the historical
	// behavior — so callers/tests that don't set it are unaffected.
	ActiveWorlds []string

	// Policy knobs — zero values fall back to package defaults so
	// callers can leave them blank.
	MaxPasswordAttempts int
	MaxEmailAttempts    int
	MinPasswordLength   int
	MinNameLength       int
	MaxNameLength       int

	// Clock drives the per-phase idle timeout (login spec §6.1). nil
	// falls back to the real clock; tests inject a ManualClock to fire
	// the timeout deterministically. Foundation F3: no direct time.Now().
	Clock clock.Clock

	// IdleTimeout bounds every interactive read. Zero (or negative)
	// disables the timeout entirely — the historical behavior — so
	// callers that don't set it read with no deadline. This is the
	// global fallback of spec §6.1.
	IdleTimeout time.Duration

	// PhaseIdleTimeouts overrides IdleTimeout for specific interactive
	// phases (spec §6.1). A phase absent from the map — or mapped to a
	// non-positive value — falls back to IdleTimeout. nil disables all
	// per-phase overrides (every phase uses the global fallback).
	PhaseIdleTimeouts map[Phase]time.Duration

	// NameGates is the ordered list of new-player name policies (spec
	// §3). The first non-allow decision wins. Empty falls back to a
	// reserved-names gate built from ReservedNames (nameGates).
	NameGates []NameGate

	// ReservedNames seeds the default name-gate's case-insensitive
	// blocklist (admin, guard, …) when NameGates is not set explicitly.
	ReservedNames []string
}

// nameGates returns the configured gates, or the built-in default (a
// reserved-names gate over ReservedNames) when none are set.
func (c Config) nameGates() []NameGate {
	if len(c.NameGates) > 0 {
		return c.NameGates
	}
	return []NameGate{ReservedNameGate(c.ReservedNames)}
}

func (c Config) idleClock() clock.Clock {
	if c.Clock != nil {
		return c.Clock
	}
	return clock.RealClock{}
}

// phaseIdle resolves the idle timeout for a phase (spec §6.1): the
// per-phase override when configured with a positive value, else the
// global IdleTimeout fallback.
func (c Config) phaseIdle(p Phase) time.Duration {
	if d, ok := c.PhaseIdleTimeouts[p]; ok && d > 0 {
		return d
	}
	return c.IdleTimeout
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
	lio := &lineIO{c: c, clock: cfg.idleClock()}
	loaded, err := runLoop(ctx, lio, cfg)
	if errors.Is(err, ErrIdleTimeout) {
		// Spec §6.1: close with a timeout reason. Send a final line so
		// the peer learns why before the transport drops.
		_ = lio.writeln(ctx, "You took too long to respond. Goodbye.")
	}
	return loaded, err
}

func runLoop(ctx context.Context, lio *lineIO, cfg Config) (*Loaded, error) {
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

		// Name-gates (spec §3) run only on the new-player path — they
		// guard entry into character creation. A reject reprompts; a
		// disconnect closes the connection.
		switch decision, reason := runNameGates(name, cfg.nameGates()); decision {
		case NameReject:
			if reason != "" {
				if err := lio.writeln(ctx, reason); err != nil {
					return nil, err
				}
			}
			continue
		case NameDisconnect:
			if reason != "" {
				_ = lio.writeln(ctx, reason)
			}
			return nil, ErrNameRejected
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
		raw, err := lio.readln(ctx, cfg.phaseIdle(PhaseName))
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

	// World gate (character-identity §5): a character may only enter a server
	// running its world. Refused before password auth or any content
	// restore, so an out-of-world character's save is read but never touched.
	// (Skipped when unstamped or when no world set is configured.)
	if save.WorldID != "" && !cfg.worldActive(save.WorldID) {
		logging.From(ctx).Warn("returning character refused: world not active here",
			slog.String("name", name),
			slog.String("world_id", save.WorldID),
			slog.Any("active_worlds", cfg.ActiveWorlds))
		_ = lio.writeln(ctx, fmt.Sprintf(
			"%q belongs to the %q world, which is not running on this server.", name, save.WorldID))
		return nil, errBackToName
	}

	for attempts := 0; attempts < cfg.maxPwAttempts(); attempts++ {
		pw, err := promptPassword(ctx, lio, "Password: ", cfg.phaseIdle(PhasePassword))
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
	pw, err := promptPassword(ctx, lio, fmt.Sprintf("Password for %s: ", email), cfg.phaseIdle(PhasePassword))
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
	return buildNewCharacter(ctx, lio, cfg, acc, name)
}

// newCharacterOnNewAccount runs the password policy + confirmation
// dance, then creates the account and binds the character. No account
// is created until both the policy check and the confirmation succeed,
// so a mistyped or short password leaves no trace on disk.
func newCharacterOnNewAccount(ctx context.Context, lio *lineIO, cfg Config, email, name string) (*Loaded, error) {
	pw, err := promptPassword(ctx, lio, fmt.Sprintf("Choose a password for %s: ", email), cfg.phaseIdle(PhasePassword))
	if err != nil {
		return nil, err
	}
	if msg := validateNewPassword(pw, cfg); msg != "" {
		_ = lio.writeln(ctx, msg)
		return nil, errBackToName
	}
	confirm, err := promptPassword(ctx, lio, "Confirm password: ", cfg.phaseIdle(PhasePassword))
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
	return buildNewCharacter(ctx, lio, cfg, acc, name)
}

func validateNewPassword(pw string, cfg Config) string {
	if len(pw) < cfg.minPwLen() {
		return fmt.Sprintf("Passwords must be at least %d characters.", cfg.minPwLen())
	}
	return ""
}

// buildNewCharacter constructs the in-memory baseline entity for a new
// character and hands it to the session layer WITHOUT persisting it
// (character-creation §2: "constructs an initial entity using the
// new-player baseline" — it does not commit).
//
// Persistence, account linking, the welcome line, and the
// "character created" log are deferred to the session's completion
// pipeline (character-creation §6.4), which runs after the creation
// wizard finishes. This is what lets a mid-creation disconnect leave no
// on-disk character (§8): nothing is written until commit. The
// authoritative name-uniqueness guard is the commit-time re-check under
// a mutex (§6.4 step 1), not this path — the earlier Players.Exists
// branch in Run is only a soft pre-check.
func buildNewCharacter(ctx context.Context, lio *lineIO, cfg Config, acc *account.Account, name string) (*Loaded, error) {
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
		// Stamp the world this character belongs to (character-identity §3):
		// the namespace of the start room it spawns into. Empty when the
		// configured start location carries no namespace (test configs).
		WorldID: worldOf(cfg.DefaultLocation),
	}
	return &Loaded{Account: acc, Player: save, New: true}, nil
}

// worldOf extracts the world id (namespace) from a namespaced room id
// ("starter-world:town-square" → "starter-world"); "" when the id carries
// no namespace (character-identity §3/§4).
func worldOf(roomID string) string {
	if ns, _, found := strings.Cut(roomID, ":"); found {
		return strings.TrimSpace(ns)
	}
	return ""
}

// worldActive reports whether worldID is in the configured active world set.
// An empty set disables the gate (every world passes) — the historical
// behavior for callers that don't configure ActiveWorlds.
func (c Config) worldActive(worldID string) bool {
	if len(c.ActiveWorlds) == 0 {
		return true
	}
	for _, w := range c.ActiveWorlds {
		if w == worldID {
			return true
		}
	}
	return false
}

func promptEmail(ctx context.Context, lio *lineIO, cfg Config) (string, error) {
	for attempts := 0; attempts < cfg.maxEmailAttempts(); attempts++ {
		if err := lio.writeln(ctx, "Email address: "); err != nil {
			return "", err
		}
		raw, err := lio.readln(ctx, cfg.phaseIdle(PhaseEmail))
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

// promptPassword reads a password with echo suppressed, bounded by the
// Password phase idle timeout (spec §6.1).
func promptPassword(ctx context.Context, lio *lineIO, prompt string, idle time.Duration) (string, error) {
	if err := lio.writeCommand(ctx, iacWillEcho); err != nil {
		return "", err
	}
	if err := lio.writeRaw(ctx, []byte(prompt)); err != nil {
		return "", err
	}
	pw, readErr := lio.readln(ctx, idle)
	// Restore echo before doing anything else so the next prompt is
	// visible — even if Read errored.
	if err := lio.writeCommand(ctx, iacWontEcho); err != nil {
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
	c     conn.Connection
	clock clock.Clock
}

// readln reads one line bounded by the given per-phase idle timeout
// (spec §6.1). A non-positive idle means no deadline (the historical
// behavior).
func (l *lineIO) readln(ctx context.Context, idle time.Duration) (string, error) {
	line, err := l.readBounded(ctx, idle)
	if err == nil {
		return line, nil
	}
	// An idle timeout is its own terminal reason (spec §6.4) — keep it
	// distinct from a clean peer close so the caller can report it.
	if errors.Is(err, ErrIdleTimeout) {
		return "", ErrIdleTimeout
	}
	if errors.Is(err, io.EOF) || errors.Is(err, conn.ErrClosed) {
		return "", ErrAborted
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", ErrAborted
	}
	return "", err
}

// readBounded reads one line, bounded by the per-phase idle timeout
// (spec §6.1) when one is configured. With no timeout it is a plain
// blocking read — the historical behavior. The timeout is driven off
// the injected Clock so it is testable without real waits; on expiry it
// cancels the in-flight read (unblocking the transport) and returns
// ErrIdleTimeout. A fresh timer is created per read, so a late timer
// from a prior phase can never affect the current one (spec §6.1).
func (l *lineIO) readBounded(ctx context.Context, idle time.Duration) (string, error) {
	if idle <= 0 {
		return l.c.Read(ctx)
	}

	clk := l.clock
	if clk == nil {
		clk = clock.RealClock{}
	}
	// Register the timer before spawning the reader so the reader being
	// observed as "running" implies the timer exists (deterministic for
	// a ManualClock-driven test).
	tick, stop := clk.Ticker(idle)
	defer stop()

	rctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() { line, err := l.c.Read(rctx); ch <- result{line, err} }()

	select {
	case r := <-ch:
		return r.line, r.err
	case <-tick:
		// cancel() (deferred) unblocks the read goroutine, which then
		// drains into the buffered channel — no leak.
		return "", ErrIdleTimeout
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (l *lineIO) writeln(ctx context.Context, s string) error {
	_, err := l.c.Write(ctx, []byte(s+"\r\n"))
	return err
}

func (l *lineIO) writeRaw(ctx context.Context, b []byte) error {
	_, err := l.c.Write(ctx, b)
	return err
}

// commandWriter is implemented by transports (telnet) that can write a
// raw protocol command sequence without the IAC-escaping their normal
// Write applies. Defined here, at the use site, per the small-interface
// convention.
type commandWriter interface {
	WriteCommand(ctx context.Context, p []byte) (int, error)
}

// writeCommand sends a telnet command sequence (e.g. IAC WILL ECHO)
// verbatim. On a transport that escapes IAC in Write (telnet), it MUST
// bypass that escaping or the command is corrupted — so it prefers the
// commandWriter path and only falls back to Write for transports that
// don't escape (or don't speak telnet at all, like test fakes).
func (l *lineIO) writeCommand(ctx context.Context, b []byte) error {
	if cw, ok := l.c.(commandWriter); ok {
		_, err := cw.WriteCommand(ctx, b)
		return err
	}
	// Fallback: a transport without WriteCommand is, by definition, not
	// the telnet Conn — it's a test fake or a non-telnet protocol
	// (e.g. WebSocket) that ignores telnet IAC sequences entirely. So
	// even if its Write escapes 0xFF, the corrupted command is harmless
	// junk to that client rather than a functional masking failure.
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
