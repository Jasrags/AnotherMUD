package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// TrainingActor is the Actor extension a training verb needs.
// connActor implements this; test fakes that don't exercise
// training can omit the methods and the verb will report a generic
// "you cannot train" message.
type TrainingActor interface {
	StatBlock() *progression.StatBlock
	TrainsAvailable() int
	SpendTrain() bool
	RaceID() string
	HasRoomTag(tag string) bool
	// AttributeSet is the character's resolved base attribute set (SR-M1),
	// authoritative for which stats are trainable.
	AttributeSet() *progression.AttributeSet
}

// trainingEntityAdapter bridges command.TrainingActor to
// progression.TrainingEntity. Methods are 1:1 — the adapter exists
// only so the command package doesn't have to import progression
// purely for the interface declaration on the actor side.
type trainingEntityAdapter struct {
	a TrainingActor
}

func (e trainingEntityAdapter) StatBlock() *progression.StatBlock { return e.a.StatBlock() }
func (e trainingEntityAdapter) TrainsAvailable() int              { return e.a.TrainsAvailable() }
func (e trainingEntityAdapter) SpendTrain() bool                  { return e.a.SpendTrain() }
func (e trainingEntityAdapter) RaceID() string                    { return e.a.RaceID() }
func (e trainingEntityAdapter) HasRoomTag(tag string) bool        { return e.a.HasRoomTag(tag) }
func (e trainingEntityAdapter) AttributeSet() *progression.AttributeSet {
	return e.a.AttributeSet()
}

// TrainHandler implements the M8.6 `train <stat>` verb (spec
// progression.md §7.4). Resolves through Context.Training and
// renders the structured result.
func TrainHandler(ctx context.Context, c *Context) error {
	if c.Training == nil {
		return c.Actor.Write(ctx, "Training is not enabled in this build.")
	}
	holder, ok := c.Actor.(TrainingActor)
	if !ok {
		return c.Actor.Write(ctx, "You cannot train.")
	}
	if len(c.Args) == 0 {
		// Helpful no-arg form: show pool + trainable list so the
		// player knows what they can spend on.
		msg := fmt.Sprintf("Trains available: %d.", holder.TrainsAvailable())
		return c.Actor.Write(ctx, msg+" Usage: train <stat>")
	}
	if len(c.Args) > 1 {
		return c.Actor.Write(ctx, "Usage: train <stat>")
	}
	res := c.Training.TryTrain(ctx, trainingEntityAdapter{a: holder}, c.Args[0])
	if res.Outcome == progression.TrainSuccess {
		// Manager.Message already names the stat ("You feel
		// stronger in str."); append only the new effective
		// value so the player sees the bump take effect.
		msg := fmt.Sprintf("%s (now %d)", res.Message, res.NewEffective)
		return c.Actor.Write(ctx, msg)
	}
	return c.Actor.Write(ctx, res.Message)
}

// PracticeHandler implements the M8.6 `practice <ability>` verb
// (spec progression.md §7.3). Resolves through Context.Training
// and renders the structured result. Until M9 proficiencies land,
// every practice attempt fails with PracticeNotLearned — exactly
// what the spec §7.3 NotLearned branch describes for an
// ungranted ability.
func PracticeHandler(ctx context.Context, c *Context) error {
	if c.Training == nil {
		return c.Actor.Write(ctx, "Training is not enabled in this build.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Practice what?")
	}
	if len(c.Args) > 1 {
		return c.Actor.Write(ctx, "Usage: practice <ability>")
	}
	holder, ok := c.Actor.(TrainingActor)
	if !ok {
		return c.Actor.Write(ctx, "You cannot practice.")
	}
	entityID := c.Actor.PlayerID()
	if entityID == "" {
		entityID = c.Actor.ID()
	}
	res := c.Training.TryPractice(ctx, trainingEntityAdapter{a: holder}, entityID, c.Args[0])
	return c.Actor.Write(ctx, res.Message)
}
