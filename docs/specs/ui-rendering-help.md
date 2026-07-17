# UI, Rendering, and Help — Feature Specification

**Status:** Draft · **Scope:** The color-tag rendering pipeline
(semantic tags, literal color tags, brace shorthand), the theme
registry that resolves tags to ANSI sequences, the IConnection
decorator that applies the renderer transparently, the per-
player prompt renderer, the panel-rendering primitive used by
wizards and structured displays, and the help-topic registry and
its query / list / disambiguation surface · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the UI feature must do, not *how*
to implement it. The specific color names, theme entries, tag
vocabulary, and help-topic schemas are policy and live in
content.

---

## 1. Overview

Tapestry's UI surface is text. The engine assembles strings with
embedded color tags; the rendering layer translates those tags
into ANSI escape sequences (for terminal clients) or strips them
to plain text (for clients without color support). On top of
this, a set of structured-rendering helpers — prompts, panels,
help topics — produce richer layouts while remaining string
output.

The pipeline:

```
feature → string with tags → ColorRenderingConnection
                                  │
                            chooses by SupportsAnsi
                                  ↓
                  ColorRenderer.RenderAnsi (with ThemeRegistry)
                  ColorRenderer.RenderPlain (strip)
                                  ↓
                          send over IConnection
```

### Core concepts

- **Color tag** — a markup construct in engine-emitted text
  that names a presentation intent (`<highlight>`, `<danger>`,
  `<item.rare>`). The renderer translates tags to ANSI based
  on the active theme. Tags can be **semantic** (named in the
  theme), **literal** (`<color fg="..." bg="...">`), or
  **brace shorthand** (`{yellow}`).
- **Theme registry** — the content-registered map from tag
  name to `{fg, bg, html}` triple. Compiled at startup into a
  fast lookup of ANSI open/close pairs.
- **ColorRenderingConnection** — an IConnection decorator that
  intercepts every send, runs it through the renderer, and
  passes the result on. Wraps the network-layer connection
  before the session sees it.
- **PromptRenderer** — a small template engine that produces a
  one-line prompt from a per-player template using `{token}`
  substitutions for vital pools.
- **Panel** — a structured `Sections of Rows of Cells` value
  that the panel renderer turns into a multi-line boxed
  string with width-aware alignment and wrapping. Used by
  wizards and tabular displays.
- **Help topic** — a content-defined `(id, title, category,
  brief, body, syntax, keywords, see-also, role)` record
  loaded from per-pack YAML and queried by the `help` command.

### Goals

1. Let features emit text with presentation intent without
   knowing whether the recipient supports color.
2. Let content control the theme without engine changes —
   themes register tag mappings via YAML; the renderer
   resolves at send time.
3. Cache the rewrite of common strings so the same prompt or
   line doesn't re-parse on every send.
4. Provide a single panel primitive other features build on
   (wizard screens, inventory tables, score panels, help
   topics).
5. Provide a help registry that loads content topics, exposes
   query / list / disambiguation, and gates per-role
   visibility.

### Non-goals

- Real-time UI (cursor positioning, animations, sprites).
  Output is line-oriented.
- HTML rendering for web clients. The theme registry exposes
  an HTML map for GMCP clients to use locally, but the
  server itself emits ANSI / plain text.
- Color profile detection beyond the negotiation-time
  capability flag (see `docs/specs/networking-protocols.md`
  §7). Once a session's `SupportsAnsi` is set, it stays set.
- Internationalization. Text is whatever the feature emitted.
- A user-configurable theme per session. Themes are global —
  changed by pack reload, not per-player.

---

## 2. Color tags

### 2.1 Tag forms

Three forms are recognized. All are case-insensitive on the
tag / color name.

**Semantic tags** name a registered theme entry:

```
<highlight>important</highlight>
<item.rare>The Sword of Glimmer</item.rare>
```

Tag content is whatever the theme defines (foreground color,
background color, possibly bold). Opening and closing tags
must match by name; nested tags are NOT explicitly stacked —
the renderer emits the inner content, then a single ANSI
reset on close (§2.4).

**Literal color tags** specify colors inline without naming a
theme entry:

```
<color fg="red">danger</color>
<color fg="yellow" bg="black">warning</color>
```

The `color` tag accepts `fg` and `bg` attributes whose values
are color names (§2.3). Either or both may be present. The
renderer ignores unrecognized attributes.

**Brace shorthand** is a compact alternative for ad-hoc color
that does not require closing:

```
{yellow}Important.{/} Continue.
```

Brace tokens are color names, plus the special `bold`, `dim`,
`reset`, and `/` (synonym for `reset`). They emit the ANSI
code immediately and do NOT auto-close — content must close
explicitly with `{/}` or `{reset}`.

### 2.2 Mixing

The three forms compose freely within a single string. The
renderer walks the input once and emits in input order. There
is no precedence: each form contributes its escape sequence
at the position where it appears.

### 2.3 Color names

The recognized color names are the 8 standard colors plus
their bright variants:

```
black red green yellow blue magenta cyan white
bright-black (dark-gray alias)
bright-red bright-green bright-yellow bright-blue
bright-magenta bright-cyan bright-white
```

The literal-tag form uses the hyphenated variant
(`bright-red`); the brace-shorthand form uses the
underscored variant (`bright_red`). This split is historical
and is flagged as an open question.

`bold`, `dim`, and `reset` are valid as brace shorthand but
not as literal-tag colors.

### 2.4 Closing semantics

Both `<tag>…</tag>` and `<color …>…</color>` close by
emitting an ANSI reset (`ESC[0m`). This means a tag nested
inside another tag will reset the outer color when it
closes, which can be surprising:

```
<a>outer <b>inner</b> still expected outer</a>
                                ^
                          but is uncolored after this
```

Content authors should avoid nesting semantic tags, OR
explicitly re-open the outer tag after the inner closes. The
brace form does not auto-reset and is safer for nested
structure:

```
{red}outer {yellow}middle{red} back to red{/}
```

### 2.5 Unknown tags pass through

The renderer recognizes a tag only when its name is in the
theme registry. Unknown opening tags are passed through as
literal characters (the `<` is emitted as-is, the rest of the
input continues to be scanned).

Closing tags follow a stricter rule: the renderer skips
closing tags for known theme tags and for `color`. Unknown
closing tags pass through. This way, a `<sometag>` typo emits
the visible characters `<sometag>` (and its matching close)
rather than mysteriously vanishing.

**Acceptance criteria**

- [ ] Semantic, literal, and brace forms are all recognized.
- [ ] Case-insensitive matching for tag and color names.
- [ ] Tag/color closes emit an ANSI reset.
- [ ] Brace shorthand emits its code without auto-close.
- [ ] Unknown opening tags pass through as literals.
- [ ] Known closing tags are consumed; unknown closing tags
      pass through.

---

## 3. Theme registry

### 3.1 Entries

A theme entry maps a tag name to:

- `fg` — a foreground color name (see §2.3) or null.
- `bg` — a background color name or null.
- `html` — an HTML color string (e.g. `#FF6633`) or null.
  Exposed via `GetHtmlMap()` for GMCP clients that want to
  render with native CSS rather than ANSI translation.

A theme is registered through pack content (typically a
`theme.yaml` in the pack's strings directory). Pack authors
declare entries; the theme registry stores them.

### 3.2 Compilation

After all packs load, the theme registry's `Compile()` step
builds a fast lookup of `AnsiPair(open, close)` for every
entry that has a non-empty open sequence (i.e. an `fg` or
`bg` that resolves). Entries with only `html` (no `fg`/`bg`)
produce no AnsiPair and the renderer treats them as no-op
for terminal output — the inner content is emitted plain.

The compiled map is read-only thereafter. Adding a tag at
runtime requires another compile.

### 3.3 Lookups

- `IsKnown(tag)` — used by the renderer's allowlist check.
  Returns true even for entries with no ANSI mapping, so
  unknown-tag passthrough works on declared-but-color-less
  tags.
- `Resolve(tag)` — returns the AnsiPair or null. Null
  signals "declared but no color" and the renderer emits the
  inner content plain.
- `GetHtmlMap()` — snapshot of `{tag → html}` for theme
  entries declaring HTML colors. The GMCP layer uses this to
  publish a stylesheet-like map to clients.

### 3.4 Literal-color resolver

`ResolveFgColor(name)` / `ResolveBgColor(name)` are static
helpers that map a color name to its ANSI code without
consulting the theme. Used by the literal-tag form (§2.1)
since literal colors don't go through theme entries.

These are exposed as static so other features can also emit
"raw" color sequences without going through the renderer —
useful for cases where the renderer's caching would be
counterproductive (e.g. dynamic per-frame banners).

**Acceptance criteria**

- [ ] Compile is idempotent and produces an open/close pair
      only when fg or bg resolves.
- [ ] IsKnown returns true for declared-but-color-less
      entries.
- [ ] Resolve returns null for declared-but-color-less.
- [ ] Static color resolvers do not require Compile.

---

## 4. The color renderer

### 4.1 Two modes

The renderer offers two entry points:

- **RenderAnsi(s)** — emit ANSI escape sequences.
- **RenderPlain(s)** — strip all tags, brace shorthand, and
  emit just text.

Internally both walk the input character by character with
identical structural recognition; only the *emission* step
differs. This guarantees that what you see plain is exactly
what you see colored with the formatting removed.

### 4.2 Caching

Each entry point has a `ConcurrentDictionary<string, string>`
cache keyed by input string. A render-of-the-same-input twice
goes through the parser once. The cache grows monotonically
across the process lifetime; there is no eviction.

Cache identity is exact input. A line that differs only in
whitespace from a previously-rendered line is a new entry.

### 4.3 Scanning order

The renderer scans left-to-right and handles, in order:

1. `<tag>` (opening tag) — semantic or literal.
2. `<color …>` — literal color (recognized before semantic
   because `color` matches a different shape).
3. `</tag>` (closing tag) — consumed when known.
4. `{name}` — brace shorthand.
5. Anything else — emitted as a literal character.

When the renderer encounters an unmatched `<`, brace, or
malformed sequence, the offending character is emitted as a
literal and scanning continues at the next position.

### 4.4 Literal-color parsing

`<color fg="X" bg="Y">…</color>`:

1. Find the closing `</color>`. If missing, treat the `<` as
   a literal and continue scanning.
2. Parse the `fg=` and `bg=` attribute values using simple
   quote-delimited matching (regex-free, single-character
   quotes only).
3. In ANSI mode, emit the resolved fg + bg codes, then the
   inner text, then a reset.
4. In plain mode, emit only the inner text.

### 4.5 Strict vs lenient

The renderer is lenient: malformed input passes through as
literal text rather than raising. A pack that writes
`<highlight>missing close` emits the literal characters and
the rest of the line is unstyled. This keeps content typos
visible to authors instead of swallowing text.

**Acceptance criteria**

- [ ] Plain and ANSI modes recognize the same constructs.
- [ ] Cached lookups never re-parse the same input.
- [ ] Unmatched `<`, `{`, or `</…>` pass through as literal.
- [ ] Literal-color attributes accept `fg`, `bg`, or both.

---

## 5. The color-rendering connection

The renderer is exposed as an **IConnection decorator**:
`ColorRenderingConnection` wraps a transport-level
connection. The decorator intercepts `SendText` and
`SendLine`:

1. If the inner connection's `SupportsAnsi` is true, call
   `RenderAnsi`.
2. Otherwise call `RenderPlain`.
3. Forward the rendered string to the inner connection.

Every other IConnection member (id, IsConnected,
SupportsAnsi, RemoteAddress, ClearScreen, Disconnect,
echo control, all events) passes through unchanged.

The session manager constructs the decorator once per
connection at session bind time; features sending output via
the session never see the underlying transport directly,
which means features can emit color tags without per-call
"does this client support color" checks.

**Acceptance criteria**

- [ ] Decorator chooses Ansi vs Plain based on `SupportsAnsi`
      at send time, not at construction time.
- [ ] All non-send members pass through identically.
- [ ] Event subscriptions are forwarded (no shadowing).

---

## 6. Tag stripping helpers

A small utility, `TagStripper`, exposes two static methods
used by features that need to reason about *visible* length
without rendering:

- **`StripTags(s)`** — drops `<…>` markup, returning the
  visible text. Used by the panel renderer to compute
  column widths.
- **`VisibleLength(s)`** — returns `StripTags(s).Length`,
  with a fast path for strings containing no `<`.

The stripper does NOT understand brace shorthand or
literal-color attributes — it works only on angle-bracket
constructs. This is sufficient because panel content is
emitted with semantic tags only; brace shorthand is reserved
for ad-hoc messages.

A `<` without a matching `>` is treated as opening an
indefinite tag — everything from `<` to end of string is
dropped. This is conservative; it errs toward shorter
visible length over emitting raw `<` characters.

**Acceptance criteria**

- [ ] `<tag>…</tag>` produces just `…`.
- [ ] `<color fg="x">…</color>` produces just `…`.
- [ ] A `<` with no closing `>` consumes the rest of the
      input.
- [ ] `VisibleLength` matches `StripTags(s).Length` for
      every input.

---

## 7. Prompt rendering

### 7.1 Template

Each player has an optional `prompt_template` property — a
string with `{token}` placeholders. When unset, a default
template is used:

```
<hp>[HP {hp}/{maxhp}]</hp> <stun>[ST ...]</stun> <mana>[MA ...]</mana> <mv>[MV ...]</mv>>
```

The default is **adaptive**: a segment for an optional resource
pool renders only when the character actually has that pool
(its max > 0). HP and movement always show; the Stun monitor
and the mana/resource pool show only for a character that has
one, so a mana-less archetype is not shown a dead `[MA 0/0]`.
A player-set custom template is not adaptive by default —
it renders exactly as typed — but §7.5 conditional segments
let a custom template opt into the same pool-aware behavior.

The template itself uses semantic color tags; the renderer
processes them later as part of the normal send pipeline.

### 7.2 Tokens

The prompt renderer substitutes a fixed set of tokens
(case-insensitive):

| Token | Value |
|---|---|
| `{hp}` | current HP |
| `{maxhp}` | maximum HP |
| `{stun}` | current Stun-monitor pool |
| `{maxstun}` | maximum Stun-monitor pool |
| `{mana}` | current resource pool |
| `{maxmana}` | maximum resource pool |
| `{mv}` | current movement pool |
| `{maxmv}` | maximum movement pool |
| `{gold}` | current gold |

Unrecognized tokens resolve to empty string, leaving a gap
in the prompt. This is intentional — it keeps a
typo-tolerant rendering path so a broken template doesn't
silently delete every prompt.

### 7.3 Flush

The session manager's `FlushPrompts` (see `docs/specs/
session-lifecycle.md` §3.5) calls the prompt renderer for
sessions whose `NeedsPromptRefresh` flag is set. The
rendered prompt is sent with a leading CR-LF so it sits on
its own line at the bottom of the terminal.

Prompts do NOT auto-render on player input — they render
*after* the next content arrives, so the player's input
echoes immediately above their fresh prompt.

**Acceptance criteria**

- [ ] The default template is used when the player has no
      `prompt_template` property.
- [ ] All listed tokens are substituted correctly.
- [ ] Unknown tokens produce empty strings, not literal
      `{x}`.
- [ ] Tokens are matched case-insensitively.

### 7.4 Setting the prompt

The `prompt` verb lets a player inspect and change their own
`prompt_template` (§7.1). It is the only writer of that property.

- `prompt` (no argument) — show the player their current template so
  they can see and edit it. When the player has no template set, show
  the default (§7.1) and identify it as the default.
- `prompt <template>` — set `prompt_template` to the rest of the input
  line verbatim (internal spacing and color tags preserved). The new
  prompt takes effect on the next prompt flush (§7.3).
- `prompt default` (alias `prompt reset`) — clear `prompt_template`,
  reverting to the default. `default` / `reset` are reserved words; a
  player cannot set a literal one-word template equal to a reserved
  word. This is an accepted limitation — multi-word templates and any
  template containing a token or tag are unaffected.

The template is stored on the player and persists across logout (the
existing `prompt_template` property, §7.1). Setting or clearing it
marks the save dirty and flags a prompt refresh so the change is
visible immediately.

The verb does not validate template content: the renderer is
typo-tolerant (unknown tokens resolve to empty per §7.2; unknown tags
pass through per §2.5). A template longer than the configured maximum
(*Max prompt template length*) is rejected with a message and leaves
the stored template unchanged, to bound abuse.

**Acceptance criteria**

- [ ] `prompt` with no template set shows the default and identifies
      it as the default.
- [ ] `prompt` with a template set shows that template.
- [ ] `prompt <template>` stores the rest-of-line verbatim; the next
      flush renders it.
- [ ] `prompt default` and `prompt reset` clear the template; the next
      flush renders the default.
- [ ] A template over the configured maximum length is rejected and the
      stored template is unchanged.
- [ ] Setting or clearing the template persists across logout.

### 7.5 Conditional segments

A template may wrap part of its content in a conditional segment so that
a hand-written custom template adapts to the character the way the
default (§7.1) does. The form is a paired marker:

```
{?name} …body… {/name}
```

The body renders only when the character **has the named pool** — its
maximum is greater than zero. `name` is one of the pool tokens (§7.2):
`hp`, `stun`, `mana`, `mv`, or `gold` (`gold` keys on the current gold
being greater than zero). For example, a custom template can show the
Stun monitor only for a character that has one:

```
<hp>[HP {hp}/{maxhp}]</hp> {?stun}<stun>[ST {stun}/{maxstun}]</stun> {/stun}<mv>[MV {mv}/{maxmv}]</mv>>
```

Rules:

- The body is rendered normally — tokens (§7.2), color tags (§2), and
  nested conditional segments of a *different* name all resolve inside
  it. Nesting two segments of the *same* name is not supported (the
  first matching `{/name}` closes the outer one).
- An **unknown** condition name (not a pool the character can have)
  hides its body. This is deliberate: a conditional exists to suppress
  a segment for an absent pool, so an unrecognized pool name reads as
  "absent". (This differs from an unknown *token*, which resolves to an
  empty gap per §7.2 — a token is expected to produce text, a
  conditional is expected to gate it.)
- Matching is case-insensitive, like tokens.
- Malformed markers are tolerated, never fatal: a `{?name}` with no
  matching `{/name}` drops the open marker and renders the rest; a
  `{/name}` with no open is dropped.

**Acceptance criteria**

- [ ] `{?name}…{/name}` renders its body when the named pool's max > 0.
- [ ] The body is omitted when the named pool is absent (max 0).
- [ ] Tokens, color tags, and different-name nested segments render
      correctly inside a shown body.
- [ ] An unknown condition name hides its body.
- [ ] A `{?name}` with no close, or a stray `{/name}`, does not corrupt
      the surrounding prompt.
- [ ] Condition names are matched case-insensitively.

---

## 8. Panel rendering

### 8.1 Panel model

A `Panel` is a value with:

- A `Width` in visible characters (default 80).
- An ordered list of `Section`s.

A `Section` has an ordered list of `Row`s and a
`SeparatorAbove` rule style (None / Minor / Major) used
between sections.

A `Row` is one of:

- **EmptyRow** — a blank line within the panel frame.
- **TitleRow** — `(left, right?)` with the left rendered as a
  title tag and the right (optional) as a subtle tag,
  padded to the panel width.
- **TextRow** — `(content, align, wrap)` — single string
  rendered with optional alignment and wrap.
- **CellRow** — `(cells, showDividers)` — a horizontal row
  of cells.
- **FooterRow** — a single-content row rendered as a footer
  (typically subtle styling).

A `Cell` has:

- `Content` — the rendered text (with embedded tags).
- `Width` — either `Fixed(n)` or `Fill` (consumes remaining
  width).
- `Align` — Left / Right / Center.
- `Wrap` — whether long content wraps to a second line.

A specialized `ProgressCell` is a Cell with `Value` and
`Max` integers, used by progress-bar renderers (the panel
renderer builds the bar from the cell width and the value
ratio — exact rendering is a renderer detail).

### 8.2 Output shape

The renderer emits a multi-line string with `\r\n` line
endings. Lines are bracketed by a vertical "frame" tag
(`<frame>|</frame>`) on each side, padded so every row is
the same visible width regardless of how many tags it
contains.

Horizontal rules between sections use:

- **Major** (`<frame>|===...|</frame>`) — strong separator.
- **Minor** (`<frame>|---...|</frame>`) — light separator.
- **None** — no rule emitted.

The first and last rule of every panel is Major regardless
of section configuration.

### 8.3 Width-aware padding

All width math uses `TagStripper.VisibleLength`. A cell with
embedded color tags ends up padded by the *visible* length,
so a colored cell aligns with a plain cell next to it.

A title row whose left content plus right content exceeds
the inner width truncates the left side, appending an
ellipsis when there's room for one. (The renderer raises if
the right side alone exceeds the inner width — that's a
content authoring error.)

### 8.4 Used by

- Character creation wizards (§5 of `docs/specs/character-
  creation.md`).
- Inventory and score display (see `docs/specs/inventory-
  equipment-items.md`).
- Help disambiguation (§10 of this spec).
- Admin reports.

The renderer itself doesn't know about its callers; it just
takes a Panel and returns a string.

**Acceptance criteria**

- [ ] All output lines are the same visible width.
- [ ] Width math uses visible length, not raw length.
- [ ] Sections emit their declared separator above; the
      first separator is suppressed.
- [ ] Major rules appear at panel top and bottom regardless
      of section config.
- [ ] Title-row right side exceeding inner width raises;
      combined exceeding triggers truncation + ellipsis on
      the left.

---

## 9. Help service

### 9.1 Topic shape

A help topic is loaded from per-pack YAML files (typically
under `<pack>/help/*.yaml`). A topic carries:

- A stable **id** (required) — bare or namespaced; the
  service stores both forms (§9.3).
- A **title** (required) — human-readable name.
- A **category** — used by category listings.
- A **brief** — one-line summary.
- A **body** — multi-line free-form text. May contain color
  tags.
- A **syntax** list — example command syntaxes shown in a
  dedicated section.
- A **keywords** list — used by fuzzy match.
- A **see-also** list — referenced topic ids.
- A **role** — optional visibility gate (§9.5).
- A computed **pack name** (set by the loader) and
  **namespaced id** (`pack:id`).

Topics missing `id` or `title` are skipped with a warning at
load time.

### 9.2 Per-pack loading

The pack loader calls `LoadPack(packName, packRoot, helpGlob,
loadOrder)`:

1. Skip if `helpGlob` is blank.
2. Resolve `<packRoot>/help`. Skip if the directory doesn't
   exist.
3. Walk every `*.yaml` recursively. Order is alphabetical
   for stability.
4. Deserialize each topic. Set its `PackName`. Add via
   `AddTopic(topic, loadOrder)`.

Additionally, the command-help generator (see `docs/specs/
commands-and-dispatch.md` §8) calls `AddTopic` with a
load-order of zero for every command that has arg
definitions. This is how typed commands get help even when
no pack ships a help file for them.

### 9.3 Indexing

Three indices:

- **By id** — maps both bare id and namespaced id to the
  topic. Both forms point at the same record.
- **By title** — maps title (case-insensitive) to the
  topic.
- **By category** — maps category name to a list of topics.

Each index entry also carries the load-order integer used to
break duplicate registrations (§9.4).

### 9.4 Load-order precedence

When two topics register the same id (or title), the higher
load-order wins. This lets a pack override an upstream
pack's help, and lets pack help override the auto-generated
command help (which loads at order 0).

Categories accumulate every loaded topic, deduplicating by
`(id, packName)` — so an override removes the previous
entry and inserts the new one.

### 9.5 Role tiers

Topics may declare a `role` value drawn from a small
hierarchy:

```
player < builder < admin
```

A topic with role `player` is visible to every player. A
topic with role `builder` is visible to builders and admins.
A topic with role `admin` is admin-only.

The role tier of the *requester* is determined at query time
from the entity id passed to the service. The default rule:

- A query with no entity id (e.g. pre-login) sees only
  role-less topics.
- A query with any entity id sees role-less and `player`
  topics. Builder / admin gating beyond that is not yet
  implemented — it's a placeholder.

Visibility filtering happens uniformly across query, list,
and categories.

### 9.6 Query

`Query(entityId, term)` resolves a search:

1. Determine the requester's tier.
2. **Exact id.** If `term` matches an indexed id (bare or
   namespaced) AND the topic is visible, return it with
   status `ok`.
3. **Exact title.** If `term` matches an indexed title AND
   the topic is visible, return it with status `ok`.
4. **Fuzzy.** Filter all topics (deduplicated by namespaced
   id) for those visible to the tier AND matching the term
   case-insensitively against either the title or any
   keyword.
   - If exactly one match → return with status `ok`.
   - If multiple matches → return with status `multiple`
     plus a list of summaries `(id, title, brief)`.
5. **No match** → status `no_match` with the original term
   echoed back.

The match precedence is exact-id > exact-title > fuzzy.
Within fuzzy, no further ranking is applied — multiple
matches are returned alphabetically by the consumer's
choice.

### 9.7 List and categories

- **List(entityId, category)** — every visible topic in
  the category as summaries.
- **Categories(entityId)** — every category name with at
  least one visible topic, sorted alphabetically.

Both pass through the role gate.

**Acceptance criteria**

- [ ] Loaded topics index by id, namespaced id, title, and
      category.
- [ ] Duplicate id / title / category-entry registrations
      resolve by load-order (higher wins; equal preserves
      newest).
- [ ] Query precedence is exact-id → exact-title → fuzzy.
- [ ] Role gate applies to query, list, and categories.
- [ ] Topics missing required fields are skipped at load
      with a warning.
- [ ] The command-help generator's order-zero topics are
      overridden by any pack-loaded help with positive
      load-order.

---

## 10. Help rendering

The help renderer is a static helper that builds three
canonical string outputs:

### 10.1 Topic render

`RenderTopic(topic, width = 78)`:

- A double-rule banner with the topic title centered.
- A subtle-tagged brief.
- A `Syntax:` section listing each syntax line indented.
- The body, split on newlines, each line indented.
- A `See also:` line listing referenced ids (when any).
- A closing rule.

The output carries color tags (`<subtle>`, etc.) intended
for the color renderer downstream.

### 10.2 Disambiguation

`RenderDisambiguation(term, matches, width = 78)`:

- A banner naming the queried term.
- "Multiple matches found:" line.
- One line per match — `id` padded to the longest id width
  plus 4 spaces, then the title.
- A footer hint "Type help [topic] for details."
- A closing rule.

### 10.3 No match

`RenderNoMatch(term)` returns a single line: `No help found
for '&lt;term&gt;'.`.

The help command handler chooses which renderer to call
based on the query's status field.

**Acceptance criteria**

- [ ] Topic render includes Syntax section iff the topic
      declares syntax.
- [ ] See-also section appears iff the topic declares
      references.
- [ ] Disambiguation rows align ids in a column.
- [ ] No-match terminates with CR-LF.

### 10.4 Command index (bare `help`)

`help` with no argument renders a **grouped command index**: every
category the requester can see, each shown as a header followed by a
compact grid of its command keywords. The categories render in a
**canonical order** (not alphabetically) so related groups sit
together — general first, then the movement / communication / combat /
items progression — with any category outside the canonical list
(pack-defined categories, or the default bucket that dynamically
registered ability verbs fall into) appended after, alphabetically, so
no command is ever hidden. A group the requester can't see (e.g. the
admin group at player tier) comes back empty from `List` and is
skipped.

The **category vocabulary and order** are policy. Command→category
assignment for engine builtins is engine-owned (it tracks the engine's
own verbs and lives beside their briefs); content packs remain free to
author topics in any category, including new ones. Per-command briefs
and full topics are one drill-down away via `help <category>` (§9.7)
and `help <command>` (§10.1) — the index itself lists keywords only, so
it stays scannable regardless of how many commands exist.

The same categorized catalog is also exposed to rich clients as a GMCP
push (a `Char.Commands` package: an ordered list of categories, each
with its commands' keyword / brief / primary-syntax), so a graphical or
web client can render clickable command menus without scraping `help`
output. The push is role-filtered identically to the index (the admin
group only reaches admins) and is shipped once per session, like other
static identity packages. The GMCP wire shape is a transport/feature
concern (see `docs/specs/networking-protocols.md`), not specified here.

**Acceptance criteria**

- [ ] Bare `help` groups commands under category headers in the
      canonical order, keywords only.
- [ ] Categories outside the canonical order render after it,
      alphabetically; none are dropped.
- [ ] A category with no requester-visible topics is omitted.

---

## 11. Look and the appearance lens

Examining the world splits into **two lenses** that answer different
questions about the same target:

- **`look <target>` — the appearance lens.** "What does this look
  like?" Works on anything visible: an item, a creature (mob), or
  another player. It renders *flavor*, never mechanics.
- **`consider <creature>` — the tactical lens.** "Can I take them
  on?" Creatures only. It renders a *qualitative impression* of the
  target's condition and relative strength, never raw mechanics.

The two never overlap in output: `look` shows no vitals or combat
numbers, and `consider` shows no prose. A player's own exact stats live
on the self sheet (`score`), not on either lens.

Verb dispatch and argument resolution (mob-by-keyword, player-by-name,
self-references) are owned by `commands-and-dispatch`; this section
specifies only what the two verbs *render*.

### 11.1 Description as content

Items and mobs carry an **optional authored description** — flavor prose
supplied by content alongside the name. It is snapshotted onto the live
instance at creation, the same way the display name is, so it is read
without consulting the originating template.

Authoring a description is never required. When a target has none, the
appearance lens renders a **generic fallback** that still names the
target (a "nothing special about &lt;name&gt;" line). The fallback
wording is policy (§14).

Players carry **no authored description**. Their appearance is
**generated** at render time (§11.3).

**Acceptance criteria**

- [ ] An item or mob with an authored description renders that prose
      under `look`.
- [ ] A target with no description renders the generic fallback, naming
      the target.
- [ ] The description is read from the instance, not re-resolved from
      the template at look time.
- [ ] An authored description is optional; content with none still
      loads and looks correctly.

### 11.2 `look <target>` resolution

`look <target>` resolves in a fixed order and renders the first match:

1. **Items** — the actor's inventory and the items in the current room,
   by keyword. A container or corpse is *looked into* (its contents are
   listed — see `inventory-equipment-items` and loot handling), not
   described. Any other item renders its description (or fallback).
2. **Creatures** — mobs in the room by keyword, then players in the
   room by name (the same mobs-first / players-by-name asymmetry the
   targeted verbs use). The matched creature renders its description
   (authored for mobs, generated for players) or the fallback.
3. **No match** — a single "you don't see that here" line.

The item display name passed to the fallback carries any decoration
markup the item-decorations spec defines (rarity/essence); the lens
does not strip it.

**Acceptance criteria**

- [ ] `look <mob-keyword>` renders the mob's appearance — a creature in
      the room is a valid look target, not a "don't see that" miss.
- [ ] `look <player-name>` renders the player's generated appearance.
- [ ] Items win ties against creatures sharing a keyword (consistent
      with other room-scan verbs).
- [ ] A container/corpse target is looked into rather than described.
- [ ] An unmatched target terminates with the not-found line.
- [ ] The actor never resolves itself through the creature path.

### 11.3 Generated player appearance

A player's appearance is **composed at render time** rather than stored,
so it stays current as the character changes. It is assembled from an
**ordered set of fragments**; today the committed fragments are the
character's **race** and **class** (their display labels), optionally
followed by the race's flavor tagline. Future descriptors (visible
equipment, title/honorific, posture, visible condition) are intended to
slot in as additional fragments without reshaping the line.

A character with neither race nor class yields **no generated prose**;
the look handler then renders the generic fallback rather than an empty
or malformed line.

The composition (fragment order, article handling, which fragments are
included) is policy (§14).

**Acceptance criteria**

- [ ] A player with race and class renders a noun phrase combining both
      display labels.
- [ ] The race tagline, when present, follows the noun phrase.
- [ ] A character missing both race and class falls back to the generic
      line — never an empty or half-formed sentence.
- [ ] The appearance reflects the character's *current* race/class, not
      a value frozen at an earlier point.

### 11.4 `consider <creature>` — the tactical lens

`consider <creature>` renders a **qualitative** size-up with **no raw
numbers** (no hit points, no armor value):

- A **condition descriptor** — a word for how hurt the target looks,
  derived from its health fraction (e.g. uninjured through dead). This
  is the same observable a player could infer by looking.
- A **relative-threat read** — a phrase answering "can I win?", derived
  from a **power comparison** of the viewer against the target. The
  comparison is a proxy weighted toward durability plus core combat
  attributes; it uses the target's *full* potential (not its current,
  possibly-wounded state — the condition descriptor already signals
  wounding). The viewer must itself be a combatant for this read; when
  it is not, the threat phrase is omitted and only the condition shows.

The descriptor bands and the threat-read bands are both policy (§14).
`consider` resolves creatures only; an item is not a `consider` target.
Self-references point the player at the self sheet instead of rendering.

**Acceptance criteria**

- [ ] `consider <creature>` emits a condition descriptor and (when the
      viewer is a combatant) a relative-threat phrase.
- [ ] No raw hit-point or armor numbers appear in the output.
- [ ] The threat phrase scales with the viewer's strength relative to
      the target's.
- [ ] A non-combatant viewer gets the condition descriptor only — the
      threat read degrades out rather than erroring.
- [ ] A self-reference points to the self sheet rather than considering
      oneself.

---

## 12. Contextual tips

Contextual tips are **one-time hints** that surface the right command
at the moment a player first meets a situation — a gentle,
opt-out layer on top of the pull-based help of §9–§10. They are aimed
at new players and never repeat.

### 12.1 Model

- Each tip has a stable **id** and a short line of copy. The catalogue
  of tips (ids + copy + when each fires) is policy.
- A tip fires **once ever per character**: the shown-once set is
  per-character state that persists across logout (a character who saw
  a tip never sees it again, even after relog).
- Tips are **opt-out**, on by default. A player can disable them, and
  re-arm them (clear the shown-once set and re-enable) — the `tips`
  verb (§12.3) owns this.

### 12.2 The room-view trigger

The committed trigger is the **arrival render** — the shared path that
shows a room after a look or any movement (walk, flee, recall,
teleport). After the room is written, at most **one** not-yet-seen tip
is shown, chosen from an ordered candidate list: a general orientation
tip first (pointing at `help` / the getting-started guide), then
situational tips whose condition holds in the current room (a merchant
present → the shopping tip; loose items on the ground → the pick-up
tip; darkness → the light-source tip). Because each tip fires once,
this drip-feeds a new player one fresh hint per room as they encounter
each situation, and falls silent once all relevant tips are seen.

A disabled player is skipped before any situational condition is
evaluated, so opting out costs nothing. The specific tip ids, their
copy, and their firing conditions are policy (§14). The login-spawn
render is not a trigger in this version — the first look or step is.

### 12.3 The `tips` verb

- `tips` (no argument) reports whether tips are on or off (it does not
  toggle — checking a preference must not change it).
- `tips on` / `tips off` set the opt-out; the choice persists.
- `tips reset` re-arms every tip (clears the shown-once set) and turns
  tips back on, so the introductory hints show again.

**Acceptance criteria**

- [ ] A tip is shown at most once ever per character; the shown-once
      set survives logout.
- [ ] At most one new tip is shown per room view; already-seen tips are
      skipped, letting the next relevant one show later.
- [ ] With tips disabled, no tip is shown and no situational condition
      is evaluated.
- [ ] `tips` reports state without changing it; `on`/`off` persist;
      `reset` re-arms and re-enables.

---

## 13. Observable events

The UI / rendering / help feature emits no engine events.
Help loading is observed via the pack-loading log path
(warnings on broken topics). Color rendering is fully
internal to the send pipeline.

The HTML map exposed by the theme registry (§3.3) is
consumed by the GMCP layer if it chooses to publish a
theme package; that publication, if any, lives in the
networking spec's GMCP section.

---

## 14. Configuration surface

The following are externally configurable and not fixed by
this spec.

| Policy | Where it applies |
|---|---|
| Theme entries (tag → fg/bg/html) | §3.1 |
| Default prompt template | §7.1 |
| Max prompt template length | §7.4 |
| Default panel width (today 80) | §8.1 |
| Default help-render width (today 78) | §10.1 |
| Role hierarchy (today player / builder / admin) | §9.5 |
| Brace-color synonym aliases | §2.3 |
| The set of recognized prompt tokens | §7.2 |
| Generic "nothing special" fallback wording | §11.1 |
| Player-appearance composition (fragment set + order, article rules) | §11.3 |
| Condition descriptor bands (health fraction → word) | §11.4 |
| Threat-read bands (power ratio → phrase) | §11.4 |
| Power-comparison proxy (which attributes, their weights) | §11.4 |

---

## 15. Open questions / future work

- **Hyphen vs underscore color names.** Literal color tags
  use `bright-red`; brace shorthand uses `bright_red`. The
  split is historical and surprising. Normalizing on one
  form (and accepting the other as an alias) would be a
  small but real ergonomics win.
- **Tag close emits unconditional reset.** Nested semantic
  tags lose their outer color when the inner closes. A
  stack-based renderer would preserve nesting, at the cost
  of one stack frame per open.
- **Render cache grows unbounded.** Both Ansi and Plain
  caches accumulate every distinct input string for the
  process lifetime. Pathological input (per-tick dynamic
  banners with timestamps) leaks memory. An LRU with a
  configurable cap would defend against this.
- **No per-player theme.** Themes are global. Players who
  want lower-contrast or color-blind-friendly variants
  have no path. A theme-override property on the entity
  could be consulted by the decorator.
- **Help role gate is a placeholder.** Currently builder /
  admin tiers don't actually elevate a player's tier — the
  query always treats players at the `player` tier. The
  hierarchy exists in code but the resolution function
  returns the same value for everyone.
- **Help fuzzy ranking is flat.** Multiple matches are
  returned without prioritizing title-prefix matches over
  keyword-substring matches. Adding a simple score would
  surface obvious matches first.
- **Prompt tokens are hardcoded.** The substitution table
  is engine-built; content can't add tokens without
  changing the renderer. A pack-registered token resolver
  would let content add `{combat_target}` or `{room_name}`.
- **Panel renderer raises on title overflow.** A right-side
  title that overflows throws. Production code would prefer
  to clamp + log rather than crash a render path.
- **No image / mxp support.** The output is plain text plus
  ANSI. Clients with richer rendering (MXP, MUSHclient
  HTML, Mudlet GUI panels) get no extra structure beyond
  what GMCP carries.
- **GMCP HTML map is read-only after compile.** Themes
  cannot be re-themed at runtime; a `recompile` operation
  would let admins iterate without restart.
- **Unknown brace tokens render as literal.** `{frobnitz}`
  emits the literal text. Whether this is the right default
  (vs warn or empty) is debatable.
- **Player appearance is race + class only.** The generated
  description (§11.3) composes just the race/class labels and
  the race tagline today. The reserved fragments — visible
  equipment, title/honorific, posture, visible condition —
  are unimplemented; each is additive but none is wired yet.
- **Threat read uses a stat proxy, not a level.** The
  `consider` relative-threat band (§11.4) compares a
  durability-weighted attribute proxy because there is no
  single character level to key off (progression is
  per-track). A level- or rating-based ladder could replace
  the proxy if a canonical "power level" is ever defined,
  and would let mobs be ranked without summing attributes.
- **Descriptions are single, static, whole-target.** A look
  target renders one description string; there is no
  keyword-addressable sub-description (look at the scar, the
  blade) and no look-through to a mob's visibly-equipped
  gear. Both are common MUD affordances left for later.

---

<!-- Generated: 2026-05-21 · Scope: ColorRenderer + ThemeRegistry + ColorRenderingConnection + AnsiPair + ThemeEntry + TagStripper + PromptRenderer + PromptProperties + Panel + PanelRenderer + Section + Row + Cell + HelpService + HelpRenderer + HelpTopic + HelpTopicSummary · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
