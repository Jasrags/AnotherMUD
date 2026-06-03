// Package quest is the M10 quest system (quests.md). This slice (M10.6)
// covers the definition model, the registry, and objective-id
// normalization; acceptance/progression/rewards/persistence/watcher land
// in later slices.
//
// Quests are pure content — the engine ships none; the registry is empty
// until packs populate it.
package quest

// Objective is a single trackable goal within a stage (§1). Target is a
// template/room id (namespaced by the loader); NPC is the delivery
// recipient template id for deliver objectives. Count is the required
// progress (normalized to >= 1).
type Objective struct {
	ID          string
	Type        string // kill / collect / deliver / visit / custom
	Target      string
	NPC         string
	Count       int
	Description string
}

// Stage is an ordered milestone within a quest (§1). Objectives within a
// stage complete in parallel; the quest advances stage-by-stage.
type Stage struct {
	ID          string // optional; seeds generated objective ids
	Description string
	Hint        string
	Objectives  []Objective
}

// Prerequisite gates acceptance (§3.2). All present gates must pass;
// absent gates (zero value / empty) are no-ops.
type Prerequisite struct {
	MinLevel           int
	Class              string
	QuestsCompleted    []string
	QuestsNotCompleted []string
}

// Reward is dispatched on completion (§5.1). Any field may be zero/empty.
type Reward struct {
	XP          int64
	Gold        int
	Items       []string // item template ids
	Abilities   []string // ability ids to teach
	ClassUnlock string
	RaceUnlock  string
}

// Definition is a content-defined quest (§2.3). ID + a non-empty stage
// list (each with >= 1 objective) are required; everything else has a
// sensible default. Abandonable defaults to true (set by the loader /
// not by the zero value).
type Definition struct {
	ID             string
	Name           string
	Classification string // main / side / daily
	Giver          string // giver template id (namespaced)
	Offer          string // giver's pitch shown by `talk`; falls back to stage-0 description
	TurnIn         bool   // completion requires returning to the giver (§4.3); false = auto-grant
	Repeatable     bool
	Abandonable    bool
	Secret         bool
	Prereq         Prerequisite
	Stages         []Stage
	Reward         Reward
	Script         string
	PackDir        string
}
