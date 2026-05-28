package quest

// Reward dispatch (§5). The dispatcher never touches the XP, currency,
// ability, or item subsystems directly — it calls four small interfaces,
// each with a no-op default, so the quest feature ships without forcing
// those systems to exist. Class/race unlocks are setters on the Player.

// DefaultTrack is the progression track quest XP is granted on (§5.2)
// when the service is not configured otherwise.
const DefaultTrack = "main"

// ExperienceGranter grants XP on a progression track (§5.3).
type ExperienceGranter interface {
	GrantExperience(entityID string, amount int64, track, source string)
}

// GoldGranter adds currency (§5.3).
type GoldGranter interface {
	AddGold(entityID string, delta int, reason string)
}

// AbilityTeacher teaches an ability at an initial proficiency (§5.3).
type AbilityTeacher interface {
	Learn(entityID, abilityID string, initialProficiency int)
}

// ItemGranter creates an item from a template and gives it to the
// player; missing templates are skipped silently (§5.2 step 6 / §5.3).
type ItemGranter interface {
	GrantItem(entityID, templateID string, silent bool)
}

type nopExperience struct{}

func (nopExperience) GrantExperience(string, int64, string, string) {}

type nopGold struct{}

func (nopGold) AddGold(string, int, string) {}

type nopAbility struct{}

func (nopAbility) Learn(string, string, int) {}

type nopItem struct{}

func (nopItem) GrantItem(string, string, bool) {}

// Dispatcher applies a Reward to a player through the four services in
// the documented order (§5.2), each step independent. A zero-value
// Dispatcher (via NewDispatcher with no options) uses no-op services, so
// dispatch succeeds silently with no effect.
type Dispatcher struct {
	xp      ExperienceGranter
	gold    GoldGranter
	ability AbilityTeacher
	item    ItemGranter
	track   string
}

// DispatcherOption configures a Dispatcher.
type DispatcherOption func(*Dispatcher)

// WithExperience sets the XP granter.
func WithExperience(x ExperienceGranter) DispatcherOption {
	return func(d *Dispatcher) {
		if x != nil {
			d.xp = x
		}
	}
}

// WithGold sets the currency granter.
func WithGold(g GoldGranter) DispatcherOption {
	return func(d *Dispatcher) {
		if g != nil {
			d.gold = g
		}
	}
}

// WithAbilities sets the ability teacher.
func WithAbilities(a AbilityTeacher) DispatcherOption {
	return func(d *Dispatcher) {
		if a != nil {
			d.ability = a
		}
	}
}

// WithItems sets the item granter.
func WithItems(i ItemGranter) DispatcherOption {
	return func(d *Dispatcher) {
		if i != nil {
			d.item = i
		}
	}
}

// WithTrack overrides the XP track (default DefaultTrack).
func WithTrack(track string) DispatcherOption {
	return func(d *Dispatcher) {
		if track != "" {
			d.track = track
		}
	}
}

// NewDispatcher builds a dispatcher; unset services default to no-ops.
func NewDispatcher(opts ...DispatcherOption) *Dispatcher {
	d := &Dispatcher{
		xp:      nopExperience{},
		gold:    nopGold{},
		ability: nopAbility{},
		item:    nopItem{},
		track:   DefaultTrack,
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Dispatch applies r to player in the §5.2 order: XP, gold, abilities,
// class unlock, race unlock, items. Each step is independent and a
// no-op when its field is zero/empty.
func (d *Dispatcher) Dispatch(player Player, r Reward) {
	id := player.EntityID()
	if r.XP > 0 {
		d.xp.GrantExperience(id, r.XP, d.track, "quest")
	}
	if r.Gold > 0 {
		d.gold.AddGold(id, r.Gold, "quest reward")
	}
	for _, abilityID := range r.Abilities {
		d.ability.Learn(id, abilityID, 1)
	}
	if r.ClassUnlock != "" {
		player.SetClass(r.ClassUnlock)
	}
	if r.RaceUnlock != "" {
		player.SetRace(r.RaceUnlock)
	}
	for _, templateID := range r.Items {
		d.item.GrantItem(id, templateID, true)
	}
}
