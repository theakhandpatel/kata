# Context-pruning extensions for the pi coding agent

**Purpose.** Reconstruct the architecture of three pi extensions that touch context cost, so the `lathe` context-budget chapter can teach the right invariants instead of copying a README.

**Sources read.** Shallow clones in `/tmp/prune-survey/`:

| Package | Repo | Version / commit | Notes |
|---|---|---|---|
| `pi-context-prune` | `championswimmer/pi-context-prune` | `2.1.0`, `093b4b53f4b2d86f1fc246935aef45b017434b76` | no core unit tests; `scripts/test-usage.mjs` only; runtime source is `index.ts` + `src/` |
| `pi-condense` | `jjuraszek/pi-condense` | `2.13.1`, `cf56a418848901688dd7eb18fccb4e024bdae7c1` | 33 test files; **renamed descendant of `pi-context-prune`** |
| `toolflow` | `3ZEROS12/toolflow` | `4.0.0`, `553c12845fb57c06895d7472e95177ce97693cd1` | 6 test harness files; different product, not a message pruner |
| `pi` reference | `earendil-works/pi` | `f9daca3480732d0042b1df3a6e27221f597a620d` | used for extension API / event / compaction semantics |

`pi-condense` is not an independent design: its changelog says it was renamed from `pi-context-prune` (`pi-condense/CHANGELOG.md:229`), its README credits “the original approach from championswimmer/pi-context-prune” (`pi-condense/README.md:200`), and it still uses `context-prune-*` custom entry names. Treat `pi-context-prune` as the ancestor and `pi-condense` as the hardened successor. Do not cite them as two independent validations of the same idea.

**Method.** Static reading of the cloned source at the pinned commits; `pi` source/docs for host semantics. I did not run live provider sessions or measure billed tokens. The Go teaching model in §9 was extracted and compiled/tested locally (`gofmt`, `go test`) to catch type and pairing mistakes; that is not evidence about the extensions’ runtime behavior.

## 0. Claim audit (code first)

| Claim | Verdict | Code evidence |
|---|---|---|
| `pi-condense` “summarizes completed tool-call batches” | **Yes** | `turn_end` capture, one LLM summary per batch, hidden `custom_message` (`pi-condense/index.ts:886`, `:527-533`) |
| `pi-condense` “replaces raw outputs with short stubs” | **Yes, with exceptions** | literal stub at `pi-condense/src/pruner.ts:137`; protected tools/paths, recovery grace, skipped/oversized batches remain raw (`src/pruner.ts:119-124`, `src/protected.ts:49-55`) |
| `pi-condense` “compresses closed tool-call chains” | **Yes** | chain detector + `rollingWindow: 3` default; drops assistant/toolResult/summary range and inserts `<compressed-chain>` (`src/chain-detector.ts:41`, `src/chain-compressor.ts:55-63`, `src/chain-range-prune.ts:222-241`, `src/types.ts:576-581`) |
| `pi-condense` “recovers any original on demand via `context_tree_query`” | **Mostly; “any” is too strong** | tool is real and model-callable (`src/query-tool.ts:19-33`); but only indexed records, only if not protected/skipped, and each answer is truncated to pi’s 2,000-line/50 KB head (`src/query-tool.ts:88-96`; `pi/packages/coding-agent/src/core/tools/truncate.ts:11-12`) |
| `toolflow` “stage-gated capability mounting” | **Yes, but only inside a `/toolflow` blueprint** | stages carry `allowedTools`, applied with `setActiveTools` (`toolflow/src/engine.ts:1187-1227`, `src/state.ts:823-839`, `src/index.ts:204-217`) |
| `toolflow` “automatic log dehydration to disk (-95% token overhead)” | **Dehydration: yes. “-95%”: not established** | thresholds vary by tool, not a uniform “>40 lines” (`src/dehydrator.ts:442-485`); `tokenSavingsRatio: ">95%"` is hard-coded (`src/dehydrator.ts:285`); per-call savings are floored at 10 tokens even when the replacement is not smaller (`src/dehydrator.ts:515`, `:547`) |
| `toolflow` “protected-file write guard” | **Yes, best-effort** | `tool_call` guard blocks direct `write`/`edit` paths and simple destructive shell tokens (`src/blast_radius.ts:230-250`, `:282-325`); comment explicitly disclaims protection against malicious shell obfuscation (`src/blast_radius.ts:12-15`) |
| `toolflow` “caches duplicate file reads” | **Yes** | `ReadCacheManager` with path/mtime/hash/turn gates (`src/read_cache.ts:129-202`) |
| `toolflow` “prunes unused tools” | **Refuted as stated** | no usage counter; pruning is stage-policy plus a prompt-keyword heavy-tool filter (`src/context_slimmer.ts:35-63`; `src/state.ts:823-839`). A tool used every turn is still removed if its stage does not list it |
| `toolflow` “blocks accidental .env/git deletions” | **Yes for direct/simple cases; bypassable** | `.env`, `.git`, lockfiles blocked directly (`src/blast_radius.ts:42-58`, `:230-250`) and in shell commands with destructive tokens (`:282-325`); `find . -name .env -delete` and interpreter writes are not matched |
| `pi-context-prune` architecture | **Summarize batch → delete `toolResult` messages from outgoing context → keep assistant `toolCall` blocks → recover via session index** | deletion: `src/pruner.ts:8-15`; summary/index: `index.ts:288-310`; recovery: `src/query-tool.ts:6-65` |

Two code-vs-README findings worth calling out separately:

1. **`toolflow`’s `session_before_compact` handler returns a shape pi ignores.** `pi` accepts only `{ cancel?: boolean; compaction?: CompactionResult }` from that event (`pi/packages/coding-agent/src/core/extensions/types.ts:1511-1514`), and the consumer reads only `cancel`/`compaction` (`pi/packages/coding-agent/src/core/agent-session.ts:2867-2875`); toolflow returns `{ customInstructions: ... }` (`toolflow/src/index.ts:535-542`). The handler therefore does not inject its “STRICT COMPACTION DIRECTIVE”. Toolflow’s actual compaction influence comes from calling `ctx.compact({ customInstructions })` after stage verification (`src/index.ts:774-779`, `:871-876`).
2. **`toolflow`’s historical `context` slimmer checks a provider-shaped message schema, not pi’s `AgentMessage` schema.** It looks for `role === "tool"`, `msg.toolCalls`/`tool_calls`, and block types `tool_use`/`tool_call` (`src/context_slimmer.ts:188`, `:224-226`, `:259`). Pi’s `context` event carries `AgentMessage[]` (`ContextEvent.messages: AgentMessage[]`, `pi/packages/coding-agent/src/core/extensions/types.ts:885-888`), where tool results are `role: "toolResult"` (`pi/packages/ai/src/types.ts:642-653`) and assistant tool calls are `content` blocks of type `"toolCall"` (`:585-587`, `:445`). Static inspection therefore says this slimmer is inert on real pi messages; the repo’s test fakes the provider shape (`toolflow/tests/test_suite.ts:2162-2164`). I did not run a live pi session to prove the no-op, but I found no adapter.

## 1. Inventory: what each package is, LOC, dependencies

Runtime LOC counts `index.ts` + non-test `src/*.ts`. Test LOC counts `*.test.ts` or `tests/*.ts`.

| Package | Runtime LOC | Test LOC | Runtime deps declared | Pi surfaces used | What it actually is |
|---|---:|---:|---|---|---|
| `pi-context-prune` 2.1.0 | 3,694 | 0 core (126-line usage script) | peer: `@earendil-works/pi-ai`, `@earendil-works/pi-coding-agent >=0.86.0`, `@earendil-works/pi-tui`, `@sinclair/typebox`; Node `fs/promises`, `path` | `turn_end`, `tool_execution_end`, `message_end`, `context`, `before_agent_start`, `session_start`, `session_tree`, `registerTool`, `appendEntry`, `sendMessage`, `setActiveTools` | LLM batch summarizer + **delete tool results** + session-index recovery; optional `context_prune` tool in agentic mode |
| `pi-condense` 2.13.1 | 7,816 | 11,741 | same peer set; Node `crypto`, `fs`, `fs/promises`, `os`, `path` | `turn_end`, `message_end`, `context`, `session_start`, `session_tree`, `session_compact`, `registerTool`, `appendEntry`, `sendMessage`, `compact` | hardened successor: stubs, chain compression, dedup, spill, protected paths, budget/delta triggers |
| `toolflow` 4.0.0 | 8,463 | 3,495 | peer: `@earendil-works/pi-coding-agent >=0.80.0`, `@sinclair/typebox`; imports `@earendil-works/pi-tui` but does **not** declare it as a peer (only devDependency in `package.json`) | `session_start`, `session_before_compact`, `before_provider_request`, `before_agent_start`, `tool_call`, `tool_result`, `context`, `turn_end`, `registerTool`, `registerCommand`, `setActiveTools`, `compact` | stage-gated tool schemas + eager log dehydration + duplicate-read cache + best-effort write guard; not an LLM summarizer |

The `pi-context-prune` repo has no `dist/` committed; I read the TypeScript source at the tagged commit. The npm tarball is built from the same source by `prepack` (`package.json`), but I did not diff the built bundle.

## 2. Taxonomy, kept separate (Q1)

The six mechanisms are independent. “Pruning” is not one thing.

| Mechanism | `pi-context-prune` | `pi-condense` | `toolflow` | Course-worthy? | Reasoning |
|---|---|---|---|---|---|
| **(a) Turn/message truncation** | Partial: deletes `toolResult` messages, keeps assistant `toolCall` blocks (`src/pruner.ts:8-15`) | Yes: closed-chain range drop of assistant + toolResult + per-batch summaries (`src/chain-range-prune.ts:222-241`) | Partial: historical context folding in `context` (`src/context_slimmer.ts:172-271`), but schema mismatch makes it inert on pi messages | **Advanced / cautionary**, not first | It is the mechanism most likely to split a tool pair. Teach it only after in-place stubbing, and require atomic pair deletion.
| **(b) Tool-output elision to a stub** | No: result is removed, no stub (`src/pruner.ts:8-15`) | Yes: one-line literal stub at `src/pruner.ts:137`; spill stub at `:129-136` | Yes: head/tail/error digest + archive path (`src/dehydrator.ts:538-545`); read-cache notice (`src/read_cache.ts:183-193`) | **Core** | Smallest deterministic mechanism; directly gradeable for budget, marking, and recovery.
| **(c) Summarization / compaction** | Yes: one LLM summary per batch, hidden `custom_message` (`index.ts:288-310`) | Yes: per-batch LLM summary + optional range fusion (`src/summarizer.ts:14-30`, `:386-393`) | No own summarizer. Influences pi compaction via `ctx.compact` at stage boundaries; the `session_before_compact` return is ignored (`src/index.ts:535-542`, `:774-779`) | **No for the minimal offline course** | Adds a model call, nondeterminism, cost, and a second token budget. It is an optional extension after the deterministic invariants are solid.
| **(d) Tool-schema pruning (unmounting definitions)** | Only one optional tool toggle: activates `context_prune` in `agentic-auto` (`index.ts:410-425`) | No: removed the `context_prune` tool and agentic mode (`pi-condense/CHANGELOG.md:240`) | Yes: stage `allowedTools` → `setActiveTools` (`src/state.ts:823-839`), plus request-level heavy-tool removal (`src/context_slimmer.ts:35-63`) | **Yes, as a separate chapter** | Different object (tool catalog, not transcript), different trigger (stage, not tokens), different failure (capability deadlock, not orphaned pair).
| **(e) Duplicate-read caching** | No | Partial: content-hash dedup of normalized identical `(toolName, resultText)` aliases to the original ref (`index.ts:331-350`, `src/indexer.ts:376-395`); not path/offset-aware | Yes: path/mtime/hash/turn-gated read cache (`src/read_cache.ts:123-232`) | **Optional** | Cheap and useful, but correctness depends on mutation invalidation; it is not required to satisfy the budget invariant.
| **(f) Externalization to disk with a retrievable pointer** | No; originals live in session custom entries, not sidecar files | Yes for oversized single results `>= 65,536` chars: sidecar `.txt` + path + preview (`src/spill.ts:69-117`, `src/types.ts:589`) | Yes: every dehydrated output goes to `.pi/toolflow/runs/...` with a path and `toolflow_inspect` hint (`src/dehydrator.ts:490-545`) | **Core, paired with (b)** | Makes elision honest. Test recovery after process exit and path traversal, not just that the stub exists.

**Per-package roll-up.** `Y` = implemented on the main path; `P` = partial/conditional; `N` = not implemented. LOC is runtime / test.

| Package | (a) msg trunc | (b) stub | (c) summary | (d) schema | (e) read cache | (f) externalize | Runtime / test LOC | Declared runtime deps |
|---|:-:|:-:|:-:|:-:|:-:|:-:|---|---|
| `pi-context-prune` | P | N | Y | P (one tool) | N | N | 3,694 / 0 core | pi-ai, pi-coding-agent, pi-tui, typebox |
| `pi-condense` | Y | Y | Y | N | P (content hash) | Y (oversized spill) | 7,816 / 11,741 | pi-ai, pi-coding-agent, pi-tui, typebox |
| `toolflow` | P (likely inert) | Y | N (indirect compact) | Y | Y | Y | 8,463 / 3,495 | pi-coding-agent, typebox (+ undeclared pi-tui import) |

`pi-context-prune` is the simple ancestor; `pi-condense` is the hardened message-pruner; `toolflow` is an orthogonal tool-surface/IO product.

## 3. Trigger: the exact condition that fires pruning (Q2)

### 3.1 `pi-context-prune`: event-driven, no token threshold

Five modes (`pi-context-prune/src/types.ts:108-118`):

```ts
export type PruneOn = "every-turn" | "on-context-tag" | "on-demand" | "agent-message" | "agentic-auto";
```

- **every-turn**: after a tool-using `turn_end`, flush immediately.

```ts
if (currentConfig.value.pruneOn === "every-turn") {
  await flushPending(ctx, { delivery: "session" });
}
```
`pi-context-prune/index.ts:489-491`

- **on-context-tag**: flush when a `context_checkpoint`/`context_tag` tool finishes.

```ts
pi.on("tool_execution_end", async (event, ctx) => {
  if (!(CONTEXT_TAG_TOOL_NAMES as readonly string[]).includes(event.toolName)) return;
  if (!currentConfig.value.enabled) return;
  if (currentConfig.value.pruneOn !== "on-context-tag") return;
  await flushPending(ctx, { delivery: "runtime" });
});
```
`pi-context-prune/index.ts:521-526`

- **agent-message**: flush when the assistant emits a final text-only message.

```ts
pi.on("message_end", async (event, ctx) => {
  if (!currentConfig.value.enabled) return;
  if (currentConfig.value.pruneOn !== "agent-message") return;
  if (!isFinalAssistantMessage(event.message)) return;
  await flushPending(ctx, { delivery: "session" });
});
```
`pi-context-prune/index.ts:533-538`

- **agentic-auto**: the model calls the `context_prune` tool; the tool is mounted only in this mode.

```ts
const shouldActivate = currentConfig.value.enabled && currentConfig.value.pruneOn === "agentic-auto";
...
pi.setActiveTools([...activeTools, CONTEXT_PRUNE_TOOL_NAME]);
```
`pi-context-prune/index.ts:414-419`

- **on-demand**: only `/pruner now` calls `flushPending`.

There is **no token-budget trigger** in `pi-context-prune`. It does not call `ctx.getContextUsage()`. The only gate is the event/mode.

Default inconsistency worth knowing: `DEFAULT_CONFIG.pruneOn` is `"agentic-auto"` (`src/types.ts:203`), but the entry point initializes the in-memory config with `"every-turn"` before `session_start` loads settings (`index.ts:45`). On a normal fresh install, `loadConfig()` returns the default and the effective mode is `"agentic-auto"`; the `"every-turn"` value only matters for a turn that starts before `session_start`. Report the persisted default, not the initializer.

Batching is `"agent-message"` by default (`src/types.ts:206`), which merges all consecutive tool batches sharing a `userTurnGroup` (`src/batch-capture.ts:201-233`). That is a span, not a token threshold.

### 3.2 `pi-condense`: flush boundaries plus optional budget/delta/frontier triggers

The default is still a boundary, not a percentage:

```ts
pruneOn: "agent-message",
batchingMode: "turn",
...
autoBudgetThreshold: null,
...
budgetTurnDelta: null,
frontierGapThresholdTokens: null,
```
`pi-condense/src/types.ts:562-594`

The agent-message flush:

```ts
pi.on("message_end", async (event, ctx) => {
  if (!currentConfig.value.enabled) return;
  if (currentConfig.value.pruneOn !== "agent-message") return;
  if (!isFinalAssistantMessage(event.message)) return;
  await flushPending(ctx, { delivery: "session", closingMessage: event.message, trigger: "message-end" });
});
```
`pi-condense/index.ts:1023-1027`

At `turn_end`, it computes three optional gates:

```ts
const usage = ctx.getContextUsage?.();
const budgetHit = shouldBudgetFlush(usage, currentConfig.value.autoBudgetThreshold);
const deltaHit = shouldDeltaFlush(usage, previousFraction, currentConfig.value.budgetTurnDelta);
...
const gapHit = gapThreshold != null && shouldFrontierGapFlush(computeMetricsSnapshot(ctx), gapThreshold);
...
if ((n > 0 || rearmedPending) && !isFlushing && (budgetHit || deltaHit || gapHit)) {
```
`pi-condense/index.ts:985-998`

The budget test is a threshold on **estimated context tokens**, capped at 300k:

```ts
return usage.tokens >= Math.min(MAX_BUDGET_WINDOW, threshold * usage.contextWindow);
```
`pi-condense/src/budget.ts:18-25`, with `MAX_BUDGET_WINDOW = 300_000` at `:10`.

`usage.tokens` is pi’s `ContextUsage`, documented as “Estimated context tokens” (`pi/packages/coding-agent/src/core/extensions/types.ts:306-312`). Pi computes it from the last assistant’s provider usage plus a chars/4 estimate for trailing messages (`pi/packages/coding-agent/src/core/compaction/compaction.ts:227-262`, `pi/packages/ai/src/utils/estimate.ts:97-107`). It is **hybrid**, not pure provider-reported, and it is `null` right after compaction until a post-compaction assistant responds.

This is a **trigger, not enforcement**. It runs at `turn_end`; a failed or slow flush leaves the next request above the threshold. None of the three packages enforces a hard per-request ceiling. That is why the course invariant (“a request must never exceed its budget”) is stronger than any surveyed implementation and must be checked before send, not inferred from a flush trigger.

Pre-flush guards that are also triggers/anti-triggers:
- `minBatchChars: 2000`: below this raw-char count, no summarizer call; the batch is skipped and the frontier advances (`index.ts:361-380`; `src/types.ts:570`).
- `spillThreshold: 65536`: a single oversized result is spilled at `turn_end` before the flush (`index.ts:936-950`; `src/types.ts:589`).
- `chainCompression.rollingWindow: 3`: only closed chains older than the newest 3 are compressed (`src/types.ts:576-581`; `src/chain-compressor.ts:55-63`).
- `recoveryGraceTurns: 3`: a `context_tree_query` output stays verbatim for 3 user-turn groups, then re-stubs (`src/types.ts:571`; `src/recovery-grace.ts:23-41`).

### 3.3 `toolflow`: per-tool-result thresholds, stage boundaries, prompt keywords

Toolflow has no token budget. Its “prune” triggers are structural.

**Log dehydration** happens on every matching `tool_result`, when the sanitized output crosses a per-tool line **or** byte threshold:

```ts
let thresholdLines = 40;
let thresholdBytes = 2500;
let headCount = 15;
let tailCount = 10;

if (toolName === "grep" || toolName === "find") {
  thresholdLines = 60; thresholdBytes = 4000; headCount = 25; tailCount = 15;
} else if (toolName === "read") {
  thresholdLines = 600; thresholdBytes = 25000; headCount = 150; tailCount = 50;
} else if (toolName === "bash" || toolName === "powershell") {
  thresholdLines = 35; thresholdBytes = 2200; headCount = 12; tailCount = 12;
} else if (toolName === "fetch_content" || toolName === "web_search" || toolName === "get_search_content") {
  thresholdLines = 25; thresholdBytes = 1500; headCount = 10; tailCount = 8;
} ...
if (lines.length <= thresholdLines && sanitized.length < thresholdBytes) {
  return { dehydrated: false, text: sanitized };
}
```
`toolflow/src/dehydrator.ts:442-487`

So the README’s “>40 lines” is only the generic default; bash is 35 lines/2,200 chars, grep/find 60/4,000, read 600/25,000. The condition is OR, not AND.

There are also non-size triggers:
- interactive prompts bypass dehydration (`INTERACTIVE_PATTERNS`, `src/dehydrator.ts:37-46`, `:406-411`);
- a repeated identical command within 45s with zero workspace mutations is replaced by a “Loop Buster” message (`src/dehydrator.ts:414-432`).

**Tool-schema pruning** is stage-driven. It fires when a `/toolflow` blueprint starts or advances, not on a token threshold. Stage advance is gated on physical artifact verification:

```ts
const hasNext = advanceStage(turnCwd);
...
const pruned = degradationMatrix.resolvePrunedToolsForStage(nextStage.stageId, nextStage.allowedTools);
applyToolScoping(pruned.allowedTools, pi);
```
`toolflow/src/index.ts:782-795`

**Heavy-schema removal** is a separate per-request trigger, active only when no blueprint is in progress:

```ts
if (!payload || !Array.isArray(payload.tools) || payload.tools.length <= 10) return;
...
const needsHeavy = /(https?:\/\/|www\.|chrome|browser|web_search|search|crawl|subagent|delegate|supervisor|advisor|council|image_describe|screenshot)/i.test(promptText);
if (needsHeavy) return;
payload.tools = payload.tools.filter((t) => !HEAVY_TOOL_NAMES.has(t.name || t.function?.name || ""));
```
`toolflow/src/context_slimmer.ts:35-63`

This is a fixed-name list (`HEAVY_TOOL_NAMES`, `:7-18`) and a prompt keyword check. It is not usage-based, and it does not update the active tool set; it only edits that request’s payload.

**Read cache** fires on a duplicate `read` only if all six gates pass: no explicit `offset`/`limit`, no recent edit failure, file >5 lines and >=200 chars, no command within 15s, cached mtime/hash match, and 1–15 turns since last read (`src/read_cache.ts:129-176`). On a hit, the tool result is replaced with a notice + symbol outline (`:183-202`).

## 4. Algorithm: what is replaced, in what order, and how adjacency is preserved (Q3)

### 4.1 `pi-context-prune`: summarize, then delete results

Capture one batch from `turn_end` (`event.message` + `event.toolResults`), attach the full `resultText` to each call (`src/batch-capture.ts:8-50`), filter the pruner’s own tool and (in agentic mode) the `context_prune` result, queue it, and flush according to mode.

Flush per batch:
1. Call the summarizer (one call per batch, parallel unless progress UI).
2. If `summaryText.length > batchRawCharCount`, skip: keep raw results, advance frontier (`index.ts:288-299`).
3. Otherwise allocate short refs `t1…`, append a hidden summary `custom_message` with `<context-prune-summary>` wrapper and a ref footer (`index.ts:287-310`; `src/summary-refs.ts:54-69`).
4. Persist a `context-prune-index` custom entry containing the **full** `resultText` (`src/indexer.ts:120-138`).
5. On every `context` event, delete every `toolResult` whose id is in the index:

```ts
export function pruneMessages(messages: any[], indexer: ToolCallIndexer): any[] {
  return messages.filter((msg) => {
    if (msg.role === "toolResult" && indexer.isSummarized(msg.toolCallId)) {
      return false;
    }
    return true;
  });
}
```
`pi-context-prune/src/pruner.ts:8-15`

What is replaced: nothing is replaced; the result message disappears. The assistant `toolCall` block remains. There is no protected recent tail: in `every-turn` it prunes the just-completed turn; in `agentic-auto` it prunes every pending batch the model chooses. The only “tail” is whatever has not been flushed yet.

Adjacency: **not preserved by the extension.** It relies on pi-ai to repair the orphan. Pi-ai does exactly that, but the repair is a synthetic error:

```ts
// Second pass: insert synthetic empty tool results for orphaned tool calls
...
result.push({
  role: "toolResult",
  toolCallId: tc.id,
  toolName: tc.name,
  content: [{ type: "text", text: "No result provided" }],
  isError: true,
  timestamp: Date.now(),
});
```
`pi/packages/ai/src/api/transform-messages.ts:179-198`

So the model sees a fake failed tool result for every pruned result. This is the single most important negative example for the course.

In `agentic-auto` mode it also appends a `<pruner-note>` to the last tool result with the count of unpruned calls (`src/reminder.ts:56-58`). That is a cadence reminder, not a per-pruned-result marker; the individual deleted result is still unmarked.

### 4.2 `pi-condense`: five ordered phases

`pruneMessages` is the transform applied to the outgoing `context` event. Its own doc comment gives the order (`src/pruner.ts:24-80`), and the code follows it:

1. **Phase 1 — stub-replace.** For each `toolResult` whose occurrence key is indexed (or legacy bare id is unambiguous), replace content with a stub. It does **not** remove the message. Protected records and recovery-grace records are skipped. (`src/pruner.ts:91-146`)
2. **Phase 1b — supersede.** Older protected reads of a path that is read again later become a one-line “superseded” stub, but only once the pruner’s rewrite floor is at/before them. (`src/pruner.ts:150-157`; `src/supersede.ts`)
3. **Phase 2 — argument purge.** Shrink summarized `toolCall` arguments, and failed `toolCall` arguments after a cooldown. Unsummarized error results stay visible. (`src/pruner.ts:159-166`; `src/error-purge.ts`)
4. **Phase 3 — chain range prune.** For each eligible chain, drop the middle assistant + toolResult + per-batch summary messages and insert one synthetic `<compressed-chain>` user message. (`src/pruner.ts:168-195`; `src/chain-range-prune.ts:103-242`)
5. **Phase 4 — orphan sweep.** Unconditionally remove any `toolResult` whose matching call is not open, where “open” is per-turn and cleared by any barrier message. Reference-preserving when nothing is swept. (`src/pruner.ts:197-216`; `src/orphan-sweep.ts:15-49`)

**Protected recent tail.** There is no fixed “keep last N messages” window for per-batch pruning. The effective tail is:
- batches not yet flushed (the queue);
- recovery outputs inside `recoveryGraceTurns` (default 3 user-turn groups);
- protected tools/paths (default paths `**/skills/**/*.md`, `**/gauntlet-overrides.md`);
- the newest `rollingWindow` (3) closed chains for chain compression.

Chain eligibility:

```ts
const candidates = chains.filter(
  (c) =>
    c.finalAssistantTimestamp !== null &&
    !alreadyCompressed.has(c.startUserTimestamp) &&
    c.middleToolCallIds.length > 0,
);
const toCompress = candidates.slice(0, Math.max(0, candidates.length - rollingWindow));
```
`pi-condense/src/chain-compressor.ts:55-63`

A chain is `[user or eligible custom] → [assistant+toolResult turns…] → [text-only assistant]` (`src/chain-detector.ts:41`). Only closed chains are eligible; an open chain at the end is dropped (`:132`).

**Adjacency.** Phase 1 replaces in place, so the `toolCall` and `toolResult` both remain. Phase 3 drops whole ranges and role-restricts what may be dropped:

```ts
const isSummary = isPerBatchSummaryMessage(msg);
const droppable = msg.role === "assistant" || msg.role === "toolResult" || isSummary;
if (dropIndices.has(i) && droppable) continue;
```
`pi-condense/src/chain-range-prune.ts:229-231`

User messages and third-party custom messages inside the range are preserved (`:222-225`), and the synthetic chain message is inserted after the start anchor (`:239-240`). Phase 4 then sweeps any leftover orphan result. Tool-call ids/names/pairing stay unchanged in the stub path (`src/pruner.ts:78-79`).

**Summarizer.** One LLM call per batch; the system prompt asks for per-tool bullets and preserves paths/identifiers/error strings verbatim (`src/summarizer.ts:14-30`). The summary is injected as a hidden `custom_message` (`display: false`), which still enters future context (`index.ts:527-533`). A failed summary leaves the batch queued for retry; processed batches are committed in order (`index.ts:606-616`).

### 4.3 `toolflow`: replace in place, fold, or unmount

Toolflow has three different algorithms depending on mechanism:

- **Dehydration** (tool output): sanitize ANSI, bypass interactive waits, check the loop buster, choose per-tool thresholds, write the full sanitized text to `.pi/toolflow/runs/<blueprint>/tool_<name>_<ts>.log`, then replace the text block with head + extracted error lines + optional tail + a marker containing the relative path and `toolflow_inspect` hint. (`src/dehydrator.ts:400-553`)
- **Read cache**: on a duplicate, replace the text block with a notice, symbol outline, and line count; tell the model to re-read with `offset`/`limit` if it needs the body. (`src/read_cache.ts:183-202`; the bypass is at `:129-131`)
- **Historical slimming**: for messages older than the last 8, truncate `role: "tool"` content >800 chars and >12 lines to head 4 + tail 2, and replace large `write`/`edit` argument bodies with “[… lines already written to disk]”. (`src/context_slimmer.ts:181-251`). As noted in §0, this checks a schema pi does not use, so its real effect on pi is doubtful.

Adjacency is preserved in the dehydration/read-cache paths because they replace content inside the existing `toolResult`; they do not remove messages. The historical slimmer also does not remove messages. Schema pruning is the only path that can make a call unexecutable, and that is a different axis (§7).

## 5. Stub format, model guidance, and how recovery actually works (Q4)

| Package | Literal stub / marker | Is the model told? | Recovery mechanism | What recovery returns |
|---|---|---|---|---|
| `pi-context-prune` | **None.** The `toolResult` is deleted. A separate summary message ends with: `**Summarized tool refs**: \`t1\`, …\nUse \`context_tree_query\` with these refs to retrieve the original full outputs.` (`src/summary-refs.ts:54-59`) | Only in the separate summary, not at the pruned position | `context_tree_query` registered tool, params `toolCallIds: string[]` (`src/query-tool.ts:6-20`) | Markdown header (`## toolRef`, Tool, Args, Status, Turn) + original text, head-truncated to 2,000 lines/50 KB (`:46-54`); no images, no spill |
| `pi-condense` | `[Summarized in pruner summary, ref \`${ref}\`. Use context_tree_query to retrieve full output.]` (`src/pruner.ts:137`). For spilled: `[Oversized output spilled to file — ${record.spillBytes ?? "?"} bytes.]` + Tool + `Preview (head):` + `Full output — read this file (offset/limit supported): ${record.spillPath}` + `Or use context_tree_query with ref \`${ref}\`.` (`:129-136`) | Yes: stub itself, plus summary ref footer (`src/summary-refs.ts:54-59`) and the query tool’s prompt guidelines (`src/query-tool.ts:26-28`) | `context_tree_query`; resolves occurrence key, dedup alias, short ref `tN`, or unambiguous bare id (`src/indexer.ts:236-264`); reads spill file if present (`src/query-tool.ts:73-81`) | Same markdown header + original text and original image blocks; per-answer truncation to 2,000 lines/50 KB (`:88-96`); reused bare ids return every occurrence labelled `id@timestamp` (`:55-60`) |
| `toolflow` | Line mode: `... [⚡ ToolFlow Token Optimizer: Dehydrated ${omittedCount} lines (~${approxTokens} tokens) to ${relPath}] [Hint: Call toolflow_inspect(id="${relPath}", offset=1, limit=50) to inspect details without context bloat] ...` (`src/dehydrator.ts:542`). Char mode: same prefix with `${omittedChars} chars` (`:514`). Read cache: `=== [⚡ ToolFlow Read Cache: "${relativePath}" (${lines.length} 行, 与第 ${lastTurn} 轮内容一致) ] ===` + `=== [⚡ 提示: 内容完全一致且未变更。若需查看或精准修改实现，请调用 read 传入 offset=<行号> limit=<行数> 按需读取] ===` (`src/read_cache.ts:183-185`) | Yes: stub includes the hint; `before_agent_start` also appends `[ToolFlow]: Large tool outputs are archived in .pi/toolflow/runs/; use toolflow_inspect to search or slice full logs.` (`src/index.ts:556-560`) | `toolflow_inspect` registered tool (`src/inspect_tool.ts:195-209`); `id`, `findText`, `offset`, `limit` | Bounded line slice or grep matches from the archived log, max 200 lines, each line clamped to 500 chars (`src/inspect_tool.ts:110-183`). It is paging, not one-shot full recovery |

**`context_tree_query` is an index lookup, not a tree traversal.** Despite the name, it does not walk pi’s session tree. It looks up an in-memory `Map` that was rebuilt from `context-prune-index` custom entries, then returns formatted text (`pi-condense/src/query-tool.ts:47-107`; `src/indexer.ts:64-148`). `pi-context-prune` has the same design with a simpler map (`src/indexer.ts:19-41`, `src/query-tool.ts:26-57`). The “tree” is only the session branch used to rebuild the index.

**“Any original” is bounded.** `pi-condense` only indexes batches that were summarized or spilled while enabled. Protected tools/paths are skipped at capture and re-checked at render, so they are not stubbed (an indexed record can survive a later protection-config change, but the raw result is restored at render). Batches skipped as trivial (`< minBatchChars`) are not indexed. A batch whose summary would be larger than the raw text is skipped and not indexed (`index.ts:517-528`). `context_tree_query` truncates each result. So the accurate claim is: “any indexed original, up to the tool’s display limit; full spill files can be read with the normal `read` tool.”

**Recovery grace.** `pi-condense` keeps the *output of the recovery tool itself* verbatim for `recoveryGraceTurns` (default 3) user-turn groups, then re-stubs it. This prevents an immediate retrieve→stub→retrieve loop from permanently pinning the original (`src/recovery-grace.ts:23-41`; `src/types.ts:571`).

## 6. Session interaction: stored log vs outgoing request (Q5)

| Question | `pi-context-prune` | `pi-condense` | `toolflow` |
|---|---|---|---|
| Does pruning mutate the stored session messages? | **No.** `context` handler returns a filtered array; session messages are untouched (`index.ts:550-588`, `src/pruner.ts:8-15`) | **No.** Phase 1 maps to new `toolResult` objects; chain compression builds a new array (`src/pruner.ts:95-146`, `src/chain-range-prune.ts:226-242`) | **Yes for dehydration and read cache.** `tool_result` handler mutates `block.text` in place (`src/index.ts:667-693`); pi passes the same content array into `_afterToolCall`, so the replacement is what gets persisted (`pi/packages/coding-agent/src/core/agent-session.ts:684-723`; `pi/packages/coding-agent/src/core/extensions/runner.ts:1190-1240`). Historical `context` slimming is request-only. |
| What persistent records does it add? | `context-prune-summary` (`custom_message`), `context-prune-index` (`custom`), `context-prune-stats`, `context-prune-frontier` (`src/types.ts:48-62`; `index.ts:303-310`, `src/indexer.ts:120-138`) | Same plus `context-prune-dedup-alias`, `context-prune-chain`, `context-prune-diagnostic`, `context-prune-flush-metrics` (`src/types.ts:50-106`); summary is `custom_message`, index/chain/alias are `custom` | Dehydrated `.log` files under `.pi/toolflow/runs/`; stats `.pi/toolflow/...`; blueprint state `.pi/blueprint_state.json` (`src/dehydrator.ts:70-78`, `src/state.ts:25-35`). Read cache and latest-log pointer are in-memory only. |
| Is the original recoverable after process exit? | **Yes if the session is resumed and the index entry is on the branch.** Rebuild on `session_start`/`session_tree` (`index.ts:428-457`, `src/indexer.ts:19-41`). Raw result text is in the custom entry, not only in memory. | **Yes for indexed originals if the session/sidecar survives.** Rebuild from `getBranch()` on start/tree (`index.ts:796-875`, `src/indexer.ts:64-148`). Spill files are on disk (`src/spill.ts:95-105`). If the session file is deleted or the sidecar is pruned, no. | **Yes for dehydrated logs until retention deletes them.** Retention: 50 files / 7 days / 30 MB per run (`src/dehydrator.ts:126-166`) and 10 runs / 7 days / 200 MB (`:172-214`). Read-cache originals are not stored; they are the live file on disk, and the cache is cleared on session start (`src/index.ts:502-505`, `src/read_cache.ts:237-250`). |
| How does it coexist with pi compaction? | No `session_compact` hook. Index/summary live in raw branch custom entries; pi’s model projection may omit older messages, but `getBranch()` still returns all entries (`pi/packages/coding-agent/src/core/session-manager.ts:1466-1469`, `:469-510`). | `session_compact` only resets the supersede floor (`index.ts:878-880`). Index is reconstructed from the raw branch; compaction’s `firstKeptEntryId` changes what the model sees, not what `getBranch()` returns. | `session_before_compact` returns an ignored shape (§0). Actual compaction influence is `ctx.compact({ customInstructions })` after stage verification (`src/index.ts:774-779`). Toolflow has no `session_tree` handler; stage state is in `.pi/blueprint_state.json`, not branch-derived. |
| How does it coexist with session tree / branch switch? | `session_tree` rebuilds index/stats/frontier and clears pending batches (`index.ts:453-457`). | `session_tree` rebuilds index, block refs, stats, frontier, clears pending/supersede (`index.ts:852-875`). | No `session_tree`; only `session_start` on `new`/`clear` resets and restores baseline (`src/index.ts:502-513`). A resumed session loads blueprint state at module load (`:61`) but does not re-apply stage tool scoping, so active tools can desync from the persisted stage. |

Pi’s own compaction explicitly never cuts at a tool result: “Never cut at tool results (they must stay with their tool call)” (`pi/packages/coding-agent/docs/compaction.md:136`). That is a useful external confirmation that result-only deletion is the wrong primitive.

`toolflow`’s in-place mutation is not inherently wrong, but it changes the contract: the session no longer contains the raw output. Recovery depends on the sidecar log and its retention policy. That is a different durability model from `pi-condense`/`pi-context-prune`, which leave the raw session messages intact and only rewrite the outgoing view.

## 7. Tool-schema pruning (`toolflow`) — its own axis (Q6)

### 7.1 What decides the mounted set

A `/toolflow <task>` run synthesizes a blueprint. Each stage has `allowedTools` (`toolflow/src/types.ts:204`; stage construction at `src/engine.ts:1272-1430`). Heavy tools are allocated either by the LLM’s `stageHeavyTools` plan or by heuristics:

```ts
const llmAlloc = llmArtifactPlan?.stageHeavyTools;
const stage1Tools = ["read", "bash", "powershell", "grep", "find"];
const stage2Tools = ["read", "edit", "write", "bash", "powershell", "grep", "find"];
const stage3Tools = ["read", "bash", "powershell", "grep", "find"];

if (llmAlloc && typeof llmAlloc === "object") {
  if (Array.isArray(llmAlloc.stage1)) {
    llmAlloc.stage1.forEach(t => { if (hasTool(t) && !stage1Tools.includes(t)) stage1Tools.push(t); });
  }
  ...
} else {
  if (l2PerceptionExts.length > 0 || hasTool("web_search") || ...) {
    ["web_search", "fetch_content", "source_check"].forEach(t => { if (hasTool(t)) stage1Tools.push(t); });
  }
  ...
}
```
`toolflow/src/engine.ts:1187-1227`

So the decision is **LLM plan + taxonomy layer heuristics + hard-coded stage baselines**, not observed usage. `GracefulDegradationMatrix.resolvePrunedToolsForStage` adds a second, stage-kind heuristic keyed on the stage id string: design allows `read/grep/find/ls/write/web_search/fetch_content` and blocks `edit/bash/powershell`; preview allows read/search plus `mcp` and blocks `edit/write`; test allows `bash/powershell/write` and blocks `edit`; code allows the broad set (`src/degradation_matrix.ts:132-179`).

### 7.2 Mount/unmount mechanics

`applyToolScoping` filters the requested names against registered tools, then calls `ctx.setActiveTools`:

```ts
export function applyToolScoping(allowedTools: string[], ctx: any): void {
  const merged = computeStageTools(allowedTools);
  if (typeof ctx?.setActiveTools === "function") {
    if (typeof ctx?.getAllTools === "function") {
      const registeredTools = ctx.getAllTools();
      const registeredNames = new Set(...);
      if (registeredNames.size > 0) {
        const safeMerged = merged.filter(t => registeredNames.has(t));
        ctx.setActiveTools(safeMerged.length > 0 ? safeMerged : Array.from(registeredNames));
        return;
      }
    }
    ctx.setActiveTools(merged);
  }
}
```
`toolflow/src/state.ts:823-839`

Pi’s contract: active set = tools declared to the model; unknown names are ignored (`pi/packages/coding-agent/docs/extensions.md:277`, `:299`). `setActiveTools` rebuilds the system prompt for the next turn (`pi/packages/coding-agent/src/core/agent-session.ts:1533-1550`).

Mount points:
- On blueprint start: record initial active set, then apply stage 1’s pruned set (`toolflow/src/index.ts:204-217`).
- On stage advance: after `verifyStageArtifacts` succeeds, `advanceStage`, then apply the next stage’s set (`:782-795`).
- On completion: restore the initial snapshot (`:864`).
- On `/toolflow reset` or `session_start` reason `new`/`clear`: reset state and apply `BASELINE_TOOLS` (`:89-91`, `:258`, `:502-513`).

There is no per-tool “request remount” path. A stage transition is the only normal remount. If the model needs a tool that is not in the stage set, it must complete the current stage artifact, reset, or ask the user; the tool itself is absent from the schema.

Two mechanical hazards in `applyToolScoping`:
- If every requested name is unregistered, it falls back to **all registered tools** (`safeMerged.length > 0 ? safeMerged : Array.from(registeredNames)`). A typo in `allowedTools` can therefore mount everything, the opposite of pruning.
- `computeStageTools([])` returns `BASELINE_TOOLS`, so an empty stage list is not “mount nothing”; it is “mount the baseline” (`src/state.ts:818-821`).

### 7.3 What the model sees if it calls an unmounted tool

Toolflow does not intercept this itself. Pi’s tool loop looks up the call in the active tool array and returns an error result:

```ts
const tool = tools.find((t) => t.name === toolCall.name);
if (!tool) {
  return {
    kind: "immediate",
    result: createErrorToolResult(`Tool ${toolCall.name} not found`),
    isError: true,
  };
}
```
`pi/packages/agent/src/agent-loop.ts:726-733`

So the model sees a tool result with `Tool <name> not found` and `isError: true`. There is no “this tool is available in stage N” hint and no automatic remount. That is a real deadlock risk if the stage policy omitted a capability needed to produce the stage artifact.

### 7.4 Request-level heavy-schema removal

Separate from `setActiveTools`, `before_provider_request` removes fixed heavy names from the outgoing payload when:
- no blueprint is in progress,
- the payload has more than 10 tools,
- the last user prompt does not match the heavy-tool keyword regex.

Code at `src/context_slimmer.ts:24-76`. This is a per-request filter, not a persistent unmount. If the prompt mentions a URL, “browser”, “search”, “subagent”, etc., everything is preserved. It does not look at tool usage, token counts, or stage.

### 7.5 Review isolation is a runtime veto, not schema pruning

`ReviewIsolationGuard` blocks calls to tools outside its allowlist during review stages:

```ts
if (reviewGuard.isActive() && !reviewGuard.isToolAllowedInReview(event.toolName)) {
  return {
    block: true,
    reason: `[ToolFlow Review Isolation] Tool "${event.toolName}" is blocked during read-only review stage.`,
    terminate: false
  };
}
```
`toolflow/src/index.ts:602-611`; allowlist at `src/review_isolation.ts:169-186`.

This is a `tool_call` block, so the schema may still declare the tool. In the Plan B stage 5, `allowedTools` includes `write` (`src/engine.ts:1430`) while the stage is marked `isReviewStage` and activates the review guard (`src/index.ts:788-794`). `write` is declared but blocked. The README’s “write tools physically stripped” is therefore only true when the stage’s `allowedTools` also omits them; the guard itself is a runtime veto.

## 8. Safety hooks (`toolflow`) (Q7)

**Interception point.** `pi.on("tool_call", ...)` runs after argument validation and before execution. A handler can mutate `event.input` or return `{ block: true, reason, terminate }` (`pi/packages/coding-agent/docs/extensions.md` “tool_call can mutate input or block execution”; `pi/packages/agent/src/agent-loop.ts:738-765`). Toolflow returns:

```ts
return {
  block: true,
  reason: check.reason,
  terminate: false
};
```
`toolflow/src/index.ts:604-611` (review) and `:617-623` (blast guard).

**Can it veto?** Yes. Pi turns the block into an error tool result whose text is the reason (`pi/packages/agent/src/agent-loop.ts:755-764`). The model sees the reason and continues (because `terminate: false`); it may retry a different path. The reason strings are mostly Chinese (`[ToolFlow 保护红线] …`), which is a UX/i18n choice, not a technical limit.

**What is protected.** The guard has two layers:

1. **Direct write-tool path check.** For tools in `WRITE_TOOLS` (`write`, `edit`, `create`, `delete`, `unlink`, `append`, `patch`, `modify`, `save`, `overwrite`), it resolves the path against cwd and blocks:
   - paths outside the workspace / drive traversal / `..` segments (`src/blast_radius.ts:221-228`);
   - any path segment `.git` or DOS 8.3 `git~N` (`:230-236`);
   - basenames matching `.env*`, `.git*`, `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `Cargo.lock`, `bun.lockb`, `pnpm-workspace.yaml`, `turbo.json`, `lerna.json`, `nx.json`, and DOS 8.3 aliases (`:42-58`, `:238-249`);
   - NTFS alternate data streams and DOS device names (`:190-219`).
2. **Shell path check.** For `bash`/`powershell`/`exec`, it first blocks destructive Git commands (`git checkout .`, `git restore .`, `git reset --hard`, `git clean -f*`) (`:286-291`), then, if the command contains a write/delete operator (`>`, `>>`, `rm`, `del`, `mv`, `cp`, `sed -i`, `Set-Content`, `truncate`, …), tokenizes and blocks any token whose path segment is `.git` or whose basename matches the protected list (`:292-323`).

**What it does not do.**
- It is an accident guard, not a sandbox; the code says so (`src/blast_radius.ts:12-15`).
- It only checks `WRITE_TOOLS`; `create_file` is not in that set even though the read-cache code treats it as a mutation (`src/index.ts:644`). A tool named `apply_patch` or `create_file` would bypass the path guard.
- The shell check only runs when its destructive-token regex matches. `find . -name .env -delete`, `python -c "open('.env','w')"`, `node -e ...`, `base64 -d | sh`, and environment-variable indirection are not matched.
- The stage-scope allowlist is advisory by default: `strictArtifactScope` is `false` and never set by the extension (`src/blast_radius.ts:33-37`; no caller in `src/`). Out-of-scope writes are silently added to `allowedExactPaths` (`:265-278`). Only the explicit test calls `setStrictArtifactScope(true)` (`tests/test_suite.ts:554`).
- It blocks legitimate writes to `package-lock.json` even when the task is dependency maintenance; there is no per-stage exception.

For the course, this is a useful example of a **best-effort veto at the tool boundary**: explicit, model-visible, fail-closed for the cases it recognizes, but not a security boundary.

## 9. Minimal teaching model in Go

The smallest honest design is **in-place tool-result stubbing with a recoverable store**, not message deletion and not LLM summarization. It satisfies the four chapter invariants by construction:

- budget: estimate the whole request, prune oldest eligible results, then refuse to send if still over;
- no orphan: never remove a `tool_use` or a `tool_result`; replace the result’s text only;
- system prompt: keep `System` outside `Messages`; `Prune` cannot touch it;
- marked: every replacement contains a sentinel and a ref; every ref resolves to the exact original.

```go
package ctxbudget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Tokens int

// Budget is the hard ceiling for the whole request, including system, messages,
// and tool schemas. Reserve is output headroom the provider needs.
type Budget struct {
	Limit   Tokens
	Reserve Tokens
}

func (b Budget) Usable() Tokens {
	if b.Limit <= b.Reserve {
		return 0
	}
	return b.Limit - b.Reserve
}

// Policy is deliberately tiny: no summarizer, no chain logic, no schema policy.
type Policy struct {
	KeepRecentTurns int    // number of newest assistant turns never pruned
	MinRawTokens    Tokens // don't stub tiny results; the stub would be noise
}

func DefaultPolicy() Policy {
	return Policy{KeepRecentTurns: 2, MinRawTokens: 200}
}

const StubSentinel = "[ctx-pruned]"

type ToolSchema struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type Block struct {
	Type      string // "text" | "tool_use" | "tool_result"
	Text      string
	ToolUseID string
	ToolName  string
	Input     json.RawMessage
}

type Message struct {
	Role   string // "user" | "assistant" | "tool"
	Blocks []Block
}

type Request struct {
	System   string
	Messages []Message
	Tools    []ToolSchema
}

type Ref string

type Stored struct {
	Ref        Ref
	ToolCallID string
	ToolName   string
	Text       string
	Bytes      int
	SHA256     string
}

type Store interface {
	Put(Stored) (Ref, error)
	Get(Ref) (Stored, bool)
}

type MemStore struct {
	byRef map[Ref]Stored
}

func NewMemStore() *MemStore { return &MemStore{byRef: map[Ref]Stored{}} }

func (s *MemStore) Put(e Stored) (Ref, error) {
	sum := sha256.Sum256([]byte(e.ToolCallID + "\x00" + e.Text))
	ref := Ref("r_" + hex.EncodeToString(sum[:8]))
	e.Ref = ref
	e.SHA256 = hex.EncodeToString(sum[:])
	s.byRef[ref] = e // idempotent: same input -> same ref -> same record
	return ref, nil
}

func (s *MemStore) Get(ref Ref) (Stored, bool) {
	e, ok := s.byRef[ref]
	return e, ok
}

// Placeholder is the only thing the model sees in place of the original.
type Placeholder struct {
	Ref           Ref
	ToolName      string
	OriginalBytes int
}

func (p Placeholder) Render() string {
	return fmt.Sprintf(
		"%s tool=%s bytes=%d ref=%s; call recover(ref) for the full original output",
		StubSentinel, p.ToolName, p.OriginalBytes, p.Ref,
	)
}

type Result struct {
	Messages   []Message
	Pruned     int
	Refs       []Ref
	Estimated  Tokens
	OverBudget bool
}

var ErrPairing = errors.New("tool_use/tool_result pairing violated")

// Estimate is a teaching approximation. Production must anchor on the last
// provider-reported usage and only estimate the trailing messages, then add
// output reserve; chars/4 alone diverges from real tokenizers.
func Estimate(r Request) Tokens {
	n := len(r.System)
	for _, m := range r.Messages {
		for _, b := range m.Blocks {
			n += len(b.Text) + len(b.Input) + len(b.ToolName) + len(b.ToolUseID)
		}
	}
	for _, t := range r.Tools {
		n += len(t.Name) + len(t.Description) + len(t.Parameters)
	}
	return Tokens((n + 3) / 4)
}

func pairingOK(msgs []Message) error {
	calls := map[string]bool{}
	results := map[string]bool{}
	for _, m := range msgs {
		for _, b := range m.Blocks {
			switch b.Type {
			case "tool_use":
				calls[b.ToolUseID] = true
			case "tool_result":
				if !calls[b.ToolUseID] {
					return fmt.Errorf("%w: result %s before call", ErrPairing, b.ToolUseID)
				}
				if results[b.ToolUseID] {
					return fmt.Errorf("%w: duplicate result %s", ErrPairing, b.ToolUseID)
				}
				results[b.ToolUseID] = true
			}
		}
	}
	for id := range calls {
		if !results[id] {
			return fmt.Errorf("%w: call %s has no result", ErrPairing, id)
		}
	}
	return nil
}

func cloneMessages(in []Message) []Message {
	out := make([]Message, len(in))
	for i, m := range in {
		out[i] = Message{Role: m.Role, Blocks: append([]Block(nil), m.Blocks...)}
	}
	return out
}

func Prune(req Request, store Store, pol Policy, budget Budget) (Result, error) {
	if err := pairingOK(req.Messages); err != nil {
		return Result{}, err
	}

	msgs := cloneMessages(req.Messages)

	// Protect the newest N assistant messages and everything after them.
	// A tool-using assistant's results follow it; starting the protected region at
	// the Nth-newest assistant keeps that call and all of its results on the
	// protected side. KeepRecentTurns <= 0 protects nothing.
	var turnStarts []int
	for i, m := range msgs {
		if m.Role == "assistant" {
			turnStarts = append(turnStarts, i)
		}
	}
	protectFrom := len(msgs)
	if pol.KeepRecentTurns > 0 && len(turnStarts) > 0 {
		idx := len(turnStarts) - pol.KeepRecentTurns
		if idx < 0 {
			idx = 0
		}
		protectFrom = turnStarts[idx]
	}

	res := Result{Messages: msgs}
	for i := 0; i < protectFrom; i++ {
		for j := range msgs[i].Blocks {
			b := &msgs[i].Blocks[j]
			if b.Type != "tool_result" || strings.HasPrefix(b.Text, StubSentinel) {
				continue
			}
			if Tokens(len(b.Text)/4) < pol.MinRawTokens {
				continue
			}
			origLen := len(b.Text)
			ref, err := store.Put(Stored{
				ToolCallID: b.ToolUseID,
				ToolName:   b.ToolName,
				Text:       b.Text,
				Bytes:      origLen,
			})
			if err != nil {
				return Result{}, err
			}
			b.Text = Placeholder{
				Ref: ref, ToolName: b.ToolName, OriginalBytes: origLen,
			}.Render()
			res.Pruned++
			res.Refs = append(res.Refs, ref)
		}
	}

	res.Estimated = Estimate(Request{System: req.System, Messages: msgs, Tools: req.Tools})
	res.OverBudget = res.Estimated > budget.Usable()
	return res, nil
}

func Recover(store Store, ref Ref) (string, error) {
	e, ok := store.Get(ref)
	if !ok {
		return "", fmt.Errorf("unknown ref %q", ref)
	}
	return e.Text, nil
}
```

**What this model deliberately removes.**
- **LLM summarization.** It makes the exercise nondeterministic and adds a second budget. Summaries are an optimization on top of honest stubbing, not a prerequisite.
- **Turn/chain deletion.** Keeping every `tool_use` and `tool_result` makes the adjacency invariant trivial. If you add chain compression later, delete the call and its result as one atomic range, never one without the other.
- **Disk spill.** `Store` is the seam. A file-backed store can be added without changing `Prune`; an in-memory store is enough for the first tests.
- **Dedup, schema pruning, provider cache scheduling, UI, session entries.** These are separate chapters or extensions. In particular, schema pruning should not be smuggled into `Prune`.
- **Exact tokenization.** `Estimate` is explicitly a teaching approximation. The invariant must be tested with a conservative estimator and a fake provider; a production harness should use provider usage + trailing estimate and reserve output tokens.

**Why `pairingOK` runs before and the design never deletes.** The only mutation is `b.Text = stub`. The `tool_use` block, the `tool_result` block, and their ids are untouched. There is no path that can produce an orphan, even if the store fails (on store failure `Prune` returns an error and the caller sends the unpruned request or refuses to send).

**Budget behavior.** `Prune` prunes all old eligible results, then reports `OverBudget`. The caller must not send an over-budget request. If it is over budget because of tool schemas or a huge system prompt, message stubbing cannot fix it; that is the bridge to the schema chapter or to a refusal. This is honest: a hard budget is a precondition, not a best-effort trigger.

## 10. Failure modes of a naive reimplementation

| # | Naive move | What breaks | Evidence / fix |
|---|---|---|---|
| 1 | Delete the `toolResult` message, keep the assistant `toolCall` | Orphan. Pi-ai injects a synthetic `isError` result with text `No result provided`; the model sees a fake tool failure. Without that repair, providers reject the orphan. | `pi-context-prune/src/pruner.ts:8-15`; `pi/packages/ai/src/api/transform-messages.ts:179-198`; fix: replace content in place, never remove the result |
| 2 | Truncate by message count or “drop middle N” | Splits a `toolCall` from its result, or drops a result whose call is in the protected prefix. | `pi/packages/coding-agent/docs/compaction.md:136` (“Never cut at tool results”); fix: operate on complete turns/ranges and validate pairing after every transform |
| 3 | Replace content with an unmarked truncation like `... [truncated] ...` | The model cannot tell it can recover, so it re-reads the same file or re-runs the same command forever; the prefix grows again. | `toolflow`’s read-cache notice explicitly tells the model to use `offset/limit` (`src/read_cache.ts:183-185`); `pi-condense`’s stub names the ref (`src/pruner.ts:137`); fix: sentinel + ref + recovery tool |
| 4 | Keep originals only in memory | Works until process exit; then every ref is dangling. | `pi-condense` rebuilds from session custom entries (`src/indexer.ts:64-148`); `toolflow` writes sidecars (`src/dehydrator.ts:490-505`); fix: store interface must be durable |
| 5 | Prune the stored session log instead of the outgoing view | Corrupts replay, branch switching, audit, and exports; a later compaction summarizes already-pruned data; the original is unrecoverable. | `pi-context-prune`/`pi-condense` only rewrite `context`; `toolflow` intentionally mutates `tool_result` and therefore accepts a different durability model (`src/index.ts:667-693`); fix: choose explicitly, document it, and never rewrite the log as a side effect of budget enforcement |
| 6 | Prune on every turn | Rewrites the prompt prefix every request; provider prompt-cache hits collapse; cost can rise even as tokens fall. | `pi-context-prune`’s `every-turn` mode (`index.ts:489-491`); `pi-condense` defaults to `agent-message` and batches (`src/types.ts:567`) and its docs argue cache-aware batching (`pi-condense/README.md:20`); fix: prune at stable boundaries, not per turn |
| 7 | Use `chars/4` as the only budget measure | Diverges from the provider tokenizer; thresholds never fire or fire too late; a request can exceed the hard limit after “pruning.” | Pi anchors on the last provider usage and estimates only trailing messages (`pi/packages/coding-agent/src/core/compaction/compaction.ts:227-262`); fix: provider usage + conservative trailing estimate + reserve, and fail closed |
| 8 | Summarize without checking the summary size | A summary longer than the raw batch grows context; the next flush summarizes the summary. | `pi-condense` skips when `summaryText.length > batchRawCharCount` (`index.ts:517-528`); `pi-context-prune` has the same guard (`index.ts:288-299`); fix: compare serialized summary vs raw and keep raw on loss |
| 9 | Key the store by bare `toolCallId` | Provider ids are reused; the model recovers the wrong occurrence’s content; dedup aliases point at the wrong original. | `pi-condense` uses occurrence keys `id@resultTimestamp` and fail-closed bare-id rules (`src/indexer.ts:182-184`, `:236-264`; `src/occurrence-key.ts`); fix: key by `(id, result timestamp)` or a monotonic occurrence id |
| 10 | Unmount a tool before its in-flight call completes | Pi returns `Tool <name> not found`; the stage cannot produce its artifact; no remount path exists. | `pi/packages/agent/src/agent-loop.ts:726-733`; `toolflow/src/state.ts:823-839`; fix: apply schema changes at turn boundaries, never mid-batch, and keep a baseline of tools needed to finish the current stage |
| 11 | Cache a read without invalidating on writes or shell commands | The model edits stale content or believes a changed file is unchanged. | `toolflow` invalidates on write/edit and within 15s of a command, checks mtime + hash (`src/read_cache.ts:73-75`, `:141-176`); fix: at minimum mtime + hash + mutation epoch |
| 12 | Trust a regex guard as a security boundary | `find -delete`, interpreter one-liners, encodings, and tool aliases bypass it; false positives block legitimate lockfile updates. | `toolflow/src/blast_radius.ts:12-15` disclaims malicious obfuscation; `:292-323` only checks destructive-token commands; fix: call it an accident guard, fail closed only where it can decide, and log/ask otherwise |

## 11. Gradable invariants (offline, scripted provider)

The verifier should be a Go test binary with a **scripted provider**: no network, fixed replies, captured requests. Sketch:

```go
type Reply struct {
	Text  string
	Calls []ToolCall
}

type ScriptedProvider struct {
	Replies  []Reply
	Requests []Request // every outgoing request, in order
}

func (p *ScriptedProvider) Complete(ctx context.Context, req Request) (Reply, error) {
	p.Requests = append(p.Requests, req)
	r := p.Replies[0]
	p.Replies = p.Replies[1:]
	return r, nil
}
```

The harness drives turns, executes the scripted tool calls against a fixture workspace (or a fake tool executor), and lets the student’s pruner transform the request. The verifier asserts on `Requests`, the store, and the session log. All checks below are deterministic and offline.

| ID | Invariant | How to observe it offline | Failure signal |
|---|---|---|---|
| **I1** | **Hard budget.** Every request that is actually sent satisfies `Estimate(req) <= Budget.Usable()`. If pruning cannot get under, `Prune` reports `OverBudget` and the harness must not send. | Scripted provider records every request. Run a long tool-output fixture (e.g., 30 reads of 8 KB each) with a small budget. Assert `max(Estimate(req)) <= usable`; assert the provider was not called for an over-budget attempt. | Any sent request over budget; or silent truncation that drops the system prompt to fit. |
| **I2** | **Pairing.** For every request: every `tool_use` id has exactly one `tool_result` with that id, every `tool_result` has a preceding `tool_use`, and no result is duplicated. The harness prunes only complete batches, so an open call is not a valid input to `Prune`; if you later model in-flight requests, allow exactly one trailing open assistant call and nothing else. | Parse each captured request into `(calls, results)`; assert set equality and counts. Also assert no tool result text is exactly `No result provided` with `isError`. | Orphan call, orphan result, duplicate result, or synthetic error. |
| **I3** | **System prompt preserved.** The serialized system prompt is byte-identical to the baseline for every request; it is non-empty and is the first system message. | Hash `req.System`; compare to the initial hash. | Any hash change, empty system, or system moved into messages. |
| **I4** | **Pruned content is marked and stored.** Every replaced tool result begins with `StubSentinel`, contains a `ref=...`, and the store has a record whose `Text` is byte-identical to the original. No raw content is replaced without a sentinel. | Before/after fixture: for each shortened result, assert sentinel + ref; for each ref, `store.Get` and compare bytes. Also assert untouched results are byte-identical. | Unmarked truncation; dangling ref; ref points to different content; store missing. |
| **I5** | **Recovery survives restart and is idempotent.** For every ref, `recover(ref)` returns the exact original. Re-running `Prune` on its own output produces the same request bytes and the same refs (no double-stubbing, no duplicate store records). A fresh store loaded from the persisted records resolves the same refs. | Call `Prune` twice; compare `json.Marshal` of messages. Recreate the store from disk/session fixture; call `Recover` for all refs. | Second prune changes bytes; duplicate refs; recovery differs after restart. |
| **I6** | **Schema mount set is exact and explicit.** For each stage, the outgoing provider payload’s tool names equal the stage policy’s mounted set (order-insensitive). A call to an unmounted tool yields an `isError` result containing `not found` and does not execute. Advancing the stage remounts exactly the next set; completion restores the initial snapshot. | Scripted provider returns a call to a removed tool; assert the tool executor was not invoked and the error is visible. Compare `payload.tools` names to the expected set per stage. | Extra schema (pruning failed), missing schema (over-pruned), silent execution of an unmounted tool, or a set that does not restore. |

Optional seventh invariant for a later chapter: **prefix stability** — between flush boundaries, the serialized prefix (system + tools + messages up to the last flushed boundary) is byte-identical, so a prompt cache would hit. This is the `pi-condense` batching argument (`README.md:20`) and is best tested only after I1–I6 pass.

## 12. One chapter or two?

**Two chapters.** The mechanisms are different enough that combining them teaches the wrong mental model.

Proposed split:

1. **“Context budget: prune without lying.”** Mechanisms (b) stub elision + (f) externalization, with (a) as a cautionary sidebar. Invariants I1–I5. The learner builds a `Prune` that replaces old tool-result text with a marked, recoverable stub; never removes a `tool_use`/`tool_result`; never touches the system prompt; refuses to send over budget. This is deterministic, offline, and directly matches the course’s four rules.
2. **“Tool surface: mount on demand.”** Mechanism (d), with toolflow as the case study. Invariant I6. The learner builds a stage policy, mounts a subset with `setActiveTools`, treats an unmounted call as an explicit error, and restores the initial set on completion. The budget is still relevant (schemas consume tokens), but the failure modes and the state machine are different.

Why not one chapter:

- **Different object.** Chapter 1 optimizes transcript bytes; chapter 2 optimizes the declared tool catalog and the system prompt’s tool section. A single “context budget” chapter would imply that removing tools is just another way to save tokens. It is not: it removes capabilities, and its correctness is about what work remains possible.
- **Different trigger.** Chapter 1 is token/usage/boundary-driven; chapter 2 is stage/phase-driven. A token threshold is the wrong trigger for unmounting a tool the current stage needs.
- **Different invariant.** Chapter 1’s hard invariant is “no orphaned `tool_use`.” Chapter 2’s is “the mounted set is exactly the stage’s set, and an unmounted call is an explicit error, never a silent execution.” Mixing them lets a student satisfy one by breaking the other (e.g., deleting tool results to save space, or unmounting a tool mid-batch).
- **Different failure modes.** Chapter 1: fake `No result provided`, unmarked stub loops, lost originals, cache busting. Chapter 2: capability deadlock (`Tool X not found`), stale stage state, fail-open mount-all fallback, review-stage write blocks. These deserve separate exercises and separate verifier signals.
- **Different safety story.** Chapter 1 is about data durability and honesty. Chapter 2 is about permissions and capability boundaries; toolflow’s write guard is a third, related but separate topic. Bundling all three into one chapter would produce a shallow tour.

If the course absolutely requires one chapter, keep the schema axis as a **sidebar with one invariant** (I6) and do not pretend it is a variant of message pruning. But the honest design is two chapters: one for the transcript, one for the toolbox.

## 13. Could not determine

- **Real token/cost savings.** No package ships a reproducible end-to-end benchmark. `toolflow`’s “-95%” is a hard-coded string (`src/dehydrator.ts:285`) and its per-call savings are floored at 10 tokens (`:515`, `:547`); `pi-condense` reports char counts, not provider-billed tokens. I did not run live provider sessions, so I cannot confirm cache-hit or billing effects.
- **`toolflow`’s `context` slimmer in a live pi session.** Static inspection shows the message schema does not match pi’s `AgentMessage` (`role: "tool"` vs `"toolResult"`, `toolCalls` vs `content`/`toolCall`). I did not run a live session to see whether some other layer normalizes the shape; I found no adapter. Treat the slimmer as suspect until tested.
- **`toolflow`’s `session_before_compact` effect.** Pi’s result type accepts only `cancel`/`compaction` (`types.ts:1511-1514`), and toolflow returns `customInstructions` (`src/index.ts:535-542`). Static inspection says ignored. I did not run a compaction to observe the summary; the code path is clear enough to call it a bug.
- **`pi-context-prune` npm bundle vs source.** The repo has no `dist/`; I read source at the tag. The `prepack` script builds from `index.ts`, but I did not diff the published bundle.
- **`pi-condense` compaction edge cases across pi versions.** It rebuilds from `getBranch()` (`src/indexer.ts:64-148`), which returns all entry types (`session-manager.ts:1466-1469`). Pi’s model projection may omit older entries, but the raw branch is the source. I did not test every pi compaction/branch-summary combination (split turns, retain-none, branch summary), so “recoverable after any compaction” is not proven; “recoverable because the index entries remain on the raw branch” is the code-supported claim.
- **`toolflow` stage resume.** `session_start` loads persisted blueprint state at module load (`src/index.ts:61`) but only re-applies tool scoping on `new`/`clear` (`:502-513`), and there is no `session_tree` handler. I could not determine whether a resumed session is expected to restore the stage’s active tools; the code suggests it does not. This is a likely desync bug, not a documented design.
- **`toolflow` review-stage write.** Plan B stage 5 declares `write` (`src/engine.ts:1430`) but activates the review guard (`src/index.ts:788-794`), which blocks `write` (`src/review_isolation.ts:169-186`). I could not determine whether this stage is expected to write its report via a different tool or whether the guard should be disabled for it. It looks contradictory.
- **`toolflow` package metadata.** It imports `@earendil-works/pi-tui` (`src/ui.ts`) but does not declare it as a peer dependency; only devDependencies include it (`package.json`). This may work in the author’s environment but is a packaging risk.

## Appendix: source map for the main claims

| Topic | File:line |
|---|---|
| `pi-context-prune` deletes results | `pi-context-prune/src/pruner.ts:8-15` |
| `pi-context-prune` summary + index | `pi-context-prune/index.ts:287-310`; `src/indexer.ts:120-138` |
| `pi-context-prune` trigger modes | `pi-context-prune/src/types.ts:108-118`; `index.ts:489-538` |
| `pi-context-prune` query tool | `pi-context-prune/src/query-tool.ts:6-65` |
| `pi-condense` five phases | `pi-condense/src/pruner.ts:24-80`, `:91-220` |
| `pi-condense` literal stub | `pi-condense/src/pruner.ts:137` |
| `pi-condense` spill stub | `pi-condense/src/pruner.ts:129-136`; `src/spill.ts:69-117` |
| `pi-condense` chain window | `pi-condense/src/chain-compressor.ts:55-63`; `src/types.ts:576-581` |
| `pi-condense` chain drop roles | `pi-condense/src/chain-range-prune.ts:222-241` |
| `pi-condense` orphan sweep | `pi-condense/src/orphan-sweep.ts:15-49` |
| `pi-condense` budget trigger | `pi-condense/src/budget.ts:18-25`; `index.ts:985-998` |
| `pi-condense` query tool | `pi-condense/src/query-tool.ts:19-107` |
| `toolflow` dehydration | `toolflow/src/dehydrator.ts:400-553` |
| `toolflow` read cache | `toolflow/src/read_cache.ts:123-232` |
| `toolflow` context slimmer | `toolflow/src/context_slimmer.ts:172-271` |
| `toolflow` stage scoping | `toolflow/src/state.ts:782-839`; `src/index.ts:204-217`, `:782-795` |
| `toolflow` heavy payload filter | `toolflow/src/context_slimmer.ts:24-76` |
| `toolflow` blast guard | `toolflow/src/blast_radius.ts:179-329` |
| `toolflow` review guard | `toolflow/src/review_isolation.ts:167-187`; `src/index.ts:602-611` |
| pi orphan repair | `pi/packages/ai/src/api/transform-messages.ts:179-198` |
| pi unmounted tool error | `pi/packages/agent/src/agent-loop.ts:726-733` |
| pi `context` excludes system | `pi/packages/coding-agent/src/core/extensions/types.ts:881-888` |
| pi compaction never cuts tool results | `pi/packages/coding-agent/docs/compaction.md:136` |
| pi active tool set | `pi/packages/coding-agent/docs/extensions.md:277`, `:299` |
| pi provider usage + trailing estimate | `pi/packages/coding-agent/src/core/compaction/compaction.ts:227-262`; `pi/packages/ai/src/utils/estimate.ts:97-107` |
