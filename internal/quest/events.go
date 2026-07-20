package quest

// Observable events (§9). The service emits these through an EventSink so
// the quest package stays decoupled from the engine event bus; the
// composition root bridges the sink to the bus.

// StartedEvent fires when a quest is accepted (§3.1). Banner is the
// player-visible block (empty when suppressed).
type StartedEvent struct {
	PlayerID string
	QuestID  string
	Banner   string
}

// ObjectiveAdvancedEvent fires when an objective's progress changes
// (§4.1).
type ObjectiveAdvancedEvent struct {
	PlayerID    string
	QuestID     string
	ObjectiveID string
	Current     int
	Required    int
}

// StageAdvancedEvent fires when the active stage moves forward (§4.2).
type StageAdvancedEvent struct {
	PlayerID   string
	QuestID    string
	StageIndex int
}

// CompletedEvent fires when the final stage's objectives complete
// (§4.3). It carries the reward amounts and lists.
type CompletedEvent struct {
	PlayerID    string
	QuestID     string
	XP          int64
	Gold        int
	Items       []string
	Abilities   []string
	Faction     []FactionReward // standing shifts granted (faction.md §5.1)
	Reputation  int             // renown shift granted (reputation.md §5.3)
	ClassUnlock string
	RaceUnlock  string
}

// ReadyToTurnInEvent fires when a turn-in quest's final-stage objectives
// all complete but rewards are withheld pending return to the giver
// (§4.3). Giver is the giver template id the player must return to.
type ReadyToTurnInEvent struct {
	PlayerID string
	QuestID  string
	Giver    string
}

// AbandonedEvent fires when a player abandons a quest (§4.5).
type AbandonedEvent struct {
	PlayerID string
	QuestID  string
}

// EventSink receives the quest lifecycle events (§9). All methods must
// be non-blocking and MUST NOT call back into the Service (the service
// holds its lock while emitting).
type EventSink interface {
	Started(StartedEvent)
	ObjectiveAdvanced(ObjectiveAdvancedEvent)
	StageAdvanced(StageAdvancedEvent)
	ReadyToTurnIn(ReadyToTurnInEvent)
	Completed(CompletedEvent)
	Abandoned(AbandonedEvent)
}

// NopEventSink discards every event. The default when no sink is wired.
type NopEventSink struct{}

func (NopEventSink) Started(StartedEvent)                     {}
func (NopEventSink) ObjectiveAdvanced(ObjectiveAdvancedEvent) {}
func (NopEventSink) StageAdvanced(StageAdvancedEvent)         {}
func (NopEventSink) ReadyToTurnIn(ReadyToTurnInEvent)         {}
func (NopEventSink) Completed(CompletedEvent)                 {}
func (NopEventSink) Abandoned(AbandonedEvent)                 {}
