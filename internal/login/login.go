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
	"strconv"
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

// runLoop drives the account-first login (character-select.md): the front
// door identifies the ACCOUNT by username (not a character name), then a
// roster lets the player pick a character or create one. Both the
// existing-account and new-account paths converge on the roster — a new
// account simply has an empty one, which routes straight to creation.
func runLoop(ctx context.Context, lio *lineIO, cfg Config) (*Loaded, error) {
	if err := lio.writeln(ctx, "Welcome to AnotherMUD."); err != nil {
		return nil, err
	}

	for {
		username, err := promptUsername(ctx, lio, cfg)
		if err != nil {
			return nil, err
		}

		var (
			acc  *account.Account
			aerr error
		)
		if cfg.Accounts.UsernameExists(username) {
			acc, aerr = authExistingAccount(ctx, lio, cfg, username)
		} else {
			acc, aerr = createNewAccount(ctx, lio, cfg, username)
		}
		if aerr != nil {
			if errors.Is(aerr, errBackToName) {
				continue
			}
			return nil, aerr
		}

		res, rerr := selectFromRoster(ctx, lio, cfg, acc)
		if rerr != nil {
			if errors.Is(rerr, errBackToName) {
				continue
			}
			return nil, rerr
		}
		return res, nil
	}
}

// errBackToName is an internal signal that a sub-phase wants to bounce
// the user back to the Name prompt without aborting the connection.
var errBackToName = errors.New("login: back to name prompt")

// promptUsername reads the account username — the account-first front door
// (character-select §2). Uses the Name-phase idle timeout. Returns the
// typed-as-typed username; the account service normalizes for lookup.
func promptUsername(ctx context.Context, lio *lineIO, cfg Config) (string, error) {
	for {
		if err := lio.writeln(ctx, "Account username:"); err != nil {
			return "", err
		}
		raw, err := lio.readln(ctx, cfg.phaseIdle(PhaseName))
		if err != nil {
			return "", err
		}
		username := strings.TrimSpace(raw)
		if !account.ValidUsername(account.NormalizeUsername(username)) {
			if err := lio.writeln(ctx, "Usernames are 3-32 characters: letters, digits, or underscore."); err != nil {
				return "", err
			}
			continue
		}
		return username, nil
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

// authExistingAccount runs the password loop for a known account username
// (character-select §2.2). On success returns the authenticated account;
// errBackToName after a wrong password, ErrPasswordCap after the cap.
func authExistingAccount(ctx context.Context, lio *lineIO, cfg Config, username string) (*account.Account, error) {
	for attempts := 0; attempts < cfg.maxPwAttempts(); attempts++ {
		pw, err := promptPassword(ctx, lio, "Password: ", cfg.phaseIdle(PhasePassword))
		if err != nil {
			return nil, err
		}
		acc, err := cfg.Accounts.AuthenticateByUsername(ctx, username, pw)
		if err != nil {
			if errors.Is(err, account.ErrAuthFailed) {
				if err := lio.writeln(ctx, "Incorrect password."); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("auth by username: %w", err)
		}
		return acc, nil
	}
	_ = lio.writeln(ctx, "Too many failed attempts. Goodbye.")
	return nil, ErrPasswordCap
}

// rosterEntry is one line of the character roster (character-select §3):
// the character's name, its world, whether it is available on this server
// (its world is in the active set), and its loaded save (nil if the load
// failed — shown unavailable).
type rosterEntry struct {
	name      string
	world     string
	available bool
	save      *player.Save
}

// selectFromRoster presents the account's characters (character-select §3-§4)
// and returns the selected one, or routes to creation. An empty roster goes
// straight to create. An out-of-world character is listed but not selectable
// (the character-identity §5 world gate, surfaced here).
func selectFromRoster(ctx context.Context, lio *lineIO, cfg Config, acc *account.Account) (*Loaded, error) {
	entries := make([]rosterEntry, 0, len(acc.Characters))
	for _, name := range acc.Characters {
		save, err := cfg.Players.Load(ctx, name)
		if err != nil {
			// A listed character whose save won't load (version drift,
			// removed file): show it, but unselectable.
			logging.From(ctx).Warn("roster: character load failed",
				slog.String("name", name), slog.Any("err", err))
			entries = append(entries, rosterEntry{name: name})
			continue
		}
		avail := save.WorldID == "" || cfg.worldActive(save.WorldID)
		entries = append(entries, rosterEntry{name: name, world: save.WorldID, available: avail, save: save})
	}
	if len(entries) == 0 {
		return createCharacter(ctx, lio, cfg, acc)
	}

	for {
		if err := printRoster(ctx, lio, entries); err != nil {
			return nil, err
		}
		raw, err := lio.readln(ctx, cfg.phaseIdle(PhaseName))
		if err != nil {
			return nil, err
		}
		choice := strings.TrimSpace(raw)
		if choice == "" {
			continue
		}
		if strings.EqualFold(choice, "n") || strings.EqualFold(choice, "new") {
			return createCharacter(ctx, lio, cfg, acc)
		}
		e := resolveRosterChoice(entries, choice)
		if e == nil {
			if err := lio.writeln(ctx, "No such character. Pick a number from the list, or 'n' to create."); err != nil {
				return nil, err
			}
			continue
		}
		if !e.available {
			msg := fmt.Sprintf("%q is not available on this server.", e.name)
			if e.world != "" {
				msg = fmt.Sprintf("%q belongs to the %q world, which is not running on this server.", e.name, e.world)
			}
			if err := lio.writeln(ctx, msg); err != nil {
				return nil, err
			}
			continue
		}
		return &Loaded{Account: acc, Player: e.save, New: false}, nil
	}
}

// printRoster renders the numbered character roster (character-select §3):
// each character with its world and an "(unavailable on this server)" marker
// when out-of-world, plus the create-new option and the selection prompt.
func printRoster(ctx context.Context, lio *lineIO, entries []rosterEntry) error {
	if err := lio.writeln(ctx, "Your characters:"); err != nil {
		return err
	}
	for i, e := range entries {
		line := fmt.Sprintf("  %d) %s", i+1, e.name)
		if e.world != "" {
			line += " [" + e.world + "]"
		}
		if !e.available {
			line += " (unavailable on this server)"
		}
		if err := lio.writeln(ctx, line); err != nil {
			return err
		}
	}
	if err := lio.writeln(ctx, "  n) create a new character"); err != nil {
		return err
	}
	return lio.writeln(ctx, "Select a character (number or name), or 'n' to create:")
}

// resolveRosterChoice matches a selection against the roster by 1-based
// index or by character name (case-insensitive). nil on no match.
func resolveRosterChoice(entries []rosterEntry, choice string) *rosterEntry {
	if n, err := strconv.Atoi(choice); err == nil {
		if n >= 1 && n <= len(entries) {
			return &entries[n-1]
		}
		return nil
	}
	for i := range entries {
		if strings.EqualFold(entries[i].name, choice) {
			return &entries[i]
		}
	}
	return nil
}

// createCharacter prompts for a new character name (validated + name-gated +
// soft-uniqueness-checked) and builds the new-character baseline stamped with
// the active world (character-select §4; character-identity §3). Persistence +
// account linking happen in the session completion pipeline.
func createCharacter(ctx context.Context, lio *lineIO, cfg Config, acc *account.Account) (*Loaded, error) {
	for {
		if err := lio.writeln(ctx, "What is your new character's name?"); err != nil {
			return nil, err
		}
		raw, err := lio.readln(ctx, cfg.phaseIdle(PhaseName))
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(raw)
		if msg := validateName(name, cfg); msg != "" {
			if err := lio.writeln(ctx, msg); err != nil {
				return nil, err
			}
			continue
		}
		// Name-gates (spec §3) guard character creation — reserved names
		// (admin, guard, …) are refused; a disconnect gate closes the conn.
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
		// Soft uniqueness pre-check (the commit-time mutex re-check in the
		// session completion pipeline is authoritative — character-creation §6.4).
		if cfg.Players.Exists(name) {
			if err := lio.writeln(ctx, fmt.Sprintf("A character named %q already exists. Choose another.", name)); err != nil {
				return nil, err
			}
			continue
		}
		return buildNewCharacter(ctx, lio, cfg, acc, name)
	}
}

// createNewAccount creates an account for a not-yet-registered username
// (character-select §2.2): name-gate the username, choose + confirm a
// password, register (email omitted — demoted/deprecated). No account is
// written until the confirmation succeeds.
func createNewAccount(ctx context.Context, lio *lineIO, cfg Config, username string) (*account.Account, error) {
	// Name-gates (spec §3) apply to the username too — reserved names
	// (admin, guard, …) cannot be accounts.
	switch decision, reason := runNameGates(username, cfg.nameGates()); decision {
	case NameReject:
		if reason != "" {
			_ = lio.writeln(ctx, reason)
		}
		return nil, errBackToName
	case NameDisconnect:
		if reason != "" {
			_ = lio.writeln(ctx, reason)
		}
		return nil, ErrNameRejected
	}

	if err := lio.writeln(ctx, fmt.Sprintf("No account named %q exists. Creating it.", username)); err != nil {
		return nil, err
	}
	pw, err := promptPassword(ctx, lio, "Choose a password: ", cfg.phaseIdle(PhasePassword))
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
		_ = lio.writeln(ctx, "Passwords did not match. Returning to the username prompt.")
		return nil, errBackToName
	}

	acc, err := cfg.Accounts.CreateWithUsername(ctx, username, "", pw)
	if err != nil {
		// A concurrent create may have taken the username while we were
		// collecting the password — bounce to the username prompt.
		if errors.Is(err, account.ErrUsernameTaken) {
			_ = lio.writeln(ctx, "That username was just taken. Try another.")
			return nil, errBackToName
		}
		return nil, fmt.Errorf("create account: %w", err)
	}
	return acc, nil
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
