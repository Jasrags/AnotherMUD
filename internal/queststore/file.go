package queststore

import "github.com/Jasrags/AnotherMUD/internal/quest"

// questFile is the on-disk YAML shape for a player's quest state. It
// mirrors quest.State with snake_case tags, keeping the pure quest
// package free of serialization concerns.
type questFile struct {
	Active    []activeQuestFile `yaml:"active,omitempty"`
	Completed []string          `yaml:"completed,omitempty"`
}

type activeQuestFile struct {
	QuestID    string                  `yaml:"quest"`
	Stage      int                     `yaml:"stage"`
	Objectives []objectiveProgressFile `yaml:"objectives,omitempty"`
}

type objectiveProgressFile struct {
	ID       string `yaml:"id"`
	Current  int    `yaml:"current"`
	Required int    `yaml:"required"`
}

// toFile converts a quest.State to its on-disk shape. A nil state yields
// an empty file.
func toFile(s *quest.State) questFile {
	if s == nil {
		return questFile{}
	}
	out := questFile{Completed: append([]string(nil), s.Completed...)}
	for _, a := range s.Active {
		af := activeQuestFile{QuestID: a.QuestID, Stage: a.StageIndex}
		for _, o := range a.Objectives {
			af.Objectives = append(af.Objectives, objectiveProgressFile{
				ID: o.ObjectiveID, Current: o.Current, Required: o.Required,
			})
		}
		out.Active = append(out.Active, af)
	}
	return out
}

// toState converts the on-disk shape back to a quest.State.
func (f questFile) toState() *quest.State {
	st := &quest.State{Completed: append([]string(nil), f.Completed...)}
	for _, af := range f.Active {
		aq := quest.ActiveQuest{QuestID: af.QuestID, StageIndex: af.Stage}
		for _, of := range af.Objectives {
			aq.Objectives = append(aq.Objectives, quest.ObjectiveProgress{
				ObjectiveID: of.ID, Current: of.Current, Required: of.Required,
			})
		}
		st.Active = append(st.Active, aq)
	}
	return st
}
