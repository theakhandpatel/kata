# Go agent-harness architecture: crush, PiG, pi

Research note for the **lathe** Go course. The goal is not feature inventory.
It is to reconstruct the machine a learner is being asked to build: where a user
message enters, what owns conversation state, how the loop decides to continue,
where tools are dispatched, where cancellation lands, and which seams are real
interfaces versus callbacks buried in loop config.

The most important finding is structural: **only one of the three repos owns its
agent loop in the same repository as its CLI.** Crush's loop is in the external
`charm.land/fantasy` module. PiG and pi are close ports of each other, but pi is
the reference contract and PiG is the Go translation. That changes which file a
course should point a learner at for each mechanism.

## Snapshot and method

All three repositories were cloned shallowly under `/tmp/goarch/`.

| Directory | Upstream | Commit | Date | Language | Size signal |
|---|---|---|---|---|---|
| `crush/` | `github.com/charmbracelet/crush` | `21dd7482afe2846d8cea8353cbd1ee7f41d2e9a9` | 2026-10-26 | Go | 107,380 non-test Go lines under `internal/` |
| `PiG/` | `github.com/MichaelKinsy/PiG` | `f2f028f8459504deb6066d393c3f1c316596f2f5` | 2026-10-25 | Go | 11,097 `.go` files excluding `third_party/` (tests dominate) |
| `pi/` | `github.com/earendil-works/pi` | `f9daca3480732d0042b1df3a6e27221f597a620d` | 2026-10-26 | TypeScript | 1,638 `.ts` files under `packages/` |

Citations are relative to those directories:

- `crush/internal/agent/agent.go:609` means `/tmp/goarch/crush/internal/agent/agent.go`, line 609.
- `PiG/agent/agent_loop.go:199` means `/tmp/goarch/PiG/agent/agent_loop.go`, line 199.
- `pi/packages/agent/src/agent-loop.ts:172` means the same under `/tmp/goarch/pi`.
- `fantasy@v0.45.1/agent.go:902` means the module cache copy of Crush's
  `charm.land/fantasy` dependency at
  `/home/darkoptimus/go/pkg/mod/charm.land/fantasy@v0.45.1/agent.go`.
- `go-sdk@v1.7.1-…/mcp/transport.go:52` means Crush's
  `github.com/modelcontextprotocol/go-sdk` dependency.
- `lathe/internal/fake/server.go:254` means the course repository.

**Method.** Static reading of the pinned commits. I did not run the three
harnesses, their test suites, or a live provider. I did run `go mod download` to
read Crush's external `fantasy` and MCP SDK dependencies at the versions its
`go.mod` pins. Claims below are source-reading claims. Where the code disagrees
with a README or package doc, the code is reported and the disagreement is
called out.

---

## 0. What all three actually agree on

Before the per-repo details, three invariants recur across all three, and a
fourth is the design rule the course should adopt even though Crush shows the
opposite mechanics. They are the load-bearing ones; everything else is product
surface.

1. **In a completed turn, every assistant tool call is answered by exactly one
   tool result, and results are sent in call order — not completion order.**
   - Crush's fantasy loop explicitly re-sorts results after parallel execution:
     `fantasy@v0.45.1/agent.go:1803-1820` says results are added "in the order
     the model called the tools, not the order they completed in."
   - PiG executes in parallel but appends result messages in source order:
     `PiG/agent/tool_execution.go:115-215`.
   - pi does the same with `Promise.all` then a source-order loop:
     `pi/packages/agent/src/agent-loop.ts:596-700`.

2. **A truncated tool call is never executed.**
   - Crush's OpenAI adapter suppresses calls when `finish_reason` is `length`,
     `content_filter`, an error, or when a missing finish reason comes with bad
     arguments: `fantasy@v0.45.1/providers/openai/language_model.go:650-690`.
   - PiG fails every call in a `length` turn instead of running it:
     `PiG/agent/agent_loop.go:313-323`.
   - pi has the same branch: `pi/packages/agent/src/agent-loop.ts:272-281`.

3. **The provider request is a snapshot; history does not mutate underneath a
   request.**
   - Crush rebuilds `[]fantasy.Message` from the database before each `Run`:
     `crush/internal/agent/agent.go:805-831`.
   - PiG builds a new run context with `slices.Concat(a.messages, prompts)`:
     `PiG/agent/agent_loop.go:158-162`.
   - pi copies the message and tool slices in `createContextSnapshot`:
     `pi/packages/agent/src/agent.ts:469-474`.

4. **Design rule: durable state advances on finalized messages, not on deltas.**
   - Crush is the exception in mechanics (which is why this is a rule, not a
     shared invariant): it writes an assistant row early and
     updates it during streaming, but terminal updates flush synchronously and
     `settleTurn` repairs unanswered calls before the turn ends:
     `crush/internal/agent/agent.go:973-1050`,
     `crush/internal/agent/settle.go:85-100`.
   - PiG persists on `message_end`: `PiG/coding/session_boundaries.go:49-95`.
   - pi persists on `message_end`: `pi/packages/coding-agent/src/core/agent-session.ts:1147-1170`.

The course should teach **1 and 2 as hard invariants**, **3 as the reason the
provider seam takes a request value rather than a pointer to live state**, and
**4 as the boundary between a UI stream and a durable transcript**.

---

## 1. Per-repo architecture

### 1.1 charmbracelet/crush

#### Shape

Crush has two layers:

- `internal/agent` — session orchestration, queues, cancellation, persistence,
  permissions, tool construction, title generation, summarization.
- `charm.land/fantasy` — the actual step loop, provider adapters, streaming
  event model, tool dispatch, retries.

`sessionAgent.Run` is the bridge. It creates a `fantasy.Agent`, converts the
database transcript into `[]fantasy.Message`, then calls `agent.Stream` with a
large callback struct. The loop itself is not in the Crush repository. This is
the single most important thing to know before reading Crush: `internal/agent/agent.go`
is an **adapter around a loop**, not the loop.

#### Entry point → request journey (`crush run`)

This is the non-interactive path; it is the shortest complete path and the one
the course's CLI most resembles. The short form is:

```
crush/main.go:25 → crush/internal/cmd/root.go:188
  → crush/internal/cmd/run.go:111 → crush/internal/app/app.go:303
  → crush/internal/agent/coordinator.go:311
  → crush/internal/agent/agent.go:609
  → fantasy@v0.45.1/agent.go:902
  → fantasy@v0.45.1/providers/openai/language_model.go:454
  → fantasy@v0.45.1/agent.go:1411
  → crush/internal/agent/agent.go:973
  → crush/internal/app/app.go:461
```

1. `main.main` (`crush/main.go:15`) calls `cmd.Execute()` (`crush/main.go:25`).
2. `cmd.Execute` (`crush/internal/cmd/root.go:188`) is the cobra/fang root. The
   `run` subcommand's `RunE` is `crush/internal/cmd/run.go:111`.
3. `runCmd.RunE` parses the prompt, handles stdin, then calls
   `appWs.App().RunNonInteractive(...)` (`crush/internal/cmd/run.go:163`).
4. `App.RunNonInteractive` (`crush/internal/app/app.go:303`):
   - `InitCoderAgentNonInteractive` (`:306`);
   - waits for MCP initialization with `mcp.WaitForInitBudget`
     (`:373`);
   - calls `AgentCoordinator.UpdateModels` (`:379`);
   - resolves or creates the session (`:382`);
   - auto-approves permissions for this session (`:409`);
   - starts the agent on a goroutine with
     `app.AgentCoordinator.Run(ctx, sess.ID, prompt)` (`:421`);
   - subscribes to `app.Messages` for output (`:433`).
5. `coordinator.Run` (`crush/internal/agent/coordinator.go:297`) delegates to
   `coordinator.run` (`:311`). That function waits for readiness (`:314`), waits
   for MCP on non-interactive runs (`:333`), refreshes models (`:342`), merges
   call options (`:357`), and invokes the `run` closure (`:395`) which calls
   `agent.Run` with a `SessionAgentCall`.
6. `sessionAgent.Run` (`crush/internal/agent/agent.go:609`):
   - `ValidateCall` (`:610`);
   - takes the per-session dispatch lock and decides cancel-on-entry, queued, or
     active (`:633-700`);
   - registers `activeRequests` with a `context.WithCancel` (`:690-693`);
   - copies tools and system prompt (`:773-777`);
   - creates `fantasy.NewAgent` (`:792`);
   - reads the session and messages (`:800-805`);
   - writes the user message (`:819`);
   - builds `history` with `preparePrompt` (`:831`);
   - calls `agent.Stream(genCtx, fantasy.AgentStreamCall{...})` (`:854`).
7. `fantasy.(*agent).Stream` (`fantasy@v0.45.1/agent.go:902`) builds the initial
   prompt (`:925`), then loops steps. Each step:
   - calls `PrepareStep` (`:942`) — Crush's callback at
     `crush/internal/agent/agent.go:866`;
   - builds a `fantasy.Call` (`:1012`);
   - calls `retryModel.Stream(ctx, streamCall)` (`:1045`), which lands in the
     provider adapter.
8. For OpenAI-compatible providers, `languageModel.Stream`
   (`fantasy@v0.45.1/providers/openai/language_model.go:454`) reads SSE:
   - text deltas are yielded as `StreamPartTypeTextDelta`;
   - tool calls are accumulated in `map[int64]streamToolCall` keyed by the wire
     `index` (`:467`);
   - continuation fragments append arguments (`:542`);
   - tool calls are finalized in index order after the stream ends (`:680`).
9. `processStepStream` (`fantasy@v0.45.1/agent.go:1411`) consumes those parts and
   invokes Crush's callbacks:
   - `OnTextDelta` → `currentAssistant.AppendContent` then
     `messages.Update` (`crush/internal/agent/agent.go:973-985`);
   - `OnToolCall` → `currentAssistant.AddToolCall` then `messages.Update`
     (`:1014-1028`);
   - `OnToolResult` → creates a separate `message.Tool` row
     (`:1031-1050`).
10. Tool calls are buffered until the stream's finish reason is known
    (`fantasy@v0.45.1/agent.go:1740-1780`). Only an explicit `tool_calls` finish
    dispatches them (`:1790-1800`). Tools with `Info().Parallel` run
    concurrently with a semaphore of 5; the rest run sequentially on the
    dispatcher goroutine (`:1690-1740`). Results are sorted back into call order
    (`:1803-1820`).
11. The loop continues only when `ShouldContinue` is true:
    `len(stepToolCalls) > 0 && stepFinishReason == FinishReasonToolCalls &&
    !hasStopTurn(toolResults)` (`fantasy@v0.45.1/agent.go:1828-1838`), plus
    Crush's `StopWhen` conditions for context exhaustion and repeated tool calls
    (`crush/internal/agent/agent.go:1108-1135`).
12. After `Stream` returns, Crush settles unanswered tool calls
    (`crush/internal/agent/settle.go:85`), optionally summarizes, removes the
    active request, publishes `RunComplete`, and re-enters `Run` for a queued
    prompt (`crush/internal/agent/agent.go:1130-1390`).
13. `App.RunNonInteractive` reads `pubsub` message events and prints only the
    new suffix of the assistant content (`crush/internal/app/app.go:461-498`).

The TUI path differs only at the entry: it calls
`AppWorkspace.AgentRun` (`crush/internal/workspace/app_workspace.go:111`),
which calls the same `AgentCoordinator.Run`.

#### State ownership

**Durable source of truth:** SQLite, through two services.

- `message.Service` (`crush/internal/message/message.go:46`) stores message
  rows. It exposes `Create`, `Update`, `List`, `ListFromSummary`, and explicit
  `Flush`/`FlushAll`.
- `session.Service` (`crush/internal/session/session.go:67`) stores session
  metadata, token counts, summary message ID, todos, and parent-session links.

**In-memory state in `sessionAgent`:**
`tools *csync.Slice[fantasy.AgentTool]` (`crush/internal/agent/agent.go:211`),
`systemPrompt`, `messageQueue`, `activeRequests`, `cancelMark`, and model
configuration. There is no in-memory conversation slice owned by
`sessionAgent`; each `Run` reads rows from `message.Service`.

**Who mutates it:** the streaming callbacks. `OnTextDelta` updates the same
assistant row repeatedly. `OnToolCall` adds a tool call to that row.
`OnToolResult` creates a separate `tool` row. `OnStepFinish` updates session
usage and saves the session. This is not an append-only transcript; it is a
mutable row set with debounced writes (`message.Service.Update` coalesces
streaming deltas, with terminal updates flushing synchronously:
`crush/internal/message/message.go:23-45`).

**What is copied per request:** `getSessionMessages` reads rows
(`crush/internal/agent/agent.go:1720`), `preparePrompt` builds a fresh
`[]fantasy.Message` (`:1572`), and `fantasy.NewAgent` receives a copied tool
slice (`:773`). The request is therefore a snapshot of the database, not a live
view.

One subtlety: the user row is created *after* `getSessionMessages` and
`preparePrompt`, so it is **not** in `history` for the first request. It is
passed separately as `Prompt` (`crush/internal/agent/agent.go:854`), and
fantasy's `createPrompt` adds it as the leading user message. That is why the
first request contains the prompt once, not twice. A naive port that appends
the user message to both `history` and `Prompt` duplicates it.

**One important repair:** `preparePrompt` collects all tool-call IDs from
assistant rows, indexes every tool result by call ID, then re-emits each result
immediately after the assistant message that requested it
(`crush/internal/agent/agent.go:1587-1650`). It does this because concurrent
writes can interleave a user message between a tool call and its result, which
strict providers reject. This is a real production scar and a good teaching
example of why stored order and request order are different concerns.

#### The loop

Crush's loop is fantasy's loop:

- It is a **step loop**, not a turn loop. One step is one model response plus
  the tools it requested.
- It buffers tool calls until the finish reason is known, so truncated calls
  never execute.
- It dispatches parallel-capable tools concurrently (cap 5) and normal tools
  sequentially. `bash`, `write`, `edit` are sequential by default; read-only
  web/MCP tools opt into parallel (`crush/internal/agent/tools/download.go:65`,
  `crush/internal/agent/tools/fetch.go:58`).
- It stops when the finish reason is not `tool_calls`, when a tool result sets
  `StopTurn`, or when a `StopWhen` condition fires.
- Abort lands as a `context.Cancel` on `genCtx`. The provider stream returns an
  error, Crush marks the assistant message `finish_reason=canceled`, and
  `settleTurn` writes an error tool result for every call that never got one
  (`crush/internal/agent/settle.go:85-130`).
- Steering is not mid-stream injection. A queued prompt is folded into the next
  step inside `PrepareStep` (`crush/internal/agent/agent.go:866-930`,
  `drainQueueForStep` at `:442`). Prompts with a non-empty `RunID` are kept as
  separate turns; prompts without one are folded.

#### Hooks and permissions

Crush has a **real interception layer**, and it is two layers deep.

1. **Shell hooks around every tool.** `buildTools` constructs a
   `hooks.Runner` from `PreToolUse` config (`crush/internal/agent/coordinator.go:890-892`)
   and wraps every tool in `hookedTool` (`:982`). The wrapper runs the hook
   before the inner tool (`crush/internal/agent/hooked_tool.go:54`). A deny
   returns an error tool response; a halt sets `StopTurn`; an allow pre-approves
   the permission prompt for that exact call ID
   (`crush/internal/agent/hooked_tool.go:62-85`). A hook may also rewrite the
   tool input (`:79-81`).
2. **Permission service inside the tool.** Tools such as `bash` and `write` call
   `permissions.Request(...)` themselves (`crush/internal/agent/tools/bash.go:229`,
   `crush/internal/agent/tools/write.go:103`). The request blocks until the UI
   grants or denies it. `AutoApproveSession` is used by non-interactive runs
   (`crush/internal/app/app.go:409`).

This is not a hook-only design: the permission gate is a first-class service
with pending requests, notification events, and persistent per-session grants
(`crush/internal/permission/permission.go:66-90`). A hook can veto a call and
can pre-approve the permission gate, but the gate itself lives in the tool.

#### MCP

Crush uses the official Go SDK (`github.com/modelcontextprotocol/go-sdk`,
pinned at `v1.7.1-0.20260825092617-32404652e60b`) rather than a hand-written
JSON-RPC client.

- `mcp.Initialize` (`crush/internal/agent/tools/mcp/init.go:338`) starts one
  goroutine per configured server (`:350-365`).
- Transport selection is `createTransport` (`:1193`):
  - stdio → `exec.CommandContext` + `mcp.CommandTransport`, with the child in
    its own process group (`:1206-1220`);
  - HTTP → `mcp.StreamableClientTransport`, optionally with OAuth (`:1270`);
  - SSE → `mcp.SSEClientTransport` (`:1346`).
- The handshake is delegated: `mcp.NewClient(...)` (`:1092`) then
  `client.Connect(mcpCtx, transport, nil)` (`:1101`). In the pinned SDK,
  `Connect` may try the SEP-2575 `server/discover` RPC before falling back to
  the legacy `initialize` / `notifications/initialized` handshake
  (`go-sdk@v1.7.1-…/mcp/client.go:308-380`).
- After connect, Crush calls `ListTools` and registers the result
  (`crush/internal/agent/tools/mcp/tools.go:154-160`), unless the server did not
  advertise the tools capability.
- `mcp.Tool` is the SDK type: `type Tool = mcp.Tool` (`crush/internal/agent/tools/mcp/tools.go:16`).
  `GetMCPTools` wraps every registered tool in an in-repo `tools.Tool`
  (`crush/internal/agent/tools/mcp-tools.go:24-40`).
- The adapter's `Info()` maps `InputSchema.properties` to `Parameters` and
  `InputSchema.required` to `Required`, and prefixes the name as
  `mcp_<server>_<tool>` (`:58-97`). `Run` asks the permission service, then
  calls `mcp.RunTool` (`:99-150`).
- Lifecycle: servers can be disabled, toggled per repository, reconnected,
  refreshed on `notifications/tools/list_changed`, and lazily renewed by
  `getOrRenewClient` (`crush/internal/agent/tools/mcp/init.go:791-845`).
  `Close` tears them down (`:304`).

The important architectural point for the course: **Crush does not implement
MCP framing or the handshake.** It delegates both to the SDK and only owns
config, lifecycle, permission, and the `ToolDef` adapter.

#### Abort and steering

- Abort: `sessionAgent.Cancel` (`crush/internal/agent/agent.go:2024`) calls the
  active request's cancel func, records a cancel mark for accepted-but-not-yet
  active runs, and clears the queue. `settleTurn` then guarantees no tool call
  outlives the turn.
- Steering: queued `SessionAgentCall`s are drained in `PrepareStep`
  (`drainQueueForStep`, `:442`). This is exactly "the queued message enters at
  the seam" — before the next provider request, never in the middle of an SSE
  stream.
- Queued prompts with a `RunID` are not folded; each becomes its own recursive
  `Run` (`:1340-1385`). This is a product concern (each caller gets its own
  `RunComplete`), not a loop concern.

#### Code vs docs

Crush's README describes product behavior, not architecture; I found no
architecture claim in it that contradicts the code. The dependency on `fantasy`
is visible in `go.mod` and imports, not in the README. A reader who only reads
`internal/agent` will overestimate how much loop logic lives in-repo.

---

### 1.2 MichaelKinsy/PiG

#### Shape

PiG is a line-by-line Go port of pi. The package split mirrors pi:

- `ai/` — provider/model types, stream adapters, auth, model catalog.
- `agent/` — the generic agent loop, message types, queues, tool-call hooks.
- `coding/` — the product layer: sessions, extensions, prompts, tools, CLI.
- `mcp/` — a hand-written MCP client and transports.

Unlike Crush, the loop is in the repository and is readable end to end. The
core files are `agent/agent_loop.go` (618 lines) and
`agent/tool_execution.go` (691 lines); the rest of `agent/` is message and event
plumbing.

#### Entry point → request journey (`pig -p` print mode)

The short form is:

```
PiG/cmd/pig/main.go:10 → PiG/coding/cli/main.go:579
  → PiG/coding/cli/print_mode.go:125
  → PiG/coding/cli/print_mode.go:571 → PiG/coding/session.go:955
  → PiG/coding/session_preflight.go:141 → PiG/agent/agent.go:1225
  → PiG/agent/agent_loop.go:152 → PiG/agent/agent_loop.go:291
  → PiG/agent/agent_loop.go:458 → PiG/ai/openai.go:1878
  → PiG/agent/agent_loop.go:317 → PiG/coding/session_boundaries.go:49
  → PiG/coding/session.go:810
  → PiG/internal/codingagent/session.go:497
```

1. `cmd/pig/main.go:10` calls `cli.Main(os.Args[1:], nil)`.
2. `coding/cli/main.go:579` is `Main`. After flag parsing and command dispatch,
   print/JSON/RPC modes are routed at `:750`; print mode calls
   `startPrintMode` at `:1258`.
3. `startPrintMode` (`coding/cli/print_mode.go:125`):
   - `coding.SessionManagerFor` (`:143`);
   - `coding.CreateAgentSessionRuntime` (`:151`);
   - `runPrintMode` (`:158`).
4. `runPrintMode` (`:164`) → `runPrintModeIn` (`:174`) → `sendPrintPrompt`
   (`:571`).
5. `sendPrintPrompt` validates auth, expands skills/templates, then calls
   `sess.SendContentWithPreflight` (`:593`).
6. `coding.(*Session).SendContentWithPreflight` (`coding/session.go:955`) →
   `prepareContentRun` (`:965`) → `BeginPreparedPrompt`
   (`coding/session_preflight.go:141`) → `agent.BeginSendMessages` (`:154`).
7. `PreparedPromptRun.Run` (`coding/session_preflight.go:128`) →
   `runPreparedPrompt` (`:211`) → `agentRun.Run` (`:134`).
8. `agent.(*PromptRun).Run` (`agent/agent.go:1225`) builds the loop config and
   calls `runLoop` (`agent/agent_loop.go:152`).
9. `loopRun.loop` (`agent/agent_loop.go:199`) drains the steering queue, then
   calls `runTurn` (`:291`).
10. `runTurn` (`:291`) → `streamAssistantResponse` (`:458`) →
    `ai.RunStreamContinuation` (`:521`) → `requestStream` (`:534`) →
    `streamFn(...)` (`:539`).
11. The default stream function eventually calls
    `ai.OpenAICompletionsAPI().Stream` via `StreamOpenAICompletions`
    (`ai/api_streams.go:186`), which reaches `openAIProvider.parseSSE`
    (`ai/openai.go:1878`).
12. `parseSSE` tracks partial tool calls in three maps — by wire index, by ID,
    and by internal stream index (`ai/openai.go:1880-1900`) — and feeds an
    `assistantStreamBuilder` (`ai/stream_builder.go:243`).
13. Back in `runTurn`, the assistant message is appended (`PiG/agent/agent_loop.go:317`),
    then `executeToolCalls` (`agent/tool_execution.go:58`) runs:
    - sequential if configured or any call targets a sequential tool (`:58`);
    - otherwise parallel (`:115`), with preparation in source order and
      execution concurrently.
    `prepareToolCall` (`:353`) validates arguments and runs the before hooks;
    `executePreparedToolCall` (`:443`) runs the tool;
    `finalizeExecutedToolCall` (`:549`) runs the after hooks and finalizes.
14. The loop emits `MessageEndEvent` for each finalized message. The Session's
    awaited listener (`coding/session_boundaries.go:49`) calls `persistMessage`
    (`coding/session.go:810`) → `inner.AppendMessage`
    (`internal/codingagent/session.go:570`) → `appendMessageEntry` (`:623`) →
    `appendEntryRecordOf` (`:365`) → `persistRecord` (`:419`) →
    `appendSessionLine` (`:497`), which appends one newline-terminated JSON line.
15. Session listeners/event forwarding (`coding/session.go:2513`) deliver the
    events to print mode, which writes the output.

The full journey after `SendContentWithPreflight` is shared by the TUI and print
mode. The TUI entry is different; the loop is the same.

#### State ownership

PiG has **two sources**, synchronized at message boundaries:

1. **In-memory loop transcript:** `Agent.messages []AgentMessage`
   (`agent/agent.go:889`), guarded by `messagesMu`.
2. **Durable append-only log:** `internal/codingagent.Session`, exposed as
   `coding.SessionManager` (`coding/session_manager.go:12`). It is a JSONL file:
   a header line, then one entry per line. Every non-header entry has `id` and
   `parentId` (`PiG/internal/codingagent/session.go:33-38`).

**Who mutates what:**

- The `Agent` appends assistant and tool-result messages to its in-memory slice
  as the loop runs (`runTurn` → `appendAssistant`, `appendToolResult`).
- The Session's `message_end` handler persists a copy: `persistMessage`
  (`coding/session.go:810`) → `AppendMessage` (`internal/codingagent/session.go:570`)
  → one JSON line.
- On resume, branch, or compaction, `BuildContext` projects the entry chain into
  `[]agent.AgentMessage` (`internal/codingagent/session.go:1175`) and
  `SetMessages` **replaces** the agent's slice
  (`coding/session_recovery.go:84`, `coding/session_transcript.go:18`). The disk
  log is append-only; the in-memory transcript is replaceable.

**Append-only?** On disk, yes for normal operation: `appendEntryRecordOf`
pushes to the in-memory slot list and `persistRecord` appends one line
(`PiG/internal/codingagent/session.go:365-430`, `:497-520`). There is a rewrite path for
migrations/branch-file creation, not for normal message appends. In memory, the
transcript is append-only during a run but can be replaced wholesale by a
projection after a branch or compaction.

**Copied per request vs referenced:**

- `runLoop` starts the run context with `slices.Concat(a.messages, prompts)`
  (`agent/agent_loop.go:158-162`), so the loop has its own slice.
- `Agent.MessagesSnapshot` clones the slice (`agent/agent.go:1349-1353`).
- The provider conversion may reuse converted messages through `llmMemo`
  (`agent/llm_memo.go:1-120`), which caches the previous conversion and copies
  unchanged prefixes. That is an allocation optimization; the provider still
  receives a `[]ai.Message` value.
- Message structs are copied by value, but nested content slices may be shared
  until converted. A hook that writes through a shared content slice is a real
  hazard; the memo comments call this out.

#### The loop

PiG's loop is a direct port of pi's `runLoop`:

- **Outer loop:** follow-up messages and explicit continuation decisions.
- **Inner loop:** tool calls, steering messages, and one assistant response per
  iteration (`agent/agent_loop.go:199-290`).
- **One turn:** `runTurn` streams one assistant response, executes its tool
  calls, runs `FinishTurn`, and emits `TurnEndEvent` (`:291-333`).
- **Stop:** an `error` or `aborted` assistant stop reason is a hard exit
  (`:300-310`). A `length` stop fails all tool calls rather than executing
  truncated arguments (`:313-323`). Otherwise, if there were tool calls and the
  batch did not set `terminate`, the loop continues. With no tool calls, it
  checks follow-ups and then ends.
- **Multiple tool calls:** `executeToolCalls` chooses sequential if configured
  or if any tool's `ExecutionMode` is sequential; otherwise parallel. The core
  file tools report `ToolModeParallel` (`internal/codingagent/tools/bash.go:61`,
  `PiG/internal/codingagent/tools/read.go:66`,
  `PiG/internal/codingagent/tools/write.go:92`). Parallel preparation is sequential in
  source order; execution is concurrent; `tool_execution_end` follows completion
  order; tool-result messages follow source order
  (`agent/tool_execution.go:115-215`).
- **Abort:** `Agent.Abort` cancels the run context (`agent/lifecycle.go:146-153`).
  The provider stream yields an aborted assistant message; `runTurn` treats it
  as a hard exit. Tools check `ctx.Err()` before execution and after hooks
  (`agent/tool_execution.go:353-407`).
- **Steering:** `Agent.steer` enqueues into `PendingMessageQueue`
  (`agent/agent.go:1636`); the loop drains it at the top of each inner iteration
  (`PiG/agent/agent_loop.go:199-205`) and after each turn (`:256`). Queue mode is
  `all` or `one-at-a-time` (`agent/queue.go:14-31`). This is turn-boundary
  injection, not mid-stream injection.

#### Hooks and permissions

PiG has **no permission service**. The README says so directly: "PiG runs with
the permissions of the user who starts it. PiG does not provide a security
sandbox..." (`PiG/README.md:169`).

What it has is a hook pipeline:

- `BeforeToolCallFunc` and `AfterToolCallFunc` are assignable properties on the
  `Agent` (`agent/agent.go:774-777`).
- `BeforeToolCallHook` is a Go function type:
  `func(ctx, toolCallID, toolName string, args json.RawMessage) ToolCallHookResult`
  (`agent/agent.go:703`).
- `ToolCallHookResult` can `Block`, set a `Reason`, set `Terminate`, rewrite
  `Args`, or carry an `Err` (`agent/agent.go:690-701`).
- `prepareToolCall` runs argument preparation, schema validation, then every
  before hook (`agent/tool_execution.go:353-407`). A hook that blocks returns an
  error tool result; the model sees it and can try something else. A hook that
  returns `Err` also produces an error tool result. A hook that rewrites `Args`
  replaces the arguments **without revalidation**, matching pi.
- After execution, `finalizeExecutedToolCall` runs the after hooks and can
  replace content, details, error flag, usage, structured content, or terminate
  hint (`PiG/agent/tool_execution.go:549-603`).

So PiG's answer to "can a hook veto a tool call?" is **yes, at the loop level,
via `Block`**, but there is no path-aware permission layer and no UI prompt.
Permission policy is an extension/host concern.

#### MCP

PiG implements MCP by hand.

- `mcp.Transport` is a real interface (`mcp/transport.go:34-42`):
  `Start`, `Send`, `Close`, `OnMessage`, `OnError`, `OnClose`.
- `StdioTransport` frames stdout into newline-delimited JSON
  (`mcp/stdio.go:317-345`). It writes one JSON object plus `\n`, tracks a
  remainder buffer, enforces a max message size, and kills the process group on
  close (`mcp/stdio.go:252-300`).
- `mcp.Client.Connect` (`mcp/client.go:355`) calls `initialize` (`:380`):
  1. `transport.Start()`;
  2. send `initialize` with `{protocolVersion, capabilities, clientInfo}`;
  3. validate the result and the negotiated protocol version;
  4. send `notifications/initialized`;
  5. transition to `connected`.
- `ListTools` (`:509`) calls `tools/list` and follows `nextCursor` pages.
- `CallTool` (`:684`) sends `tools/call`.
- Tool conversion: `coding/mcpext.CreateMcpToolDefinition`
  (`coding/mcpext/tools.go:470`) builds an `extension.ToolDefinition` with
  `Parameters: toParameters(tool.InputSchema)` and an output schema. The name is
  `mcp__<server>__<tool>`, sanitized to `[A-Za-z0-9_]` and shortened with a hash
  on collision or length (`:109-118`).
- Lifecycle: `Connection.open` (`coding/mcpext/runtime.go:613`) retries
  transient HTTP errors, `connectOnce` (`:656`) creates the client, connects,
  registers `notifications/tools/list_changed`, checks server capabilities,
  lists tools, and stores them (`:696`). `refreshTools` (`:787`) updates the
  list; `Close` (`:809`) shuts the connection down.

#### Code vs docs: session path mismatch

`PiG/coding/doc.go:53` says:

> Sessions persist as JSONL files at `<CWD>/.pig/sessions/<id>.jsonl`.

The code says otherwise. `defaultSessionDir` uses the agent directory:
`<agentDir>/sessions/<encoded-cwd>/` (`internal/codingagent/session_manager.go:67-85`),
and the filename is `<timestamp>_<id>.jsonl`
(`internal/codingagent/session_manager.go:121-123`). This is a documentation
bug, not a runtime ambiguity. The code is the truth.

---

### 1.3 earendil-works/pi (TypeScript reference)

#### Shape

Pi is the reference contract PiG ports. The relevant packages:

- `packages/ai` — provider/model types and adapters. `StreamFunction` is the
  provider seam (`packages/ai/src/types.ts:398-402`).
- `packages/agent` — the generic loop (`agent-loop.ts`) and `Agent` state
  machine (`agent.ts`).
- `packages/coding-agent` — product layer: `AgentSession`, `SessionManager`,
  tools, extensions, CLI, print/TUI/RPC modes.
- `packages/mcp` — hand-written MCP client and transports.

#### Entry point → request journey (`pi -p` print mode)

The short form is:

```
pi/packages/coding-agent/src/cli.ts:6
  → pi/packages/coding-agent/src/main.ts:574
  → pi/packages/coding-agent/src/modes/print-mode.ts:33
  → pi/packages/coding-agent/src/core/agent-session.ts:2092
  → pi/packages/coding-agent/src/core/agent-session.ts:1873
  → pi/packages/agent/src/agent.ts:382
  → pi/packages/agent/src/agent-loop.ts:111
  → pi/packages/agent/src/agent-loop.ts:172
  → pi/packages/agent/src/agent-loop.ts:390
  → pi/packages/ai/src/api/openai-completions.ts:411
  → pi/packages/agent/src/agent-loop.ts:518
  → pi/packages/agent/src/agent.ts:575
  → pi/packages/coding-agent/src/core/agent-session.ts:1103
  → pi/packages/coding-agent/src/core/session-manager.ts:1204
  → pi/packages/coding-agent/src/core/session-manager.ts:1172
```

1. `packages/coding-agent/src/cli.ts:6` calls `main(process.argv.slice(2))`.
2. `main.ts:574` parses flags, builds services and the session manager, then
   calls `createAgentSessionRuntime` (`:866`) and `runPrintMode` (`:988`).
3. `modes/print-mode.ts:33` `runPrintMode` calls `session.prompt(...)` at
   `:132`.
4. `core/agent-session.ts:2092` `AgentSession.prompt` handles extension
   commands, input handlers, skill/template expansion, auth checks, then
   `_startRun` (`:1873`).
5. `_startRun` calls `this.agent.prompt(messages)` (`:1899`).
6. `packages/agent/src/agent.ts:382` `Agent.prompt` → `runPromptMessages`
   (`:441`) → `runWithLifecycle` (`:517`) → `runAgentLoop`
   (`agent-loop.ts:111`).
7. `runAgentLoop` (`:111`) → `runLoop` (`:172`).
8. `runLoop` drains steering (`:180`), then `streamAssistantResponse` (`:390`).
9. `streamAssistantResponse` applies `transformContext` (`:399`), converts to
   LLM messages with `convertToLlm` (`:404`), resolves the API key (`:409`), and
   calls `streamFunction(...)` (`:412`).
10. `ModelRuntime.streamSimple` (or `stream`) resolves the provider and calls
    `prepared.provider.stream(...)` (`pi/packages/coding-agent/src/core/model-runtime.ts:731-760`).
11. The OpenAI-compatible adapter parses SSE in
    `packages/ai/src/api/openai-completions.ts`. It tracks tool-call blocks by
    **both** stream index and ID (`:411-413`), creates a block on the first
    fragment (`:485-535`), and appends argument deltas (`:639-660`).
12. Back in `runLoop`, tool calls are executed by `executeToolCalls` (`:518`):
    sequential (`:540`) or parallel (`:596`); preparation is source-order,
    execution concurrent, final result messages source-order.
13. `Agent.processEvents` (`pi/packages/agent/src/agent.ts:575`) reduces events into `_state.messages`
    on `message_end` (`:585-600`).
14. `AgentSession._handleAgentEvent` (`agent-session.ts:1103`) persists on
    `message_end` (`:1147-1170`) via `sessionManager.appendMessage` (`:1165`).
15. `SessionManager.appendMessage` (`session-manager.ts:1204`) creates an entry
    with `parentId = leafId`, then `_appendEntry` (`:1191`) pushes it and calls
    `_persist` (`:1172`), which appends one JSON line
    (`appendFileSync(this.sessionFile, ...)` at `:1187`).

#### State ownership

- **In-memory loop state:** `Agent._state.messages` (`packages/agent/src/agent.ts:194-330`).
  The loop works on a copy returned by `createContextSnapshot`
  (`:469-474`); `processEvents` pushes finalized messages back into the real
  state (`:575-610`).
- **Durable state:** `SessionManager` (`session-manager.ts:987`). JSONL entries
  with `id`/`parentId`; the leaf pointer selects a branch. `buildSessionContext`
  projects the branch to messages (`:576`), and `buildContextEntries` handles
  compaction (`:476`).
- **Who mutates:** the `Agent` appends in memory; `AgentSession` appends to the
  session log on `message_end`. Resume/compaction/branch re-projects and
  replaces the agent's initial state.
- **Copied per request:** `createContextSnapshot` copies `_state.messages` and
  `_state.tools` (`:469-474`). `convertToLlm` produces a new `Message[]`
  (`:404`). History is not mutated under the request.

#### The loop

Same shape as PiG:

- outer loop for follow-ups/explicit continuation, inner loop for tool calls and
  steering (`agent-loop.ts:172-390`);
- error/aborted stop reasons are hard exits (`:254-265`);
- `length` fails all tool calls (`:272-281`);
- tool batch terminates early only if every finalized result sets
  `terminate: true` (`:700-705`);
- parallel execution is `Promise.all` after source-order preparation
  (`:596-700`);
- abort is an `AbortController` signal passed to the stream function and to
  every tool (`:412-417`, `:718-787`).

#### Hooks and permissions

Pi has **no built-in permission system**. Its README states this explicitly
(`pi/README.md:90`).

The interception layer is extension handlers installed onto the agent:

- `AgentSession._installAgentToolHooks` assigns
  `agent.beforeToolCall = (context) => this._beforeToolCall(context)` and the
  after equivalent (`pi/packages/coding-agent/src/core/agent-session.ts:652-655`).
- `_beforeToolCall` dispatches the extension `tool_call` event
  (`agent-session.ts:658-680`). The extension result is a
  `BeforeToolCallResult` with `block`, `reason`, and `terminate`
  (`packages/agent/src/types.ts:66-80`).
- `_afterToolCall` dispatches `tool_result` and can replace content, details,
  isError, usage, structured content (`agent-session.ts:684-730`).
- The loop's `prepareToolCall` enforces the block before execution
  (`agent-loop.ts:738-766`).

So pi's "permission" is an extension concern: a `tool_call` handler can inspect
the call and block it, but the core ships no path policy, no prompt, and no
persistent grants.

#### MCP

- `McpTransport` is an interface (`packages/mcp/src/transports/transport.ts:18-31`):
  `start`, `send`, `close`, `onMessage`, `onError`, `onClose`, optional
  `setProtocolVersion`.
- `StdioTransport` frames stdout on newline, strips `\r`, ignores blank lines,
  parses one JSON-RPC message per line, enforces a max message size, and kills
  the process tree on close (`packages/mcp/src/transports/stdio.ts:180-205`,
  `:100-140`).
- `Client.connect` (`packages/mcp/src/client.ts:204`) starts the transport, sends
  `initialize` (`:221-235`), validates the result, checks the negotiated
  protocol version, sends `notifications/initialized` (`:243`), and transitions
  to `connected`.
- `listTools` (`:296`) follows pages; `callTool` (`:378`) sends `tools/call`.
- The coding-agent extension adapts each `McpTool` to a `ToolDefinition` with
  `parameters: toParameters(tool.inputSchema)` and name
  `mcp__<server>__<tool>` (`packages/coding-agent/src/extensions/mcp/tools.ts:312-340`,
  `createMcpToolName` at `:84`).
- Lifecycle: `Connection.open` retries transient HTTP errors
  (`extensions/mcp/runtime.ts:340-357`); `connectOnce` creates the client,
  connects, registers `notifications/tools/list_changed`, checks capabilities,
  and lists tools (`:359-395`); `refreshTools` updates them (`:432`).

---

## 2. Cross-repo comparison

| Dimension | Crush | PiG | pi |
|---|---|---|---|
| Role | Production Go TUI harness | Go line-by-line port of pi | TypeScript reference contract |
| Where the loop lives | External `charm.land/fantasy` | `agent/agent_loop.go` | `packages/agent/src/agent-loop.ts` |
| Provider seam | `fantasy.LanguageModel` | `agent.StreamFn` + `ai.Provider` | `StreamFunction` + `models.Provider` |
| Seam shape | `iter.Seq[StreamPart]` | `*AssistantMessageEventStream` | `AssistantMessageEventStream` |
| Tool seam | `fantasy.AgentTool` | `agent.AgentTool` | `AgentTool` |
| Tool registry | `*csync.Slice[fantasy.AgentTool]`; no interface | `[]AgentTool` behind `toolsMu`; no interface | `AgentState.tools`; no interface |
| Conversation source of truth | SQLite rows (`message.Service`) | `Agent.messages` + JSONL `Session` entries | `Agent._state.messages` + JSONL `SessionManager` |
| History append-only? | No; assistant row is updated per delta, tool rows created | Disk yes; memory append during run, replaced on branch/compact | Disk yes; memory append during run, replaced on branch/compact |
| Persist trigger | stream callbacks + `OnStepFinish` | `message_end` | `message_end` |
| Tool execution default | Sequential; per-tool `Parallel` opt-in, cap 5 | Parallel; any sequential tool forces sequential | Parallel; any sequential tool forces sequential |
| Result ordering | Re-sorted to call order | Source order | Source order |
| Tool error boundary | Go error is fatal in fantasy; Crush tools return text errors | Go error becomes tool result | Throw/`isError` becomes tool result |
| Permission gate | `permission.Service` + shell `PreToolUse` hooks | None; `beforeToolCall` hooks only | None; extension `tool_call` hooks only |
| Hook veto | Yes (deny/halt), plus permission deny | Yes (`Block`) | Yes (`block`/throw) |
| Hook rewrite args | Yes (`updated_input`) | Yes (`Args`) | Yes (extension result) |
| MCP | Official go-sdk; stdio/HTTP/SSE | Hand-written JSON-RPC; stdio/HTTP | Hand-written JSON-RPC; stdio/HTTP |
| MCP tool name | `mcp_<server>_<tool>` | `mcp__<server>__<tool>` | `mcp__<server>__<tool>` |
| Abort mechanism | Per-session context + `activeRequests` + settle | `context.CancelFunc` on Agent | `AbortController` |
| Steering point | `PrepareStep` (between steps) | loop boundary (between turns) | loop boundary (between turns) |
| Compaction | Auto-summarize via `StopWhen` + `Summarize` | JSONL compaction entries projected | JSONL compaction entries projected |
| Surface size | Very large; loop external | Very large; tests dominate | Large; packages split cleanly |

The table's most useful rows for a course are **history source**, **persist
trigger**, **tool error boundary**, and **steering point**. Those are the four
places where a learner's first design usually diverges from all three references.

---

## 3. Seams, verbatim

This section quotes the actual declarations. Declarations, signatures, and field
lists are verbatim; comments are trimmed where they only restate a field name or
repeat a contract already stated above. No signature or field is paraphrased.
Where a repo has no interface for a seam, that is reported explicitly rather
than inventing one.

### 3.1 Provider / model client

#### Crush — `charm.land/fantasy`

Import path: `charm.land/fantasy` (module `charm.land/fantasy v0.45.1`).

`fantasy@v0.45.1/model.go:260`:

```go
// LanguageModel represents a language model that can generate responses and stream responses.
type LanguageModel interface {
	Generate(context.Context, Call) (*Response, error)
	Stream(context.Context, Call) (StreamResponse, error)

	GenerateObject(context.Context, ObjectCall) (*ObjectResponse, error)
	StreamObject(context.Context, ObjectCall) (ObjectStreamResponse, error)

	Provider() string
	Model() string
}
```

`StreamResponse` is an iterator, not a channel
(`fantasy@v0.45.1/model.go:195`):

```go
type StreamResponse = iter.Seq[StreamPart]
```

The provider registry seam is `fantasy.Provider`
(`fantasy@v0.45.1/provider.go:8`):

```go
// Provider represents a provider of language models.
type Provider interface {
	Name() string
	LanguageModel(ctx context.Context, modelID string) (LanguageModel, error)
}
```

Crush itself does not declare a provider interface. It calls
`fantasy.NewAgent(largeModel.Model, ...)`, where `largeModel.Model` is a
`fantasy.LanguageModel` built by `coordinator.buildModel`.

#### PiG — `agent.StreamFn` and `ai.Provider`

Import paths: `github.com/MichaelKinsy/PiG/agent`,
`github.com/MichaelKinsy/PiG/ai`.

The loop's provider seam (`PiG/agent/agent.go:865`):

```go
// StreamFn starts one provider request for an already normalized transcript.
// Mirrors upstream StreamFn.
type StreamFn func(ctx context.Context, model *ai.Model, transcript ai.TranscriptContext, options ai.StreamOptions) (*ai.AssistantMessageEventStream, error)
```

The provider-level seam (`PiG/ai/types.go:934`):

```go
// Provider is the interface implemented by each LLM backend.
type Provider interface {
	// ID returns the provider identifier (e.g. "openai", "github-copilot").
	ID() string
	// Stream initiates a request from an already normalized transcript.
	Stream(ctx context.Context, transcript TranscriptContext, opts StreamOptions) (*AssistantMessageEventStream, error)
	// Close releases any persistent connections or background resources.
	io.Closer
}
```

The function type the model runtime exposes (`PiG/ai/models_runtime.go:34`):

```go
type ModelsStreamFunction func(context.Context, *Model, TranscriptContext, StreamOptions) (*AssistantMessageEventStream, error)
```

Note the difference: `StreamFn` takes a `*ai.Model`; `ai.Provider.Stream` does
not. The provider instance is already bound to its model/transport.

#### pi — `StreamFunction`

Import path: `@earendil-works/pi-ai`.

`pi/packages/ai/src/types.ts:398`:

```ts
export type StreamFunction<TApi extends Api = Api, TOptions extends StreamOptions = StreamOptions> = (
	model: Model<TApi>,
	context: TranscriptContext,
	options?: TOptions,
) => AssistantMessageEventStream;
```

The loop's config field is `streamFunction` (`AgentLoopConfig` at
`pi/packages/agent/src/types.ts:193`), and `Agent` receives a `streamFn`
(`pi/packages/agent/src/agent.ts:115-140`). `models.Provider` is the
provider-registry object (`pi/packages/ai/src/models.ts:158`), not the loop
seam.

#### Course mapping

The course's seam is a lossy compression of all three:

```go
type Provider interface {
    Chat(ctx context.Context, req Request) (Message, Usage, error)
}
```

That is intentional. All three references expose streaming because a TUI wants
tokens as they arrive and a tool call can start before the full response ends.
The course grades the final reassembled `Message`, so it moves SSE parsing
*behind* `Chat`. This is safe for every deterministic check: the fake server can
still split text and tool arguments into fragments, and `ctx` still carries
cancellation. If the course later wants token-by-token rendering, add a second
method (`ChatStream`) or a callback; do not change `Chat` mid-arc.

### 3.2 Tool

#### Crush — `fantasy.AgentTool`

Import path: `charm.land/fantasy`.

`fantasy@v0.45.1/tool.go:93`:

```go
// AgentTool represents a tool that can be called by a language model.
// This matches the existing BaseTool interface pattern.
type AgentTool interface {
	Info() ToolInfo
	Run(ctx context.Context, params ToolCall) (ToolResponse, error)
	ProviderOptions() ProviderOptions
	SetProviderOptions(opts ProviderOptions)
}
```

`ToolInfo` and `ToolCall`/`ToolResponse` are concrete structs
(`fantasy@v0.45.1/tool.go:14-48`):

```go
type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Required    []string       `json:"required"`
	Parallel    bool           `json:"parallel"` // Whether this tool can run in parallel with other tools
}

type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"`
}

type ToolResponse struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	// Data contains binary data for image/media responses (e.g., image bytes, audio data).
	Data []byte `json:"data,omitempty"`
	// MediaType specifies the MIME type of the media (e.g., image/png, audio/wav).
	MediaType string `json:"media_type,omitempty"`
	Metadata  string `json:"metadata,omitempty"`
	IsError   bool   `json:"is_error"`
	StopTurn  bool   `json:"stop_turn,omitempty"`
}
```

#### PiG — `agent.AgentTool`

Import path: `github.com/MichaelKinsy/PiG/agent`.

`PiG/agent/agent.go:362`:

```go
// AgentTool is the interface that all tools must implement.
type AgentTool interface {
	// Name is the tool's name as known to the LLM.
	Name() string
	// Label returns a human-readable display name for the tool header.
	// If empty, Name() is used for display. Mirrors upstream AgentTool.label.
	Label() string
	// Schema returns the JSON Schema for the tool's parameters.
	Schema() ai.ToolSchema
	// Execute runs the tool.
	Execute(ctx context.Context, toolCallID string, params json.RawMessage,
		onUpdate ToolUpdateCallback) (AgentToolResult, error)
	// ExecutionMode returns the tool's parallelism preference.
	ExecutionMode() ToolExecutionMode
}
```

`AgentToolResult` is large because PiG preserves upstream's structured details,
usage, termination hint, and member order. The load-bearing fields are content,
details, and isError (`PiG/agent/agent.go:250-330`).

#### pi — `AgentTool`

Import path: `@earendil-works/pi-agent-core`.

`pi/packages/agent/src/types.ts:466`:

```ts
export interface AgentTool<TParameters extends TSchema = TSchema, TDetails = any> extends Tool<TParameters> {
	/** Human-readable label for UI display. */
	label: string;
	/** Optional compatibility shim for raw tool-call arguments before schema validation. Must return an object that matches `TParameters`. */
	prepareArguments?: (args: unknown) => Static<TParameters>;
	/** JSON Schema of `structuredContent` in successful results. */
	outputSchema?: TSchema;
	/** Execute the tool call. Throw on failure, or return a result with `isError: true`; do not only describe the failure in `content`. */
	execute: (
		toolCallId: string,
		params: Static<TParameters>,
		signal?: AbortSignal,
		onUpdate?: AgentToolUpdateCallback<TDetails>,
	) => Promise<AgentToolResult<TDetails>>;
	/** Per-tool execution mode override. */
	executionMode?: ToolExecutionMode;
}
```

`AgentToolResult` (`pi/packages/agent/src/types.ts:424-450`) carries
`content`, `details`, optional `structuredContent`, `usage`, `isError`, and
`terminate`.

### 3.3 Tool registry

**None of the three repos declares a registry interface.** This is a real
finding, not an omission in this note.

- **Crush** holds a concurrent slice:
  `tools *csync.Slice[fantasy.AgentTool]` (`crush/internal/agent/agent.go:211`).
  `SetTools` replaces the slice (`:2142-2144`). Building the list is a
  coordinator function, `buildTools` (`crush/internal/agent/coordinator.go:861`),
  which appends concrete tools, filters by agent allowlist, adds MCP tools,
  sorts by name, and wraps with hooks. There is no `Register(name, tool)` API.
- **PiG** holds `opts.Tools []AgentTool` behind `toolsMu sync.RWMutex`
  (`PiG/agent/agent.go:888-890`). `SetTools` clones the slice
  (`:1009-1013`); `toolList` clones it for readers (`:1016-1020`). Dispatch is a
  linear scan by name in `findTool` (`PiG/agent/tool_execution.go:342-349`).
- **pi** holds `AgentState.tools`; its setter clones the incoming slice and
  its getter returns the current slice (`pi/packages/agent/src/agent.ts:100-120`). Dispatch is
  `currentContext.tools?.find(t => t.name === toolCall.name)`
  (`pi/packages/agent/src/agent-loop.ts:726`).

The closest thing to a registry seam is the **tool list passed into the loop**,
not a registry object. For a teaching harness, introducing a small `Registry`
type is a genuine improvement: it gives one place to enforce duplicate-name
errors, deterministic ordering, panic recovery, and runtime registration for
MCP.

### 3.4 Session store

#### Crush — two interfaces

Import paths: `github.com/charmbracelet/crush/internal/session`,
`github.com/charmbracelet/crush/internal/message`.

`crush/internal/session/session.go:67`:

```go
type Service interface {
	pubsub.Subscriber[Session]
	Create(ctx context.Context, title string) (Session, error)
	CreateTitleSession(ctx context.Context, parentSessionID string) (Session, error)
	CreateTaskSession(ctx context.Context, toolCallID, parentSessionID, title string) (Session, error)
	Get(ctx context.Context, id string) (Session, error)
	GetLast(ctx context.Context) (Session, error)
	List(ctx context.Context) ([]Session, error)
	Save(ctx context.Context, session Session) (Session, error)
	SetChannel(ctx context.Context, sessionID, channel string) (Session, error)
	UpdateTitleAndUsage(ctx context.Context, sessionID, title string, promptTokens, completionTokens int64, cost float64) error
	Rename(ctx context.Context, id string, title string) error
	Delete(ctx context.Context, id string) error
	MCPDisabledServers(ctx context.Context) ([]string, error)
	SetMCPServerDisabled(ctx context.Context, name string, disabled bool) error
	MCPServersEnabled(ctx context.Context) ([]string, error)

	// Agent tool session management
	CreateAgentToolSessionID(messageID, toolCallID string) string
	ParseAgentToolSessionID(sessionID string) (messageID string, toolCallID string, ok bool)
	IsAgentToolSession(sessionID string) bool
}
```

`crush/internal/message/message.go:46`:

```go
type Service interface {
	pubsub.Subscriber[Message]
	Create(ctx context.Context, sessionID string, params CreateMessageParams) (Message, error)
	Update(ctx context.Context, message Message) error
	Get(ctx context.Context, id string) (Message, error)
	List(ctx context.Context, sessionID string) ([]Message, error)
	// ListFromSummary returns the messages at or after summaryMessageID,
	// which is all a compacted session still sends.
	ListFromSummary(ctx context.Context, sessionID, summaryMessageID string) ([]Message, error)
	ListUserMessages(ctx context.Context, sessionID string) ([]Message, error)
	ListAllUserMessages(ctx context.Context) ([]Message, error)
	GetLastAssistantMessage(ctx context.Context, sessionID string) (Message, error)
	Delete(ctx context.Context, id string) error
	DeleteSessionMessages(ctx context.Context, sessionID string) error

	// Flush synchronously drains any pending debounced state for the
	// given message ID, performs the SQL write, and publishes the
	// resulting [pubsub.UpdatedEvent].
	Flush(ctx context.Context, id string) error
	FlushAll(ctx context.Context) error
}
```

#### PiG — concrete `*codingagent.Session`

Import path: `github.com/MichaelKinsy/PiG/coding` (public alias) or
`github.com/MichaelKinsy/PiG/internal/codingagent` (implementation; internal to
the module).

`PiG/coding/session_manager.go:12`:

```go
type SessionManager = icodingagent.Session
```

The persistence methods are concrete, not an interface
(`PiG/internal/codingagent/session.go`):

```go
func (s *Session) AppendEntry(entry any) error
func (s *Session) Persist(entry SessionEntry) error
func (s *Session) BuildContext(leafID *string) []agent.AgentMessage
```

`AppendEntry` marshals one line and calls `appendEntryRecord`
(`:336-343`). `Persist` writes an entry the session already holds
(`:414-417`). `BuildContext` projects the branch (`:1175`). There is no
`SessionStore` interface to swap for a fake.

#### pi — concrete `SessionManager`

Import path: `@earendil-works/pi-coding-agent`.

`pi/packages/coding-agent/src/core/session-manager.ts:987` is
`export class SessionManager`. `appendMessage` (`:1204`) creates the entry;
`_appendEntry` (`:1191`) pushes it; `_persist` (`:1172`) appends one JSON line.
No interface.

### 3.5 Permission / hook gate

#### Crush — `permission.Service`

Import path: `github.com/charmbracelet/crush/internal/permission`.

`crush/internal/permission/permission.go:66`:

```go
type Service interface {
	pubsub.Subscriber[PermissionRequest]
	// GrantPersistent grants a permission request and remembers the grant
	// for the session. It returns true if this call actually resolved the
	// pending request; false if the request had already been resolved
	// (e.g., by another concurrent caller) or is unknown.
	GrantPersistent(permission PermissionRequest) bool
	// Grant grants a permission request. It returns true if this call
	// actually resolved the pending request; false if the request had
	// already been resolved or is unknown.
	Grant(permission PermissionRequest) bool
	// Deny denies a permission request. It returns true if this call
	// actually resolved the pending request; false if the request had
	// already been resolved or is unknown.
	Deny(permission PermissionRequest) bool
	Request(ctx context.Context, opts CreatePermissionRequest) (bool, error)
	// ActiveRequest returns the request that is currently awaiting a
	// decision, or false when no request is pending. A request is only
	// reported while it can still be resolved; a request whose waiter
	// was cancelled is not returned.
	ActiveRequest() (PermissionRequest, bool)
	AutoApproveSession(sessionID string)
	SetSkipRequests(skip bool)
	SkipRequests() bool
	SubscribeNotifications(ctx context.Context) <-chan pubsub.Event[PermissionNotification]
}
```

The shell-hook side is a concrete `*hooks.Runner`, not an interface. The wrapper
is `hookedTool` (`crush/internal/agent/hooked_tool.go:18-21`), and the veto is:

```go
if result.Decision == hooks.DecisionDeny || result.Halt {
	reason := fmt.Sprintf("Tool call blocked by hook. Reason: %s", result.Reason)
	if result.Halt {
		reason = fmt.Sprintf("Turn halted by hook. Reason: %s", result.Reason)
	}
	resp := fantasy.NewTextErrorResponse(reason)
	resp.StopTurn = result.Halt
	resp.Metadata = hookMetadataJSON(result)
	return resp, nil
}
```

That is `crush/internal/agent/hooked_tool.go:62-75`.

The hook decision types (`crush/internal/hooks/hooks.go:46-76`):

```go
type Decision int

const (
	// DecisionNone means the hook expressed no opinion.
	DecisionNone Decision = iota
	// DecisionAllow means the hook explicitly allowed the action.
	DecisionAllow
	// DecisionDeny means the hook blocked the action.
	DecisionDeny
)

// Decision.String() is elided.

type HookResult struct {
	Decision     Decision
	Halt         bool   // If true, halt the whole turn.
	Reason       string // Deny or halt reason (same field, different audience).
	Context      string
	UpdatedInput string // Shallow-merge patch against tool_input (opaque JSON).
}
```

#### PiG — no permission service, only hook funcs

Import path: `github.com/MichaelKinsy/PiG/agent`.

`PiG/agent/agent.go:690`:

```go
type ToolCallHookResult struct {
	Block     bool
	Reason    string
	Terminate bool
	// Mutated args (if the hook wants to rewrite them). They execute without
	// revalidation, as upstream executes a hook's mutated args.
	Args json.RawMessage
	// Err is the error the hook threw. Upstream's beforeToolCall throws instead (agent-loop.ts prepareToolCall catch): the tool does not run and its error result carries Err's message, even an empty one, with no default reason and no terminate.
	Err error
}
```

`PiG/agent/agent.go:703`:

```go
// BeforeToolCallHook is called before a tool executes.
// Return ToolCallHookResult{Block: true} to prevent execution.
type BeforeToolCallHook func(ctx context.Context, toolCallID, toolName string, args json.RawMessage) ToolCallHookResult
```

There is no `permission.Service` equivalent. The README states the design
explicitly (`PiG/README.md:169`).

#### pi — extension handler, no core service

`pi/packages/agent/src/types.ts:66`:

```ts
export interface BeforeToolCallResult {
	block?: boolean;
	reason?: string;
	/**
	 * Hint that the agent should stop after the current tool batch when this call is blocked.
	 */
	terminate?: boolean;
}
```

The loop enforces it (`pi/packages/agent/src/agent-loop.ts:755-765`):

```ts
if (beforeResult?.block) {
	const result = createErrorToolResult(beforeResult.reason || "Tool execution was blocked");
	if (beforeResult.terminate === true) {
		result.terminate = true;
	}
	return {
		kind: "immediate",
		result,
		isError: true,
	};
}
```

### 3.6 MCP client

#### Crush — go-sdk client + transport

Import paths: `github.com/modelcontextprotocol/go-sdk/mcp`,
`github.com/charmbracelet/crush/internal/agent/tools/mcp`.

Crush does not declare an MCP client interface. It uses the SDK's
`*mcp.Client`, created at `crush/internal/agent/tools/mcp/init.go:1092`, and the
SDK's transport interface (`go-sdk@v1.7.1-…/mcp/transport.go:52`):

```go
type Transport interface {
	// Connect returns the logical JSON-RPC connection.
	//
	// It is called exactly once by [Server.Connect] or [Client.Connect].
	Connect(ctx context.Context) (Connection, error)
}
```

The SDK's stdio transport is newline-delimited JSON
(`go-sdk@v1.7.1-…/mcp/transport.go:121-124`):

```go
// A StdioTransport is a [Transport] that communicates over stdin/stdout using
// newline-delimited JSON.
type StdioTransport struct{}
```

Crush's in-repo seam is the tool adapter, not the client. `mcp.Tool` is an alias
for the SDK type (`crush/internal/agent/tools/mcp/tools.go:16`), and
`GetMCPTools` returns `[]*tools.Tool`, where `tools.Tool` implements
`fantasy.AgentTool` (`crush/internal/agent/tools/mcp-tools.go:24-40`).

#### PiG — hand-written client + `mcp.Transport`

Import path: `github.com/MichaelKinsy/PiG/mcp`.

`PiG/mcp/transport.go:34`:

```go
type Transport interface {
	Start() error
	Send(ctx context.Context, message JSONRPCMessage) error
	Close() error
	OnMessage(listener MessageListener) (dispose func())
	OnError(listener ErrorListener) (dispose func())
	OnClose(listener CloseListener) (dispose func())
}
```

The client is concrete: `*mcp.Client` (`PiG/mcp/client.go:101`), with
`NewClient` (`:128`), `Connect` (`:355`), `ListTools` (`:509`), `CallTool`
(`:684`). There is no `MCPClient` interface.

#### pi — hand-written class + `McpTransport`

Import path: `@earendil-works/pi-mcp` (workspace package `packages/mcp`).

`pi/packages/mcp/src/transports/transport.ts:18`:

```ts
export interface McpTransport {
	start(): Promise<void>;
	send(message: JsonRpcMessage, options?: McpTransportSendOptions): Promise<void>;
	close(): Promise<void>;
	onMessage(listener: McpTransportMessageListener): () => void;
	onError(listener: McpTransportErrorListener): () => void;
	onClose(listener: McpTransportCloseListener): () => void;
	setProtocolVersion?(version: string): void;
}
```

The client is `export class Client` (`pi/packages/mcp/src/client.ts`), with
`connect` (`:204`), `listTools` (`:296`), `callTool` (`:378`). No interface.

### 3.7 Seam summary

| Seam | Crush | PiG | pi |
|---|---|---|---|
| Provider | `fantasy.LanguageModel` (external) | `agent.StreamFn`, `ai.Provider` | `StreamFunction`, `models.Provider` |
| Tool | `fantasy.AgentTool` (external) | `agent.AgentTool` | `AgentTool` |
| Registry | none; `*csync.Slice` | none; `[]AgentTool` + mutex | none; `AgentState.tools` |
| Session | `session.Service` + `message.Service` | concrete `*codingagent.Session` | concrete `SessionManager` |
| Gate | `permission.Service` + `*hooks.Runner` | `BeforeToolCallHook` func | `beforeToolCall` callback |
| MCP | go-sdk `*mcp.Client` + `mcp.Transport` | concrete `*mcp.Client` + `mcp.Transport` | concrete `Client` + `McpTransport` |

The course should define interfaces for the two seams the references leave
concrete: **session store** and **tool registry**. Those are exactly the seams a
deterministic test suite wants to substitute. The provider and tool seams are
already interfaces in Crush/PiG; the course's `Provider` mirrors that. The gate
is a function or one-method interface; that is sufficient.

---

## 4. The request journey as one ordered sequence

The three journeys differ in names, but the stages are the same. This table is
the compressed sequence; §1 has the full per-repo call chain.

| # | Stage | Crush | PiG | pi |
|---|---|---|---|---|
| 1 | CLI entry | `main.main` → `cmd.Execute` (`crush/main.go:15,25`) | `cmd/pig/main.go:10` → `cli.Main` (`PiG/coding/cli/main.go:579`) | `cli.ts:6` → `main` (`pi/packages/coding-agent/src/main.ts:574`) |
| 2 | Mode dispatch | `runCmd.RunE` (`crush/internal/cmd/run.go:111`) | `startPrintMode` (`PiG/coding/cli/print_mode.go:125`) | `runPrintMode` (`pi/packages/coding-agent/src/modes/print-mode.ts:33`) |
| 3 | Build/choose session | `App.resolveSession` (`crush/internal/app/app.go:382`) | `SessionManagerFor` + `CreateAgentSessionRuntime` (`PiG/coding/cli/print_mode.go:143,151`) | `createAgentSessionRuntime` (`pi/packages/coding-agent/src/main.ts:866`) |
| 4 | Submit prompt | `AgentCoordinator.Run` (`crush/internal/app/app.go:421`) | `sendPrintPrompt` → `SendContentWithPreflight` (`PiG/coding/cli/print_mode.go:571,593`) | `session.prompt` (`pi/packages/coding-agent/src/modes/print-mode.ts:132`) |
| 5 | Claim run / prepare | `sessionAgent.Run` (`crush/internal/agent/agent.go:609`) | `prepareContentRun` → `BeginPreparedPrompt` (`PiG/coding/session.go:965`, `PiG/coding/session_preflight.go:141`) | `_startRun` (`pi/packages/coding-agent/src/core/agent-session.ts:1873`) |
| 6 | Build loop context | `getSessionMessages` + `preparePrompt` (`crush/internal/agent/agent.go:805,831`) | `runLoop` `slices.Concat` (`PiG/agent/agent_loop.go:158`) | `createContextSnapshot` (`pi/packages/agent/src/agent.ts:469`) |
| 7 | Provider request | `agent.Stream` → `LanguageModel.Stream` (`crush/internal/agent/agent.go:854`; `fantasy@v0.45.1/agent.go:1045`) | `runTurn` → `requestStream` → `streamFn` (`PiG/agent/agent_loop.go:291,534,539`) | `streamAssistantResponse` → `streamFunction` (`pi/packages/agent/src/agent-loop.ts:390,412`) |
| 8 | Wire parsing | OpenAI SSE in fantasy (`fantasy@v0.45.1/providers/openai/language_model.go:454`) | `parseSSE` (`PiG/ai/openai.go:1878`) | OpenAI adapter (`pi/packages/ai/src/api/openai-completions.ts:411`) |
| 9 | Finalize assistant | `OnTextDelta`/`OnToolCall` (`crush/internal/agent/agent.go:973,1014`) | `appendAssistant` (`PiG/agent/agent_loop.go:317`) | `processEvents` `message_end` (`pi/packages/agent/src/agent.ts:575`) |
| 10 | Execute tools | fantasy `processStepStream` + `executeSingleTool` (`fantasy@v0.45.1/agent.go:1411,802`) | `executeToolCalls` (`PiG/agent/tool_execution.go:58`) | `executeToolCalls` (`pi/packages/agent/src/agent-loop.ts:518`) |
| 11 | Persist finalized | `messages.Create/Update` callbacks; `OnStepFinish` (`crush/internal/agent/agent.go:1031,1053`) | `message_end` → `persistMessage` (`PiG/coding/session_boundaries.go:49`, `PiG/coding/session.go:810`) | `message_end` → `appendMessage` (`pi/packages/coding-agent/src/core/agent-session.ts:1165`) |
| 12 | Render / complete | `App.RunNonInteractive` prints message deltas (`crush/internal/app/app.go:477`) | print mode event stream | print mode event stream |

Two stage-level observations:

- **Crush persists during stage 9, not at stage 11.** It creates the assistant
  row before the stream, updates it on every delta, and creates tool rows as
  results arrive. PiG/pi finalize the whole assistant message first, then persist
  on `message_end`. This is why "what is the source of truth" has different
  answers in the three repos.
- **All three buffer tool calls past stage 8.** The loop does not dispatch a
  tool when the first tool-call fragment arrives. It waits for the stream to
  end and the finish reason to be known. That is the single most important
  defense against truncated arguments.

---

## 5. Minimal teaching model in Go

The course already fixes the outer seam in `harness/types.go`:

```go
type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
	Name       string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Provider interface {
	Chat(ctx context.Context, req Request) (Message, Usage, error)
}
```

Do not replace those. The following is the smallest additional surface that
makes chapters 02–11 implementable and gradeable. Every type below is
load-bearing for at least one graded claim; the "removed" notes say what to
leave out and why.

### 5.1 Provider: keep `Chat`, hide SSE

```go
// harness/provider_openai.go

type wireRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Tools       []wireTool    `json:"tools,omitempty"`
	Stream      bool          `json:"stream"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // always "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireTool struct {
	Type     string `json:"type"` // always "function"
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type wireChunk struct {
	Choices []struct {
		Delta struct {
			Role      string         `json:"role,omitempty"`
			Content   string         `json:"content,omitempty"`
			ToolCalls []wireDeltaTC  `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type wireDeltaTC struct {
	Index    *int   `json:"index,omitempty"` // the stable key
	ID       string `json:"id,omitempty"`    // only on the first fragment
	Function struct {
		Name      string `json:"name,omitempty"`      // only on the first fragment
		Arguments string `json:"arguments,omitempty"` // every fragment
	} `json:"function"`
}
```

The accumulator is the part to teach:

```go
type callAcc struct {
	id, name, args string
}

func assembleToolCalls(deltas []wireDeltaTC, acc map[int]*callAcc, open *int) {
	for _, d := range deltas {
		idx := *open // fallback: no index means "the call still being built"
		if d.Index != nil {
			idx = *d.Index
			*open = idx
		} else if idx < 0 {
			idx = len(acc) // first fragment also omitted index: start a call
			*open = idx
		}
		c := acc[idx]
		if c == nil {
			c = &callAcc{}
			acc[idx] = c
		}
		if d.ID != "" {
			c.id = d.ID
		}
		if d.Function.Name != "" {
			c.name = d.Function.Name
		}
		c.args += d.Function.Arguments // append, never assign
	}
}

// open := -1; for each chunk: assembleToolCalls(chunk.Choices[0].Delta.ToolCalls, acc, &open)
```

Then, after `[DONE]`, sort the map keys ascending and emit `[]ToolCall` in that
order. That is the entire chapter-02 lesson.

**Removed:** streaming API, retries/backoff, OAuth, provider registry, model
catalog, multimodal content, tool choice, response formats, prompt caching,
websockets, Anthropic. Safe because the course has one wire format, one fake
server, and `ctx` covers cancellation. Retries can be added later as a wrapper
around `Chat` without changing the seam.

### 5.2 Tool: one method, errors are results

```go
// harness/tool.go

type Tool interface {
	Def() ToolDef
	Run(ctx context.Context, call ToolCall) (string, error)
}
```

Two methods. `Def` is what the model sees; `Run` is what the harness executes.
Returning `(string, error)` keeps Go's normal error idiom, but the **loop must
convert a non-nil error into a tool result**, never abort the run:

```go
content, err := tool.Run(ctx, call)
if err != nil {
	if ctx.Err() != nil {
		return "", ctx.Err() // real cancellation
	}
	content, err = "tool error: "+err.Error(), nil
}
```

That boundary is the difference between Pi/PiG (tool error is a result) and
Crush's fantasy (a Go error is a critical error that ends the step; see
`fantasy@v0.45.1/agent.go:847-856`). Crush's own tools work around it by
returning `fantasy.NewTextErrorResponse(...)`, and a recent commit is titled
"stop tool failures from ending the agent turn." The course should teach the
Pi/PiG boundary, not the fantasy one.

**Removed:** structured `details`, images/audio, progress callbacks, per-tool
execution mode, schema generation from Go types, output schemas, terminate
hints. Safe because the course's tools are text-in/text-out and the grader
asserts on model-visible content. Panic recovery is five lines in the registry
and worth keeping; it turns a bad tool into a failed result instead of a dead
process.

### 5.3 Registry: one map, deterministic order

```go
// harness/registry.go

type Registry struct {
	mu     sync.RWMutex
	byName map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry
func (r *Registry) Add(t Tool) error          // duplicate name is an error
func (r *Registry) Defs() []ToolDef           // sorted by Name
func (r *Registry) Call(ctx context.Context, call ToolCall) (content string, err error)
```

`Call` does four things, in order:

1. look up `call.Name`; unknown → `"unknown tool: <name>"`, `nil` error;
2. defer `recover()`; a panic → `"tool panicked: <value>"`, `nil` error;
3. call `Run`;
4. convert a non-nil `Run` error to a result and `nil` error, **unless**
   `ctx.Err() != nil`; cancellation is the only error that escapes `Call`.

`Defs` must sort by name. Map iteration order is random; a request whose
`tools` array changes order between turns is a real source of nondeterminism
and cache misses. All three references sort or preserve a stable order
(Crush sorts `filteredTools` by `Info().Name` at
`crush/internal/agent/coordinator.go:973-975`; Pi/PiG preserve the configured
loadout order).

**Removed:** allow/deny lists, per-agent tool sets, tool loadout declarations in
the transcript, dynamic enable/disable, MCP namespacing. Safe for chapters
03–10. Chapter 11 adds MCP tools with `Add` and prefixes their names.

### 5.4 Session: append-only JSONL, IDs are idempotence keys

```go
// harness/session.go

type Entry struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"` // "message" | "summary"
	Message   Message `json:"message,omitempty"`
	Summary   string  `json:"summary,omitempty"`
	Timestamp int64   `json:"ts"`
}

type Store interface {
	Append(ctx context.Context, sessionID string, e Entry) error
	Load(ctx context.Context, sessionID string) ([]Entry, error)
}
```

Minimal implementation:

```go
type JSONLStore struct {
	dir string
	mu  sync.Mutex
}

func (s *JSONLStore) Append(ctx context.Context, sessionID string, e Entry) error
func (s *JSONLStore) Load(ctx context.Context, sessionID string) ([]Entry, error)
```

Rules that must be in the implementation, not just the type:

- one `O_APPEND|O_CREATE|O_WRONLY` open per append, one `Write` of
  `json.Marshal(e)` plus `'\n'`;
- `MkdirAll` the session directory once;
- `Load` scans lines, ignores a final line without `\n` only if it fails to
  parse (a torn write), and errors on any other malformed line;
- `Append` keeps an in-memory `seen map[string]struct{}` loaded from the file
  and returns nil for a duplicate `ID`;
- `sessionID` is sanitized before it becomes a filename (`^[A-Za-z0-9_-]+$`).

The loop supplies IDs:

- user entry: the caller's `turnID`;
- assistant entry: `turnID + ":a" + strconv.Itoa(step)`;
- tool entry: `turnID + ":t:" + call.ID`.

That makes retrying the same turn idempotent without hashing content.

**Removed:** branching/parent IDs, compaction entries, labels, custom entry
types, migration/versioning, HTML export, concurrent writers, file watching.
Safe for a linear course. A `summary` entry is enough for chapter 07; the
projection rule is "if a summary entry exists, send the summary followed by
messages after it," not "rewrite old lines."

### 5.5 Loop: one step at a time, one result per call

```go
// harness/loop.go

type EventKind string

const (
	EventAssistant  EventKind = "assistant"
	EventToolCall   EventKind = "tool_call"
	EventToolResult EventKind = "tool_result"
	EventDone       EventKind = "done"
)

type Event struct {
	Kind    EventKind
	Message Message
	Call    ToolCall
	Result  string
}

type Loop struct {
	Provider Provider
	Tools    *Registry
	Store    Store
	Gate     Gate // optional
	MaxSteps int  // e.g. 32
	Steer    <-chan Message // optional; drained before each Chat
	Emit     func(Event)

	// LastUsage is the usage of the most recent Chat. Chapter 07 reads it
	// to decide when to prune; the minimal loop only records it.
	LastUsage Usage
}

func (l *Loop) Run(ctx context.Context, sessionID, turnID string, user Message) (Message, error)
```

The algorithm, in order:

1. `entries, err := Store.Load(ctx, sessionID)`; build `history []Message`.
2. Append `user` to `history`; `Store.Append` with ID `turnID`.
3. For `step := 0; step < MaxSteps; step++`:
   1. Drain `Steer` non-blocking; append each as a user message and persist
      with a fresh ID (`turnID + ":s" + n`). This is the seam for chapter 08.
   2. `req := Request{Messages: slices.Clone(history), Tools: Tools.Defs()}`
      — a snapshot, never a pointer to `history`.
   3. `assistant, usage, err := Provider.Chat(ctx, req)`; `l.LastUsage = usage`.
      - `err != nil`: if `ctx.Err() != nil`, return it; otherwise return the
        error (provider failure is a harness failure, not a tool failure).
   4. Append `assistant` to `history`; persist with ID
      `turnID + ":a" + step`; emit `EventAssistant`.
   5. If `len(assistant.ToolCalls) == 0`, emit `EventDone`, return assistant.
   6. For each `call` in `assistant.ToolCalls`, **in order**:
      - if `Gate != nil`, call `Gate.BeforeTool`; on deny, synthesize
        `content = "denied: " + reason`; on allow, use any rewritten
        `call.Arguments`;
      - `content, err := Tools.Call(ctx, call)`; if `err != nil`, return it
        (cancellation is the only escaping error);
      - `toolMsg := Message{Role: RoleTool, ToolCallID: call.ID,
        Name: call.Name, Content: content}`;
      - append `toolMsg` to `history`; persist with ID `turnID + ":t:" +
        call.ID`; emit `EventToolResult`.
   7. Continue the loop. Do **not** call the model once per tool result; call
      it once after the whole batch.
4. `MaxSteps` exceeded: return `ErrMaxSteps` (and leave the transcript with the
   last assistant message and all tool results, so a resume is well-formed).

The event order is the grammar the chapter-04 checks should assert:

```
assistant
tool_call(call_1)
tool_result(call_1)
tool_call(call_2)
tool_result(call_2)
assistant
done
```

**Removed:** parallel tool execution, stop conditions beyond `MaxSteps`,
auto-summarization, retries, follow-up queues, agent events for extensions,
per-step model/tool replacement, `PrepareStep`/`PrepareNextTurn` hooks. Safe:
sequential execution is deterministic and matches the course's single fake
server; `MaxSteps` is the only loop guard a learner needs; summarization is a
chapter-07 concern layered on top, not inside the loop.

### 5.6 Gate: one method, deny is a result

```go
// harness/gate.go

type Decision struct {
	Allow  bool
	Reason string
	Args   string // if non-empty, replaces call.Arguments
}

type Gate interface {
	BeforeTool(ctx context.Context, call ToolCall) (Decision, error)
}
```

Rules:

- `Gate == nil` means allow.
- `Allow == false` produces a tool result, not a run error. The model can see
  the denial and try another approach.
- `Args != ""` replaces the raw arguments **before** the tool decodes them. If
  you validate before rewriting, you have a TOCTOU hole.
- A non-nil `error` is a harness failure (abort the run), not a denial. This
  distinction is what keeps a buggy hook from looking like a policy decision.

That is the smallest gate that can veto, rewrite, and be tested. It is
deliberately **before-only**. Pi/PiG also have after-hooks because they let
extensions replace a tool result; the course has no such feature and no UI
rendering, so an after-hook is not load-bearing.

**Removed:** shell-command hooks, JSON payload/env protocol, matcher regexes,
persistent per-session grants, interactive permission prompts, auto-approve,
path sandboxing, after-hooks. Safe: the course's chapter 03 grades tool
behavior, not policy; chapter 08 grades abort/steering, not approvals. Adding a
shell-hook protocol would add a subprocess, a JSON contract, and a timeout
policy without a graded claim.

### 5.7 MCP: newline JSON-RPC, one request at a time

```go
// harness/mcp.go

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type MCPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int
	mu     sync.Mutex // one request at a time
}

func DialMCP(ctx context.Context, command string, args ...string) (*MCPClient, error)
func (c *MCPClient) Initialize(ctx context.Context) error
func (c *MCPClient) ListTools(ctx context.Context) ([]ToolDef, error)
func (c *MCPClient) CallTool(ctx context.Context, name, arguments string) (string, error)
func (c *MCPClient) Close() error
```

Minimal wire rules:

- write `json.Marshal(msg)` plus `'\n'`; read with `bufio.Reader.ReadBytes('\n')`
  or a scanner with a large buffer;
- do **not** use `Content-Length` headers (that is LSP, not MCP stdio);
- `Initialize` sends:
  ```json
  {"jsonrpc":"2.0","id":1,"method":"initialize","params":{
    "protocolVersion":"2025-06-18",
    "capabilities":{},
    "clientInfo":{"name":"lathe","version":"0.1.0"}}}
  ```
  then reads the response, checks `result.capabilities.tools`, then sends
  `{"jsonrpc":"2.0","method":"notifications/initialized"}` with **no id**;
- `ListTools` sends `tools/list` and decodes `result.tools[].inputSchema` into
  `ToolDef.Parameters` (the schema object itself, not a string);
- `CallTool` sends `tools/call` with `{name, arguments}` where `arguments` is a
  JSON object, not a string;
- `Close` closes stdin, waits briefly, then kills the process group.

The adapter is the only bridge to the native tool seam:

```go
type mcpTool struct {
	client     *MCPClient
	def        ToolDef
	remoteName string
}

func (t mcpTool) Def() ToolDef { return t.def }

func (t mcpTool) Run(ctx context.Context, call ToolCall) (string, error) {
	return t.client.CallTool(ctx, t.remoteName, call.Arguments)
}
```

Register it with a namespaced `ToolDef.Name` (e.g.
`"mcp__" + server + "__" + tool`) so two servers cannot collide and the model
can see provenance.

**Removed:** HTTP/SSE transports, OAuth, resources, prompts, sampling, roots,
progress notifications, pagination, `tools/list_changed`, reconnect, request
cancellation, server-initiated requests, JSON-RPC batching, protocol version
negotiation beyond one accepted version. Safe for chapter 11's stated claim:
"stdio JSON-RPC handshake, `tools/list` → `ToolDef`s, `tools/call`." Everything
else is a separate chapter or a production concern.

### 5.8 What was removed, and why it is safe

| Area | Kept | Removed | Why safe |
|---|---|---|---|
| Provider | `Chat`, SSE reassembly, typed HTTP error, usage, ctx | streaming API, retries, OAuth, provider registry, multimodal, caching | one fake server, one wire format; `Chat` returns the same final message; retries wrap it later |
| Tool | `Def`, `Run`, text result, error-as-result | images, details, progress, output schema, per-tool mode | course tools are text; grader asserts model-visible text |
| Registry | map + mutex, sorted defs, panic recovery | allowlists, loadout declarations, enable/disable | single agent, fixed tool set until MCP chapter |
| Session | JSONL append, IDs, load, summary entry | branching, labels, migrations, concurrent writers, export | linear course; summary entry covers budget chapter |
| Loop | sequential steps, max steps, events, steering channel | parallel tools, retries, auto-compaction, follow-ups, step hooks | deterministic; one model call per batch; max steps bounds runaway |
| Gate | before-only veto/rewrite | shell hooks, permission UI, persistent grants, path sandbox | no policy chapter; one function covers veto and rewrite |
| MCP | stdio newline JSON-RPC, initialize, list, call | HTTP/SSE, OAuth, resources/prompts, pagination, notifications | chapter claim is exactly the minimal subset |

---

## 6. Naive-reimplementation failure modes

These are written as graded cases. Each names the mechanism, the wrong code
shape, and the assertion that catches it. They are the cases a competent Go
developer actually ships on the first attempt; the references above show the
same scars.

### 6.1 Provider wire format / SSE

1. **Assign instead of append.**
   `msg.Content = delta` passes a one-chunk fixture and truncates a real
   answer. The fake server deliberately splits text into four fragments
   (`lathe/internal/fake/server.go:245-252`). Assert full equality.
2. **Parse `[DONE]` as JSON.**
   `json.Unmarshal([]byte(" [DONE]"))` returns an error, so a successful stream
   looks like a parse failure. Assert `[DONE]` returns the accumulated message
   with no error.
3. **Ignore `scanner.Err()`.**
   A mid-stream connection reset looks like a short, valid answer. The fake
   server hijacks and closes the connection after one chunk
   (`lathe/internal/fake/server.go:232-242`). Assert a non-nil error that is not a
   successful short message.
4. **Check JSON before HTTP status.**
   A 429 with an HTML/JSON error body becomes "invalid character '<'" instead
   of `*harness.HTTPError`. Assert `errors.As(err, *HTTPError)` and `Status`.
5. **Omit `"stream": true`.**
   The server may return one non-SSE JSON body; the client then waits for
   `[DONE]` forever or parses zero chunks. Assert the recorded request has
   `stream == true`.
6. **Only process chunks with content.**
   Usage rides on a chunk with an empty delta and a `finish_reason`
   (`lathe/internal/fake/server.go:289-295`). Assert `Usage` equals the fixture.
7. **Use `context.Background()` for the request.**
   Cancel test hangs until the server times out. Assert `errors.Is(err,
   context.Canceled)` promptly after cancel.
8. **Split SSE on `"\n\n"`.**
   Comment lines (`: keepalive`), `event:`, `id:`, and `data:` without a space
   break naive splitting. Assert a fixture with a comment line and `data:{...}`.
9. **Use a default 64 KiB scanner buffer.**
   A large tool argument or a long text chunk yields `bufio.Scanner: token too
   long`. Either raise `Scanner.Buffer` or use `bufio.Reader`. Assert a >64 KiB
   argument round-trips.
10. **Treat a non-2xx as a stream.**
    `http.Client.Do` returns a response for 4xx/5xx; you must check
    `StatusCode` before `Content-Type` or body parsing. Assert a typed error.

### 6.2 Tool-call fragment reassembly

1. **Key fragments by `id`.**
   Continuation fragments omit `id` and `name`
   (`lathe/internal/fake/server.go:254-281`). A map keyed by `id` sees an empty key,
   creates a new call, or overwrites the first. Assert two calls with distinct
   IDs and full arguments.
2. **Assign arguments instead of appending.**
   `call.Arguments = frag` leaves only the last third. Assert the exact JSON
   string, not just that it parses.
3. **Unmarshal each fragment as complete JSON.**
   The first fragment is `{"pa` — a parse error if you decode eagerly. Decode
   only after `[DONE]` / finish. Assert no error and full arguments.
4. **Use the wire `index` as the public call ID.**
   `index` is a small integer, not an ID; two turns reuse index 0. Use the
   provider `id` when present, and a synthetic ID (`tool-call-0`) only when the
   provider omits it. Assert IDs are stable across calls and unique.
5. **Assume `index` is always present.**
   Some OpenAI-compatible servers omit it on continuation chunks. If you index
   by a zero value, you merge unrelated calls. Keep a "last open call" fallback
   (`PiG/ai/openai.go:2060-2075` does this with three maps).
6. **Reuse one accumulator across steps.**
   A second model response with index 0 appends to the previous call. Clear the
   map at the start of each response. Assert the second assistant message has
   only its own calls.
7. **Forget the empty-arguments case.**
   A call with no argument fragments should produce `"{}"` or an explicit
   error, not a call whose `Arguments` is `""` and whose tool then fails to
   unmarshal. Pick one policy and assert it.
8. **Execute a truncated call.**
   `finish_reason == "length"` with a partial JSON argument must not run. Assert
   the tool was not invoked and the model sees an error result.
9. **Emit one tool result per delta.**
   The assistant message has N calls, not N×fragments. Assert exactly N
   `tool` messages after the batch.
10. **Pair results by position instead of by ID.**
    Parallel completion order differs from call order. Assert result i has
    `tool_call_id == call[i].ID`.
11. **Concurrent map writes.**
    Parallel tool execution writing a shared `map[string]string` panics. Either
    keep tools sequential (course choice) or guard the map and sort results
    afterward.
12. **Let a tool error become a run error.**
    A failed `read` should be a tool result the model can recover from, not a
    dead turn. Assert the next request still happens and contains a `tool`
    message with the error text.

### 6.3 Tools that touch disk

1. **`exec.CommandContext` kills only the shell.**
   A bash tool that starts `sleep 100 &` leaves the child alive. Use a process
   group and kill the group (Crush does this for MCP at
   `crush/internal/agent/tools/mcp/init.go:1206-1220`; PiG does it for MCP at
   `PiG/mcp/stdio.go:252-300`).
2. **Wait before reading pipes.**
   `cmd.Wait()` before draining stdout/stderr deadlocks when the child writes
   more than the pipe buffer. Read concurrently, then wait.
3. **Truncate bytes, not runes.**
   Cutting at `maxBytes` in the middle of a UTF-8 sequence produces invalid
   JSON when the result is marshalled. Cut at a rune boundary; prefer
   head+tail with a marker.
4. **Let `read` return unbounded output.**
   One 2 GB file blows the context. Cap lines and bytes, and include a clear
   truncation marker plus the path so the model can ask for a range.
5. **Join paths without cleaning.**
   `filepath.Join(workdir, "../etc/passwd")` escapes. Resolve with
   `filepath.Abs`/`EvalSymlinks`, then require the result to be inside the
   workspace. `strings.HasPrefix` on the joined path is not enough (`/work`
   vs `/work2`).
6. **Allowlist by prefix.**
   `strings.HasPrefix(cmd, "ls")` permits `ls; rm -rf /`. Parse the command or
   use a real shell parser; a prefix check is a security bug, not a permission
   system.
7. **Ignore the exit code.**
   A failing command with useful stderr becomes a "success" result. Include
   exit code and stderr in the tool content; mark it as an error if nonzero.
8. **Write non-atomically.**
   `os.WriteFile` truncates before writing; a crash leaves an empty file. Write
   to a temp file in the same directory, `Sync`, `Rename`.
9. **Forget parent directories.**
   `write` to `a/b/c.txt` fails if `a/b` does not exist. `MkdirAll` the parent.
10. **Use `os.Stdout` in a tool.**
    A tool that prints to stdout corrupts the CLI's protocol output (especially
    in print/JSON mode). Tools must return strings, not print. Pi's print mode
    explicitly redirects stray stdout (`pi/packages/coding-agent/src/core/output-guard.ts:45-63`,
    called from `main.ts:657`).
11. **Panic on malformed JSON.**
    A tool that indexes `params["path"].(string)` panics on a missing field. The
    registry should recover, but the tool should also decode into a typed struct
    and return a clear error.
12. **Block forever on a hung command.**
    No timeout, no ctx check. Use `exec.CommandContext` plus a `WaitDelay`, and
    return `ctx.Err()` when the run is aborted.

### 6.4 The agent loop

1. **Stop on `finish_reason == "stop"` without checking tool calls.**
   A provider that returns `stop` with tool calls would end the turn before
   executing them. Follow the references: execute calls when present, and stop
   only when there are no calls (or the batch terminates).
2. **Continue on `tool_calls` with zero calls.**
   An empty `tool_calls` array with `finish_reason == "tool_calls"` loops
   forever. Require `len(calls) > 0`.
3. **No max steps.**
   A model that repeatedly calls the same failing tool spins forever. Add
   `MaxSteps` and return a typed error.
4. **One model call per tool result.**
   Sending an assistant message with N calls, then calling the model after each
   result, produces N+1 requests and a transcript the provider may reject.
   Append all N results, then call once.
5. **Append the next assistant message before the tool results.**
   The transcript becomes `assistant(calls), assistant, tool, tool`; providers
   require the results to directly follow the call. Assert the order.
6. **Use `role: "user"` for tool results.**
   OpenAI requires `role: "tool"` and `tool_call_id`. A user-role result is
   ignored or rejected.
7. **Mutate the request slice while the provider is reading it.**
   Passing `history` directly (not a clone) and appending a steering message
   during the HTTP call is a data race. Clone at the seam.
8. **Abort leaves a dangling call.**
   Cancel after the model requested a tool but before the result exists. On
   resume, the provider sees a call with no result. Settle it with an error
   result, as Crush does (`crush/internal/agent/settle.go:85-130`).
9. **Abort discards the partial assistant message.**
   A canceled turn should still persist what the model said plus a
   `finish_reason=canceled` marker, not vanish. Otherwise the UI and the next
   request disagree about what happened.
10. **Steering injected mid-stream.**
    Appending a user message to the live history while the model is streaming
    changes the meaning of the current response. Drain the queue at the top of
    the next step (Pi/PiG) or in `PrepareStep` (Crush).
11. **Error tool results treated as fatal.**
    A tool returning `error` kills the run, so the model never gets to recover.
    Convert to a result unless `ctx.Err() != nil`.
12. **Event log missing a tool result.**
    The grader's grammar requires one `tool_result` per `tool_call`. If the
    loop emits only the final assistant text, chapter 04 cannot distinguish a
    correct batch from a lost result.

### 6.5 Sessions

1. **Rewrite the JSONL file on every append.**
   Not append-only; a crash mid-rewrite loses the whole session. Append one
   line with `O_APPEND`.
2. **Omit the trailing newline.**
   The next append concatenates two JSON objects on one line; replay fails.
   Always write `line + "\n"` in one write.
3. **Fail the whole load on a torn last line.**
   A crash during the last write leaves a partial line. Drop only a final
   unterminated line that does not parse; a malformed terminated line is a real
   corruption error.
4. **Replay tool results without their calls.**
   After pruning or a partial crash, a `tool` message with no matching assistant
   call makes OpenAI reject the request. Validate pairing on load; drop orphans
   or insert a synthetic call.
5. **Key idempotence on content.**
   The same user prompt sent twice is a legitimate second turn. Use the
   caller's turn ID as the entry key; skip only an exact duplicate ID.
6. **Concurrent `Append` calls without a mutex.**
   Two goroutines open `O_APPEND` and write; on some filesystems the lines
   interleave. Serialize appends in the store.
7. **Store only user and assistant messages.**
   Resume then has no tool results, so the first replayed request is malformed.
   Persist every message role that the provider needs.
8. **Regenerate IDs on every load.**
   Idempotence and parent links break. IDs are assigned once at append time and
   read back verbatim.
9. **Use the session ID as a path without sanitizing.**
   `../../etc/passwd` becomes a file path. Restrict to `[A-Za-z0-9_-]`.
10. **Load entries in map order.**
   Go map iteration is random. Preserve file order (or the parent chain) and
   project deterministically.
11. **Prune a tool call without its result.**
   The model sees half a pair. Prune by whole turn/batch, or replace both sides
   with a marked stub.
12. **Summarize by deleting old lines.**
   The append-only log is the audit trail. Append a summary entry and make the
   projection start from it; do not rewrite history.

### 6.6 Permission / hook gate

1. **Fail open on a hook error.**
   A hook that times out or crashes is treated as "allow," so a policy failure
   becomes a security hole. Decide explicitly: error means deny (or abort), not
   allow.
2. **Return a hook denial as a Go error.**
   The loop dies instead of showing the model a denial result. Return
   `(content, nil)` with the denial text, or a typed result.
3. **Ignore the hook result.**
   Running the hook but always executing the tool is the most common wiring bug.
   Assert the tool was not called when `Allow == false`.
4. **Validate before rewriting arguments.**
   A hook rewrites `Args`, but the tool decodes the old args. Rewrite first,
   then decode/validate.
5. **Block the UI thread on a permission prompt.**
   In non-interactive mode there is no one to answer; the run hangs. Auto-approve
   (Crush does at `crush/internal/app/app.go:409`) or fail with a clear error.
6. **Leak a persistent grant across sessions.**
   Key grants by `(session, tool, action, path)`, not tool name alone. Crush's
   `PermissionKey` does this (`crush/internal/permission/permission.go:94-99`).
7. **Check a path before resolving symlinks.**
   Permission says `/work/link` is inside the workspace; the tool follows the
   link to `/etc`. Resolve first, then check.
8. **Bypass the gate in a subagent.**
   The child calls the tool directly because it constructs its own registry.
   Route every tool call through the same gate, or explicitly document the
   trust boundary.
9. **No hook timeout.**
   A shell hook that never exits hangs the turn. Use `context.WithTimeout` and
   abandon/reap the process. Crush's hook runner does this
   (`crush/internal/hooks/runner.go:172-200`).
10. **After-hook mutates a result already persisted.**
    The UI and the transcript show different results. Run the after-hook before
    emitting the tool result, as Pi/PiG do.
11. **Hook output parsing panics on empty stdout.**
    An empty hook result is "no opinion," not a JSON error. Parse empty as
    `DecisionNone` (Crush does at `crush/internal/hooks/input.go:80-84`).
12. **Hook matcher regex compiled per call.**
    Not a correctness bug, but a latency bug on every tool call. Compile once at
    config load, as `hooks.NewRunner` does (`crush/internal/hooks/runner.go:44-70`).

### 6.7 MCP

1. **Use `Content-Length` framing.**
   That is LSP. MCP stdio is newline-delimited JSON. A `Content-Length` header
   is parsed as a JSON line and fails.
2. **Let the server write logs to stdout.**
   A stray `fmt.Println` in the server corrupts the JSON stream. Servers must
   log to stderr; clients should surface stderr separately.
3. **Skip `notifications/initialized`.**
   Many servers reject `tools/list` until they see the initialized
   notification. Send it after validating the `initialize` result.
4. **Call `tools/list` without checking capabilities.**
   A resources-only server does not implement it and returns `-32601`. Check
   `result.capabilities.tools` first (PiG does at
   `PiG/coding/mcpext/runtime.go:690-699`).
5. **Pass the MCP tool object, or a JSON string, instead of its `inputSchema`.**
   The course's `ToolDef.Parameters` is the schema object itself. If your
   native seam splits `properties`/`required` (as `fantasy` does), decode
   `inputSchema` and split it; if it takes a full JSON Schema, pass it through
   unchanged. Crush's adapter shows the split
   (`crush/internal/agent/tools/mcp-tools.go:70-97`); PiG's `toParameters`
   shows the pass-through (`PiG/coding/mcpext/tools.go:470-545`). The failure
   is wrapping the whole tool object or double-encoding the schema as a string.
6. **Use the raw remote tool name.**
   Two servers both expose `search`; the model cannot distinguish them.
   Namespace (`mcp__server__tool`) and sanitize to the provider's allowed
   character set.
7. **Send `arguments` as a JSON string.**
   `tools/call` expects an object. A string is either rejected or double-encoded
   and the server sees no fields.
8. **Reuse request IDs.**
   Concurrent calls with the same `id` make responses ambiguous. Increment under
   a mutex; never use `0` as a sentinel if the server echoes it.
9. **Read with a 64 KiB scanner.**
   A large tool schema or result exceeds the default and stops the scanner.
   Raise the buffer or use `bufio.Reader`.
10. **Kill only the direct child.**
    `npx`/`uvx` spawn grandchildren; closing stdin and killing the parent leaves
    orphans. Use a process group and kill the group.
11. **Treat JSON-RPC error responses as transport errors.**
    A `-32602` is a valid response with an error; surface `error.message` to the
    model/host, not "connection failed."
12. **Treat a notification as a response.**
    Notifications have no `id`; correlating them with the pending request hangs
    or panics. Route by `id != nil`.
13. **Assume response IDs are numbers.**
    JSON-RPC allows strings. Decode `id` as a union or `json.RawMessage` and key
    by its canonical string form (PiG's `JSONRPCID` does this at
    `PiG/mcp/jsonrpc.go:17-80`).
14. **Ignore server-initiated requests.**
    A server `ping` or `roots/list` request needs a response, or the server
    stalls. At minimum answer `ping` with `{}`.
15. **Only read the first page of `tools/list`.**
    Servers paginate with `nextCursor`. Follow it until absent (PiG does at
    `PiG/mcp/client.go:509-520`).

---

## 7. Best file to read

The criterion is "clearest example, not biggest." For each area, read the
smallest file that shows the mechanism end to end.

| Area | Best overall | Best Go | Why |
|---|---|---|---|
| Provider SSE / reassembly | `pi/packages/ai/src/api/openai-completions.ts:390-700` | `PiG/ai/openai.go:1878-2140` | The TS file is a single readable stream state machine; the Go port keeps the same structure with explicit maps. |
| Tool-call fragment keying | `pi/packages/ai/src/api/openai-completions.ts:485-535` | `PiG/ai/openai.go:2050-2135` | Shows the dual index/ID map and why neither key alone is enough. |
| Tool interface | `pi/packages/agent/src/types.ts:466-500` | `PiG/agent/agent.go:362-376` | Five methods in Go; the TS version shows the same shape with TypeBox. |
| Smallest real tool | `pi/packages/coding-agent/src/core/tools/write.ts:44-97` | `crush/internal/agent/tools/write.go:46-195` | 53 lines vs 150; both show schema, validation, path handling, error result. |
| Agent loop | `pi/packages/agent/src/agent-loop.ts:172-390` | `PiG/agent/agent_loop.go:199-370` | The outer/inner loop is visible in one screen; no extension machinery. |
| Tool dispatch/order | `pi/packages/agent/src/agent-loop.ts:518-700` | `PiG/agent/tool_execution.go:58-215` | Parallel preparation, concurrent execution, source-order results. |
| Session persistence | `PiG/internal/codingagent/session.go:414-540` | same | One append function, one persist function, one JSON line; no branching noise. |
| Session projection | `pi/packages/coding-agent/src/core/session-manager.ts:476-620` | `PiG/internal/codingagent/session.go:1175-1205` | Compaction-aware branch projection in one function. |
| Permission/hook wrapper | `crush/internal/agent/hooked_tool.go:54-95` | same | The whole veto/rewrite/allow-preapproval flow in 40 lines. |
| Permission service | `crush/internal/permission/permission.go:66-250` | same | A real gate with pending requests and grants; no other repo has one. |
| Hook block in the loop | `pi/packages/agent/src/agent-loop.ts:718-787` | `PiG/agent/tool_execution.go:353-407` | Shows exactly where validation, before-hook, block, and arg rewrite sit. |
| MCP client handshake | `pi/packages/mcp/src/client.ts:204-260` | `PiG/mcp/client.go:355-430` | Initialize, validate, initialized notification, connected. |
| MCP stdio framing | `pi/packages/mcp/src/transports/stdio.ts:180-205` | `PiG/mcp/stdio.go:317-345` | Newline JSON, remainder buffer, size limit, parse. |
| MCP tool adapter | `pi/packages/coding-agent/src/extensions/mcp/tools.ts:312-345` | `PiG/coding/mcpext/tools.go:470-545` | Schema → native tool def, namespaced name, call/result conversion. |
| Abort + queues | `PiG/agent/queue.go:1-133` | same | The smallest queue with `all`/`one-at-a-time` drain semantics. |
| Event → persistence | `pi/packages/coding-agent/src/core/agent-session.ts:1103-1170` | `PiG/coding/session_boundaries.go:49-95` | The exact `message_end` hook that makes a transcript durable. |

If a learner reads only three files, they should be:
`pi/packages/agent/src/agent-loop.ts`, `PiG/agent/tool_execution.go`, and
`crush/internal/agent/hooked_tool.go`. Those three cover the loop, the tool
batch, and the gate.

---

## 8. Could not determine

These are open after static reading. They are listed rather than guessed.

1. **Crush's TUI keypress → `AgentRun` path.** I traced `crush run` and the
   shared `AgentCoordinator.Run`. The TUI's key handling and dialog flow were
   not traced; they likely call `AppWorkspace.AgentRun`
   (`crush/internal/workspace/app_workspace.go:111`), but the exact Bubble Tea
   message path is unverified.
2. **Crush's loop behavior under a future `fantasy` version.** The loop is an
   external dependency. Line numbers and behavior are for `v0.45.1`; a bump can
   change the streaming dispatch and stop conditions.
3. **Whether Crush's MCP handshake always sends legacy `initialize` first.**
   The pinned go-sdk tries SEP-2575 `server/discover` before the legacy
   `initialize` fallback (`go-sdk@v1.7.1-…/mcp/client.go:308-380`). A server that
   only implements `initialize` will see the fallback, but I did not run it
   against a fake server to observe the exact first bytes.
4. **PiG's interactive TUI prompt path.** Only print mode was traced. The TUI
   shares `Session.SendContentWithPreflight` and the same loop, but the
   keypress/command dispatch was not traced.
5. **Pi's interactive TUI prompt path.** Same: print mode was traced; the
   interactive mode shares `AgentSession.prompt` and the loop, but the TUI
   keypress path was not traced.
6. **Whether any of the three has a named "tool registry" seam elsewhere.** I
   searched for `RegisterTool`, `ToolRegistry`, `SetTools`, and `findTool`.
   Crush and PiG have no registry interface; Pi's extension system has a tool
   registry for extensions, but the core loop still receives a plain
   `AgentTool[]`. The extension registry was not analyzed.
7. **PiG/Pi permission emulation via extensions.** Both can block a tool call
   from a `beforeToolCall`/`tool_call` handler. Whether an extension can
   implement a path-level policy with the same guarantees as Crush's
   `permission.Service` (symlink resolution, persistent grants, UI prompt) was
   not tested. The core does not provide one.
8. **Token accounting and context-budget algorithms.** The three repos count
   tokens differently (provider usage, estimates, cache tokens). That is the
   subject of a separate chapter; I only noted where usage is persisted.
9. **Compaction correctness.** I read the projection code paths but did not run
   a session through compaction, branch, or resume. The course's chapter 07
   should test those invariants independently.
10. **Test suites.** None were run. The repositories contain large test suites
    (PiG alone has thousands of tests). The claims here are source-reading
    claims, not test-run claims.
11. **Line-number stability.** The clones are shallow at the commits in the
    snapshot table. A later commit will shift lines, especially in `agent.go`
    and `session-manager.ts`. The cited function names are the stable anchor.

---

## Appendix: verification commands

```bash
mkdir -p /tmp/goarch
cd /tmp/goarch
git clone --depth 1 https://github.com/charmbracelet/crush crush
git clone --depth 1 https://github.com/MichaelKinsy/PiG PiG
git clone --depth 1 https://github.com/earendil-works/pi pi

# commits used above
(cd crush && git rev-parse HEAD)
(cd PiG   && git rev-parse HEAD)
(cd pi    && git rev-parse HEAD)

# Crush's external loop and MCP SDK, at the versions its go.mod pins
(cd crush && go mod download charm.land/fantasy github.com/modelcontextprotocol/go-sdk)
```

No files outside `docs/research/` were modified. The clones live under
`/tmp/goarch/` and are disposable.
