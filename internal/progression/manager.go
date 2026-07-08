package progression

import "context"

// Manager owns the XP/level operations from spec §5. It holds the
// track registry and an EventSink for downstream emission; the
// per-entity state lives on the entity (ProgressionState) and is
// passed in by callers.
//
// Manager is safe for concurrent use as long as ProgressionState
// is — the Manager itself holds no mutable state per call. The
// state's internal lock orders all level/XP mutations on a single
// entity; cross-entity calls don't share state.
type Manager struct {
	tracks *TrackRegistry
	sink   EventSink
}

// NewManager returns a Manager backed by the given registry and
// event sink. The sink may be nil for tests that don't care about
// emitted events; production wiring uses a bus-backed adapter
// (cmd/anothermud).
func NewManager(tracks *TrackRegistry, sink EventSink) *Manager {
	if sink == nil {
		sink = nopSink{}
	}
	return &Manager{tracks: tracks, sink: sink}
}

// Tracks returns the underlying registry. Exposed for callers that
// want to enumerate tracks (admin commands, renderers) without
// having to hold a parallel reference.
func (m *Manager) Tracks() *TrackRegistry { return m.tracks }

// GrantResult is the structured return from GrantExperience. The
// fields cover the diagnostic surface a caller (admin command,
// quest reward) wants to render: how much XP was actually added,
// what the new total is, and whether any level-ups fired.
type GrantResult struct {
	// Track is the resolved track name. Empty when TrackUnknown is
	// true.
	Track string
	// TrackUnknown is true when the named track is not in the
	// registry. The call is a silent no-op per spec §5.4 step 1;
	// the flag lets callers surface the diagnostic if they want.
	TrackUnknown bool
	// XPAdded is the amount actually added (== amount today; the
	// field exists so a future "XP multiplier" hook can report the
	// effective grant without callers having to subtract).
	XPAdded int64
	// NewXP is the entity's total XP on this track after the grant.
	NewXP int64
	// OldLevel / NewLevel bracket the level-ups (if any). NewLevel
	// == OldLevel means no cascade fired.
	OldLevel int
	NewLevel int
}

// GrantExperience adds amount to entityID's XP on trackName. Emits
// progression.xp.gained once, then cascades through every level
// threshold the new total crosses (emitting progression.level.up
// per step). Cascade stops at MaxLevel or when the next threshold
// is undefined (spec §5.4).
//
// A grant on an unknown track is a silent no-op (returns
// TrackUnknown=true). An amount <= 0 is also a no-op (returns the
// current totals with XPAdded=0); the spec does not specify
// negative grants here — DeductExperience is the explicit path.
func (m *Manager) GrantExperience(ctx context.Context, state *ProgressionState, entityID, trackName string, amount int64, source string) GrantResult {
	trackName = canonTrackName(trackName) // one key for the registry AND the state map
	if state == nil {
		return GrantResult{Track: trackName, TrackUnknown: true}
	}
	td, ok := m.tracks.Get(trackName)
	if !ok {
		return GrantResult{Track: trackName, TrackUnknown: true}
	}
	if amount <= 0 {
		state.mu.Lock()
		curLevel, curXP := m.lazyInitLocked(state, trackName)
		state.mu.Unlock()
		return GrantResult{
			Track:    trackName,
			XPAdded:  0,
			NewXP:    curXP,
			OldLevel: curLevel,
			NewLevel: curLevel,
		}
	}

	state.mu.Lock()
	oldLevel, oldXP := m.lazyInitLocked(state, trackName)
	newXP := oldXP + amount
	state.xp[trackName] = newXP
	newLevel := oldLevel
	// Cascade through thresholds while the entity is below
	// MaxLevel AND the next threshold is defined AND new total has
	// reached it.
	for newLevel < td.MaxLevel {
		threshold := td.GetXpForLevel(newLevel + 1)
		if threshold < 0 || newXP < threshold {
			break
		}
		newLevel++
		state.levels[trackName] = newLevel
	}
	state.mu.Unlock()

	m.sink.OnXPGained(ctx, entityID, trackName, amount, newXP, source)

	for lvl := oldLevel + 1; lvl <= newLevel; lvl++ {
		if td.OnLevelUp != nil {
			td.OnLevelUp(entityID, trackName, lvl)
		}
		m.sink.OnLevelUp(ctx, entityID, trackName, lvl-1, lvl)
	}

	return GrantResult{
		Track:    trackName,
		XPAdded:  amount,
		NewXP:    newXP,
		OldLevel: oldLevel,
		NewLevel: newLevel,
	}
}

// DeductResult mirrors GrantResult for the XP-loss path.
type DeductResult struct {
	Track        string
	TrackUnknown bool
	// XPLost is the amount actually removed. May be less than the
	// caller asked for if the request would push XP below the
	// current level's threshold (spec §5.5).
	XPLost int64
	NewXP  int64
	Level  int
}

// DeductExperience removes XP without de-leveling. The XP floor is
// the current level's threshold (or 0 for level 1); XP cannot drop
// below this floor. Emits progression.xp.lost only when actual
// loss > 0 (spec §5.5).
func (m *Manager) DeductExperience(ctx context.Context, state *ProgressionState, entityID, trackName string, amount int64) DeductResult {
	trackName = canonTrackName(trackName) // one key for the registry AND the state map
	if state == nil {
		return DeductResult{Track: trackName, TrackUnknown: true}
	}
	td, ok := m.tracks.Get(trackName)
	if !ok {
		return DeductResult{Track: trackName, TrackUnknown: true}
	}
	if amount <= 0 {
		state.mu.Lock()
		curLevel, curXP := m.lazyInitLocked(state, trackName)
		state.mu.Unlock()
		return DeductResult{
			Track:  trackName,
			XPLost: 0,
			NewXP:  curXP,
			Level:  curLevel,
		}
	}

	state.mu.Lock()
	curLevel, curXP := m.lazyInitLocked(state, trackName)
	floor := max(td.GetXpForLevel(curLevel),
		// Undefined threshold for the current level — treat as 0
		// (level 1 fallback). Same reasoning as spec §5.5: the
		// floor on level 1 is "the threshold for level 1, which is
		// 0".
		0)
	target := max(curXP-amount, floor)
	actualLoss := curXP - target
	if actualLoss > 0 {
		state.xp[trackName] = target
	}
	state.mu.Unlock()

	if actualLoss > 0 {
		m.sink.OnXPLost(ctx, entityID, trackName, actualLoss, target)
	}
	return DeductResult{
		Track:  trackName,
		XPLost: actualLoss,
		NewXP:  target,
		Level:  curLevel,
	}
}

// TrackInfo is the structured view spec §5.6 specifies, suitable
// for the score panel and GMCP packages.
type TrackInfo struct {
	Track                 string
	Level                 int
	XP                    int64
	XpToNext              int64
	CurrentLevelThreshold int64
	MaxLevel              int
	// Overflow at max level: xp - currentLevelThreshold. Zero
	// below max.
	Overflow int64
}

// GetTrackInfo returns the structured view for trackName on state.
// Returns (zero, false) if the track is unknown. Lazy-inits the
// state under its lock so subsequent reads see level 1.
func (m *Manager) GetTrackInfo(state *ProgressionState, trackName string) (TrackInfo, bool) {
	trackName = canonTrackName(trackName) // canonical key, matching GrantExperience
	if state == nil {
		return TrackInfo{}, false
	}
	td, ok := m.tracks.Get(trackName)
	if !ok {
		return TrackInfo{}, false
	}
	state.mu.Lock()
	level, xp := m.lazyInitLocked(state, trackName)
	state.mu.Unlock()

	cur := max(td.GetXpForLevel(level), 0)
	info := TrackInfo{
		Track:                 trackName,
		Level:                 level,
		XP:                    xp,
		CurrentLevelThreshold: cur,
		MaxLevel:              td.MaxLevel,
	}
	if level >= td.MaxLevel {
		info.XpToNext = 0
		info.Overflow = max(xp-cur, 0)
	} else {
		next := td.GetXpForLevel(level + 1)
		if next < 0 {
			info.XpToNext = 0
		} else {
			info.XpToNext = max(next-xp, 0)
		}
	}
	return info, true
}

// ResetTrack sets (level=1, xp=0) for trackName on state. Emits
// progression.track.reset. The level reset is downward, so no
// level-up cascade fires (spec §5.7).
func (m *Manager) ResetTrack(ctx context.Context, state *ProgressionState, entityID, trackName string) {
	trackName = canonTrackName(trackName) // symmetry with the other entry points — one state key
	if state == nil {
		return
	}
	if !m.tracks.Has(trackName) {
		return
	}
	state.mu.Lock()
	state.levels[trackName] = 1
	state.xp[trackName] = 0
	state.mu.Unlock()
	m.sink.OnTrackReset(ctx, entityID, trackName)
}

// lazyInitLocked seeds (level=1, xp=0) for trackName if the state
// has no entry yet. Returns the post-init (level, xp). Caller MUST
// hold state.mu.
func (m *Manager) lazyInitLocked(state *ProgressionState, trackName string) (int, int64) {
	level := state.levels[trackName]
	xp := state.xp[trackName]
	if level == 0 && xp == 0 {
		// First interaction: seed level 1, xp 0.
		state.levels[trackName] = 1
		level = 1
	}
	return level, xp
}
