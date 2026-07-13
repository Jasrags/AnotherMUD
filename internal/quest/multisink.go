package quest

// MultiSink fans one quest lifecycle event out to several EventSinks in order.
// The composition root uses it to run both the session notifier (player-facing
// banners) and the quest-spawn lifecycle (quest-spawns.md) off the same events
// without either knowing about the other. Each member must obey the EventSink
// contract (non-blocking, no re-entry into the Service).
type MultiSink []EventSink

func (m MultiSink) Started(e StartedEvent) {
	for _, s := range m {
		s.Started(e)
	}
}

func (m MultiSink) ObjectiveAdvanced(e ObjectiveAdvancedEvent) {
	for _, s := range m {
		s.ObjectiveAdvanced(e)
	}
}

func (m MultiSink) StageAdvanced(e StageAdvancedEvent) {
	for _, s := range m {
		s.StageAdvanced(e)
	}
}

func (m MultiSink) ReadyToTurnIn(e ReadyToTurnInEvent) {
	for _, s := range m {
		s.ReadyToTurnIn(e)
	}
}

func (m MultiSink) Completed(e CompletedEvent) {
	for _, s := range m {
		s.Completed(e)
	}
}

func (m MultiSink) Abandoned(e AbandonedEvent) {
	for _, s := range m {
		s.Abandoned(e)
	}
}
