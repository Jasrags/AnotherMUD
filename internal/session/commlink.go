package session

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// CommlinkOnboarding supplies the pack-authored data the engine delivers as a
// first-entry commlink call — a fixer pinging the new runner's commlink. The
// engine stays pack-neutral (it only frames + gates the message); the content
// lives on the configured fixer NPC. See onboarding-guide.md.
type CommlinkOnboarding interface {
	// Welcome returns the configured fixer's onboarding message and whether the
	// feature is configured with a non-empty message.
	Welcome() (string, bool)
	// CarriesCommlink reports whether any of the given carried entity ids is a
	// commlink item — the device-dependency gate (no commlink, no call).
	CarriesCommlink(inventory []entities.EntityID) bool
}

// SetCommlink installs the first-entry commlink-onboarding service. Set once at
// startup; a nil svc disables the feature (no call is ever delivered).
func (m *Manager) SetCommlink(svc CommlinkOnboarding) {
	m.mu.Lock()
	m.commlink = svc
	m.mu.Unlock()
}

// commlinkWelcomeOnceID keys the shown-once record for the first-entry call.
const commlinkWelcomeOnceID = "onboarding:commlink-welcome"

// DeliverCommlinkCallFor delivers the one-time first-entry commlink call to the
// online character, if the feature is configured, the runner carries a commlink,
// and they have not seen it. Idempotent and safe to call from every world-entry
// path: it is invoked both at enter-world (covering returning characters whose
// inventory is already restored) and at the end of the character.created grant
// pass (covering a fresh character, whose commlink is granted after enter-world).
// The shown-once record makes whichever path reaches it first the only delivery.
func (m *Manager) DeliverCommlinkCallFor(ctx context.Context, playerID string) {
	if m == nil {
		return
	}
	m.mu.RLock()
	svc := m.commlink
	m.mu.RUnlock()
	if svc == nil {
		return // feature off for this world
	}
	a, ok := m.GetByPlayerID(playerID)
	if !ok || a == nil {
		return
	}
	msg, ok := svc.Welcome()
	if !ok {
		return // no configured fixer / empty message
	}
	// Device-dependency: the call only reaches a runner who actually carries a
	// commlink. No commlink → no call — and we do NOT mark it shown, so acquiring
	// one on a later login still triggers the call.
	if !svc.CarriesCommlink(a.Inventory()) {
		return
	}
	// Once per character. Unlike a contextual tip this fires even with tips
	// disabled and carries no "Tip:" framing — it is a story beat, not a hint.
	if !a.markShownOnce(commlinkWelcomeOnceID) {
		return
	}
	_ = a.Write(ctx, framedCommlinkCall(msg))
}

// framedCommlinkCall wraps the pack-authored message in a pack-neutral comm
// frame. The message itself carries any world-specific call-to-action.
func framedCommlinkCall(msg string) string {
	return "\n>> Your commlink chimes — incoming transmission.\n\n" + msg + "\n\n>> Transmission ends.\n"
}

// markShownOnce records id in the shown-once set and reports whether it was newly
// recorded (true = first time for this character). Unlike ShowTipOnce it does NOT
// gate on the tips opt-out and writes nothing itself — for one-time story beats
// (the commlink onboarding call) that must fire once even with tips off. Reuses
// the TipsSeen shown-once set, so no save-version bump.
func (a *connActor) markShownOnce(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tipsSeen == nil {
		a.tipsSeen = make(map[string]struct{})
	}
	if _, seen := a.tipsSeen[id]; seen {
		return false
	}
	a.tipsSeen[id] = struct{}{}
	if a.save != nil {
		a.save.TipsSeen = append(a.save.TipsSeen, id)
		a.markDirtyLocked()
	}
	return true
}
