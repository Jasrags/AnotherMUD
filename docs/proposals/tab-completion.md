# Proposal: Input Tab-Completion

**Status:** Largely implemented — see `docs/specs/tab-completion.md` (authoritative). · **Type:** Feature proposal · **Audience:** engine
**Shipped:** Phase 0 substrate; the line-mode `suggest` stopgap; and Phase 1 *server side* (inbound GMCP dispatch + `Input.Complete` request/response, §13). **Remaining:** the client integration (`docs/clients/tab-completion-gmcp.md`) and Phase 2 char-mode for raw-telnet TAB.

## Decisions taken so far (steering this draft)

These were settled in review and shape the phasing below; the spec will inherit them:

- **Sequence: Phase 0 substrate first.** Candidate *enumeration* is net-new and is needed by every transport option, so it is built and validated transport-agnostically before the surface (GMCP vs char-mode) is chosen. See §3–§4.
- **Audience: Mudlet-class GMCP clients _and_ raw-telnet/nc parity both matter.** This keeps server-side char-mode (Option A) on the committed roadmap as a later phase rather than a "maybe never" — with the hard tension that real TAB on a raw line-mode client *requires* char-mode (§4).

## 1. Problem / motivation

Text input is the entire interface, and right now every command and every operand must be typed in full and spelled correctly. That's friction on the most common actions (`get longsword from chest`, `kill grizzled bandit`) and a real barrier for new players who don't yet know the verb set. Tab-completion is the single highest-leverage quality-of-life improvement available to a text client.

AnotherMUD is well-positioned to do the *hard* part — contextual argument completion — because the substrate it needs is mostly present: the command registry knows every verb, and `keyword.Resolve`/`keyword.ResolveAll`/`splitOrdinal` already encode how operands match against items, inventory, players, and the ordinal/`all.` scheme. **What is _not_ present is an enumeration surface**: today the resolvers answer "resolve this finished token to one entity, or error" — they do not answer "given this player, this room, this prefix, what are the valid candidates here?" That gap is the real work, and §3 is built around closing it.

> Correction vs. the first draft: the original claimed "the intelligence completion needs is already built." That overstated it. The *matching primitives* exist and are reusable; *candidate enumeration* is net-new (see §3 and the substrate notes in §6).

## 2. Goals & non-goals

**Goals.** Complete command verbs from the registry. Complete *contextual arguments* using the existing keyword/ordinal matching primitives — this is the prize, because "complete the second word of `get <thing here>` to the things actually in reach" is what makes the world feel responsive, and it's the thing only the server can know. Present multiple matches sanely, including interaction with the existing ordinal/keyword scheme (`2.ring`, `all.gem`). Do all of this without regressing existing clients.

**Non-goals (rule out now).** This is *not* a proposal to build server-side line editing as an end in itself — cursor movement, history scrollback, kill-line, a terminal emulator living in the server. Note, however, that the raw-telnet parity goal (§4) means char-mode line editing is a *later committed phase*, not permanently out of scope — it is simply not v1 and gets its own proposal. Also out of scope for v1: fuzzy/typo-correcting matching, inline ghost-text suggestions (fish-shell style), and completion of free-text fields (say/tell message bodies). These are plausible later, not now.

## 3. Proposed approach (the shape)

Treat completion as two features that get conflated and shouldn't be. **Verb completion** is nearly static — prefix-match against the command registry (which already does prefix resolution for *submitted* lines; enumeration is the new bit). **Argument completion** is dynamic and contextual — it asks "what are the valid candidates for the current argument position, given this player, this room, this prefix?"

The center of gravity is therefore **building a candidate-enumeration layer** in the command/resolver package and then exposing it — *not* new matching logic (that's reused) and *not*, in the first instance, transport plumbing.

**Phase 0 — enumeration substrate (unconditional, transport-agnostic).** Build a candidate-enumeration pass that, for a given partial line, returns:
- verb candidates (registry prefix scan), and
- argument candidates for the current slot, by driving the existing `keyword`/ordinal primitives over the in-context scope (inventory, room contents, entities, doors).

It is prefix-filtered, ordinal-aware (meshes with `2.ring`/`all.ring` rather than fighting it), and capped (§6 enumeration-cost risk). It is exercised behind a test and/or a throwaway debug verb — **no transport commitment**. This is the phase that contains the genuinely ambiguous design work (ordinal interaction, presentation rule, caps), so it ships first and cheapest, and de-risks every later surface.

**Phase 1+ — surface.** Only after Phase 0's cost and shape are known do we pick how completion is *rendered and applied*. That is the §4 fork.

## 4. Key decision: where is completion rendered and applied?

The intelligence stays server-side regardless (the client can't know the verb set or the resolver rules). The open question is purely the *surface*. Telnet traditionally does line editing client-side, and a line-mode server **sees nothing until Enter** — which is the crux of the raw-telnet tension below.

**Option A — Server-side, character mode.** Negotiate character-at-a-time telnet (suppress-go-ahead + echo), server intercepts Tab, computes completion (Phase 0 layer), rewrites the input line. *Pro:* works for any character-mode telnet client with zero client cooperation — **the only option that gives raw `telnet`/`nc` users real TAB**. *Con:* the server must then own *all* line editing — echo, backspace, cursor, history — i.e. become a terminal, and this changes the input model for every connection that negotiates it. Large, separable build.

**Option B — Server candidates, client rendering, over GMCP.** Client sends the partial line on Tab; server replies with candidates (Phase 0 layer); client renders/applies locally. *Pro:* server never grows a terminal; raw line-mode clients are unaffected (no regression). Fits the engine's "server knows state, GMCP surfaces it, client renders" grain. *Con:* benefits only GMCP-capable clients, and requires **net-new inbound GMCP plumbing** — see the reality check below.

**Phasing given the decisions in the preamble.** Because both Mudlet-class clients *and* raw-telnet parity are wanted:

- **Phase 0:** enumeration substrate (above).
- **Phase 1:** Option B surface for GMCP-capable telnet clients (Mudlet et al.). Cheaper than A, can't regress anyone, and there is a *real* consumer today.
- **Phase 2:** Option A char-mode, as its own proposal, to deliver raw-telnet/nc parity. This is now a committed eventual phase (not "if ever"), because raw-telnet parity is a stated value — and **there is no cheaper way to give a raw line-mode client real TAB** (the server can't see the keystroke until Enter). The only line-mode-compatible alternative is an explicit `complete <partial>` verb that *lists* candidates — not real TAB; noted in §7 as a possible stopgap, not a substitute.

### Reality check on Option B's "reuse" (corrected)

The first draft billed B as reusing the existing GMCP layer. That is half-true and worth stating precisely:

- GMCP today is **server→client only** — the implemented packages (Char.Vitals, Room.Info, Char.Items.List, Char.Combat, Char.Effects, Char.Experience, Comm.Channel.Text, Char.Login, Char.StatusVars, Char.Status) are all *push*. B reuses this *send* direction.
- The **receive** direction is net-new on both transports: telnet has a `GmcpHandler` hook (`internal/conn/telnet/gmcp.go`) that is **never wired anywhere in the engine** (only in tests), and the WebSocket read loop **silently drops inbound GMCP frames** (`internal/conn/ws/ws.go`). B requires building client→server GMCP dispatch before a completion request can even arrive.

### Reality check on the consumer (corrected)

The first draft leaned on "the web client most people actually use." **There is no web client.** The repo ships a WebSocket *transport* (`internal/conn/ws/`) that speaks the line+GMCP protocol, but no browser UI consumes it — no HTML/JS/TS, no `web/`, no `package.json`. So B's near-term beneficiary is **Mudlet-class scriptable telnet clients**, which are real and can do client-side completion on GMCP. B's value is therefore **gated on shipping the client-side integration** (a Mudlet package/profile). Absent that, B ships a correct server with no consumer. This must be an explicit deliverable of Phase 1, not an afterthought.

## 5. Alternatives considered & rejected

Pure client-side completion (no server involvement) was rejected: the client has neither the verb set nor the resolver rules, so it could at best complete from GMCP state it already holds (own inventory, room contents) and would silently fail elsewhere — inconsistent and confusing. Building completion as bespoke per-command logic (each verb declares its own completions) was rejected in favor of driving the existing `keyword`/ordinal primitives, since those already encode "what matches in this slot" and duplicating that would drift.

## 6. Dependencies & risks

**Reusable substrate (exists):** the command registry (verb candidates + existing prefix resolution for submitted lines); `keyword.Resolve` / `keyword.ResolveAll` / `splitOrdinal` (`internal/keyword/resolver.go`, public) for argument matching and the ordinal/`all.` scheme; the GMCP *send* path (`internal/gmcp/`) for Option B's response leg.

**Net-new surface (must build):**
- **Phase 0:** the candidate-enumeration layer itself — there is no enumeration operation on `ArgResolver`/`ArgResolverRegistry` today (`ArgResolver` is `func(ResolverInput) (ResolverOutput, error)` — resolve-one-token only). Either add enumeration methods per scope, or a new input-layer that drives `keyword`/ordinal directly. Ordinal-aware prefix filtering and caps live here.
- **Phase 1 (Option B):** inbound GMCP dispatch on both telnet and WebSocket (wire the existing telnet hook; stop dropping inbound WS GMCP), a completion GMCP package (request/response), and the Mudlet-side integration that consumes it.

**Risks worth naming now.**
- **Visibility leakage.** The primer notes `CanSee` is effectively always-true today (visibility rules are greenfield), so there's no leak risk *yet* — but completion is a textbook information-leak vector, so the eventual spec must state that candidates respect visibility rules once those land. Flagging pre-spec is the point: it's invisible until the day it isn't.
- **Enumeration cost.** Naive generation could enumerate every item in a crowded room or every online player on each Tab; Phase 0 must prefix-filter and cap. Implementation concern, not a blocker.
- **Ordinal/keyword interaction.** Completing `ring` with three rings present must mesh with `2.ring`/`all.ring` rather than fight it — a real design question, owned by Phase 0.

## 7. Presentation decisions

**Signed off 2026-06-03** (the surface decisions Phase 1+ inherits; mirrored in the spec):

- **Multiple-match presentation: DECIDED → longest-common-prefix + list.** Complete to the shared prefix, then show the candidate list (classic shell). Scales to many matches; matches muscle memory. (Under Option B a client-render concern; under A server-rendered.)
- **Activation: DECIDED → automatic when the client advertises the completion capability.** No per-player/account setting.
- **Compute model: DECIDED → request/response (compute only on Tab).** No channel spam, far simpler. Push (proactive streaming for live ghost-text) is the *future* — it's additive over the same Phase 0 query and can layer on later without changing the request/response path. v1 explicitly does **not** build push.

Shipped:

- **Raw-telnet stopgap: BUILT 2026-06-03 → the `suggest <partial>` player verb.** Lists completion candidates in line mode (no TAB key) for raw `telnet`/`nc` users — the same Phase 0 query the admin `complete` verb runs, rendered for players (single match → completed line, multiple → list + LCP hint). Ships now rather than waiting for Phase 2 char-mode; demand was real (the project's own testing is on raw telnet).

## 8. Rough sizing

- **Phase 0 (enumeration substrate):** the bulk of the *thinking* (ordinal interaction, presentation rule, caps), modest *plumbing*. Independently testable, no transport risk. Build and validate this first.
- **Phase 1 (Option B surface):** more than the first draft implied — inbound GMCP dispatch on *both* transports + a completion package + the Mudlet-side integration (without which it ships dead). Still well-bounded once Phase 0 exists.
- **Phase 2 (Option A char-mode):** a genuinely separate, larger effort that drags line-editing into the server. Gets **its own proposal**; does not ride along on this one. On the roadmap because raw-telnet parity is a stated value, but sequenced last.

---

*Per the house pipeline, no acceptance-criteria or configuration-surface tables here — those belong to the spec once §4's surface decision and the §7 questions are signed off. This proposal's job is to get Phase 0 built and the §7 questions decided.*
