# Proposal: Gender as content — core-pack YAML, theme-style overridable

**Status:** design-first; **decisions locked 2026-06-14** (D1–D6 below), no code yet
· **Type:** content-model + engine-debt slice (moves a hardcoded vocabulary into a
content registry; first per-character pronoun support)
**Builds on:** WoT S2 Phase 3b — gender was added as a character attribute
(`Save.Gender`, save **v22**) but the option set is a hardcoded literal
(`internal/session/creation_flow.go` `genderOptions`) and pronouns are still the
fixed `emote.DefaultPronouns` (they/them) for every actor.
**Follows the pattern of:** the theme + channel-map registries
(`internal/render/theme.go`, `content/core/theme.yaml`) — a small **global,
non-namespaced, later-wins** content vocabulary, NOT the namespaced/priority race
registry.
**Constraints honored:** compatible with the existing pack loader + registries; no
hardcoded gender strings in render-facing engine code; **no save migration** while
the default id set stays `male`/`female`.

## 1. What this is and why

Gender today is a hardcoded `[]wizard.Option{Male, Female}` in the session layer,
and `internal/emote/pronouns.go` ships a single `DefaultPronouns` ("they/them")
used for *every* actor — its own doc says "per-entity overrides land when
character-creation pronouns ship." This slice does both: it moves the gender
**vocabulary** (options, labels, pronouns) into content YAML a pack can extend or
override, and it makes pronoun substitution **per-character** by resolving an
actor's pronouns from its gender.

Gender stays **mechanics-free in the engine**. The only mechanical consumer — the
WoT saidin/saidar affinity — already derives from the gender *id* in the setting
layer (`cmd/anothermud/affinity.go` reads `"male"`/`"female"`); it does not read a
property off the gender definition. A gender definition is identity + presentation.

## 2. What belongs in a gender definition (D-fields)

| Field | Purpose |
|---|---|
| `id` (map key) | stable, lowercase, **global** — what `Save.Gender` and `AllowedGenders` reference |
| `label` | player-facing menu/display text ("Male") |
| `order` | intentional menu ordering (Male, Female, Neutral — not alphabetical) |
| `pronouns` | the four-form set: `subject` / `object` / `possessive` / `reflexive` |
| `enabled` (optional, default `true`) | suppression toggle for overrides (see §4 D-override) |

No `mechanical effects` field (D6: no `tags` either, deferred). Pronoun fields
default to the gender-neutral they/them set when omitted, so a minimal entry is
just `label` + `order`.

## 3. Where it lives + override semantics

**Core-pack YAML** (`content/core/genders.yaml`), loaded into a new global
**`GenderRegistry`** — not engine-hardcoded (the status quo this replaces) and not
per-pack-only (which would duplicate the vocabulary in every pack).

**Override = per-id later-wins**, exactly like `ThemeRegistry.Register`:
- **Add** — a pack declares a new id → appended.
- **Override** — a pack re-declares an existing id → replaced (later pack in
  dependency order wins).
- **Suppress** — an `enabled: false` override entry hides a core gender (the wizard
  offers only enabled entries). Mirrors the rarity `visible` flag and the
  registry convention of *never deleting* — you override-to-disable.
- **Not** whole-list-replace (too blunt; a pack adding one option would have to
  restate the defaults and drift when core changes).

Gender ids are **global, not namespaced** (like theme tags / channel names, and
unlike rooms/areas), because `AllowedGenders` on race/class/background already
references them as bare global strings.

## 4. Decisions (locked 2026-06-14)

- **D1 — Core ships `male` / `female` / `neutral`.** `male` → he/him/his/himself,
  `female` → she/her/her/herself, `neutral` → they/them/their/themselves. Ids
  `male`/`female` are unchanged from v22, so **no save migration**. *Accepted
  output change:* existing male/female characters now render **gendered** pronouns
  (he/she) in emotes instead of the current they/them — no save change, but visible.
- **D2 — WoT forces binary.** `content/wot/genders.yaml` ships
  `neutral: { enabled: false }`, suppressing the non-binary option to keep the
  saidin/saidar affinity gate clean. (`male`/`female` are inherited from core; the
  WoT file only carries the suppression override.)
- **D3 — One slice, pronoun threading included.** Registry + YAML + wizard-built-
  from-registry + `connActor.Pronouns()` + the emote call sites all land together,
  closing the `pronouns.go` they/them TODO. (Per-step commits + go-review.)
- **D4 — Mobs get gender now.** An optional `gender:` id on `MobFile` resolves mob
  pronouns; absent → they/them. NPCs read correctly in emotes/dialogue from day one.
- **D5 — New `internal/gender` leaf package** owns `Gender` + `GenderRegistry` +
  `PronounSet`; `internal/emote` consumes the `PronounSet` type from it (keeps the
  dependency acyclic and groups gender concerns). Minor churn to emote's existing
  `PronounSet` references.
- **D6 — Defer `tags`.** No setting-semantics `tags` field in the v1 schema (YAGNI;
  the engine has no consumer, and adding a content field later is cheap). The WoT
  affinity keeps deriving from the gender id in `cmd/anothermud`, not from gender data.

## 5. The schema (concrete)

`content/core/genders.yaml`:

```yaml
# Global gender vocabulary. Ids are NOT namespaced; a later pack overrides an
# entry by re-declaring its id (later-wins), mirroring theme tags. Pronoun fields
# omitted default to the gender-neutral they/them set.
genders:
  male:
    label: Male
    order: 1
    pronouns: { subject: he,   object: him,  possessive: his,   reflexive: himself }
  female:
    label: Female
    order: 2
    pronouns: { subject: she,  object: her,  possessive: her,   reflexive: herself }
  neutral:
    label: Neutral
    order: 3
    pronouns: { subject: they, object: them, possessive: their, reflexive: themselves }
```

`content/wot/genders.yaml` (suppress the non-binary option — D2):

```yaml
genders:
  neutral:
    enabled: false
```

YAML decode shapes (mirroring `ThemeFile`):

```go
// GendersFile is the YAML shape for a pack's gender vocabulary.
type GendersFile struct {
    Genders map[string]GenderEntryFile `yaml:"genders"`
}

type GenderEntryFile struct {
    Label    string          `yaml:"label,omitempty"`
    Order    int             `yaml:"order,omitempty"`
    Enabled  *bool           `yaml:"enabled,omitempty"` // pointer: absent ⇒ true
    Pronouns *PronounSetFile `yaml:"pronouns,omitempty"`
}

type PronounSetFile struct {
    Subject    string `yaml:"subject,omitempty"`
    Object     string `yaml:"object,omitempty"`
    Possessive string `yaml:"possessive,omitempty"`
    Reflexive  string `yaml:"reflexive,omitempty"`
}
```

`internal/gender` (leaf — D5):

```go
type PronounSet struct{ Subject, Object, Possessive, Reflexive string }

// DefaultPronouns is they/them — the fallback for an unset/unknown gender.
var DefaultPronouns = PronounSet{"they", "them", "their", "themselves"}

type Gender struct {
    ID       string
    Label    string
    Order    int
    Enabled  bool
    Pronouns PronounSet
    Pack     string // diagnostic, mirrors Race.Pack
}

type GenderRegistry struct { /* map[string]*Gender, later-wins per id */ }
func (r *GenderRegistry) Register(g *Gender)          // later-wins
func (r *GenderRegistry) Get(id string) (*Gender, bool)
func (r *GenderRegistry) Enabled() []*Gender          // enabled, order-sorted — wizard menu
func (r *GenderRegistry) Pronouns(id string) PronounSet // Get(id).Pronouns or DefaultPronouns
```

## 6. Pronoun resolution — the engine seam (Q4)

`emote.Substitute` already takes a `PronounSet` and stays generic. What's missing
is resolving a *character's* set from its gender:

- `connActor.Pronouns()` → `gender.GenderRegistry.Pronouns(a.save.Gender)`, which
  **falls back to `DefaultPronouns`** (they/them) when gender is unset (pre-v22) or
  unknown. → no migration; existing behavior preserved for the gender-less.
- The three `emote.Subject{… Pronouns: emote.DefaultPronouns}` sites in
  `internal/command/emote.go` become `Pronouns: actor.Pronouns()` /
  `target.Pronouns()`.
- Mobs (D4): `MobInstance` resolves pronouns from its template `gender:` id via the
  same registry; absent → they/them.
- `PronounSet` moves to `internal/gender`; `internal/emote` consumes it (D5).

## 7. Patterns followed (for consistency)

| Concern | Existing pattern copied |
|---|---|
| Content shape (single file, `key: {…}` map) | `content/core/theme.yaml` (`ThemeFile`) |
| Registry + override | `render.ThemeRegistry` / `ChannelMap` — global, later-wins |
| Manifest glob | add `genders: [genders.yaml]` to `ContentPaths`, like `theme:` |
| Loader wiring | `decodeTheme` → `dst.Theme.Register`; add `decodeGenders` → `dst.Genders.Register` |
| Wizard options from a registry | `raceOptions` / `classOptions` build menus from registries — `genderOptions` becomes the same |
| Suppression flag | rarity `visible` |
| **Not** this | race/class **namespaced ids + priority** — wrong shape for a small global vocabulary |

## 8. Tradeoffs (recommended vs alternatives)

- **vs. keep it hardcoded** (status quo) — recommended adds a registry + loader +
  one YAML file (~the theme footprint). Small, and the only option that satisfies
  the no-hardcoded-strings constraint and lets a setting re-theme gender without code.
- **vs. namespaced + priority (race pattern)** — heavier and wrong-shaped: ids are
  global (AllowedGenders treats them so) and priority override is overkill for a
  ~3-entry vocabulary.
- **vs. per-pack-only (no core default)** — forces duplication in every pack;
  rejected.
- **Suppression via `enabled` flag vs a manifest allowlist** — the flag keeps all
  override logic in the registry and mirrors rarity `visible`; an allowlist splits
  gender config across two files.

## 9. Build order (one arc, per-step commits + go-review)

1. **`internal/gender` leaf package** — `Gender`, `GenderRegistry` (later-wins),
   `PronounSet` + `DefaultPronouns`; unit tests (register/override/suppress/
   order-sort/pronoun-fallback).
2. **Loader + manifest + registries wiring** — `GendersFile`/`decodeGenders`,
   `ContentPaths.Genders` glob, `Registries.Genders`, register into `dst.Genders`
   (global, later-wins). Loader test.
3. **Content** — `content/core/genders.yaml` (3 entries) +
   `content/wot/genders.yaml` (suppress `neutral`).
4. **Wizard from registry** — `creation_flow.go` builds the gender step from
   `Genders.Enabled()` (order-sorted), deleting the hardcoded `genderOptions`.
   Update the creation-flow tests' option-index expectations.
5. **Mob gender (D4)** — optional `gender:` on `MobFile` → `MobInstance` pronoun
   resolution.
6. **Pronoun threading** — move `PronounSet` to `internal/gender`,
   `connActor.Pronouns()`, swap the emote call sites; live-verify an emote renders
   "he/she/they" by gender.

## 10. Open questions

All six design forks are resolved (D1–D6). Remaining items to confirm *during
implementation* (not blockers):

- **AllowedGenders validation.** Should the loader warn when a race/class/background
  `AllowedGenders` entry names a gender id not in the registry (a likely typo), or
  stay silent like the current fail-soft race/class id handling? Lean: a load-time
  warn (not fatal), consistent with other "unknown id tolerated but logged" sites.
- **Wizard gender-gating.** The wizard still ignores `AllowedGenders` (the
  pre-existing dynamic-eligibility gap). Out of scope here; this slice only makes
  the *option list* content-driven, not the per-race/class gating.
- **Capitalization of pronouns mid-sentence.** The emote token grammar (`$s`, `$M`)
  is lowercase-only today. Sentence-initial capitalization is a pre-existing emote
  concern, unchanged by this slice.
