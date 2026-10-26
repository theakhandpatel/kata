# How harnesses test themselves

### PiG, crush, pi, and what lathe's verifier should become

Research stream D for lathe. Read-only source survey plus targeted executions.
Repos cloned shallow into `/tmp/selftest/`:

| Repo | Commit | Date | What it is |
|---|---|---|---|
| `MichaelKinsy/PiG` | `f2f028f8459504deb6066d393c3f1c316596f2f5` | 2026-10-25 | Go port of pi, pinned to Pi 1.4.0 |
| `charmbracelet/crush` | `21dd7482afe2846d8cea8353cbd1ee7f41d2e9a9` | 2026-10-26 | Go coding agent |
| `earendil-works/pi` | `f9daca3480732d0042b1df3a6e27221f597a620d` | 2026-10-26 | TypeScript reference implementation |

Code excerpts preserve identifiers, literals, and line content; indentation is
normalized for Markdown. A block labeled with a line range is one contiguous excerpt;
where I needed two distant regions, they are separate blocks with their own ranges.

The short answer: **none of them tests the model.** Each moves the non-determinism to a
seam, replaces the far side with a fixture, and asserts on what the harness did to that
fixture. The interesting differences are *where* the seam is, *what* is recorded, and
*how much machinery* exists to stop the fixture from lying.

| Question | PiG | crush | pi |
|---|---|---|---|
| Fake boundary | provider interface **and** HTTP/SSE **and** real binaries | HTTP transport (VCR cassettes) **and** provider interface | provider interface (`faux`) **and** `fetch` **and** terminal |
| What is asserted | exact output bytes, JSONL records, complete HTTP request bodies, event order, files | agent messages, tool calls/results, files, usage; cassette request match | final text, event order, transcript, session; eval judge |
| Determinism | scripted faux provider, one-write SSE, JSON aliases, env scrub, oracle pin | cassettes replay exact bytes; fixed prompt clock; JSON-normalized request matching | faux provider, fake timers where needed, statistical evals |
| Smallest unit | pure helper / vector | pure conversion function | pure function |
| Largest test | real `pig` vs real `pi` (tmux/RPC/CLI) | full agent run against cassette | real model in Docker, LLM judge |
| Failure output | line diff, JSON path, artifacts, rerun script, faithfulness reminder | `cmp.Diff` on cassette body mismatch, testify diffs | vitest diff; eval report with blocked pairs and cost |
| Cost control | 811 hermetic scenarios, 1 live; `parity-fast` one pair | 14 cassettes, no API calls | unit tests on faux; evals opt-in, not in CI |

> **Bottom line for lathe.** Keep the seam and the fake HTTP server. Replace
> `Reply{Text, ToolCalls}` with a list of frames; assert the whole request; add an event
> grammar and named invariants; record goldens only from `--solution`; and mutation-test
> every case. Do not build a parity harness.

---

## 1. The shape of the problem

A coding agent is a loop around four kinds of impurity: the model, the network, the
clock, and the filesystem. You cannot make the model deterministic without replacing
it, and if you replace too much you are no longer testing the harness. Every serious
suite therefore picks a boundary and accepts a fidelity debt:

- **Transport boundary** (fake HTTP): highest fidelity, because the real SDK, retry,
  streaming, and auth code all run. Lowest control: you must speak the provider's wire
  format exactly.
- **Provider boundary** (fake `Provider`/`LanguageModel`): fast and expressive, but it
  skips the SDK's parsing and transport behavior.
- **Process boundary** (run the real binary): proves the integrated product, but is
  slow, platform-dependent, and hard to attribute.
- **Model boundary** (real API): not a test; an eval. Statistical, paid, and
  non-reproducible by construction.

Where each suite puts its fakes:

| Boundary | PiG | crush | pi | lathe |
|---|---|---|---|---|
| HTTP transport | `provider_fixture.go` SSE server | VCR recorder as `http.Client.Transport` | injected `fetch` / `sseFetch` | `internal/fake` `httptest.Server` |
| Provider interface | `TestFauxProvider` (Go) + TS extension | `fakeLanguageModel` | `fauxProvider` | the learner's real `Provider` |
| Model behavior | prompt-classified rule table | recorded cassette response | queued `AssistantMessage` / factory | scripted `Reply` |
| Clock | real clocks + JSON timestamp aliases | fixed prompt date; cassette latency skipped | `Date.now()` in faux; fake timers where needed | hardcoded `created`; no clock seam |
| Process / terminal | real binaries in tmux/RPC/CLI | real in-process app + services | `VirtualTerminal` (xterm) / Docker eval | none; in-process |

The mature suites use *more than one* boundary and are explicit about which claim each
test buys. PiG's `covers` field, crush's VCR-vs-fake split, and pi's unit-vs-eval split
are three different answers to the same architectural question: **what is the smallest
fixture that can still express the bug?**

For lathe the answer is unusually clean: the learner implements exactly one seam
(`harness.Provider`), and the course owns everything below it (`internal/fake`). The
verifier can therefore be both high-fidelity and fully offline. The risk is the
opposite of PiG's: not too little fidelity, but too little *self-scrutiny* of the
fixture and the assertions.

---

## 2. Per-repo testing architecture

### 2.1 PiG: a differential port with a running oracle

PiG is a Go port of pi, and its self-testing problem is not "does the agent work?"
but "does the Go port behave like the pinned TypeScript original?" That changes the shape of
the suite completely: the oracle is another *program*, not a stored expectation.

**Scale at this commit.** 7,935 `_test.go` files in the repository, 601 of them under
`test/`; 821 TOML parity scenarios under `test/parity/scenarios/`; 3,398 files under
`test/parity/` overall. (The brief's "~4,000 test files" does not match any count I
could reproduce; see §9.)

**The tiers.**

| Tier | Location | Boundary | What it buys |
|---|---|---|---|
| Pure unit / table test | `ai/*_test.go`, `tui/*_test.go`, ... | none | parser, conversion, width, cost math |
| SDK vectors | `test/parity/sdk-vectors/` | upstream function | a ported pure helper returns the same value as Pi's implementation |
| Provider observation oracle | `ai/test_faux_observation_oracle_test.go`, `coding/testdata/rpc33-observation/` | provider event stream | the Go event stream matches Pi's *delivery interleaving*, not just content |
| Paired scenario | `test/parity/scenarios/**/*.toml` | real binaries | one observable behaves identically in `pig` and `pi` |
| Live scenario | `scenarios/print/01-print-mode-arithmetic.toml` | real model | the integrated product can still call a provider |

**What is faked, and where.** PiG fakes at *three* boundaries, deliberately:

1. **Provider interface.** `ai/test_faux.go` is a `Provider` implementation with
   scripted responses. It is compiled into the binary but only enabled by
   `PIG_TEST_FAUX=1`:

   ```go
   // ai/test_faux.go:64-79
   // TestFauxProvider is a deterministic, test-only provider used by the
   // integration parity harness. It is deliberately NOT part of the normal model
   // catalog and is only enabled when the binary is launched with the
   // PIG_TEST_FAUX=1 environment variable.
   //
   // Why this exists: live-model parity gates in tmux are prone to model/provider
   // nondeterminism. Upstream is the source of truth, so parity gates must use a
   // deterministic oracle when the live provider does not reliably materialize a
   // scenario. This provider gives pig a stable scripted backend while upstream
   // gets the same scripted behavior via a temp extension that registers an
   // equivalent custom provider.
   type TestFauxProvider struct {
       Scenario string

       afterPush func()

       toolCallMu       sync.Mutex
       toolCallCounters map[string]uint64
   }
   ```

   The Pi side is the same rule table in TypeScript, registered as an extension:
   `test/parity/testdata/test-faux-provider.ts:700-718`.

2. **HTTP transport.** For wire-protocol claims, `test/parity/runner/provider_fixture.go`
   starts an `httptest.Server` that speaks OpenAI Completions, Anthropic Messages,
   Google Generative AI, or Mistral Conversations:

   ```go
   // test/parity/runner/provider_fixture.go:50-64
   // providerFixture describes one hermetic provider API: where its base URL points, how to read the newest turn from a request, and how to write the two replies (a read tool call and a text echo).
   type providerFixture struct {
       baseURLPath string
       // lastTurn returns the newest conversation turn's text and whether that turn is a tool result.
       lastTurn func(body []byte) (text string, toolResult bool, err error)
       toolCall func() []sseEvent
       reply    func(text string) []sseEvent
       // errorBody is the JSON returned with HTTP 400 for the HTTP_ERROR prompt.
       errorBody string
   }

   // sseEvent is one server-sent event; an empty name writes a data-only record.
   type sseEvent struct {
       name string
       data any
   }
   ```

   The handler routes on the newest turn: `HTTP_ERROR` returns 400, `READ` returns a
   tool call, anything else is echoed as text
   (`test/parity/runner/provider_fixture.go:94-126`). The whole SSE body is delivered
   in **one** `Content-Length` write (`:180-200`) so headers and body arrive together.

3. **The process/terminal.** TUI scenarios run both real binaries under tmux or a
   headless terminal. No terminal is faked. The runner gives each binary its own copy
   of the home, agent dir, and cwd; it masks the cwd digits before comparison and
   removes ambient terminal/proxy/CI variables (`test/parity/README.md:74-95`).

**What is asserted.** The assertion vocabulary is the clearest statement of PiG's
philosophy (`test/parity/README.md:165-183`). The strongest comparators are exact:

| Assertion | Contract |
|---|---|
| `output_equal` | byte-identical output except individually declared replacements |
| `escaped_output_equal` | byte-identical tmux captures including SGR escapes |
| `json_output_equal` | complete JSONL objects, field presence/null/type/order preserved |
| `stderr_equal` | complete separately captured stderr |
| `artifact_equal` | complete file the command wrote |
| `both_contain` / `*_match_regex` | weaker, for cases equality is not a valid contract |

Request payloads are asserted in the provider-wire probes: each side captures the
**complete HTTP request body** and prints it as `wireBody`; the scenario compares the
two JSON outputs (`scenarios/providers-registry/04-provider-wire-payloads.toml:32-75`,
`test/parity/testdata/provider-wire-pig/main.go:430`). Event sequences are asserted as
JSONL records, either complete (`json_output_equal`) or by field path
(`json_aliases`). Session files are compared as JSONL output in the RPC/JSON scenarios;
`session_file` aliases validate the `timestamp_uuid.jsonl` filename and bind the UUID to
the session identity without deleting it (`test/parity/README.md:195`). Final text is
just another output assertion. For paired scenarios, there is **no per-scenario golden
transcript**: the peer binary is the golden.

**Granularity.** PiG tests pure helpers with table tests and generated vectors, protocol
shapes with provider probes, and user-visible behavior with paired binaries. The line
is: *if the behavior is a pure function or a wire shape, test it directly; if it is
observable to a user, run the real binary.* It does not try to unit-test the agent loop
in isolation from the provider stream; it compares the whole event stream instead.

---

### 2.2 crush: VCR at the transport, fakes at the interface, real everything else

Crush is a Go application with a clean dependency seam (`fantasy.LanguageModel`) and a
large integration surface (SQLite, permissions, LSP, MCP, TUI). Its tests use two
different fakes for two different questions.

**Fake 1: HTTP transport, via VCR.** Full-agent tests inject a recorder as the
`http.Client.Transport`:

```go
// internal/agent/common_test.go:42-62
type builderFunc func(t *testing.T, r *vcr.Recorder) (fantasy.LanguageModel, error)

type modelPair struct {
    name       string
    largeModel builderFunc
    smallModel builderFunc
}

func hyperBuilder(model string) builderFunc {
	return func(t *testing.T, r *vcr.Recorder) (fantasy.LanguageModel, error) {
		provider, err := openaicompat.New(
			openaicompat.WithBaseURL("https://hyper.charm.land/v1"),
			openaicompat.WithAPIKey(os.Getenv("CRUSH_HYPER_API_KEY")),
			openaicompat.WithHTTPClient(&http.Client{Transport: r}),
		)
		if err != nil {
			return nil, err
		}
		return provider.LanguageModel(t.Context(), model)
	}
}
```

The recorder is per-test and keyed by `t.Name()`:

```go
// charmbracelet/x/vcr recorder.go:38-70
func NewRecorder(t *testing.T, opts ...Option) *Recorder {
	o := options{
		mode:           recorder.ModeRecordOnce,
		keepAllHeaders: false,
	}
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			t.Fatalf("vcr: failed to apply option: %v", err)
		}
	}

	cassetteName := filepath.Join("testdata", t.Name())

	r, err := recorder.New(
		cassetteName,
		recorder.WithMode(o.mode),
		recorder.WithMatcher(customMatcher(t)),
		recorder.WithMarshalFunc(customMarshaler),
		recorder.WithSkipRequestLatency(true), // disable sleep to simulate response time, makes tests faster
		recorder.WithHook(hookRemoveHeaders(o.keepAllHeaders), recorder.AfterCaptureHook),
	)
	if err != nil {
		t.Fatalf("vcr: failed to create recorder: %v", err)
	}

	t.Cleanup(func() {
		if err := r.Stop(); err != nil {
			t.Errorf("vcr: failed to stop recorder: %v", err)
		}
	})

	return r
}
```

`ModeRecordOnce` means: replay if the cassette exists; record if it does not
(`go-vcr` `pkg/recorder/recorder.go:73-79`). Thirteen cassettes live under
`internal/agent/testdata/TestCoderAgent/deepseek-v4/`; one test-only cassette covers a
timeout. Each is a full HTTP transcript, including the SSE chunks and their original
boundaries:

```yaml
# internal/agent/testdata/TestCoderAgent/deepseek-v4/read_a_file.yaml:1-28
---
version: 2
interactions:
- id: 0
  request:
    proto: HTTP/1.1
    proto_major: 1
    proto_minor: 1
    content_length: 773
    host: ""
    body: '{"messages":[{"content":"You will generate a short title based on the first message a user begins a conversation with.\n\n<rules>\n- Keep the title in the same language that the user wrote their message in.\n- Ensure it is not more than 50 characters long.\n- The title should be a summary of the user''s message.\n- It should be one line long.\n- Do not use quotes or colons.\n- The entire text you return will be used as the title.\n- Never return anything that is more than one sentence (one line) long.\n</rules>\n\n /no_think","role":"system"},{"content":"Generate a concise title for the following content:\n\nRead the go mod\n <think>\n\n</think>","role":"user"}],"model":"deepseek-v4-flash-0731","max_tokens":40,"stream_options":{"include_usage":true},"stream":true}'
    headers:
      Accept:
      - application/json
      Content-Type:
      - application/json
      User-Agent:
      - Charm-Crush/devel (https://charm.land/crush)
    url: https://hyper.charm.land/v1/chat/completions
    method: POST
  response:
    proto: HTTP/2.0
    proto_major: 2
    proto_minor: 0
    content_length: -1
    uncompressed: true
    body: |+
      data: {"id":"chatcmpl-308fca35a16f54438f9f32ae","object":"chat.completion.chunk","created":1787170339,"model":"deepseek-v4-flash-0731","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}
```

The VCR matcher is effectively a hidden whole-request assertion. It matches method, URL,
and body, falling back to JSON `DeepEqual` so key order does not matter; on a body
mismatch it prints a `cmp.Diff` of the expected and actual request
(`/tmp/selftest/x/vcr/matcher.go:17-69`).

**Fake 2: provider interface, for narrow unit tests.** When the test is about a wrapper
rather than the SDK, crush implements `fantasy.LanguageModel` directly:

```go
// internal/agent/request_timeout_test.go:17-50
// fakeLanguageModel is a [fantasy.LanguageModel] stub that records the
// context its methods were called with and can be configured with a custom
// stream body.
type fakeLanguageModel struct {
    generateCtx context.Context
    streamCtx   context.Context
    stream      func(yield func(fantasy.StreamPart) bool)
}

func (f *fakeLanguageModel) Generate(ctx context.Context, _ fantasy.Call) (*fantasy.Response, error) {
    f.generateCtx = ctx
    return &fantasy.Response{}, nil
}

func (f *fakeLanguageModel) Stream(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
    f.streamCtx = ctx
    if f.stream == nil {
        return func(yield func(fantasy.StreamPart) bool) {
            yield(fantasy.StreamPart{})
        }, nil
    }
    return f.stream, nil
}

func (f *fakeLanguageModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
    return &fantasy.ObjectResponse{}, nil
}

func (f *fakeLanguageModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
    return nil, nil
}

func (f *fakeLanguageModel) Provider() string { return "fake" }
func (f *fakeLanguageModel) Model() string    { return "fake-model" }
```

**Real objects everywhere else.** Crush does not fake the app, the database, or the
services. `internal/app/testing.go` builds a real `App` with a live event broker and a
real permission service, minus the config/database/LSP/MCP/coordinator boot:

```go
// internal/app/testing.go:13-33
// NewForTest constructs a minimal [App] suitable for in-process tests
// that need a working event broker and permission service without
// booting a real config, database, LSP, MCP, or agent coordinator.
//
// The returned App has:
//
//   - A live `events` broker that [App.SendEvent] publishes to and
//     [App.Events] subscribes from.
//   - A real [permission.Service] whose request and notification
//     brokers are fanned into the events broker, so subscribers to
//     [App.Events] observe the same permission events the production
//     wiring would deliver to SSE clients.
//   - An [App.agentNotifications] broker.
func NewForTest(ctx context.Context) *App {
    app := &App{
        Permissions:        permission.NewPermissionService("", false, nil),
        Questions:          question.NewService(),
        globalCtx:          ctx,
        events:             pubsub.NewBroker[tea.Msg](),
        serviceEventsWG:    &sync.WaitGroup{},
        tuiWG:              &sync.WaitGroup{},
        agentNotifications: pubsub.NewBroker[notify.Notification](),
        runCompletions:     pubsub.NewBroker[notify.RunComplete](),
    }
```

`internal/agent/agenttest.NewCoordinator` similarly wires a **real** production
`agent.Coordinator` through the real constructor, with a provider that never gets
called on the cancel path (`internal/agent/agenttest/coordinator.go:1-60`).

**What is asserted.** The full-agent tests assert on the resulting session and
workspace, not on a stored transcript. `TestCoderAgent` has 30 subtests; the "read a
file" case walks messages, finds the `view` tool call, finds the matching tool result,
and checks its content; the "update a file" case checks the file on disk:

```go
// internal/agent/agent_test.go:151-186
require.True(t, foundRead, "Expected to find a read operation")
require.True(t, foundWrite, "Expected to find a write operation")

mainGoPath := filepath.Join(env.workingDir, "main.go")
content, err := os.ReadFile(mainGoPath)
require.NoError(t, err)
require.Contains(t, strings.ToLower(string(content)), "hello from crush")
```

UI layout tests use checked-in golden files via `charmbracelet/x/exp/golden`
(`internal/ui/diffview/udiff_test.go:9-36`). Pure conversions get ordinary table tests
(`internal/agent/convert_test.go:9-30`). Session state is asserted through the real
SQLite message/session services, not through a session file. The full-agent tests do not
assert the exact final assistant text; the model's wording is not the contract there.

**Granularity.** Smallest: a pure conversion function. Largest: a full agent run
against a 425-line cassette. The line is: *if the behavior depends on the model's
decisions, use a cassette; if it is a transform, unit-test it; if it is layout, golden
it.* The cassette is both the fixture and the request-shape assertion.

---

### 2.3 pi: two suites, deliberately separated

Pi's testing architecture is the most explicitly tiered. It has a fast deterministic
unit suite and a slow non-deterministic eval suite, and they do not share a config, a
runner, or a claim.

**Unit suite: fake at the provider interface.** `packages/ai/src/providers/faux.ts`
registers a real `Provider` whose `stream` function plays queued responses. The core
types:

```ts
// packages/ai/src/providers/faux.ts:104-117
export interface FauxProviderState {
    callCount: number;
    deferredFetchCount: number;
    cancelledDeferred: DeferredHandle[];
}

export type FauxResponseFactory = (
    context: TranscriptContext,
    options: SimpleStreamOptions | undefined,
    state: FauxProviderState,
    model: Model<string>,
) => AssistantMessage | Promise<AssistantMessage>;

export type FauxResponseStep = AssistantMessage | FauxResponseFactory;
```

It emits `start`, `text_start`, `text_delta`, `text_end`, `toolcall_*`, and `done`
events with a simulated usage estimate (`faux.ts:527-586`). It is a provider, not an
HTTP server: tests using it skip the wire format entirely.

**Unit suite: fake at `fetch`.** When the test *is* about the wire format, pi injects a
`fetch` whose response streams SSE from a generator:

```ts
// packages/ai/test/streaming-tool-arguments.test.ts:24-51
/** A fetch whose response streams `events` as server-sent events, produced lazily. */
function sseFetch(events: () => Iterable<string>): typeof fetch {
    return async () => {
        const iterator = events()[Symbol.iterator]();
        const encoder = new TextEncoder();
        const body = new ReadableStream<Uint8Array>({
            pull(controller) {
                let chunk = "";
                for (let count = 0; count < 16; count++) {
                    const next = iterator.next();
                    if (next.done) {
                        if (chunk) controller.enqueue(encoder.encode(chunk));
                        controller.close();
                        return;
                    }
                    chunk += next.value;
                }
                controller.enqueue(encoder.encode(chunk));
            },
        });
        return new Response(body, { status: 200, headers: { "content-type": "text/event-stream" } });
    };
}
```

This is a true chunk-boundary test: the `pull` loop coalesces 16 events per network
chunk, so the parser sees arbitrary boundaries. The test then feeds 16-, 24-, and
32-character tool-argument deltas through the real provider modules
(`streaming-tool-arguments.test.ts:1-22, 330-400`).

Request payloads are asserted *without any HTTP at all*: a test passes an `onPayload`
hook that captures the payload and throws a sentinel before the request is sent:

```ts
// packages/ai/test/sampling-options.test.ts:52-60
function capturingOptions(onCapture: (payload: SamplingPayload) => void) {
    return {
        apiKey: "fake-key",
        onPayload: (payload: unknown) => {
            onCapture(payload as SamplingPayload);
            throw new PayloadCaptured();
        },
    };
}
```

That is the cheapest possible request-shape test: no server, no socket, no response.

**Unit suite: fake at the terminal.** `packages/tui/test/virtual-terminal.ts`
implements the `Terminal` interface with `@xterm/headless`; TUI components render into a
real terminal emulator and the test reads the viewport. No PTY, no tmux, no screen
scraping (`virtual-terminal.ts:1-30`).

**The coding-agent suite harness** uses the real `AgentSession` and the faux provider:

```ts
// packages/coding-agent/test/suite/README.md:3-10
Use `test/suite/` for the new harness-based test suite around `AgentSession` and `AgentSessionRuntime`.

Rules:
- Use `test/suite/harness.ts`
- Use the faux provider from `packages/ai/src/providers/faux.ts`
- Do not use real provider APIs, real API keys, network calls, or paid tokens
- Keep these tests CI-safe and deterministic
```

`createHarness` builds a real `AgentSession`, in-memory `SessionManager`, in-memory
`AuthStorage`, and a registered faux provider; it exposes `setResponses`,
`appendResponses`, and the recorded `events`
(`packages/coding-agent/test/suite/harness.ts:145-220`).

**Eval suite: real model, deliberately not a test.** `packages/evals` is a separate
package with a separate config. The documentation evals run a real model in a Docker
container, once with and once without documentation, and compute pass-rate lift with a
structured judge; the smaller host evals run locally with ordinary Vitest assertions.
The runner's rules:

```md
<!-- packages/evals/README.md:7-18 -->
Eval definitions are flat under `evals/`:

- `*.docs.eval.ts` is a documentation-lift eval. `eval:docs` runs each case in isolated `without_docs` and `with_docs` containers and reports lift.
- Other `*.eval.ts` files are host evals. `eval:host` runs them with Vitest on this machine. They are ordinary vitest-evals suites, not paired comparisons.
```

```ts
// packages/evals/evals/smoke.eval.ts:1-17
import { expect } from "vitest";
import { describeEval } from "vitest-evals";
import { createPiCodingAgentHarness } from "../src/harness.ts";

const harness = createPiCodingAgentHarness({ noTools: "all" });

describeEval("Answer a basic prompt", { harness }, (it) => {
    it("returns the expected answer", async ({ run }) => {
        const result = await run("What's the capital of France? Respond with only the city name.");
        expect(result.output.trim()).toBe("Paris");
        expect(result.errors).toEqual([]);
        expect(result.usage).toMatchObject({
            provider: process.env.PI_PROVIDER,
            model: process.env.PI_MODEL,
        });
        expect(result.usage.totalTokens).toBeGreaterThan(0);
    });
});
```

Unit tests use an in-memory `SessionManager`; the eval runner writes the native session
JSONL for each arm into `<variant>/sessions/*/session.jsonl` as an artifact
(`packages/evals/README.md:94-101`). The unit and eval tiers therefore answer different
questions about persistence: unit tests assert the in-memory transcript, evals preserve
the real session file for inspection.

The eval config is serial (`maxWorkers: 1`, `fileParallelism: false`), has 300-second
timeouts, and is a separate Vitest project from the unit tests
(`packages/evals/vitest.evals.config.ts:4-34`). Evals are **not** part of `npm test`
and are not referenced by `.github/workflows/ci.yml`; they need `PI_PROVIDER` and
`PI_MODEL`, i.e. credentials. The eval `test` script runs only the eval package's own
unit tests, not the eval definitions (`packages/evals/package.json`).

Unlike PiG and crush, pi's unit suite does not keep a golden-file corpus: assertions
are code, `toMatchSnapshot` appears only a handful of times, and there are no
`__snapshots__` directories. The "expected value" is usually an inline literal or an
event-order assertion.

**Granularity.** Smallest: a pure helper. Middle: provider SSE parser, agent loop,
session, TUI render. Largest: a real model in a container. The line is explicit: *if it
must be deterministic, it goes in `vitest` with faux; if it measures a capability, it
goes in `vitest-evals` with repetitions, cost accounting, and a judge.* There is no
attempt to make the eval deterministic; instead the report exposes flakiness
(`packages/evals/src/report.ts:336-357`).

### 2.4 Where the line is, and what it costs

The three suites draw the same line at different heights:

| Claim being tested | PiG | crush | pi |
|---|---|---|---|
| A pure function returns the right value | Go table test, SDK vector | Go table test | Vitest unit test |
| A wire format is parsed/serialized correctly | provider-wire probe, SSE fixture | VCR cassette (whole request/response) | injected `fetch` + SSE generator |
| The loop/session/tool orchestration is correct | faux provider + real binary | faux `LanguageModel` or full agent + cassette | `AgentSession` + faux provider |
| The user-visible product works | real `pig` vs real `pi` in tmux/RPC/CLI | full agent + real services (SQLite, permissions) | virtual terminal or Docker eval |
| The model is good at a task | one live scenario | not tested | eval with real model, judge, repetitions |

The line is consistent across all three: **the more of the real system a test keeps,
the fewer claims it can make cheaply; the more it fakes, the more precisely it can
attribute a failure.** PiG keeps the most (two real binaries) and pays in time,
platform coupling, and fixture duplication. Pi keeps the least for its unit suite
(faux provider) and moves the real model to a separate, statistical tier. Crush sits
in between: real app/services, recorded transport.

Cost follows the boundary:

- **PiG** has 821 scenarios; 811 carry the `hermetic` tag (816 files mention it), and `make parity` runs only the tagged ones,
  and `make parity-fast` cuts each to one pair. `parity-durable` is the 3-pair gate;
  `parity-live` is a release check. No model call is made in the normal gate.
- **crush** has 14 cassettes and no API call on replay; `SkipRequestLatency(true)`
  removes the recorded delays. The model is replaced at the `http.Client.Transport`
  boundary, so the SDK, retry, and streaming code still run.
- **pi** has 706 unit test files on the faux provider and 7 eval definitions. Evals are
  opt-in, serial, 300-second-budgeted, and absent from the public CI workflows. The
  unit suite is the free, fast gate; the eval suite is the paid, statistical one.

For lathe, the cost model is simpler than all three: one in-process fake, no
network, no model, no container. The suite should stay under a second per chapter.
If it does not, the fixture is doing too much work or a case is waiting on wall time.

---

## 3. PiG parity deep dive

This is the part most worth studying, and the part most worth *not* copying wholesale.

### 3.1 What `test/parity/` actually contains

| Path | Files | Purpose |
|---|---:|---|
| `scenarios/` | 821 TOML + fixtures | one file per observable; 28 family directories, plus `testdata/` and one legacy top-level TOML |
| `runner/` | 89 files | scenario loader, drivers, comparators, isolation, scheduling |
| `testdata/` | 555 files | paired probe programs (`*-pi.mjs`, `*-pig/`), faux provider, fixtures |
| `unit-evidence/` | 128 files | reviewed Go unit assertions with a compiling mutation proof |
| `interfaces/` | 91 | generated upstream interface/test inventories and reviewed mappings |
| `interface-closure/` | 320 | evidence graph for missing interface cells |
| `closure/` | 70 | deterministic closure schema/store/derivation |
| `correspondence/` | 20 | source-to-target correspondence compiler |
| `pin-ledgers/` | 73 | what was verified at each upstream pin |
| `coverage.md` | 1 | generated report (805 KB at this commit) |

This is not a test suite; it is a **verification compiler** for a port. The scenarios are
the executable claims, the inventories are the denominator, and the ledgers are the
review trail.

### 3.2 Anatomy of a paired scenario

A scenario is not a recorded request/response pair. It is one *claim* plus the driver
configuration needed to observe it.

- **Defined** by the TOML: `name`, `description`, `covers`, `tags`, `driver`, fixture
  selection, `[assert]`, and an optional `[diverge]`.
- **Recorded** in the comments: the upstream version, probe date, command, raw
  observation, and the source lines that justify the claim.
- **Replayed** by running `pig` and `pi` under the same fixture and comparing the two
  observations. The oracle is the pinned binary, not a stored transcript.

From the README:

```toml
# test/parity/README.md:102-121
name = "startup-banner"
description = "PiG and Pi reach the interactive prompt"
covers = [
  "packages/coding-agent/src/main.ts",
]
tags = ["fast", "hermetic", "tui"]
driver = "interactive-tmux"

[tmux]
width = 100
height = 35
ready_pattern_pig = "(auto)"
ready_pattern_pi = "pi v"
ready_timeout_seconds = 30

[assert]
both_reach_ready = true
runs = 3
```

A richer one shows the shape of a behavioral claim:

```toml
# test/parity/scenarios/json/05-json-failing-bash-result-iserror.toml:1-7
# Behavior family: json
# Surface: a bash call that exits non-zero RETURNS an error result, so `isError` stays inside tool_execution_end.result.
#
# Upstream evidence (Pi 0.99.1, dist/cli.js, probed 2026-09-30; test-faux twin, prompt "Run: bash failing exit", command `echo out; exit 3`):
#   {"type":"tool_execution_end","toolCallId":...,"toolName":"bash","result":{"content":[{"type":"text","text":"out\n\n\nCommand exited with code 3"}],"structuredContent":{"output":"out\n","truncated":false,"exit_code":3,"wall_time_seconds":0},"isError":true},"isError":true}
# bash.ts:403-409 returns {content, details, structuredContent, isError: true}; agent-loop.ts:912-919 emits the finalized result as `result` and the call's flag as `isError`.
# A thrown error would give createErrorToolResult (agent-loop.ts:906-910), which has no isError. Pig omitted result.isError for every failing bash call.
```

```toml
# test/parity/scenarios/json/05-json-failing-bash-result-iserror.toml:31-43
[assert]
exit_code = 0
runs = 3
output_equal = true
stderr_equal = true
both_contain = [
  '"type":"tool_execution_end"',
  '"text":"failed-bash-done"',
]
both_match_regex = [
  # bash.ts:392 rounds the measured wall clock to 0.1 s: the field's number shape is the contract, not its value.
  '"structuredContent":\{"output":"out\\n","truncated":false,"exit_code":3,"wall_time_seconds":[0-9]+(\.[0-9])?\},"isError":true\},"isError":true,"durationMs":[0-9]+\}',
]
```

The comment records the upstream probe: version, date, command, and the exact JSON
observed. This is the *evidence* for the claim, not the expected output. The expected
output is recomputed from the pinned Pi binary each run.

### 3.3 How the pair is executed

`TestParity` discovers every TOML and turns each into a subtest
(`test/parity/runner/runner_test.go:44-150`). For each scenario, `RunScenario` runs the
two binaries and evaluates one assertion set against both:

```go
// test/parity/runner/runner.go:117-134
		// Assertions are checked after every paired run. Durability runs continue
		// only while every completed pair is green; retries and remaining declared
		// runs cannot turn an earlier failure into a pass. Runtime-ratio checks need
		// the complete sample and are evaluated only below.
		probe := &ScenarioOutcome{
			Scenario:     sc,
			Pig:          o.Pig,
			Pi:           o.Pi,
			SkipPerfGate: true,
		}
		EvaluateOutcome(probe)
		if len(probe.Failures) > 0 {
			break
		}
	}
	EvaluateOutcome(o)
	return o
}
```

There are six drivers (`interactive-tmux`, `headless-terminal`, `print-mode`,
`cli-mode`, `rpc-mode`, `extension-host`), each isolating the binary in its own cwd,
home, agent dir, and temp root (`test/parity/README.md:123-149`). The fixture can be:

- no fixture (real provider or no provider),
- `openai_fixture = true` (local HTTP/SSE, OpenAI shape),
- `provider_fixture = "<api>"` (same for Anthropic/Google/Mistral),
- `PIG_TEST_FAUX=1` + a Pi extension (in-process provider script),
- a checked-in changelog fixture.

The faux provider is duplicated in Go (`ai/test_faux.go`) and TypeScript
(`test/parity/testdata/test-faux-provider.ts`), so the fixture itself becomes a test
target. PiG polices the duplication from the Go side: `TestTestFauxToolCallIDs` runs the
TS fixture's Node test (`ai/test_faux_ids_test.go:12-14`), `TestTestFauxLimitsMatchParityOracle`
reads the TS source for the model limits (`ai/test_faux_limits_test.go:11-15`), and the
observation oracle compares the Go event stream to a recording of the TS fixture
(`ai/test_faux_observation_oracle_test.go:113-160`). This is the honest way to maintain a
two-language fixture, and the clearest example of PiG's maintenance tax.

### 3.4 How equality is proven

It is **differential testing**, not a proof of semantic equivalence. The strongest
comparators are exact, and normalization is a reviewed, enumerated exception:

```go
// test/parity/runner/asserts.go:146-176
	compareOutputs := func(label string, transform func(string) string) {
		for run := range min(len(o.Pig.Runs), len(o.Pi.Runs)) {
			g := transform(o.Pig.Runs[run].Output)
			p := transform(o.Pi.Runs[run].Output)
			for _, rule := range a.NormalizeReplace {
				if strings.TrimSpace(rule.Reason) == "" {
					continue
				}
				re, err := regexp.Compile(rule.Pattern)
				if err != nil {
					o.Failures = append(o.Failures,
						fmt.Sprintf("invalid normalize_replace pattern %q: %v",
							rule.Pattern, err))
					continue
				}
				g = re.ReplaceAllString(g, rule.With)
				p = re.ReplaceAllString(p, rule.With)
			}
			if g != p {
				o.Failures = append(o.Failures,
					fmt.Sprintf("run %d %s mismatch: pig=%q pi=%q", run+1, label, trunc(g, 80), trunc(p, 80)))
			}
		}
	}
	if a.JSONOutputEqual {
		for run := range min(len(o.Pig.Runs), len(o.Pi.Runs)) {
			if err := compareJSONResults(o.Pig.Runs[run], o.Pi.Runs[run], a.JSONAliases); err != nil {
				o.Failures = append(o.Failures, fmt.Sprintf("run %d JSON output: %v", run+1, err))
			}
		}
	}
```

```go
// test/parity/runner/asserts.go:187-189
	if a.OutputEqual {
		compareOutputs("output", func(s string) string { return s })
	}
```

`json_output_equal` is stricter about structure: it parses complete JSONL records,
preserves field presence, nulls, scalar types, array/record order, string whitespace,
and exact numeric values, and rejects malformed records and duplicate keys
(`test/parity/README.md:173`). The comparator walks the two values and reports the first
path that differs:

```go
// test/parity/runner/json_compare.go:120-156
		if x, ok := a.(string); ok {
			if y, ok := b.(string); ok && slices.Equal(aliasText(x, gRoots, gLiterals), aliasText(y, pRoots, pLiterals)) {
				return nil
			}
		}
		switch x := a.(type) {
		case map[string]any:
			y, ok := b.(map[string]any)
			if !ok {
				break
			}
			keys := make([]string, 0, len(x)+len(y))
			for key := range x {
				keys = append(keys, key)
			}
			for key := range y {
				if _, ok := x[key]; !ok {
					keys = append(keys, key)
				}
			}
			slices.Sort(keys)
			for _, key := range keys {
				av, aok := x[key]
				bv, bok := y[key]
				next := path + "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(key)
				if aok != bok {
					if optionalDurationPresence(rules, next, av, bv, aok) {
						continue
					}
					return fmt.Errorf("%s: field presence differs (pig=%t pi=%t)", next, aok, bok)
				}
				if err := compare(next, av, bv); err != nil {
					return err
				}
			}
			return nil
		case []any:
			y, ok := b.([]any)
			if !ok {
				break
			}
			for i := range min(len(x), len(y)) {
				if err := compare(fmt.Sprintf("%s/%d", path, i), x[i], y[i]); err != nil {
					return err
				}
			}
```

The `json_aliases` DSL is how PiG keeps exact comparison while allowing fields that
genuinely vary. Every rule names `paths`, a `kind`, and a `reason`; the comparator
validates that the reason is non-empty and the kind is known before comparing
(`json_compare.go:53-58`). Kinds are `id`, `timestamp`, `duration`, `duration_ms`,
`path`, `literal`, and `session_file`. For `id`, it maintains a bijection so identity
reuse and parent/leaf references stay consistent:

```go
// test/parity/runner/json_compare.go:256-270
func compareIdentity(path, pig, pi, group string, pairs map[string]*identityPair) error {
    pair := pairs[group]
    if pair == nil {
        pair = &identityPair{map[string]string{}, map[string]string{}}
        pairs[group] = pair
    }
    if old, ok := pair.forward[pig]; ok && old != pi {
        return fmt.Errorf("%s: identity reference changed: pig=%q pi=%q (previous pi=%q)", path, pig, pi, old)
    }
    if old, ok := pair.reverse[pi]; ok && old != pig {
        return fmt.Errorf("%s: identity reuse: pig=%q pi=%q (previous pig=%q)", path, pi, pig, old)
    }
    pair.forward[pig], pair.reverse[pi] = pi, pig
    return nil
}
```

The comparator ladder is explicit in the README: use `escaped_output_equal` for exact
TUI, `output_equal` for exact process/protocol output, `output_layout_equal` when
placement is the contract, `output_normalized_equal` only for identified unstable
fields, and substring assertions only when equality is not a valid contract
(`test/parity/README.md:185-193`). A normalization rule without a reason is a load
error; the scenario linter also rejects weak comparators without a rationale.

### 3.5 What `coverage.md` is generated from

`coverage.md` is **not** a test report. It is an accounting report built from five
inputs:

1. `docs/parity/PORT_MAP.md` — 710 upstream paths, each with a port status.
2. Every scenario's `covers = [...]` array — 821 scenarios, split into behavioral vs
   boot-only/registration-only/smoke-only/deferred.
3. The last run's JSON results (`-pig-parity.results`) — per-scenario pass/fail and
   runtimes. Optional; without it, `last run` says `not run`.
4. `test/parity/unit-evidence/*.json` — reviewed Go tests that name an exact upstream
   path, package, test names, upstream source/test reference, and a written **compiling
   mutation** proof that the test failed. The generator rejects unknown paths, missing
   tests, and incomplete records (`test/parity/cmd/coverage/unit_evidence.go:17-87`).
5. Upstream test porting stats and package/interface accounting from
   `test/parity/interfaces/` and `.upstream/current`.

The report's headline is deliberately conservative:

```md
<!-- test/parity/coverage.md:1-9 -->
# PORT_MAP coverage report

Generated from `710` PORT_MAP entries and `821` parity scenarios.

## Status

- **Porting:** 633 / 635 intended-portable entries ✅ (99.7%).
- **Behavior verification:** 569 / 633 ported entries (89.9%); 0 weak-only, 64 untested.
```

The generator distinguishes behavioral verification from boot-only, registration-only,
smoke-only, and deferred evidence; weak evidence stays visible but does not count
(`test/parity/README.md:199-212`). `coverage-strict` fails if any intended-portable row
is incomplete or has no behavioral coverage. The report is regenerated after a
successful parity run; `coverage-drift` fails if the committed file differs.

### 3.6 How divergences are kept from silently drifting

A divergence is a first-class record, not a comment. The machinery has four independent
locks:

1. **One file per record, generated ledger.** `docs/parity/divergences/D<N>.md` is the
   source; `docs/parity/DIVERGENCES.md` is generated by `make divergences`. Every
   active record must contain `SCRUTINIZED:approved`, a `Remove when:` condition, and
   durable evidence; the quality checker also rejects stale `pi vX.Y.Z` references,
   "caller-free" code, and paths that do not exist
   (`automation/ci/check-divergence-quality.py:48-72`).
2. **Call-site markers.** Every active `D<N>` must have a matching
   `// pig divergence (D<N>): ...` in production source, and every marker must have a
   record. Untyped `pig divergence` comments fail
   (`automation/ci/check-divergence-consistency.sh`).
3. **Scenario lint.** Any `D<N>` mentioned in a scenario must exist in
   `DIVERGENCES.md` or `additive-features.md`; this prevents fake-citing a divergence to
   justify a skip (`test/parity/cmd/lint/main.go:41-44, 483-490`). A `[diverge]` block
   can exempt exact `pig_contains`/`pi_contains` strings from the symmetric comparison
   (`test/parity/runner/schema.go:289-294`); the exemption must resolve to a numbered
   divergence record. I did not find a loader check that enforces the `diverge.reason`
   field itself.
4. **Coverage and known gaps.** `coverage.md` marks weak-only and untested rows;
   `known-gaps.toml` can only shrink: a listed gap that is no longer observed fails, and
   an unlisted gap fails (`test/parity/known-gaps.toml:6-20`).

There is also a content-hash lock for behavior contracts:
`behavior-contracts.toml` pins an upstream source slice by `sha256` and
`behaviorcheck` fails on drift (`test/parity/cmd/behaviorcheck/main.go:296-312`). This
is how a test that claims to mirror a specific upstream branch stays honest when the
upstream file changes.

### 3.7 What parity does not prove

PiG is unusually honest about this:

> Pinned Pi is the behavior oracle. A scenario proves only its asserted path.
> Generated inventories define review obligations, not acceptance.
> — `test/parity/AGENTS.md:13`

The suite is a finite set of observations. It cannot prove that Go and TypeScript are
semantically equivalent; it can only prove that on the scenarios written, with the
normalizations declared, the observed outputs match, and that every intended-portable
path has at least one reviewed behavioral claim. That is the right claim for a port.
For a course, it is more proof than you need and much more maintenance than you want.

---

## 4. Determinism techniques

The central design decision is **which non-determinism is part of the contract**. PiG
freezes almost everything and normalizes the rest; pi deliberately leaves some
non-determinism in the faux provider because its assertions do not depend on it;
lathe must freeze chunk boundaries because they *are* the lesson.

| Source of variation | PiG | crush | pi | Lesson for lathe |
|---|---|---|---|---|
| **SSE chunk boundaries** | Provider fixture writes the whole body in one `Content-Length` write (`provider_fixture.go:122-125`); faux provider emits one delta per text block | Cassette replays the exact recorded chunks, boundaries included | `sseFetch` coalesces 16 events per pull; faux splits randomly (`faux.ts:289-299`) | Freeze them explicitly in the fixture. Ch01 is *about* reassembly, so a single fixed four-fragment split is not enough; the fixture must name each frame. |
| **Timing** | No sleeps in assertions; TUI waits for barriers (`wait_visible_contains`, `wait_stable_milliseconds`); perf gates separate (`README.md:139`) | `WithSkipRequestLatency(true)` removes recorded response delays | `vi.useFakeTimers` in 27 test files; evals measure real latency | No wall-clock assertions. Use a logical clock and a per-case deadline; never `time.Sleep` as a synchronization primitive. |
| **Random IDs** | Per-session counters for tool calls (`ai/test_faux.go:300-340`); JSON `id` aliases with a bijection (`json_compare.go:256-270`) | Real IDs from the model are in the cassette; tests do not assert them | `fauxToolCall` generates an ID unless the test supplies one (`faux.ts:63-70, 163-165`); deterministic assertions pass explicit IDs | Inject an ID source (`call_1`, `call_2`, ...); if an ID must vary, alias it, don't delete it. |
| **Timestamps** | Real clocks; JSON `timestamp` alias preserves presence/type/syntax (`json_compare.go:84-90, 195-205`) | Prompt date fixed to `1/1/2025` (`common_test.go:125-131`); response timestamps not asserted | `Date.now()` in faux messages; eval timings measured | Fixed clock in the fixture; timestamp normalization with a reason only when the chapter is about time. |
| **Token counts** | Fixed usage in provider fixtures (`10/3/13`); faux sets `130000` for the over-window trigger (`test_faux.go:227-229`) | Recorded live usage; not asserted | Faux estimates usage from text length (`faux.ts:246-287`) | Fixed usage per fixture; test explicit and absent usage separately. |
| **Temp paths** | `{{TEMP}}` token; cwd copies named `parity-snap-cwd-<10 digits>` so both paths are equal length; digits masked (`README.md:74-92`) | `t.TempDir` for DB; fixed `/tmp/crush-test/<test>` for cwd | `mkdtemp` isolated home/workspace; eval artifacts record the root | Use `t.TempDir`, then alias the root to `<WORK>` before comparison. |
| **Scheduling / interleaving** | Recorded observation oracle replays Pi's push counts at each delivery tick (`ai/test_faux_observation_oracle_test.go:113-160`) | Not modeled; VCR replays complete responses | Faux uses `queueMicrotask`; tests await events | If the chapter is about streaming, record the *event delivery order*, not just the final text. |
| **Environment** | Scrubs proxies, terminal, WSL/SSH, CI, multiplexer vars; per-binary homes (`README.md:90`) | Fixed config, clears skills/context/LSP for cassette stability (`common_test.go:143-156`) | `test.sh` runs with an empty env, isolated HOME/TMPDIR, `TZ=UTC`, no API keys (`test.sh:39-84`) | Scrub env; pin `TZ`; set `HOME`/temp; never read ambient config. |
| **Fixture provenance** | Hand-written faux table, generated vectors from upstream code, recorded observation oracles from pinned Pi, one live scenario | Cassettes replay recorded Hyper/DeepSeek responses (provenance undocumented) | Faux responses written in tests; evals use live model | Record from the **reference solution**, never from the learner; hand-write the protocol fixture. |

Two details are worth stealing immediately.

**One-write SSE is a determinism technique, not a fidelity technique.** PiG writes the
whole provider response in one syscall so chunk boundaries cannot vary between runs.
That is exactly wrong for lathe chapter 01, where reassembly is the skill being
graded. The correct adaptation is not "copy PiG's one write"; it is "make the chunk
boundaries *data* in the fixture, and assert that any valid boundary sequence produces
the same result."

**The recorded observation oracle is the strongest idea in PiG.** For event streams,
final content is not enough: Go's goroutine scheduling can deliver the same events in a
different order, and consumers observe partial messages. PiG records the upstream event
stream *with the push count at each delivery tick* and compares the Go stream to it,
printing a unified diff on mismatch
(`ai/test_faux_observation_oracle_test.go:146-160`). This is how you test an event
model without pretending scheduling is deterministic.

---

## 5. Failure ergonomics

The difference between a useful suite and a frustrating one is entirely in the failure
message. All three repos invest here; lathe already has the best *pedagogical*
format and the weakest *diagnostic* machinery.

### 5.1 PiG: first difference, artifacts, and a reminder not to cheat

PiG's line diff is index-aligned, not LCS-minimal, and deliberately shows the first
divergence with context rather than a minimal patch:

```go
// test/parity/runner/failure_hints.go:37-56
// LineDiff produces a readable line-by-line diff of two strings.
// It is not minimal in the LCS sense; it lines up by index and shows
// the first chunk where outputs diverge. For agents reading test logs
// this is much more useful than the 80-char truncated form.
//
// Format (when the strings differ):
//
//  first diff at line N
//  ─── pig ───
//    N-1: <context line>
//  ! N:   <pig line>
//    N+1: <context line>
//  ─── pi ───
//    N-1: <context line>
//  ! N:   <pi line>
//    N+1: <context line>
//  (M more differing lines)
func LineDiff(pig, pi string) string {
```

JSON failures report the JSON pointer path and both values:
`return fmt.Errorf("%s: pig=%s pi=%s", path, jsonDiagnostic(a), jsonDiagnostic(b))`
(`json_compare.go:185`). That is often enough to find the field, but not to see the
surrounding record; the failure artifacts fill that gap.

On failure, the runner writes a replayable directory and prints its path
(`test/parity/runner/runner_test.go:204-233`). The directory contains the scenario
TOML, `context.json` (binary paths, freshness, failures, medians), `pig.stdout`,
`pi.stdout`, `pig.stderr`, `pi.stderr`, `pig.escaped`, `pi.escaped`, `diff.txt`, and
`rerun.sh`:

```go
// test/parity/runner/artifacts.go:94-101
if pigRun >= 0 && piRun >= 0 {
    _ = write("diff.txt", LineDiff(o.Pig.Runs[pigRun].Output, o.Pi.Runs[piRun].Output))
}
rerun := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\ncd %q\nPIG_PARITY_PIG_BIN=%q go test -tags=parity ./test/parity/runner -run 'TestParity/%s$' -count=1 -v -timeout 3m\n",
    mustRepoRootForArtifact(o.Scenario.SourcePath), pig.Path, shellSingleQuoteSafeRegex(o.Scenario.Name))
```

The most distinctive failure aid is not technical. Every failed scenario prints a
"faithfulness reminder" aimed at the coding agent reading the log:

```go
// test/parity/runner/failure_hints.go:17-35
const FaithfulnessReminder = `
─── faithfulness reminder ───────────────────────────────────────────
Scenarios serve faithfulness. Faithfulness does not serve scenarios.

If a pig observable diverges from real upstream behavior, the pig
code is wrong: even if some existing scenario currently asserts the
divergent output.

Do NOT:
  • weaken a comparator (output_equal → output_normalized_equal → substring)
  • add a # lint-known-gap: for a difference you have diagnosed as a pig bug
  • narrow a crop window to dodge the diff
  • revert a faithfulness fix to make a scenario pass

DO:
  • re-probe upstream pi for the asserted surface in this loop
  • update the scenario's evidence comment, crop, and comparator
  • if the fix's blast radius exceeds the loop, stop and tell the user
─────────────────────────────────────────────────────────────────────`
```

This exists because the failure mode is not "the developer cannot read a diff"; it is
"the developer rationalizes the diff away." Agentlathe's analogue is a one-line
reminder that the chapter README is the spec, and that weakening a case is not a fix.

PiG also lists sibling scenarios that share `covers` paths, so a fix can be checked
against every other claim about the same upstream file
(`failure_hints.go:135-160`). That is a cheap and powerful idea: when a case about
`Chat` fails, show the other cases about `Chat`.

### 5.2 crush: a diff at the boundary

Crush relies on Go's normal test tooling: `testify` assertions show expected/actual,
and `golden.RequireEqual` compares layout against checked-in golden files. The VCR matcher adds a targeted
diff when a request does not match a cassette:

```go
// charmbracelet/x/vcr matcher.go:36-55
		requestContent := normalizeLineEndings(reqBody)
		cassetteContent := normalizeLineEndings(i.Body)
		if requestContent == cassetteContent {
			return true
		}
		var content1, content2 any
		if err := json.Unmarshal([]byte(requestContent), &content1); err != nil {
			printDiff(t, requestContent, cassetteContent)
			return false
		}
		if err := json.Unmarshal([]byte(cassetteContent), &content2); err != nil {
			printDiff(t, requestContent, cassetteContent)
			return false
		}
		if isEqual := reflect.DeepEqual(content1, content2); !isEqual {
			printDiff(t, requestContent, cassetteContent)
			return false
		}
		return true
```

```go
// charmbracelet/x/vcr matcher.go:68-69
func printDiff(t *testing.T, requestContent, cassetteContent string) {
    t.Logf("Request interaction not found for %q.\nDiff:\n%s", t.Name(), cmp.Diff(cassetteContent, requestContent))
}
```

The failure mode is good when it fires, but ambiguous: "interaction not found" can mean
the harness changed a prompt, the provider changed a model ID, or the cassette is stale.
Crush does not have a per-field request assertion; the cassette match is all-or-nothing.
That is a tradeoff worth noting: VCR gives you a whole-request golden for free, but its
diagnostic is a diff of a 37 KB JSON body.

### 5.3 pi: vitest diffs and an eval report that refuses to overclaim

Unit failures are ordinary Vitest diffs, which are good for structured values and
event arrays. The eval report is more interesting: it refuses to publish a headline
when pairs are blocked, and it flags saturation and flakiness rather than hiding them:

```ts
// packages/evals/src/report.ts:336-357
function comparisonFlags(pairs: readonly Pair[], controlPassRate: number | null, treatmentPassRate: number | null) {
	const flags: Array<"no-lift" | "negative-delta" | "control-saturated" | "treatment-saturated" | "flaky"> = [];
	if (controlPassRate !== null && treatmentPassRate !== null) {
		if (controlPassRate === treatmentPassRate) flags.push("no-lift");
		if (treatmentPassRate < controlPassRate) flags.push("negative-delta");
		if (controlPassRate === 1) flags.push("control-saturated");
		if (treatmentPassRate === 1) flags.push("treatment-saturated");
	}
	const outcomes = new Map<string, Set<string>>();
	for (const pair of pairs) {
		const key = JSON.stringify([pair.control.caseId, pair.control.variant]);
		const control = outcomes.get(key) ?? new Set<string>();
		control.add(String(pair.control.score >= 1));
		outcomes.set(key, control);
		const treatmentKey = JSON.stringify([pair.treatment.caseId, pair.treatment.variant]);
		const treatment = outcomes.get(treatmentKey) ?? new Set<string>();
		treatment.add(String(pair.treatment.score >= 1));
		outcomes.set(treatmentKey, treatment);
	}
	if ([...outcomes.values()].some((values) => values.size > 1)) flags.push("flaky");
	return flags;
}
```

```ts
// packages/evals/src/report.ts:378-396
			const blockedPairCount = totalPairs - pairs.length;
			const publishHeadline = blockedPairCount === 0 && pairs.length > 0;
			const controlPassRate = publishHeadline
				? pairs.filter(({ control }) => control.score >= 1).length / pairs.length
				: null;
			const treatmentPassRate = publishHeadline
				? pairs.filter(({ treatment }) => treatment.score >= 1).length / pairs.length
				: null;
			return {
				evalSet,
				totalPairs,
				eligiblePairs: pairs.length,
				blockedPairs: blockedPairCount,
				controlPassRate,
				treatmentPassRate,
				lift:
					controlPassRate === null || treatmentPassRate === null
						? null
						: difference(treatmentPassRate, controlPassRate),
```

The formatted report is a small table with pairs, pass rate, flags, token delta, tool
delta, latency delta, estimated cost, operational totals, and blocked pair reasons
(`report.ts:432-490`). It is designed for a human deciding whether a docs change
helped, not for a CI pass/fail.

### 5.4 lathe today: the right content, too little evidence

Agentlathe's failure format is genuinely good. A real run of the stub:

```
chapter 01 -- talk to the model

  FAIL 01 sends a well-formed streaming request
        why: An OpenAI-compatible server routes on the path, reads the model
             from the body, and authenticates with a bearer token. Get any of
             those wrong and you get a 401 or 404 that looks like a logic bug.
        error: this chapter is not implemented yet: harness: not implemented yet: OpenAICompat.Chat
        hint: Log the http.Request you build before you send it.
        more: lathe hint 01 request-shape
```

And the reference solution:

```
  PASS 01 sends a well-formed streaming request
  ...
SUITE PASSED  6/6 cases passed
```

The four-part contract — `why`, `expected`/`actual`, `hint`, `more` — is better than
most production suites. The gap is what happens when `expected` and `actual` are large:
`grader.printFailure` prints multiline values with a fixed indent, but there is no
first-difference marker, no structural diff, no event trace, and no saved artifact:

```go
// internal/grader/grader.go:144-160
var m *Mismatch
if errors.As(err, &m) {
    want := render(m.Want)
    got := render(m.Got)
    if m.Label != "" {
        fmt.Printf("        %s %s\n", dim("field:"), m.Label)
    }
    if strings.Contains(want, "\n") || strings.Contains(got, "\n") {
        fmt.Printf("        %s\n%s", dim("expected:"), indent(want, "          "))
        fmt.Printf("        %s\n%s", dim("actual:"), indent(got, "          "))
    } else {
        fmt.Printf("        %s %s\n", dim("expected:"), want)
        fmt.Printf("        %s %s\n", dim("actual:  "), got)
    }
}
```

`render` falls back to `%+v` for structs, slices, maps, pointers, and interfaces
(`grader.go:220-241`). That is readable for a `Usage` struct or a short message list,
and unreadable for a 40-event transcript or a full request body. A learner debugging
chapter 04 will need the trace, not a Go-printed struct.

---

## 6. The transferable design

### 6.1 Steal these ten things

| # | Steal | PiG evidence | Agentlathe adaptation |
|---|---|---|---|
| 1 | **One scenario = one observable + one claim.** | `covers` + description + driver + assertions (`README.md:99-121`) | Keep the current `Case` granularity. Add `Covers []string` naming the README mechanism and source file, so coverage is machine-checkable. |
| 2 | **Differential oracle, not just a pass/fail reference.** | `pig` vs pinned `pi` on the same fixture (`runner.go:84-131`) | Run the learner and the reference solution against the *same* fixture in two temp dirs; compare observations and report the first divergent event. `--solution` already builds the second workspace; make it an oracle, not a separate run. |
| 3 | **Record expected values only from the oracle.** | "Do not skip a case, and do not regenerate expected values from the port." (`sdk-vectors/README.md:50`) | `--record` refuses to run unless `--solution` is set; goldens are written from the reference, never from the learner. |
| 4 | **Normalize only with a written reason.** | Every `normalize_replace` and `json_alias` has a `reason`; missing reason fails (`asserts.go:150-153`, `json_compare.go:53-58`) | A `Normalize` list on each golden: `id`, `timestamp`, `path`, `literal`. The runner rejects a rule with an empty reason. |
| 5 | **Event-sequence grammar, not just final text.** | RPC/JSON scenarios compare complete event streams; observation oracles compare delivery order | For ch04/07/08/10, assert a grammar over the event log and print the first violating event with expected/actual. |
| 6 | **Mutation proof per mechanism.** | `unit-evidence` requires a compiling mutation that made the named test fail (`unit_evidence.go:17-25`) | For each case, a `mutations/<case>.patch` applied to the reference must make exactly that case fail at the named assertion. This is the strongest anti-vacuous-test tool. |
| 7 | **No silent zero.** | `require-parity-ran` fails when no scenario ran (`README.md:54`) | `Suite` fails on `len(cases)==0`; `lathe` fails if a chapter has no cases or a case has no assertions. |
| 8 | **Failure artifacts + rerun command.** | `artifacts.go:30-102` | Write `tmp/lathe/<case>/fixture.json`, `requests.jsonl`, `observation.json`, `trace.txt`, `diff.txt`, `rerun.sh`. |
| 9 | **Scrub and pin the environment.** | Per-binary homes, cwd copies, `{{TEMP}}`, env scrub (`README.md:74-95`) | Pin `TZ`, isolate `HOME`, use `t.TempDir`, and never read ambient config. |
| 10 | **Test the test infrastructure.** | PiG has tests for the runner, comparator, coverage generator, and the faux provider itself | `internal/fake`, `internal/grader`, and `internal/hints` need their own tests. The verifier is a product; it needs a verifier. |

Two more ideas are worth stealing in weaker form:

- **Sibling cases.** When a case fails, list other cases that cover the same mechanism
  or source file (PiG `SiblingScenarios`, `failure_hints.go:135-160`). For lathe,
  "other cases that exercise `Chat`" is a useful hint that the bug is systemic.
- **A content-hash lock for claims.** PiG pins the upstream source slice a behavior
  contract mirrors (`behavior-contracts.toml`, `sha256`). Agentlathe could pin the
  README section or the reference file a case claims to test, so a case cannot silently
  drift away from the lesson it grades.

### 6.2 Do not copy these

| Do not copy | Why not |
|---|---|
| **821 paired scenarios / 3,398 files / six drivers.** | This is the cost of porting a full product. Agentlathe has 11 chapters and one seam. 6–12 cases per chapter is the right order of magnitude. |
| **Two-language faux provider tables.** | PiG pays a permanent tax keeping `ai/test_faux.go` and `test-faux-provider.ts` behaviorally identical, then writes oracle tests to police the duplication. Agentlathe has one language and one implementation; use the reference solution as the second opinion instead. |
| **tmux / headless terminal / screen crops.** | Agentlathe grades the harness in-process. A terminal driver would add platform flakiness for no pedagogical gain. The TUI is explicitly out of scope. |
| **`runs = 3` by default.** | PiG needs durability because its real binaries are timing-sensitive. Agentlathe's fake is deterministic; three identical runs buy nothing. Run once; if a case is flaky, fix the fixture. |
| **`runtime_ratio_max`, perf gates, owner credentials, live-model tiers.** | These are release gates for a product. A course must be free, offline, and identical on every machine. Keep live mode as an ungraded README recipe, per issue 10. |
| **Full JSON alias algebra (id bijections, durations, session filenames).** | Most of it exists because PiG compares complete real session files. Agentlathe's chapters 01–04 have no session files. Start with `id`, `timestamp`, `path`, `literal`; add a kind only when a chapter needs it. |
| **Vitest-evals / Docker / LLM judges.** | These measure model capability, not harness correctness, and they cost money. Agentlathe's contract is "no API key, no network." A judge would violate the product. |
| **A coverage dashboard and a port map.** | PiG's coverage report is an accounting artifact for a port. Agentlathe needs a one-screen `lathe progress` and a linter that every README claim has a case, not a 805 KB report. |
| **A general-purpose assertion language.** | PiG supports 18 comparators because it compares terminals, JSON, files, and processes. Agentlathe needs a handful of comparison ops, one sequence op, one golden comparator, and named invariants. Every extra op is a new way for a case to be wrong. |

### 6.3 The verdict

Agentlathe's verifier should be a **differential, fixture-driven, self-tested grader**,
not a parity harness.

- **Differential** because the reference solution already exists and is the cheapest
  oracle available. Running learner and reference against the same fixture gives you a
  second observation for free and turns "did it pass?" into "where did it diverge?"
- **Fixture-driven** because chapter cases should be data, not 200-line Go closures.
  A fixture that names every SSE frame can express interleaving, omitted IDs, malformed
  frames, and mid-stream death; a `Reply{Text, ToolCalls}` cannot.
- **Self-tested** because the verifier is the product. A grader bug that passes a wrong
  answer is worse than a missing chapter. Mutation proofs, hint-completeness checks, and
  a stub-must-fail test are not optional polish; they are the contract.

The current chapter check is a good first draft of the *feedback* layer. It is not yet
a verifier architecture. The next chapter should not add another hand-written Go
closure; it should add the fixture format and the assertion runner that all remaining
chapters share.

---

## 7. Minimal verifier architecture in Go

The declarations below are the whole design: a fixture the fake server can replay, an
observation the driver records, a small assertion language, a runner, and a report. It
is deliberately compact: about 130 core declarations plus a 45-line supporting schema,
with no implementation. It fits in one package and uses only the standard library plus
`net/http/httptest`.

### 7.1 Principles

1. **The verifier owns its schema.** Do not import `harness.Message` into the verifier.
   The learner may change `types.go`; the verifier's observation format must not change
   with it. A per-chapter `Driver` adapts learner types to verifier types.
2. **The fixture owns the bytes.** Every SSE frame, including its boundary, is data.
   A `Frame` can be structured (`Text`, `Index`/`ID`/`Name`/`Args`, `Usage`, `Finish`) or raw (`Raw`), so
   the same format can express a normal stream, a malformed frame, a comment, or a
   truncated connection.
3. **The observation is the evidence.** Requests (exact bytes), messages, events, files,
   usage, and error. Assertions address it by path; goldens store it; diffs compare it.
4. **Assertions are data, not closures.** The only Go function in a case is the
   `Driver`, which knows how to call the chapter's harness. Everything else is
   inspectable, lintable, and recordable.
5. **The runner never trusts a zero.** Zero cases, zero requests, zero events, or
   unconsumed fixture turns are failures.

### 7.2 The declarations

The core is about 130 declarations (160 lines with spacing). It is followed by the
handful of schema types it references. Nothing here is chapter-specific; a per-chapter `Driver` is the only
adapter.

```go
package verify

import (
	"context"
	"encoding/json"
	"time"
)

// Fixture is the scripted model: one Turn per provider request.
type Fixture struct {
	Model string            `json:"model"`
	Clock time.Time         `json:"clock"`          // injected, never time.Now
	Seed  map[string]string `json:"seed,omitempty"` // workspace files
	Turns []Turn            `json:"turns"`
	Limit Limits            `json:"limit,omitempty"`
}

type Limits struct {
	MaxRequests int `json:"maxRequests"`
	MaxEvents   int `json:"maxEvents"`
	TimeoutMs   int `json:"timeoutMs"`
}

type Turn struct {
	Status int     `json:"status,omitempty"` // 0 => 200 SSE
	Body   string  `json:"body,omitempty"`   // non-2xx body
	Frames []Frame `json:"frames,omitempty"`
	Cut    string  `json:"cut,omitempty"` // eof|reset|hang
}

// Frame is one SSE event. Raw is full wire bytes and wins over the fields.
type Frame struct {
	Raw    string `json:"raw,omitempty"`
	Text   string `json:"text,omitempty"`
	Index  *int   `json:"index,omitempty"` // tool-call index; nil for text
	ID     string `json:"id,omitempty"`    // first fragment only
	Name   string `json:"name,omitempty"`
	Args   string `json:"args,omitempty"` // appended verbatim
	Usage  *Usage `json:"usage,omitempty"`
	Finish string `json:"finish,omitempty"`
}

// Observation is the evidence: requests are captured by the fake server, the
// rest is filled by the chapter Driver.
type Observation struct {
	Requests []Request         `json:"requests"`
	Messages []Message         `json:"messages"`
	Events   []Event           `json:"events"`
	Files    map[string]string `json:"files,omitempty"`
	Usage    Usage             `json:"usage"`
	Err      string            `json:"error,omitempty"`
}

type Request struct {
	Method string              `json:"method"`
	Path   string              `json:"path"`
	Header map[string][]string `json:"header"`
	Body   json.RawMessage     `json:"body"` // exact bytes
}

type Event struct {
	Type string         `json:"type"`
	Turn int            `json:"turn"`
	Seq  int            `json:"seq"`
	Data map[string]any `json:"data,omitempty"`
}

type Case struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Why     string    `json:"why"`
	Hints   [3]string `json:"hints"` // checked at load
	Fixture Fixture   `json:"fixture"`
	Input   Input     `json:"input"`
	Then    []Assert  `json:"then"`
}

type Input struct {
	Op          string            `json:"op"` // chat|run|resume|...
	Model       string            `json:"model,omitempty"`
	Messages    []Message         `json:"messages,omitempty"`
	Tools       []ToolDef         `json:"tools,omitempty"`
	Files       map[string]string `json:"files,omitempty"`
	MaxTokens   int               `json:"maxTokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
}

// Assert addresses an Observation by JSON pointer; * and ** are wildcards.
type Assert struct {
	Op    Op     `json:"op"`
	Path  string `json:"path,omitempty"`
	Want  any    `json:"want,omitempty"`
	Regex string `json:"regex,omitempty"`
	Seq   []Step `json:"seq,omitempty"`
	Name  string `json:"name,omitempty"` // golden or invariant name
}

type Op string

const (
	OpEq        Op = "eq"
	OpSubset    Op = "subset"
	OpContains  Op = "contains"
	OpMatch     Op = "match"
	OpLen       Op = "len"
	OpPresent   Op = "present"
	OpAbsent    Op = "absent"
	OpSeq       Op = "seq"
	OpGolden    Op = "golden"
	OpInvariant Op = "invariant"
)

// Step is one event pattern. Type=="" is a group over Of or a choice over
// Alt, optionally repeated.
type Step struct {
	Type   string            `json:"type,omitempty"`
	Repeat string            `json:"repeat,omitempty"` // "", "*", "+", "?"
	Of     []Step            `json:"of,omitempty"`
	Alt    []Step            `json:"alt,omitempty"`
	Where  map[string]string `json:"where,omitempty"`
}

type Runner struct {
	Root   string
	Driver Driver
	Update bool // record goldens; reference only
	Now    func() time.Time
}

// Driver adapts one chapter's harness. The runner owns the fake server, seeds,
// clock, IDs, timeouts, request capture, and report.
type Driver interface {
	Do(ctx context.Context, env Env, in Input) (Observation, error)
}

type Report struct {
	Chapter  string        `json:"chapter"`
	Cases    []CaseResult  `json:"cases"`
	Passed   int           `json:"passed"`
	Failed   int           `json:"failed"`
	Duration time.Duration `json:"duration"`
}

type CaseResult struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Why      string    `json:"why"`
	Status   string    `json:"status"` // pass|fail|error|skip
	Hint     string    `json:"hint,omitempty"`
	Failures []Failure `json:"failures,omitempty"`
	Trace    []Event   `json:"trace,omitempty"` // last N events
}

type Failure struct {
	Assert string `json:"assert"`
	Path   string `json:"path"`
	Want   string `json:"want"`
	Got    string `json:"got"`
	Diff   string `json:"diff,omitempty"`
	Event  int    `json:"event,omitempty"`
}
```

The supporting schema. These are the shapes a fixture and a golden need; they are
small enough to keep in the same package.

```go
type Usage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
	ToolCallID string     `json:"toolCallId,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type Golden struct {
	Version   int         `json:"version"`
	Normalize []Normalize `json:"normalize"`
	Want      Observation `json:"want"`
}

// Kind is id|timestamp|duration|path|literal. Reason is mandatory.
type Normalize struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	With   string `json:"with,omitempty"`
	Reason string `json:"reason"`
}

type Env struct {
	BaseURL string
	Dir     string
	Clock   time.Time
	IDs     *IDs
}

type IDs struct {
	Prefix string
	N      int
}
```

### 7.3 What a fixture looks like

A chapter-01 case as data. The frames are the contract: the second frame starts in the
middle of a sentence, the third carries usage, the fourth is `[DONE]`.

```json
{
  "id": "text-reassembly",
  "title": "reassembles text split across SSE chunks",
  "why": "The response arrives as many small deltas. Assigning instead of appending passes a one-chunk test and silently truncates real answers.",
  "hints": [
    "Each data: line is a partial update, not a full message.",
    "Accumulate with strings.Builder or []byte, then convert once at the end.",
    "Append each delta, skip the [DONE] sentinel, and read usage from the final chunk."
  ],
  "fixture": {
    "model": "gpt-4o-mini",
    "clock": "2026-01-01T00:00:00Z",
    "turns": [{
      "frames": [
        {"text": "The quick "},
        {"text": "brown fox."},
        {"usage": {"promptTokens": 42, "completionTokens": 7, "totalTokens": 49}, "finish": "stop"},
        {"raw": "data: [DONE]\n\n"}
      ]
    }],
    "limit": {"maxRequests": 1, "maxEvents": 16, "timeoutMs": 2000}
  },
  "input": {
    "op": "chat",
    "model": "gpt-4o-mini",
    "messages": [
      {"role": "system", "content": "You are terse."},
      {"role": "user", "content": "hi"}
    ]
  },
  "then": [
    {"op": "eq", "path": "requests/0/path", "want": "/v1/chat/completions"},
    {"op": "eq", "path": "requests/0/header/Authorization/0", "want": "Bearer test-key"},
    {"op": "eq", "path": "requests/0/body", "want": {
      "model": "gpt-4o-mini",
      "stream": true,
      "messages": [
        {"role": "system", "content": "You are terse."},
        {"role": "user", "content": "hi"}
      ]
    }},
    {"op": "eq", "path": "messages/0/role", "want": "assistant"},
    {"op": "eq", "path": "messages/0/content", "want": "The quick brown fox."},
    {"op": "eq", "path": "usage/totalTokens", "want": 49}
  ]
}
```

The whole-request assertion is the important addition. The current check asserts
`path`, `authorization`, `model`, `stream`, and `messages` separately; it would accept a
request with an extra bogus field or a missing `Content-Type`. A whole-body assertion
(or a `subset` if the chapter deliberately allows provider-added fields) closes that.

For chapter 04, the same format expresses the grammar from issue 02:

```json
{
  "op": "seq",
  "path": "events",
  "seq": [
    {"type": "agent_start"},
    {"type": "turn_start"},
    {"repeat": "*", "alt": [
      {"type": "text_delta", "repeat": "*"},
      {"type": "", "of": [
        {"type": "tool_use"},
        {"type": "tool_result", "where": {"toolCallId": "call_1"}}
      ]}
    ]},
    {"type": "turn_end"},
    {"type": "agent_end"}
  ]
}
```

The matcher reports the first violating event, e.g.:

```
FAIL 04 tool-call pairing
  why: every tool_use must be followed by its tool_result before the turn ends.
  at: events/7
  expected: tool_result(toolCallId=call_1)
  actual:   text_delta("done")
  trace: 5 turn_start / 6 tool_use(call_1) / 7 text_delta("done")
  hint: Emit the tool result before the next model request, not after the turn.
```

### 7.4 The runner algorithm

1. **Load and validate.** `len(Case.Hints)==3`, every hint non-empty, `len(Case.Then)>0`,
   `len(Fixture.Turns)>0`, unique case IDs. Fail the suite on any violation.
2. **Create the fake server.** For each request, take the next `Turn`, emit its `Frames`
   in order (structured frames are encoded to the provider's SSE shape; `Raw` is written
   verbatim), capture the exact request bytes, and enforce `MaxRequests`. If the cursor
   runs past the last turn, **fail**; do not answer politely.
3. **Create the environment.** Temp dir, seed files, `Env{BaseURL, Dir, Clock, IDs}`.
   Set `TZ=UTC`, `HOME=<temp>`, and no ambient config.
4. **Run the driver.** `context.WithTimeout`. A panic becomes `Status: error`; a returned
   error becomes `Observation.Err`; a timeout becomes a failure with the last event.
5. **Check completeness.** Every fixture turn consumed, `len(Requests)` equals the
   number of turns, `len(Events) <= MaxEvents`, no unexpected request.
6. **Evaluate `Then` in order.** Collect all failures, not just the first, so the learner
   gets the whole picture. For `OpGolden`, normalize the observation with the golden's
   rules and compare; for `OpInvariant`, call the named function in a registry.
7. **Report.** Write text with the existing `why`/`expected`/`actual`/`hint` format plus
   a trace and a first-difference marker; write JSON to `tmp/lathe/<chapter>.json`;
   write artifacts on failure. Exit nonzero if any case failed.

### 7.5 Record, replay, and mutation

**Record.** `lathe test NN --solution --record` runs the *reference* driver, normalizes
the observation, and writes `chapters/NN/checks/testdata/<case>.golden.json`. Without
`--solution`, `--record` is refused. This is PiG's "do not regenerate expected values
from the port" rule adapted to a course. Goldens are a regression net for the reference
and a byte-level contract for the learner; they do not replace the `why`-bearing
assertions.

**Replay.** Normal test runs compare against inline `Then` assertions first, then any
`OpGolden` assertion. A golden mismatch shows the first differing path and a unified
diff, not a wall of JSON. The golden includes its normalization rules and their reasons,
so a reviewer can see exactly what was allowed to vary.

**Mutation.** `lathe verify NN` applies each `testdata/mutations/<case>.patch` to a
copy of the reference solution, runs the case, and asserts that it fails at the named
assertion path. A case that still passes under its mutation is not discriminating. This
is the single highest-value idea in PiG's `unit-evidence`, and it is cheap at
lathe's scale: one patch per mechanism, not per upstream path.

**Invariants.** Some properties are better expressed as code than as a path assertion:

```go
var Invariants = map[string]func(Observation) error{
    "no-orphan-tool-result": func(o Observation) error { /* every tool_result has a tool_use */ },
    "tool-pairs-not-split":  func(o Observation) error { /* pruning never separates a pair */ },
    "within-budget":         func(o Observation) error { /* usage <= chapter budget */ },
    "append-only-session":   func(o Observation) error { /* session lines are never rewritten */ },
}
```

The registry is small and named; a case references `{"op":"invariant","name":"..."}`.
This keeps the fixture declarative without turning the assertion language into a
programming language.

### 7.6 What is deliberately absent

No terminal driver, no tmux, no screen capture, no Docker, no live model, no LLM judge,
no coverage dashboard, no port map, no 18-comparator algebra, no cross-language fixture
duplication. Those are PiG-specific costs. The minimal verifier has one fake (HTTP/SSE),
one oracle (the reference solution), one observation schema, ten assertion ops, and a
handful of named invariants. That is enough for all eleven chapters.

---

## 8. Gaps in lathe today

The current verifier is a good chapter-01 grader. It is not yet the verifier the course
needs for chapters 02–11. The gaps below are ordered by what blocks the next chapter.

### 8.1 What is already right (keep it)

- **The seam is the right one.** `Provider` is small, stable, and testable; the fake
  speaks real HTTP/SSE, not a mock of the learner's code (`harness/types.go:83-87`,
  `chapters/01-chat-with-the-model/README.md:31-39`).
- **The fake deliberately fragments text.** `split(reply.Text, 4)` forces append-vs-assign
  to matter (`internal/fake/server.go:247-252, 303-322`). This is the opposite of PiG's
  one-write determinism and is correct for chapter 01.
- **Negated assertions reject stubs.** `WantedErr` refuses a "not implemented" error, so
  a blank chapter cannot pass an "expect an error" case (`grader.go:75-88`).
- **Cases do not stop each other.** `Suite` runs every case and reports the whole picture
  (`grader.go:93-128`); panics are recovered per case (`:130-137`).
- **The feedback contract is excellent.** `why` names the mechanism, expected/actual is
  the evidence, and the hint ladder is one keystroke away (`README.md:87-106`).
- **`--solution` is isolated.** It copies the repo to a temp dir, overlays cumulative
  solutions, and does not record progress (`cmd/lathe/main.go:305-336`).
- **It is genuinely free and fast.** Six cases run in 0.044s with no network or key.

### 8.2 P0 — fix before chapter 02

| Gap | Evidence | Why it blocks | Fix |
|---|---|---|---|
| **Fixture cannot express the stream shape.** | `Reply` has only `Text` and `ToolCalls`; the handler emits all text, then all tool calls (`server.go:247-283`). `split` cuts text into four roughly equal fragments (`:303-322`). Issue 05 is open. | Chapter 02 is about tool-call fragments keyed by index, omitted `id`/`name`, interleaving, and `finish_reason: tool_calls`. The current fake cannot express the bug. | Replace `Reply` with `[]Frame` (raw/text/call/usage/finish/cut). Keep a helper that expands a simple text reply into frames for chapter 01. |
| **No whole-request assertion.** | `Recorded.Raw` and `Header` are captured (`server.go:74-88`) but no check uses them; `ContentType` is captured and unused, and `Requests()` is never called (checks use `Last()`). Ch01 asserts only five fields (`ch01_test.go:44-60`). | A harness can send extra fields, omit `Content-Type`, or send the wrong `Accept` and still pass. The reference sets both headers (`solutions/ch01/harness/provider_openai.go:72-73`) but the checks never verify them. | Assert `requests/0/body` as a whole object (or `subset` if deliberately lenient), plus `Content-Type` and `Accept`. |
| **No per-case timeout or subtest.** | `Suite` loops sequentially (`grader.go:101-112`); `runCase` recovers panics but does not bound time (`:130-137`). | A harness that blocks on a read hangs the whole chapter until `go test -timeout` kills everything, with no per-case attribution. | `t.Run(c.ID, ...)`, `context.WithTimeout`, and `Limits.TimeoutMs` per case. |
| **The verifier is untested.** | The only `_test.go` in the repo is the chapter check; `internal/fake`, `internal/grader`, and `internal/hints` have no tests. | A bug in the fake or grader silently misgrades every learner. The suite can pass vacuously. | Add meta-tests: fake-adherence, grader golden output, hint completeness, solution contract, stub-must-fail, mutation proofs. |
| **Hints are not validated.** | `Hints []string` is a slice; `hints.Get` returns nil for an unknown case (`hints.go:49-53`); `printFailure` only prints `Hints[0]` and the `more:` line if `len>1` (`grader.go:162-167`). | A missing or extra rung is invisible; a typo in a case ID silently removes the ladder. | `Hints [3]string`; `TestHintsComplete` checks every case has exactly three non-empty rungs; linter checks no orphan hint keys. |
| **The suite can pass with zero cases.** | `Suite` with `len(cases)==0` prints `SUITE PASSED 0/0` and returns (`grader.go:114-117`). | A chapter whose cases were accidentally deleted looks green. | Fail on zero cases, zero assertions, or zero requests. |
| **Out-of-script requests are hidden.** | `next()` returns `Reply{Text: ""}` when the cursor is exhausted (`server.go:131-141`). | A harness that makes an extra request gets a polite empty answer and may still pass on final text. | Fail the case on overrun; assert `len(requests) == len(turns)` and all turns consumed. |
| **The runner scrapes stdout.** | `filterTestOutput` marks pass if any line contains `SUITE PASSED` (`main.go:284-303`); `summarize` searches for `"cases passed"` (`:261-272`); `ignoreExit` swallows any `*exec.ExitError` (`:274-280`). | A panic after the summary, or a compile error with stale output, can be misread. There is no machine-readable report for CI or progress. | Emit `Report` as JSON; have `lathe` read it; keep text rendering as a view. |

### 8.3 P1 — needed before chapters 04, 06, 07

| Gap | Evidence | Why it matters | Fix |
|---|---|---|---|
| **No event log or sequence grammar.** | There is no event type in the verifier; chapter 04's grammar is still an open issue (`issues/02-loop-grammar.md`). | The central claim of chapter 04 is an event sequence, not a final message. Inline `Eq` cannot express "no orphan tool_result." | `Event` + `Seq` DSL (§7) with first-violation messages. |
| **No invariants or property tests.** | `grader` offers only `Eq`, `Eqf`, `NoErr`, `ErrIs`, `WantedErr` (`grader.go:44-88`). | Budget, pruning, and session chapters need invariants ("never exceed", "never split a pair", "append-only"). | Named invariant registry; `FuzzReadSSE` (any split of the same bytes yields the same message); grammar fuzz. |
| **No clock or ID seam.** | The fake hardcodes `created: 1700000000` and `chatcmpl-fake` (`server.go:217, 219`), and tool IDs `call_%d` (`:259-261`). | Chapters 06/07/10 need deterministic session IDs, timestamps, and resume behavior. Normalizing after the fact is harder than injecting at the seam. | `Env.Clock`, `Env.IDs`; fixture `clock`; IDs allocated in one place. |
| **No workspace fixture.** | Checks only exercise HTTP; there is no seed dir or file assertion. | Chapters 03/05/09/10 need files (`AGENTS.md`, skills, subagent workspace) and assertions on what the harness wrote. | `Fixture.Seed`, `Observation.Files`, `t.TempDir`, path normalization. |
| **The fake is a fixed queue, not a responder.** | `Server.replies` is a slice with a cursor (`server.go:91-97, 131-141`). | Chapter 02+ needs "if the request contains this tool result, reply with that." A fixed queue forces the test to encode the entire conversation as a flat list and breaks under a legitimate extra turn. | `Turn.Match` (predicate on the request) or a per-session queue; PiG's `classify` is the model to copy. |
| **No concurrency/subagent support.** | One global cursor under a mutex (`server.go:131-141`); no per-session state. | Chapter 10 (subagent) and any parallel tool-call test need independent request streams and deterministic interleaving. | Per-session queues; request IDs; a scheduler that records arrival order. |
| **No golden transcripts or replay.** | There is no `testdata/` under `chapters/`; every expectation is inline. | A byte-exact transcript is the best regression net for the reference and the best way to teach "the whole event stream is the contract." | `Golden` with normalization and `--record` restricted to `--solution`. |

### 8.4 P2 — polish

| Gap | Evidence | Fix |
|---|---|---|
| **No real diff.** | `Mismatch` renders with `%+v`; multiline output is only indented (`grader.go:144-160, 220-241`). | Add a first-difference pointer and a small unified diff for strings/JSON. |
| **No failure artifacts.** | Failures print to stdout; nothing is saved. | Write fixture, requests, observation, trace, diff, and `rerun.sh` under `tmp/lathe/<case>/`. |
| **No coverage of claims.** | `Case` has no `Covers`; the README definition-of-done is prose. | Add `Covers []string`; linter checks every README claim maps to a case and every case names a claim. |
| **No CI gate.** | The repo has no `.github/workflows`. | `go test ./...`, `lathe test all --solution`, `lathe verify --mutations` on every push. |
| **No normalization policy.** | Not needed yet; will be for sessions/timestamps. | `Normalize` list with mandatory reasons; reject unreasoned rules. |
| **No `--json` report.** | `lathe` only prints text. | Read the runner's JSON; keep the current text as the human view. |

### 8.5 Two specific under-tests in chapter 01

1. **The solution's extra behavior is not graded.** `solutions/ch01/harness/provider_openai.go`
   implements `req.Model` override, optional `MaxTokens`, optional `Temperature`, and
   `toWireTools`. The checks never exercise `req.Model`, `MaxTokens`, `Temperature`, or
   tools. A learner can omit all of it and still get 6/6. Either grade it or remove it
   from the reference; otherwise the reference teaches behavior the course does not
   require.
2. **The SSE framing tolerance is described but not tested.** The README says a harness
   must tolerate `:` comments, `event:`/`id:` fields, and blank lines
   (`chapters/01-chat-with-the-model/README.md:82-84`), but the fake only emits `data:` lines. A learner
   can split on `\n\n` and pass. Add frames for a comment, an `event:` line, a `data:`
   line without a space, and CRLF; assert the same final message.

---

## 9. Could not determine

- **What the brief's "~4,000 test files" refers to.** At PiG commit `f2f028f`, I count
  7,935 `_test.go` files (601 under `test/`), 821 parity TOML scenarios, and 3,398 files
  under `test/parity/`. None of those is ~4,000. It may refer to a historical snapshot,
  a subset (e.g. ported upstream test files: 685), or a different counting rule. I did
  not guess.
- **The internals of PiG's `closure/`, `interface-closure/`, and `correspondence/`
  evidence graph.** I read their purpose, schema-level role, and the coverage report's
  use of them, but not enough of the derivation code to assert how every closure cell is
  computed. I make no claim about their correctness.
- **The exact provenance of crush's cassettes.** There is no recording script or README
  for `internal/agent/testdata/`; the YAML contains real Hyper/DeepSeek response IDs,
  timestamps, and durations, and the recorder defaults to `ModeRecordOnce`. I infer they
  were recorded from a live provider once and checked in, but I cannot determine when,
  with which model build, or by what command.
- **Whether pi's evals run in any CI beyond the public workflows.** `.github/workflows/`
  has no eval reference; `npm test` does not run them. A private or scheduled job could
  exist outside the repo. I only assert what the checked-in workflows show.
- **Whether PiG's one live scenario currently passes.** It is tagged
  `live`/`real-llm`/`owner-credentials` and needs the owner's provider credentials
  (`scenarios/print/01-print-mode-arithmetic.toml:9-26`). I did not run it.
- **Full-suite runtime and pass status for PiG, crush, and pi.** PiG needs the pinned Pi
  binary and tmux; crush would need network for any missing cassette; pi needs a full
  build and, for evals, credentials. I did not run the full suites. I did run
  lathe's reference and stub checks (see the verification log).
- **Agentlathe chapters 02–11 are not specified enough to grade the verifier against
  them.** The issues in `.scratch/lathe/issues/` are open questions, not designs.
  The gaps in §8 are against the chapter-01 verifier and the stated chapter arc.
- **The best normalization policy for future lathe chapters.** Session IDs,
  timestamps, and temp paths will need rules, but the exact set depends on the session
  format decision (issue 03), which is still open.

---

## 10. Verification log

What I actually ran, beyond reading:

```text
$ cd /tmp/selftest/PiG && git rev-parse HEAD
f2f028f8459504deb6066d393c3f1c316596f2f5

$ find /tmp/selftest/PiG -name '*_test.go' | wc -l
7935
$ find /tmp/selftest/PiG/test/parity/scenarios -name '*.toml' | wc -l
821
$ find /tmp/selftest/PiG/test/parity -type f | wc -l
3398

$ cd /tmp/selftest/crush && git rev-parse HEAD
21dd7482afe2846d8cea8353cbd1ee7f41d2e9a9
$ grep -rh '^func Test' --include='*_test.go' internal | wc -l
1946
$ find internal/agent/testdata -name '*.yaml' | wc -l
14

$ cd /tmp/selftest/pi && git rev-parse HEAD
f9daca3480732d0042b1df3a6e27221f597a620d
$ find packages -name '*.test.ts' | wc -l
706
$ find packages/evals/evals -name '*.eval.ts' | wc -l
7
```

Agentlathe, from the project root:

```text
$ ./lathe test 01 --solution
...
SUITE PASSED  6/6 cases passed
  -> chapter 01 passed (6/6 cases passed)

$ NO_COLOR=1 go test ./chapters/01-chat-with-the-model/checks/ -run TestChapter01 -count=1
...
SUITE FAILED  0/6 cases passed, 6 failed
```

The first run proves the reference contract; the second proves the stub fails for the
intended reason (`this chapter is not implemented yet: ...`) rather than a compile error
or a nil dereference.

Citations are to the commits above. Line numbers will drift; the quoted text and the
commit hash are the stable evidence.
