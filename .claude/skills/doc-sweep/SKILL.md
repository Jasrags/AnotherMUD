---
name: doc-sweep
description: Freshen the AnotherMUD tracking docs (README, CLAUDE.md, BACKLOG, DEFERRED-BACKLOG, ROADMAP) against the live codebase, and clean up / compress the project memory index (MEMORY.md) to stay under its size limit. Run after shipping milestones, when a MEMORY.md size warning appears, or periodically as hygiene.
user-invocable: true
disable-model-invocation: true
---

# Doc Sweep — tracking-doc freshness + memory compression

A repeatable two-part hygiene pass: **(A)** freshen the tracking docs against
what actually shipped, and **(B)** prune/compress the project memory index so it
stays under its hard size limit. Do **A then B** — freshness first, because B's
"what's done" deletions rely on knowing what shipped.

Be surgical. Preserve each doc's tone and structure. Verify every claim against
git/code — never trust a doc's own stale text. Do **not** commit unless the user
asks.

## Paths (this project)

- Tracking docs (in repo): `README.md`, `CLAUDE.md`, `docs/BACKLOG.md`,
  `docs/DEFERRED-BACKLOG.md`, `docs/ROADMAP.md`
- Memory index: `/Users/jrags/.claude/projects/-Users-jrags-Code-Jasrags-AnotherMUD/memory/MEMORY.md`
- Memory topic files: the same directory (one `*.md` per memory)
- **MEMORY.md hard limit: 24,400 bytes.** Target ≥ ~2 KB headroom after a pass.
  (The limit is on the index only — topic files on disk don't count, since only
  MEMORY.md loads into context each session.)

> Memory dir is `~/.claude/projects/<url-encoded-cwd>/memory/`. If the hardcoded
> path above ever misses, re-derive it from the current working directory.

---

## Step 0 — Gather ground truth (always run first)

```sh
cd /Users/jrags/Code/Jasrags/AnotherMUD
echo "=== recent ships ===";            git log --oneline -25
echo "=== highest milestone in ROADMAP ==="; grep -oE '^## M[0-9]+' docs/ROADMAP.md | sort -V | tail -3
echo "=== spec count ===";              ls docs/specs/*.md | grep -v README | wc -l
echo "=== save version ===";            grep -rn 'CurrentVersion' internal/player/*.go | head -1
echo "=== MEMORY.md size ===";          wc -c /Users/jrags/.claude/projects/-Users-jrags-Code-Jasrags-AnotherMUD/memory/MEMORY.md
echo "=== doc git status ===";          git status --short README.md CLAUDE.md docs/BACKLOG.md docs/DEFERRED-BACKLOG.md docs/ROADMAP.md
```

Note the four cross-doc invariants from this output — they must agree everywhere:
**highest completed milestone**, **spec count**, **which specs are still
"written ahead of code"** (contracts not yet built), and **save version**.

---

## Part A — Doc freshness sweep

Read each doc, compare against Step 0, fix staleness. The recurring failure
modes (seen in prior sweeps):

- **README.md** — milestone status blockquote ("M0–M__ complete"); the
  "What works today" list; the **login-flow** description in Quick Start (a
  frequent offender — it has drifted before); the spec count in the
  Documentation table.
- **CLAUDE.md** — the repo-status paragraph's milestone list + spec count + the
  "specs still written ahead of code" parenthetical + the save-version note
  (`player.CurrentVersion`) and its migration-chain description.
- **docs/BACKLOG.md** — **delete-on-ship rule (strict):** when an item ships,
  **delete its line entirely** — never strike through, never `[x]`. Check §1
  (specced-ready), §2 (greenfield), the "Candidate next themes" tables, and the
  "Picking rubric" for items that have since shipped (cross-ref git log). Fix
  the intro prose's "still written ahead of code" list. Watch for **dangling
  references** — a candidate-theme row pointing at a §1 item you just deleted.
- **docs/DEFERRED-BACKLOG.md** — a point-in-time snapshot; it's allowed to lag.
  Don't regenerate the whole thing — add a short "not yet folded in" note at the
  top pointing to the newer memory files for milestones shipped since its last
  "Generated" date.
- **docs/ROADMAP.md** — the append-only done-log; usually current (per-slice
  commits update it). Skim the tail for the latest milestone; rarely needs edits.

**Method:** surgical edits only. For the larger mechanical doc edits, you may
delegate README and BACKLOG to **parallel subagents** — give each the *verified*
facts from Step 0 and an explicit edit list so they don't go exploring. Do the
CLAUDE.md + DEFERRED-BACKLOG touch-ups yourself.

**Consistency gate (end of Part A):**

```sh
cd /Users/jrags/Code/Jasrags/AnotherMUD
grep -oE 'M0.M[0-9]+ (are )?complete|[0-9]+ (behavior )?specs' README.md CLAUDE.md
```
Confirm milestone number + spec count match across README and CLAUDE, and that
BACKLOG's "ahead of code" list matches CLAUDE's.

---

## Part B — Memory cleanup / compression

The index accumulates two kinds of dead weight that don't belong in an
always-loaded file: **(1) done-with-no-open-hook entries** (status already in
ROADMAP/git) and **(2) granular legacy deferred-fix entries** already aggregated
in `docs/DEFERRED-BACKLOG.md`. The job is to cut **entry count**, not just prose.

### Inventory first

```sh
cd /Users/jrags/.claude/projects/-Users-jrags-Code-Jasrags-AnotherMUD/memory/
python3 - <<'PY'
import re
txt=open("MEMORY.md").read()
print(f"TOTAL {len(txt)} bytes, limit 24400, headroom {24400-len(txt)}")
for p in re.split(r'(?m)^## ',txt)[1:]:
    head=p.splitlines()[0].strip(); n=len(re.findall(r'(?m)^- \[',p))
    print(f"  {n:>3} entries  {len(p):>6} bytes  {head[:50]}")
PY
```

### The three levers (ranked by impact)

1. **Collapse the legacy block (biggest lever).** The per-milestone M1–M19
   deferred-fix lines duplicate `docs/DEFERRED-BACKLOG.md`. Replace the whole
   block with **one prose pointer** to that doc, keeping only the still-open
   **MEDIUM+** hooks inline (so a reader knows what's live without opening 50
   files). Topic files stay on disk.
2. **Delete done-no-open entries (zero-risk).** Any entry whose work is fully
   SHIPPED/RESOLVED with **no open hook** is pure dead weight — its "done" status
   lives in ROADMAP/git. Delete the index line (leave the topic file on disk).
3. **Complete-then-delete small-tail entries (durable, low bytes/effort).** If an
   entry's *only* open item is a small/LOW fix, fixing it in code retires the
   whole entry. Do this opportunistically when already in that code — **not** as
   a size campaign (poor bytes-per-effort).

Apply 1 + 2 for size; mention 3 as follow-up. Aim to land MEMORY.md at ≤ ~22 KB
(≥ 2 KB headroom) so it doesn't immediately re-trip the warning.

### The hygiene principle (state it, keep it)

> The index holds **open hooks + active-arc orientation + durable
> reference/feedback**. Shipped-and-closed work gets **deleted** from the index
> once ROADMAP/git record it — the index is not a second done-log.

### Verification gate (must all pass before done)

```sh
cd /Users/jrags/.claude/projects/-Users-jrags-Code-Jasrags-AnotherMUD/memory/
sz=$(wc -c < MEMORY.md); echo "size $sz / 24400 (headroom $((24400-sz)))"
[ "$sz" -le 24400 ] && echo "OK under limit" || echo "STILL OVER — trim more"
echo "--- broken links (index → missing file; must be empty):"
grep -oE '\]\(([a-z0-9-]+\.md)\)' MEMORY.md | sed 's/](//;s/)//' | sort -u \
  | while read f; do [ -f "$f" ] || echo "BROKEN: $f"; done
echo "--- entry count:"; grep -c '^- \[' MEMORY.md
```

- Under the limit with headroom.
- **No broken links.**
- **No entry silently dropped** — if you removed a line, it was deliberate
  (done-no-open or collapsed-to-pointer), not an accident. Topic files for
  removed entries remain on disk.
- Every retained entry still carries its open hooks + `[[cross-links]]`.

---

## Output

Report concisely:
- Part A: what was stale and what you changed per doc (before/after gist).
- Part B: before/after MEMORY.md size + entry count, which entries were
  collapsed/deleted, and that verification passed.
- Remind the user nothing was committed (memory lives outside the repo; the
  tracking-doc edits are staged in the working tree).
