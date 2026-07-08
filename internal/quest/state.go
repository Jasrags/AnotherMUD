package quest

import "slices"

// ObjectiveProgress is a player's progress on one objective of an active
// quest (§1). Required is copied from the objective's count at the time
// the stage was entered so progress survives a content count change.
type ObjectiveProgress struct {
	ObjectiveID string
	Current     int
	Required    int
}

// Complete reports whether the objective has reached its required count.
func (o ObjectiveProgress) Complete() bool { return o.Current >= o.Required }

// ActiveQuest is a runtime record of a quest a player is working on
// (§1): the quest id, the current stage index, and per-objective
// progress for that stage.
//
// AwaitingTurnIn marks a turn-in quest (§4.3, def.TurnIn) whose final
// stage's objectives are all done but whose rewards have not yet been
// claimed: the quest stays Active in this state until the player returns
// to the giver (the `talk` / TurnIn path), which dispatches the rewards
// and moves it to completed. Auto-grant quests never enter this state —
// they complete the instant the last objective lands.
type ActiveQuest struct {
	QuestID        string
	StageIndex     int
	Objectives     []ObjectiveProgress
	AwaitingTurnIn bool
}

// stageComplete reports whether every objective in the active stage is
// done.
func (a *ActiveQuest) stageComplete() bool {
	// An objective-less stage is "not done" rather than instantly
	// complete. Registry validation forbids empty stages, but a
	// corrupted/hand-edited save could install one via LoadState; this
	// keeps that from triggering an immediate stage-advance/completion.
	if len(a.Objectives) == 0 {
		return false
	}
	for i := range a.Objectives {
		if !a.Objectives[i].Complete() {
			return false
		}
	}
	return true
}

// State is a player's quest state (§1): the active quests and the set of
// completed quest ids (stored as a list; see §4.3 on duplicates).
type State struct {
	Active    []ActiveQuest
	Completed []string
}

func (s *State) findActive(questID string) *ActiveQuest {
	for i := range s.Active {
		if s.Active[i].QuestID == questID {
			return &s.Active[i]
		}
	}
	return nil
}

func (s *State) hasCompleted(questID string) bool {
	return slices.Contains(s.Completed, questID)
}

// removeActive drops every active entry for questID, returning whether
// any were removed.
//
// The in-place filter is safe: `range` copies each ActiveQuest value
// before the loop body runs, and `append` only ever writes to a prefix
// slot (len(kept) <= current index), so it never clobbers an element the
// range has not yet read.
func (s *State) removeActive(questID string) bool {
	kept := s.Active[:0]
	removed := false
	for _, a := range s.Active {
		if a.QuestID == questID {
			removed = true
			continue
		}
		kept = append(kept, a)
	}
	s.Active = kept
	return removed
}

// clone deep-copies the state so a snapshot can be persisted or returned
// without aliasing the live slices.
func (s *State) clone() *State {
	if s == nil {
		return &State{}
	}
	out := &State{
		Active:    make([]ActiveQuest, len(s.Active)),
		Completed: append([]string(nil), s.Completed...),
	}
	for i, a := range s.Active {
		out.Active[i] = ActiveQuest{
			QuestID:        a.QuestID,
			StageIndex:     a.StageIndex,
			Objectives:     append([]ObjectiveProgress(nil), a.Objectives...),
			AwaitingTurnIn: a.AwaitingTurnIn,
		}
	}
	return out
}

// newActiveQuest builds the active record for a quest at the given stage,
// seeding each objective at zero progress with the stage's required
// counts.
func newActiveQuest(questID string, stageIndex int, stage Stage) ActiveQuest {
	objs := make([]ObjectiveProgress, len(stage.Objectives))
	for i, o := range stage.Objectives {
		objs[i] = ObjectiveProgress{ObjectiveID: o.ID, Current: 0, Required: o.Count}
	}
	return ActiveQuest{QuestID: questID, StageIndex: stageIndex, Objectives: objs}
}
