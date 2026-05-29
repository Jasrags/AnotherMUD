package session

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/wizard"
)

func twoRaceOneClass(t *testing.T) (*progression.RaceRegistry, *progression.ClassRegistry) {
	t.Helper()
	rr := progression.NewRaceRegistry()
	if err := rr.Register(&progression.Race{ID: "human", DisplayName: "Human"}); err != nil {
		t.Fatalf("register human: %v", err)
	}
	if err := rr.Register(&progression.Race{ID: "elf", DisplayName: "Elf"}); err != nil {
		t.Fatalf("register elf: %v", err)
	}
	cr := progression.NewClassRegistry()
	if err := cr.Register(&progression.Class{ID: "fighter", DisplayName: "Fighter"}); err != nil {
		t.Fatalf("register fighter: %v", err)
	}
	return rr, cr
}

// wizFakeIO is a minimal wizard.IO for driving the flow directly.
type wizFakeIO struct{ lines []string }

func (f *wizFakeIO) Write(_ context.Context, msg string) error {
	f.lines = append(f.lines, msg)
	return nil
}
func (f *wizFakeIO) SetEcho(context.Context, bool) {}

func TestNewCreationFlow_AssemblesRaceAndClass(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	flow := NewCreationFlow(rr, cr)
	if flow == nil {
		t.Fatal("flow should not be nil with races+classes")
	}
	e := &creationEntity{}
	in := wizard.NewInstance(flow, e, &wizFakeIO{}, nil)
	in.Start(context.Background())
	// race menu order is sorted by id: elf(1), human(2). Pick human=2.
	in.Input(context.Background(), "2")
	// only class is fighter=1.
	in.Input(context.Background(), "1")
	st, _ := in.Input(context.Background(), "yes")

	if st != wizard.StatusCompleted {
		t.Fatalf("status = %v, want Completed", st)
	}
	if e.raceID != "human" || e.classID != "fighter" {
		t.Errorf("race/class = %q/%q, want human/fighter", e.raceID, e.classID)
	}
	if ok, _ := flow.OnComplete(context.Background(), e); !ok {
		t.Error("OnComplete should accept a confirmed character")
	}
}

func TestNewCreationFlow_ConfirmNoFailsValidation(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	flow := NewCreationFlow(rr, cr)
	e := &creationEntity{}
	in := wizard.NewInstance(flow, e, &wizFakeIO{}, nil)
	in.Start(context.Background())
	in.Input(context.Background(), "elf")     // prefix match
	in.Input(context.Background(), "fighter") // prefix match
	in.Input(context.Background(), "no")      // decline

	if !e.rejected {
		t.Fatal("confirm 'no' should set rejected")
	}
	if ok, msg := flow.OnComplete(context.Background(), e); ok || msg == "" {
		t.Errorf("OnComplete after decline = (%v, %q), want (false, non-empty)", ok, msg)
	}
}

func TestNewCreationFlow_NilWhenNoContent(t *testing.T) {
	if NewCreationFlow(progression.NewRaceRegistry(), progression.NewClassRegistry()) != nil {
		t.Error("empty registries should yield a nil flow (no choices)")
	}
	if NewCreationFlow(nil, nil) != nil {
		t.Error("nil registries should yield a nil flow")
	}
}

// scriptedConn feeds queued input lines to runCreation and captures
// output. Read returns io.EOF once the script is exhausted (simulating a
// mid-creation disconnect).
type scriptedConn struct {
	inputs  []string
	writes  []string
	readIdx int
}

func (s *scriptedConn) ID() string { return "scripted" }
func (s *scriptedConn) Read(context.Context) (string, error) {
	if s.readIdx >= len(s.inputs) {
		return "", io.EOF
	}
	line := s.inputs[s.readIdx]
	s.readIdx++
	return line, nil
}
func (s *scriptedConn) Write(_ context.Context, p []byte) (int, error) {
	s.writes = append(s.writes, string(p))
	return len(p), nil
}
func (s *scriptedConn) Close() error { return nil }

func newPlayerLoaded(name string) *login.Loaded {
	return &login.Loaded{
		New:    true,
		Player: &player.Save{Version: player.CurrentVersion, ID: "p-1", Name: name, Location: "x:1"},
	}
}

func TestRunCreation_PopulatesRaceClassOnSave(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr)}
	loaded := newPlayerLoaded("Bob")
	conn := &scriptedConn{inputs: []string{"elf", "fighter", "yes"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.Race != "elf" || loaded.Player.Class != "fighter" {
		t.Errorf("save race/class = %q/%q, want elf/fighter", loaded.Player.Race, loaded.Player.Class)
	}
}

func TestRunCreation_NilFlowIsNoop(t *testing.T) {
	loaded := newPlayerLoaded("Bob")
	conn := &scriptedConn{}
	if err := runCreation(context.Background(), conn, Config{CreationFlow: nil}, loaded); err != nil {
		t.Fatalf("nil-flow runCreation: %v", err)
	}
	if loaded.Player.Race != "" {
		t.Error("nil flow should not touch the save")
	}
}

func TestRunCreation_DisconnectReturnsError(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr)}
	loaded := newPlayerLoaded("Bob")
	// Disconnect after the race choice (script exhausts → io.EOF).
	conn := &scriptedConn{inputs: []string{"elf"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err == nil {
		t.Fatal("expected an error on mid-creation disconnect")
	}
	if loaded.Player.Class != "" {
		t.Error("disconnect must not have assembled a class")
	}
}

// Confirm "no" restarts the flow against a fresh entity; the second pass
// (yes) commits.
func TestRunCreation_DeclineRestarts(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr)}
	loaded := newPlayerLoaded("Bob")
	conn := &scriptedConn{inputs: []string{
		"human", "fighter", "no", // decline → restart
		"elf", "fighter", "yes", // second pass commits
	}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.Race != "elf" {
		t.Errorf("after restart race = %q, want elf (the second pass)", loaded.Player.Race)
	}
	// A restart message was written.
	joined := strings.Join(conn.writes, "")
	if !strings.Contains(joined, "start over") {
		t.Errorf("expected a restart message; writes = %q", joined)
	}
}

// A flow whose validation always fails (here: declining every confirm)
// hits the restart cap and aborts rather than looping forever.
func TestRunCreation_RestartCapAbandons(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr)}
	loaded := newPlayerLoaded("Bob")
	// Decline forever: each pass is race, class, "no" → restart.
	var script []string
	for i := 0; i < maxCreationRestarts+2; i++ {
		script = append(script, "elf", "fighter", "no")
	}
	conn := &scriptedConn{inputs: script}

	if err := runCreation(context.Background(), conn, cfg, loaded); !errors.Is(err, ErrCreationAbandoned) {
		t.Fatalf("err = %v, want ErrCreationAbandoned", err)
	}
	if loaded.Player.Race != "" {
		t.Error("an abandoned creation must not assemble a character")
	}
}

func TestRunCreation_HelpPassthroughDoesNotAdvance(t *testing.T) {
	rr, cr := twoRaceOneClass(t)
	cfg := Config{CreationFlow: NewCreationFlow(rr, cr)} // Help nil → "not available"
	loaded := newPlayerLoaded("Bob")
	// A help line at the race step must NOT consume the step: the
	// following real "elf" selection still lands.
	conn := &scriptedConn{inputs: []string{"help races", "elf", "fighter", "yes"}}

	if err := runCreation(context.Background(), conn, cfg, loaded); err != nil {
		t.Fatalf("runCreation: %v", err)
	}
	if loaded.Player.Race != "elf" {
		t.Errorf("race = %q, want elf (help must not have advanced past the race step)", loaded.Player.Race)
	}
}
