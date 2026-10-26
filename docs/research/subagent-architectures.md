# Subagent architectures in pi extensions

Research note for the **lathe** Go course chapter on subagents.

This is a code-first reconstruction, not a README summary. Every claim below was checked against the checked-out source at the commits in the table. Where a README and the implementation disagree, the code is reported and the disagreement is called out.

## Snapshot and method

All repositories were cloned shallowly under `/tmp/sa-survey/`. Paths in this document are relative to that directory unless stated otherwise.

| Repo directory | Upstream | Commit | Package / version | Language | Source size* |
|---|---|---|---|---|---|
| `nicobailon_pi-subagents` | `github.com/nicobailon/pi-subagents` | `d9c5ca2` (2026-10-26) | `pi-subagents` 0.83.0, private package | TypeScript (ESM) | 103,095 nonblank lines, 339 `.ts` files under `src/` |
| `tintinweb-pi-subagents` | `github.com/tintinweb/pi-subagents` | `ca93a52` (2026-10-13) | `@tintinweb/pi-subagents` 0.22.0 | TypeScript (ESM) | 21,051 nonblank lines, 59 `.ts` files under `src/` |
| `championswimmer_pi-subagent-manager` | `github.com/championswimmer/pi-subagent-manager` | `51f1645` (2026-10-22) | `pi-subagent-manager` 1.1.0 | TypeScript (ESM) | 13,250 nonblank lines, 35 `.ts` files under `src/` |

\* Nonblank lines in `src/**/*.ts`; tests, examples, generated files, and `node_modules` excluded. This is a size signal, not a `cloc` measurement.

These are three independent pi extensions. The two projects named `pi-subagents` share a name and a host API, not a codebase: different package names, authors, file layouts, tool names, and execution models. There is no common git history in the shallow snapshots.

## Ground truth: what a pi extension tool may return

All three projects ultimately use pi's `registerTool()` tool contract. That contract determines what "the child result goes back to the parent" can mean.

`@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts:453-520` defines:

```ts
export interface ToolDefinition<TParams extends TSchema = TSchema, TDetails = unknown, TState = any> {
    name: string;
    label: string;
    description: string;
    promptSnippet?: string;
    promptGuidelines?: string[];
    parameters: TParams;
    // ...
    execute(toolCallId: string, params: Static<TParams>, signal: AbortSignal | undefined, onUpdate: AgentToolUpdateCallback<TDetails> | undefined, ctx: ExtensionToolContext): Promise<AgentToolResult<TDetails>>;
    // ...
}
```

`@earendil-works/pi-agent-core/dist/types.d.ts:370-390` defines the result:

```ts
export interface AgentToolResult<T = JsonValue | undefined> {
    /** Text or image content returned to the model. */
    content: (TextContent | ImageContent)[];
    /** Arbitrary structured details for logs or UI rendering. */
    details: T;
    /** Machine-readable result matching the tool's `outputSchema` ... */
    structuredContent?: JsonValue;
    /** Usage from the final tool execution itself, if available. ... */
    usage?: Usage;
    /** Report a failure without throwing. ... */
    isError?: boolean;
    /** Hint that the agent should stop after the current tool batch. ... */
    terminate?: boolean;
}
```

Pi's own docs state the error boundary explicitly (`@earendil-works/pi-coding-agent/docs/extensions.md:247-251`):

> A tool defines a name, model-facing description, TypeBox parameter schema, and `execute()` function.
> Its result requires model-facing `content` and a `details` field for rendering or state reconstruction.
> ...
> Throw from `execute()` to produce a failed tool result.
> Returning an object does not mark it as an error.

Consequences for the chapter:

1. One `execute()` invocation corresponds to one parent `tool_result`. The host pairs it with the assistant `toolCall` that triggered it.
2. If a subagent result is delivered later as a custom message or a new user message, it is **not** a `tool_result` for the original spawn call. That is acceptable for background work, but it means the spawn call must already have a well-formed receipt result.
3. A failed or cancelled child can still produce a valid `tool_result`. Throwing is only one way to report failure, and it discards any structured `details` the caller wanted.

## Per-repo contributions at a glance

| Repo | Unique contribution | What it is really good at | Main cost |
|---|---|---|---|
| `nicobailon/pi-subagents` | A production-grade delegation runtime: true forked child sessions, foreground and detached-runner background execution, parallel/chain/workflow modes, tool and spawn budgets, durable result artifacts. | The hard isolation and failure cases: real session branch, cancellation/timeout envelopes, ordered `SingleResult[]`, background result files. | Enormous surface area; many modes and adapters obscure the minimal child loop. |
| `tintinweb/pi-subagents` | A Claude-Code-shaped `Agent` tool: fresh child by default, explicit textual `inherit_context`, background-first execution with foreground blocking, foreground/background concurrency pools, child-safe nested tools. | The smallest end-to-end shape of "spawn child, run child session, return text"; its context builder is one function. | Still carries workflow, fleet UI, scheduling, worktree, and Claude-compatibility machinery. |
| `championswimmer/pi-subagent-manager` | Persistent, addressable subagent **threads** with hierarchical task paths, pause/steer/resume, a durable registry, a root mailbox, and a tree UI. | Lifecycle and manager state: children are long-lived sessions, not one-shot calls. | It is a different product model; it does not demonstrate the simple "return one child result as a tool result" design cleanly. |

---

## 1. Registration: what an agent definition is

All three are ordinary pi packages. Their `package.json` declares the **extension entry point**, not individual agents:

- `nicobailon`: `"pi": { "extensions": ["./index.ts"] }` (`package.json:70-73`), with `agents/` shipped via `files` (`package.json:48-55`).
- `tintinweb`: `"pi": { "extensions": ["./src/index.ts"] }` (`package.json:62-65`).
- `pi-subagent-manager`: `"pi": { "extensions": ["./src/index.ts"] }` (`package.json:33-36`), with `agents/` in `files`.

Agent definitions themselves are discovered at runtime from the filesystem (and, for the two `pi-subagents` projects, from installed packages). That separation matters: installing the extension does not register a fixed list of agents, and adding an agent file can change the model-facing tool catalog without a reinstall.

### 1.1 `nicobailon/pi-subagents`

The model-facing tool is named `subagent`, not `Agent` (`src/extension/index.ts:749-759`). A definition is a Markdown file with YAML frontmatter plus a body that becomes the agent's system prompt.

The runtime shape is `AgentConfig` (`src/agents/agents.ts:147-210`):

```ts
export interface AgentConfig {
	name: string;
	runner?: AgentRunnerConfig;
	localName?: string;
	packageName?: string;
	packageSourceName?: string;
	packageSourceVersion?: string;
	packageSourceRoot?: string;
	description: string;
	advertise?: boolean;
	aliases?: string[];
	tools?: string[];
	excludeTools?: string[];
	allowNestedSubagents?: boolean;
	allowedAgents?: string[];
	mcpDirectTools?: string[];
	model?: string;
	modelProvider?: string;
	fast?: boolean;
	thinking?: string | false;
	systemPromptMode: SystemPromptMode;
	inheritProjectContext: boolean;
	inheritGlobalContext: boolean;
	inheritSkills: boolean;
	defaultContext?: AgentDefaultContext;
	defaultAsync?: boolean;
	defaultTimeoutMs?: number;
	defaultToolTimeoutMs?: number;
	defaultAcceptance?: AcceptanceInput;
	acceptanceRole?: AcceptanceRole;
	systemPrompt: string;
	source: AgentSource;
	filePath: string;
	discoveryPriority?: number;
	skills?: string[];
	skillPath?: string[];
	extensions?: string[];
	extensionsFromDefault?: boolean;
	subagentOnlyExtensions?: string[];
	mutationTools?: string[];
	output?: string;
	outputMode?: OutputMode;
	outputSchema?: JsonSchemaObject;
	defaultReads?: string[];
	defaultProgress?: boolean;
	interactive?: boolean;
	maxSubagentDepth?: number;
	toolBudget?: ToolBudgetConfig;
	permissions?: PermissionRules;
	memory?: AgentMemoryConfig;
	machine?: string;
	/** Name of a user-configured `runnerLaunchers` entry that wraps this agent's background runner. */
	launcher?: string;
	disabled?: boolean;
	extraFields?: Record<string, string>;
	override?: BuiltinAgentOverrideInfo;
	modelSource?: AgentModelSourceInfo;
	maxThinking?: ThinkingLevel;
	/**
	 * Digest of the parsed definition, set when a runtime overlay such as the
	 * Intercom bridge rewrites launch-affecting fields. ...
	 */
	definitionDigest?: string;
}
```

A real definition (`agents/researcher.md:1-13`) shows the frontmatter vocabulary:

```yaml
---
name: researcher
description: Autonomous web researcher — searches, evaluates, and synthesizes a focused research brief
tools: read, write, web_search, fetch_content, get_search_content, source_check
thinking: medium
systemPromptMode: replace
inheritProjectContext: true
inheritSkills: false
output: research.md
defaultProgress: true
---
```

A full reference example in the repo's own docs (`docs/agents.md:398-430`) adds fields such as `package`, `aliases`, `excludeTools`, `extensions`, `subagentOnlyExtensions`, `skills`, `skillPath`, `async`, `timeoutMs`, `toolTimeoutMs`, `acceptance`, `acceptanceRole`, `interactive`, `maxSubagentDepth`, `allowNestedSubagents`, and `allowedAgents`.

The parser is deliberately permissive about fields it does not know:

```ts
const { frontmatter, body } = parseFrontmatter(content);

if (!frontmatter.name || !frontmatter.description) {
	continue;
}

const localName = frontmatter.name;
name = localName;
```

(`src/agents/agents.ts:2187-2194`)

Unknown keys are preserved in `extraFields` (`src/agents/agents.ts:2295-2298`). Name and description are mandatory; an agent without either is skipped.

Discovery is layered. The effective discovery function starts from these roots (`src/agents/agents.ts:3052-3057`):

```ts
const effectiveCwd = path.resolve(cwd);
const userDirOld = path.join(getAgentDir(), "agents");
const userDirNew = path.join(os.homedir(), ".agents");
const { readDirs: projectAgentDirs, candidateDirs: projectCandidateDirs, preferredDir: projectAgentsDir } = resolveNearestProjectAgentDirs(effectiveCwd);
const userSettingsPath = getUserAgentSettingsPath();
const projectSettingsPath = getProjectAgentSettingsPath(effectiveCwd);
```

The project roots are the nearest configured project's `.agents` and `.pi/agents` directories (`src/agents/agents.ts:2446-2457`). Package agents, user agents, and project agents are merged with this precedence (`src/agents/agents.ts:3098`):

```ts
const merged = mergeAgentsForScope(scope, userAgents, projectAgents, builtinAgents, packageAgents);
```

In practice the priority is: project overrides user, user overrides builtin, package agents are the lowest tier. A declined project-trust decision removes project discovery entirely (`src/agents/agents.ts:3044-3050`). The registry also supports package-qualified runtime names (`buildRuntimeName`) and aliases, so "agent name" is not necessarily one string.

One important design choice: the agent file can carry a **lot** of capability policy — tool allowlist, excluded tools, extension allowlist, nested delegation grant, max depth, model, thinking, timeout, skills, memory, and even worktree / remote machine settings. Registration is therefore not just prompt loading; it is policy loading.

### 1.2 `tintinweb/pi-subagents`

The tool is named `Agent` (`src/index.ts:1750-1753`, via `SUBAGENT_TOOL_NAMES.AGENT` at `src/agent-runner.ts:44-49`). The agent shape is `AgentConfig` (`src/types.ts:34-94`):

```ts
/** Unified agent configuration — used for both default and user-defined agents. */
export interface AgentConfig {
  name: string;
  /** UI name. `display_name` wins; Claude Code's `name` is accepted as a fallback. */
  displayName?: string;
  /** Claude Code-compatible name color (named color or #RRGGBB). */
  color?: string;
  description: string;
  builtinToolNames?: string[];
  /** Raw `ext:` selector entries from the `tools:` CSV, e.g. ["ext:foo", "ext:bar/x"].
   * Presence of any entry flips extension tools to an explicit allowlist. */
  extSelectors?: string[];
  /** Tool denylist — these tools are removed even if `builtinToolNames` or extensions include them. */
  disallowedTools?: string[];
  /** true = inherit all, string[] = only listed, false = none */
  extensions: true | string[] | false;
  /** Extension-name denylist applied after the `extensions:` include set. Exclude wins.
   * Plain canonical names only (case-insensitive); no paths, no wildcard. */
  excludeExtensions?: string[];
  /** true = inherit all, string[] = only listed, false = none */
  skills: true | string[] | false;
  model?: string;
  thinking?: ThinkingLevel;
  maxTurns?: number;
  /** Persist this subagent as a normal pi session instead of keeping it in memory only. */
  persistSession?: boolean;
  /** Write the subagent's .output transcript. Defaults to true; false suppresses only that transcript. */
  outputTranscript?: boolean;
  /** Optional session directory used when persistSession is true. Omitted = pi's normal session location. */
  sessionDir?: string;
  /**
   * Nested delegation, off by default: undefined = no nested tools;
   * "all" = any enabled agent; string[] = only those agent types.
   */
  allowedSubagents?: "all" | string[];
  systemPrompt: string;
  promptMode: "replace" | "append";
  /** Default for spawn: fork parent conversation. undefined = caller decides. */
  inheritContext?: boolean;
  /** Default for spawn: run in background. undefined = caller decides. */
  runInBackground?: boolean;
  /** Default for spawn: no extension tools. undefined = caller decides. */
  isolated?: boolean;
  /** Persistent memory scope — agents with memory get a persistent directory and MEMORY.md */
  memory?: MemoryScope;
  /**
   * Isolation mode — "worktree" runs the agent in a temporary git worktree,
   * "off" refuses one even when the caller asks (frontmatter outranks params).
   */
  isolation?: IsolationMode;
  /** true = this is an embedded default agent (informational) */
  isDefault?: boolean;
  /** false = agent is hidden from the registry */
  enabled?: boolean;
  /** Where this agent was loaded from. `"package"` means an installed pi package
   * declared it ... */
  source?: "default" | "project" | "global" | "package";
  /** Path of the .md it was loaded from. Unset for embedded defaults. */
  sourcePath?: string;
}
```

A checked-in example (`.pi/agents/auditor.md:1-13`) is small and Claude-Code-like:

```yaml
---
description: Security Code Reviewer
tools: read, grep, find, bash
model: anthropic/claude-haiku-4-5-20251001
thinking: off
max_turns: 10
---

You are a lightweight security auditor. ...
```

Discovery order is explicit and the later load wins (`src/custom-agents.ts:82-97`):

```ts
const globalDir = join(getAgentDir(), "agents");
const workspaceProjectDir = join(cwd, ".agents", "agents");
const projectDir = join(cwd, ".pi", "agents");

const pkgDirs = opts.packageDirs ?? packageAgentDirs(cwd);
const pkgFiles = opts.packageFiles ?? packageAgentFiles(cwd);
// ...
const agents = new Map<string, AgentConfig>();
for (const dir of pkgDirs) loadFromDir(dir, agents, "package", strict, excluded); // lowest priority
for (const file of pkgFiles) loadOneFile(file, agents, "package", strict);
loadFromDir(globalDir, agents, "global", strict);
loadFromDir(workspaceProjectDir, agents, "project", strict); // shared workspace
loadFromDir(projectDir, agents, "project", strict);          // highest priority (overwrites)
```

The type name is the frontmatter `name` if present, otherwise the filename (Claude Code's rule). A `:` in a declared name is rejected (`src/custom-agents.ts:150-172`). Project files parse strictly at startup; later reloads warn and skip. Package-provided agent files have a 1 MB ceiling (`src/custom-agents.ts:219-234`). The registry is reloaded on every `Agent` invocation (`src/index.ts:1946-1947`), which is how newly added `.md` files work without a restart.

The defaults are `general-purpose`, `Explore`, and `Plan` (`src/types.ts:14`). `general-purpose` deliberately omits `builtinToolNames` to mean "all available tools"; `Explore` uses the read-only tool set (`src/default-agents.ts:9-40`).

### 1.3 `championswimmer/pi-subagent-manager`

The tool is `agent_spawn` (`src/orch/tools.ts:73-103`), and the definition type is `AgentType` (`src/types.ts:10-43`):

```ts
export interface AgentType {
  name: string;
  description: string;
  models?: string[];
  /** @deprecated Use ordered models instead. */
  model?: string;
  /** Advisory display names. Not provider/model pins and never used for selection. */
  modelSuggestions?: string[];
  thinkingLevel?: ThinkingLevel;
  /** Pi semantic token for the type pill background and task path foreground. */
  color?: string;
  /** Optional literal Nerd Font glyph; displayed only when the labs setting is enabled. */
  icon?: string;
  tools?: { allow?: string[]; block?: string[] };
  systemPrompt: string;
  filePath?: string;
  source?: "bundled" | "user" | "project";
  /**
   * How this definition is customized relative to its base.
   * - fork: a full `<name>.md` copy (owns the system prompt).
   * - override: a settings-only `<name>.yml` merged on top of the base
   *   (no Markdown body; unset fields follow the base).
   * ...
   */
  customization?: {
    kind: "fork" | "override";
    scope: "user" | "project";
    filePath: string;
  };
  baseSource?: "bundled" | "user" | "project";
  baseFilePath?: string;
}
```

The parser is strict: unknown frontmatter keys are an error, `name` must match a safe identifier, and `description` must be non-empty (`src/prefs/config.ts:199-219`). `tools` is a structured object, not a CSV string:

```ts
if (Object.hasOwn(data, "tools")) result.tools = toolPolicy(data.tools);
```

(`src/prefs/config.ts:250`)

There are two file forms (`src/prefs/config.ts:722-810`):

- `<name>.md` — a full definition ("fork" in this project's sense, unrelated to session forks), with YAML frontmatter and a Markdown body.
- `<name>.yml` — a settings-only override merged on top of the base; the prompt body always comes from the base.

Discovery is manager-owned and does not scan arbitrary pi agent packages (`src/prefs/config.ts:915-934`):

```ts
return scope === "user"
  ? resolve(this.options.agentDir, SUBAGENT_MANAGER_DIR, AGENTS_DIR)
  : resolve(
      this.options.cwd,
      ".pi",
      "agent",
      SUBAGENT_MANAGER_DIR,
      AGENTS_DIR,
    );
```

The layers are `bundled` (`agents/` next to the extension) → user (`<agentDir>/subagent-manager/agents`) → project (`<cwd>/.pi/agent/subagent-manager/agents`), with project only when `includeProject` / project trust is true. The store validates every file and fails closed for malformed or duplicate definitions (`src/prefs/config.ts:606-663`).

### 1.4 Registration comparison

| Question | `nicobailon/pi-subagents` | `tintinweb/pi-subagents` | `pi-subagent-manager` |
|---|---|---|---|
| Model-facing tool | `subagent` | `Agent` | `agent_spawn` (plus status/wait/steer/output) |
| Definition source | `.md` + YAML frontmatter | `.md` + YAML frontmatter | `.md` full definition, `.yml` settings override |
| Type name | frontmatter `name`, package-qualified runtime names possible | frontmatter `name`, else filename | frontmatter `name`, strictly validated |
| Discovery roots | builtin, package, user (`~/.pi/agent/agents`, `~/.agents`), nearest project `.agents`/`.pi/agents` | package, global, `.agents/agents`, `.pi/agents` | bundled, user, project manager directories |
| Precedence | project > user > builtin > package | project > workspace > global > package | project > user > bundled |
| Unknown frontmatter | preserved in `extraFields` | ignored/available to parser | rejected |
| Reload | discovery cache + explicit reload paths | every `Agent` invocation | `ConfigStore.reload()` on session/import/settings changes |

---

## 2. Context construction: exactly what the child sees

This is the most important architectural difference between the projects, and the place where the word "inherit" means three different things.

### 2.1 `nicobailon/pi-subagents`: fresh by default, real session branch for `fork`

The launch context is a two-valued enum (`src/runs/shared/context-mode.ts:1`):

```ts
export type ContextMode = "fresh" | "fork";
```

Resolution is explicit, with `fresh` as the final default (`src/shared/fork-context.ts:83-87`). An explicit `context: "fork"` is strict and fails when the parent has no persisted session or current leaf; an implicit `defaultContext: fork` falls back to fresh when a branch cannot be created. That distinction is documented in `docs/configuration.md:280-282` and implemented by `canPreferFork()` (`src/shared/fork-context.ts:89-105`).

```ts
export function resolveSubagentLaunchContext(input: SubagentLaunchContextInput): SubagentExecutionContext {
	if (input.explicitContext !== undefined) return input.explicitContext;
	const preferredContext = input.defaultSubagentContext ?? input.agentDefaultContext ?? "fresh";
	return preferredContext === "fork" && input.canUseImplicitFork ? "fork" : "fresh";
}
```

**Fresh child.** The child is a new pi `AgentSession` created on a new session file. Its message list does not contain parent turns. The parent supplies only the task prompt. The child's system prompt is built from its own `AgentConfig` (`systemPromptMode: replace` by default; `append` keeps pi's base prompt). The code path creates a new session manager and prompts it (`src/runs/shared/child-session.ts:600-629`), then the runner sends the first prompt (`src/runs/background/run-child-session.ts:728-729`):

```ts
messageBaseline = created.messages.length;
await created.prompt(input.prompt);
```

There is no code that injects the parent's message array into a fresh child. This is a genuine empty-context start, not a summarized one.

**Forked child.** A fork is a **real branch of the parent's persisted pi session**, not a textual transcript. `createForkContextResolver()` requires a parent session file and a current leaf (`src/shared/fork-context.ts:163-173`), then creates a branch:

```ts
const sourceManager = openSession(parentSessionFile, sessionDir);
const sessionFile = sourceManager.createBranchedSession(leafId);
```

(`src/shared/fork-context.ts:206-207`)

The child opens that branched JSONL file as its session file. It therefore inherits the parent's conversation messages and — because it is a real branch — the parent's system prompt. Tool declarations are the parent's declarations plus any child-only tools introduced at the fork point; pi keeps a parent tool the child lacks declared, and a child-only tool is declared when the fork begins (`docs/tool-reference.md:140-142`). The repo's docs state the system-prompt consequence (`docs/agents.md:392`):

> These fields and `systemPromptMode` shape a fresh child's system prompt. A forked child (`context: "fork"`) sends its parent's whole system prompt instead, project context, global context, and skills included, and gets its agent prompt in its task message.

There is also a `pruned` fork mode that rewrites the inherited session to fit a session budget, using a model to summarize overflow items (`docs/configuration.md:280-299`). The code for pruning is in `src/runs/foreground/subagent-executor.ts:7741-7756`; it is a real fork that is rewritten before the child opens it. Even a pruned fork keeps the parent's system prompt; it only reduces inherited content.

The important architectural point: **fork in nicobailon is a session-level copy, not a prompt-level concatenation.** Unmatched tool calls are not a concern in the normal case because the branch is a valid persisted session; the code additionally sanitizes unsafe Anthropic thinking blocks (`src/shared/fork-context.ts:110-159`).

### 2.2 `tintinweb/pi-subagents`: fresh session, optional textual transcript prefix

A fresh child is created by `createAgentSession()` with a fresh session manager. There is no inherited message array. The child's effective prompt is just the task.

`inherit_context` does **not** create a forked session. It builds a text representation of the parent branch and prepends it to the user prompt (`src/context.ts:15-57`):

```ts
export function buildParentContext(ctx: ExtensionContext): string {
  const entries = ctx.sessionManager.getBranch();
  if (!entries || entries.length === 0) return "";

  const parts: string[] = [];

  for (const entry of entries) {
    if (entry.type === "message") {
      const msg = entry.message;
      if (msg.role === "user") {
        const text = typeof msg.content === "string"
          ? msg.content
          : extractText(msg.content);
        if (text.trim()) parts.push(`[User]: ${text.trim()}`);
      } else if (msg.role === "assistant") {
        const text = extractText(msg.content);
        if (text.trim()) parts.push(`[Assistant]: ${text.trim()}`);
      }
      // Skip toolResult messages — too verbose for context
    } else if (entry.type === "compaction") {
      // Include compaction summaries — they're already condensed
      if (entry.summary) {
        parts.push(`[Summary]: ${entry.summary}`);
      }
    }
  }

  if (parts.length === 0) return "";

  return `# Parent Conversation Context
The following is the conversation history from the parent session that spawned you.
Use this context to understand what has been discussed and decided so far.

${parts.join("\n\n")}

---
# Your Task (below)
`;
}
```

And it is applied as a prefix to a single user prompt (`src/agent-runner.ts:1217-1224`):

```ts
// Build the effective prompt: optionally prepend parent context
let effectivePrompt = prompt;
if (options.inheritContext) {
  const parentContext = buildParentContext(ctx);
  if (parentContext) {
    effectivePrompt = parentContext + prompt;
  }
}
```

What is included:

- User text and assistant text from the current branch.
- Compaction summaries.
- **Not** tool results (`// Skip toolResult messages — too verbose for context`).
- **Not** assistant tool calls as structured message content; tool calls are not represented as tool-use blocks. Any text the assistant wrote around them is included.

What is not included by `inherit_context` itself:

- The parent's conversation as structured messages. The child's session contains one user message whose content happens to begin with a transcript. It is not a branch and cannot be resumed as the parent's branch.
- Any tool schema or tool declaration. The child's tools are resolved independently.

System-prompt inheritance is a **separate axis** controlled by `promptMode`. The default is `replace`, so the parent's system prompt is absent. If the agent definition sets `prompt_mode: append`, `buildAgentPrompt()` deliberately includes the parent system prompt alongside the child's own prompt (`src/prompts.ts:38-50`):

```ts
 * - "replace" mode: env header + config.systemPrompt (full control, no parent identity)
 * - "append" mode: parent system prompt + sub-agent context + env header + config.systemPrompt
 * - "append" with empty systemPrompt: pure parent clone
```

So `inherit_context` and `prompt_mode: append` are independent: the first copies a textual conversation summary into the user message; the second copies the parent's system prompt into the child's system prompt. A child can have either, both, or neither.

This is why the project's own type comment is slightly misleading: `inheritContext` is documented as "Default for spawn: fork parent conversation" (`src/types.ts:70`), and the README says "optionally fork the parent conversation into a sub-agent". The implementation is a **text rendering**, not a session fork. The code comment in `src/context.ts:16` is precise: "Build a text representation of the parent conversation context." This is a real README/code divergence and should be reported as such.

One more consequence: because the parent context is part of the first user message, it is inside the same message as the task. There is no machine-checkable boundary between "inherited context" and "task" except the literal separator string. A subagent can be prompt-injected by text in its own transcript, and the parent cannot later remove the inherited portion without rewriting the user message.

### 2.3 `championswimmer/pi-subagent-manager`: copied message snapshot from the lexical parent

This project inherits a filtered **message array**, not a session branch and not a text transcript.

`inheritContext()` (`src/orch/paths.ts:25-72`) clones the parent messages and removes:

- all `system` messages;
- assistant messages whose stop reason is `error` or `aborted`;
- assistant tool calls that do not have a matching later `toolResult`;
- `toolResult` messages that do not match a prior tool call.

The core of the filter is here:

```ts
/** Snapshot conversation, not the parent's prompt/loadout; retain only matched tool exchanges. */
export function inheritContext(messages: readonly AgentMessage[]): AgentMessage[] {
  const copy = structuredClone(messages);
  const matchedCalls = new Set<string>();
  const matchedResults = new Set<number>();
  const pending = new Map<string, { messageIndex: number; blockIndex: number }[]>();

  for (const [messageIndex, message] of copy.entries()) {
    if (message.role === "assistant") {
      if (message.stopReason === "error" || message.stopReason === "aborted") continue;
      for (const [blockIndex, block] of message.content.entries()) {
        if (block.type !== "toolCall") continue;
        const queue = pending.get(block.id);
        const occurrence = { messageIndex, blockIndex };
        if (queue) queue.push(occurrence);
        else pending.set(block.id, [occurrence]);
      }
      continue;
    }
    if (message.role !== "toolResult") continue;
    const queue = pending.get(message.toolCallId);
    const occurrence = queue?.shift();
    if (!occurrence) continue;
    matchedCalls.add(`${occurrence.messageIndex}:${occurrence.blockIndex}`);
    matchedResults.add(messageIndex);
    if (queue && !queue.length) pending.delete(message.toolCallId);
  }

  return copy.flatMap((message, messageIndex): AgentMessage[] => {
    if (message.role === "system") return [];
    if (message.role === "assistant") {
      if (message.stopReason === "error" || message.stopReason === "aborted") return [];
      message.content = message.content.filter(
        (block, blockIndex) =>
          block.type !== "toolCall" || matchedCalls.has(`${messageIndex}:${blockIndex}`),
      );
      if (!message.content.length) return [];
      if (
        message.stopReason === "toolUse" &&
        !message.content.some((block) => block.type === "toolCall")
      )
        message.stopReason = "stop";
    }
    if (message.role === "toolResult" && !matchedResults.has(messageIndex)) return [];
    return [message];
  });
}
```

The comment at the top is the design statement: "Snapshot conversation, not the parent's prompt/loadout; retain only matched tool exchanges." This solves the provider-pairing problem that a naive fork-by-copy would create: an unmatched `tool_use` or `tool_result` is invalid in a model request, so it is removed rather than left to fail at the provider. It also means the currently executing `agent_spawn` call — whose result does not exist yet — is not copied into the child. That is a subtle but important reason the filtered snapshot is safe to build during tool execution.

The manager builds the inherited array from the **lexical parent**, not the caller (`src/orch/manager.ts:334-339`):

```ts
const inherited =
  parent === "/root"
    ? inheritContext(this.options.rootSnapshot())
    : parentRecord?.driver
      ? inheritContext(parentRecord.driver.snapshot())
      : [];
```

For a child of `/root`, this is a snapshot of the main conversation. For a deeper child, it is a snapshot of the immediate parent's driver (`/root/a/b` inherits `/root/a`, not `/root`). Independent roots (`/k`) get no `/root` history. If the parent driver is not yet initialized, the context is marked pending and resolved later (`src/orch/manager.ts:969-990`).

The inherited messages are then physically appended to the new child session before the first prompt (`src/orch/runtime.ts:326-348`):

```ts
for (const message of options.inherited) {
  // Summary messages are projections, not appendable SDK session entries.
  if (message.role === "compactionSummary" || message.role === "branchSummary") {
    sessionManager.appendMessage({
      role: "user",
      content: message.summary,
      timestamp: message.timestamp,
    });
  } else if (message.role === "custom") {
    sessionManager.appendCustomMessageEntry(
      message.customType,
      message.content,
      message.display,
      message.details,
    );
  } else if (
    message.role === "user" ||
    message.role === "assistant" ||
    message.role === "toolResult" ||
    message.role === "bashExecution"
  ) {
    sessionManager.appendMessage(message);
  }
}
```

The system prompt is **not** inherited. The child gets its own type prompt plus a small thread-identity appendix (`src/orch/runtime.ts:528-541`):

```ts
systemPrompt: [
  options.type.systemPrompt,
  `Your thread path is ${options.path}.`,
  options.parentPath
    ? `Your lexical parent is ${options.parentPath}; inherited conversation comes only from that parent.`
    : "You are an independent root thread with no inherited history.",
  ...
].join("\n\n"),
```

So the child's final message list is:

1. its own system prompt (from the agent type, not the parent's);
2. a cloned, filtered transcript of the lexical parent's messages, including valid tool call/result pairs;
3. the new task as a user message.

The snapshot is frozen at spawn time: later parent turns do not change the child's inherited messages. This is stated in the project's docs, and matches the code (`rootSnapshot` or `driver.snapshot()` is called once).

### 2.4 Context comparison

| Aspect | `nicobailon/pi-subagents` | `tintinweb/pi-subagents` | `pi-subagent-manager` |
|---|---|---|---|
| Default | `fresh` | `fresh` | parent snapshot (unless independent root) |
| Fresh child messages | task only | task only | task only for an independent root |
| "Inherited" mechanism | real session branch / fork file | text transcript prepended to the first user prompt | cloned, filtered `AgentMessage[]` appended to child session |
| Parent system prompt inherited? | yes for fork; no for fresh (`append` keeps pi's base prompt) | no by default (`replace`); yes with `prompt_mode: append` | no |
| Parent tool calls/results inherited? | yes for fork (real branch) | no (tool results explicitly skipped) | only matched pairs; unmatched calls/results dropped |
| Compaction summaries | branch contains them | included as `[Summary]` text | inherited messages may contain them; runtime maps summary roles |
| Frozen at spawn? | branch snapshot | branch read at spawn; text fixed | cloned once; pending resolution if parent not ready |
| Can be resumed as a branch? | yes for fork | no; inherited text is just a user message | child is its own session; not the parent branch |

The takeaway for the course: "inherit context" is not one operation. Decide separately whether to copy **system prompt**, **messages**, and **tool definitions**. The projects answer these differently, and each combination has different isolation and validation consequences.

---

## 3. Tool scoping and recursion

### 3.1 `nicobailon/pi-subagents`

The per-agent declarations are `tools` (allowlist), `excludeTools` (denylist), `extensions` / `subagentOnlyExtensions` (which extension tools may load), and the nested-delegation grant (`allowNestedSubagents`, `allowedAgents`, `maxSubagentDepth`).

`tools` is an explicit allowlist when present. The launch planner does this (`src/runs/shared/child-tool-plan.ts:329-351`):

```ts
const ceilingFilteredBuiltinTools =
  input.tools === undefined
    ? allowedToolSet
      ? [...allowedToolSet].filter((tool) => tool !== "subagent_command")
      : []
    : (input.requireReadTool &&
      requestedBuiltinTools.length > 0 &&
      !requestedBuiltinTools.includes("read") &&
      !allowedToolSet
        ? ["read", ...requestedBuiltinTools]
        : requestedBuiltinTools
      ).filter((tool) => !allowedToolSet || allowedToolSet.has(tool));
const declaredBuiltinTools = ceilingFilteredBuiltinTools;
const excludeTools = [...new Set((input.excludeTools ?? []).map((tool) => tool.trim()).filter(Boolean))];
const excludedToolSet = new Set(excludeTools);
const effectiveDeclaredBuiltinTools = declaredBuiltinTools.filter((tool) => !excludedToolSet.has(tool));
const fanoutAuthorized = effectiveDeclaredBuiltinTools.includes("subagent") || (
  input.allowNestedSubagents === true &&
  !excludedToolSet.has("subagent") &&
  (!allowedToolSet || allowedToolSet.has("subagent"))
);
```

So:

- `tools` omitted → default pi builtins (and ambient extension tools, subject to exclusions) are allowed, but orchestration tools are stripped from ordinary children.
- `tools: [...]` → only those builtins plus the declared MCP/extension tools.
- `tools: []` → no builtins.
- `excludeTools` always removes names, even ones in `tools`.
- Nested delegation is authorized only by `tools: subagent` **or** `allowNestedSubagents: true`, and `allowedAgents` narrows the target set.

The child's active set is additionally filtered at runtime. `installExtensionToolScope()` computes an `inScope()` set and installs a `beforeToolCall` gate (`src/agent-runner.ts:264-334`):

```ts
const inScope = (): Set<string> => {
  const keep = new Set(toolNames.filter((t) => !disallowedSet?.has(t)));
  ...
  for (const name of EXCLUDED_TOOL_NAMES) keep.delete(name);
  for (const name of readmitToolNames) keep.add(name);
  return keep;
};
...
session.agent.beforeToolCall = async (context, signal) => {
  if (!inScope().has(context.toolCall.name)) {
    return {
      block: true,
      reason: `Tool "${context.toolCall.name}" is not available to this subagent.`,
    };
  }
  ...
};
```

This is a second line of defense: even if a tool is registered by an extension after the session starts, it is blocked unless the agent's scope admits it.

The recursion guard is a depth counter plus the grant. The default maximum is 2 (`src/shared/types.ts:2961`), and the check is (`src/shared/types.ts:3042-3046`):

```ts
export function checkSubagentDepth(configMaxDepth?: number, runtime?: SubagentDepthContext): { blocked: boolean; depth: number; maxDepth: number } {
	const depth = resolveCurrentSubagentDepth(runtime);
	const maxDepth = resolveCurrentMaxSubagentDepth(configMaxDepth, runtime);
	const blocked = depth >= maxDepth;
	return { blocked, depth, maxDepth };
}
```

At the cap, a child that still has a `subagent` tool gets a normal, well-formed tool error (`src/runs/foreground/subagent-executor.ts:7600-7614`):

```ts
const { blocked, depth, maxDepth } = checkSubagentDepth(deps.config.maxSubagentDepth, deps.childRuntime);
if (blocked) {
  return {
    content: [
      {
        type: "text",
        text:
          `Nested subagent call blocked (depth=${depth}, max=${maxDepth}). ` +
          "You are running at the maximum subagent nesting depth. " +
          "Complete your current task directly without delegating to further subagents.",
      },
    ],
    isError: true,
    details: { mode: "single" as const, results: [] },
  };
}
```

There is also a child boundary prompt that tells the model not to behave like the parent orchestrator (`src/runs/shared/subagent-prompt-runtime.ts:40-55`):

```ts
export const CHILD_SUBAGENT_BOUNDARY_INSTRUCTIONS = [
	"You are a child subagent, not the parent orchestrator.",
	"The parent session owns delegation, orchestration, review fanout, and follow-up worker launches.",
	"Ignore prior parent-only orchestration instructions in inherited conversation history.",
	"Do not propose or run subagents. Complete only your assigned role-specific task with the tools available to you.",
	...
];
```

This is defense in depth: the model is told not to delegate, the tool is absent unless granted, and the executor refuses at the depth cap.

### 3.2 `tintinweb/pi-subagents`

The per-agent declarations are `tools` (CSV), `disallowed_tools`, `extensions`, `exclude_extensions`, `isolated`, and `allowed_subagents`. The tool set is derived from pi's own factories, not a hardcoded list (`src/agent-types.ts:9-22`):

```ts
const READ_ONLY_TOOLS = ["read", "bash", "grep", "find", "ls"];

export const BUILTIN_TOOL_NAMES: string[] = [
  ...new Set([...createCodingTools("."), ...createReadOnlyTools(".")].map((t) => t.name)),
];
```

`tools` parsing is CSV, with `*` / `all` expanding to all builtins (`src/custom-agents.ts:355-372`):

```ts
function parseToolsField(val: unknown): { builtinToolNames: string[]; extSelectors: string[] | undefined } {
  const entries = csvList(val, BUILTIN_TOOL_NAMES);
  const isWildcard = (e: string) => e === "*" || e.toLowerCase() === "all";
  const hasWildcard = entries.some(isWildcard);
  const plain = entries.filter(e => !isWildcard(e) && !e.startsWith("ext:"));
  const extEntries = entries.filter(e => e.startsWith("ext:"));
  return {
    builtinToolNames: hasWildcard ? [...new Set([...BUILTIN_TOOL_NAMES, ...plain])] : plain,
    extSelectors: extEntries.length > 0 ? extEntries : undefined,
  };
}
```

`tools` omitted means all builtins; `tools: none` / empty means none. `disallowed_tools` is applied after the allowlist, and an explicit `ext:` selector flips extension tools to an allowlist (`src/custom-agents.ts:40-46`).

The runner builds the child's tool scope with a three-part rule (`src/agent-runner.ts:1004-1030`):

```ts
const disallowedSet = agentConfig?.disallowedTools
  ? new Set(agentConfig.disallowedTools)
  : undefined;

// Nested delegation tools (opt-in, ownership-scoped). Empty unless the agent
// set `allowed_subagents` and a nestedRuntime was provided — and never when
// isolated. ...
const effectiveMaxDepth = options.nestedRuntime?.maxSubagentDepth ?? getMaxSubagentDepth();
const nestedRuntime = options.nestedRuntime && options.nestedRuntime.depth < effectiveMaxDepth
  ? options.nestedRuntime
  : undefined;
const nestedTools = agentConfig?.allowedSubagents && nestedRuntime && !isolated
  ? createNestedSubagentTools({
      manager: nestedRuntime.manager,
      pi: options.pi,
      parentAgentId: nestedRuntime.parentAgentId,
      depth: nestedRuntime.depth,
      maxSubagentDepth: effectiveMaxDepth,
      allowedSubagents: agentConfig.allowedSubagents,
      configCwd,
    })
  : [];
```

Then the active set is rebuilt by `installExtensionToolScope()` with the extension allow/deny logic and a `beforeToolCall` gate (`src/agent-runner.ts:264-334`). The orchestration tool names are explicitly excluded (`src/agent-runner.ts:44-52`):

```ts
export const SUBAGENT_TOOL_NAMES = {
  AGENT: "Agent",
  WORKFLOW: "SubagentWorkflow",
  GET_RESULT: "get_subagent_result",
  STEER: "steer_subagent",
} as const;

/** Names of tools registered by this extension that subagents must NOT inherit. */
const EXCLUDED_TOOL_NAMES: string[] = Object.values(SUBAGENT_TOOL_NAMES);
```

Nested tools are re-admitted only when the agent opted in (`src/agent-runner.ts:1086-1117`). The nested tool itself enforces the depth cap and the allowlist (`src/nested-tools.ts:199-226`):

```ts
if (context.depth >= context.maxSubagentDepth) {
  return textResult(
    `Nested subagent call blocked (depth=${context.depth}, max=${context.maxSubagentDepth}). Complete the task directly.`,
    true,
  );
}
...
const resolvedType = resolveEnabledTypeIn(registry, rawType);
if (resolvedType === undefined) {
  return textResult(
    `Unknown or disabled nested agent type: "${rawType}". Allowed: ${availableIn(registry).join(", ") || "none"}.`,
    true,
  );
}
const allowed = allowedTypesIn(registry);
if (allowed !== undefined && !allowed.has(resolvedType)) {
  return textResult(
    `Nested agent type "${resolvedType}" is not allowed for this parent. Allowed: ${[...allowed].join(", ")}.`,
    true,
  );
}
```

The default max depth is 2, counted from the main session (`src/nested-tools.ts:42-48`):

```ts
/**
 * Hard ceiling on nesting for every branch: main session = 0, its subagents = 1,
 * their children = 2. `0`/`1` disables nesting entirely. Set from
 * `subagents.json` (`maxSubagentDepth`). Read when a subagent session is built,
 * so a change applies to sessions started after it.
 */
let maxSubagentDepth = 2;
```

At the cap, the nested tools are not injected at all (`nestedRuntime` becomes undefined); if a model somehow calls `Agent`, it is not in its active set and `beforeToolCall` blocks it. If the nested tool is present but the depth check trips, it returns a failed result string.

### 3.3 `championswimmer/pi-subagent-manager`

`AgentType.tools` is `{ allow?: string[]; block?: string[] }`. `selectTools()` applies it (`src/prefs/config.ts:563-584`):

```ts
export function selectTools(
  policy: AgentType["tools"],
  available: string[],
  mode: ToolFilteringMode = "allowed",
): string[] {
  const names = new Set(available);
  if (mode === "all") return [...names];
  // Ignored lists do not participate in validation or tool selection.
  const relevantPolicy =
    mode === "all-except-blocked"
      ? policy?.block === undefined
        ? {}
        : { block: policy.block }
      : policy;
  const validated = relevantPolicy === undefined ? {} : toolPolicy(relevantPolicy);
  for (const name of [...(validated.allow ?? []), ...(validated.block ?? [])]) {
    if (!names.has(name)) throw new Error(`Unavailable tool name: ${name}`);
  }
  const allow = mode === "all-except-blocked" ? names : new Set(validated.allow ?? []);
  const block = new Set(validated.block ?? []);
  return [...names].filter((name) => allow.has(name) && !block.has(name));
}
```

The default `toolFiltering` mode is `"allowed"` (`src/prefs/settings.ts:74-80`), and in that mode **an empty or missing `allow` list means no tools at all**. This is a notable contrast with tintinweb, where omitted `tools` means all builtins. The bundled agents therefore always declare an explicit `tools.allow` (for example `agents/coder.md:10-23`).

All child sessions receive the same verb set from `agentTools()` (`src/orch/tools.ts:22-189`), including `agent_spawn`, `agent_wait`, `agent_status`, `agent_steer`, `agent_pause`, `agent_stop`, and `agent_output`. The per-agent `tools.allow` decides which of those survive. `agent_spawn` is present only when the definition includes it; for example `agents/architect.md:13-25` includes it, while `agents/coder.md` does not.

The recursion guard is:

- **Depth / level:** default `maxLevels: 3` including L1 (`src/prefs/settings.ts:74-77`). `spawn()` computes the level from path segments and rejects over the limit (`src/orch/manager.ts:317-326`):

```ts
const segments = path.slice(1).split("/");
// Independent roots still represent agents launched by the main L1 conversation.
const level = segments.length + (segments[0] === "root" ? 0 : 1);
const maxLevels = this.options.maxLevels ?? DEFAULT_MANAGER_SETTINGS.maxLevels;
if (
  this.options.maxLevels === undefined && this.options.maxDepth !== undefined
    ? segments.length - 1 > this.options.maxDepth
    : level > maxLevels
)
  throw new Error("Agent depth limit reached");
```

- **Ownership / addressability:** a child may spawn only its immediate children, and may address only descendants (`src/orch/manager.ts:302-315`, `1048-1055`):

```ts
if (caller !== "/root" && parent !== caller)
  throw new Error("An agent may spawn only its immediate children");
```

```ts
private assertAccess(caller: string, path: string, self = false): void {
  caller = canonicalPath(caller);
  if (caller === "/root" && path !== "/root") return;
  if (self && path === caller) return;
  if (!isDescendant(path, caller))
    throw new Error(
      `${caller} may address only its descendants (not siblings, unrelated trees or ancestors)`,
    );
}
```

- **Parent liveness:** a non-root caller must be in a working state and not stopping (`src/orch/manager.ts:301-306`).

If a child ignores its tool policy and calls `agent_spawn` anyway, the call is either absent from its active tools (usual case) or throws from `spawn()`; pi turns a thrown tool `execute()` into a failed tool result. There is no separate "child cannot spawn" boolean: the guard is the depth/level check plus the per-type allowlist plus path ownership.

### 3.4 Tool scoping comparison

| Question | `nicobailon/pi-subagents` | `tintinweb/pi-subagents` | `pi-subagent-manager` |
|---|---|---|---|
| Allowlist | `tools` array, explicit if present | `tools` CSV, omitted = all builtins, `none` = none | `tools.allow`, default mode: empty = none |
| Denylist | `excludeTools` | `disallowed_tools` | `tools.block` |
| Extension tools | `extensions` / `subagentOnlyExtensions` / `ext:` selectors | `extensions`, `exclude_extensions`, `ext:` selectors, `isolated` | inherited root tools, filtered by allow/block |
| Runtime enforcement | `beforeToolCall` scope gate | `beforeToolCall` scope gate | active tool set only; spawn checks at manager |
| Nested grant | `tools: subagent` or `allowNestedSubagents` + `allowedAgents` | `allowed_subagents` | `agent_spawn` in `tools.allow` |
| Depth cap | 2 by default, counted from main 0 | 2 by default, counted from main 0 | `maxLevels` 3 including L1 |
| Child can spawn sibling? | no (paths / depth / grant) | no (nested tool is ownership-scoped) | no, `assertAccess` rejects |

---

## 4. Dispatch and concurrency

### 4.1 `nicobailon/pi-subagents`

The tool has two dispatch modes:

- **Foreground**: the tool call blocks until the child finishes. The child session is created in the parent pi process (`src/runs/shared/child-session.ts:1-8`).
- **Background**: `async` (default true, `src/extension/config.ts:253-254`) starts a detached runner process and returns a receipt. The runner writes status/result files and notifies the parent on completion.

The default is documented in the file header (`src/extension/index.ts:9`):

```ts
 * Toggle: async parameter (default: true; set asyncByDefault:false in config.json to opt out)
```

`src/extension/config.ts:253-254` is the implementation:

```ts
export function resolveAsyncByDefault(config: Pick<ExtensionConfig, "asyncByDefault">): boolean {
	return config.asyncByDefault !== false;
}
```

The child-session header makes the process boundary explicit (`src/runs/shared/child-session.ts:1-8`):

```ts
/**
 * In-process child sessions.
 *
 * A child is a pi `AgentSession` created inside the process that owns it: the
 * parent pi process for foreground children, the detached runner process for
 * background children. The factory is injectable so tests can script a child
 * without the real runtime; the default implementation wraps
 * `createAgentSession` from a pi package module.
 */
```

Foreground dispatch has an explicit one-at-a-time guard for blocking calls (`src/runs/foreground/subagent-executor.ts:8237-8246`):

```ts
const { depth } = checkSubagentDepth(deps.config.maxSubagentDepth, deps.childRuntime);
const dispatchParams = applyForceTopLevelAsyncOverride(requestParams, depth, deps.config.forceTopLevelAsync === true);
const runsForeground = dispatchParams.clarify === true || (dispatchParams.async ?? deps.asyncByDefault) !== true;
if (!runsForeground) return execute(id, requestParams, signal, onUpdate, ctx).then(withAggregatedToolUsage);
if (deps.state.subagentInProgress === true) return duplicateSubagentCallResult(requestParams);
deps.state.subagentInProgress = true;
```

The duplicate result is explicit (`src/runs/foreground/subagent-executor.ts:4656-4664`):

```ts
function duplicateSubagentCallResult(params: SubagentParamsLike): AgentToolResult<Details> {
	return {
		content: [{
			type: "text",
			text: "Rejected: a subagent call is already in progress. Issue exactly ONE subagent call per turn.",
		}],
		isError: true,
		details: { mode: inferExecutionMode(params), results: [] },
	};
}
```

That guard applies to **foreground** calls only. Background tool calls bypass it because `runsForeground` is false. In practice, the supported way to run parallel foreground children is a single `subagent` call with `tasks: [...]` or a workflow; pi may dispatch multiple separate tool calls with `Promise.all`, but this extension deliberately rejects a second blocking spawn in the same turn.

The parallel batch form is first-class. `tasks` is documented in the schema as "Parallel children; results in order" (`src/extension/schemas.ts:307`). The results array is indexed by launch order, and the index is part of the result contract (`src/shared/types.ts:1279-1305`):

```ts
export interface SingleResult {
	/**
	 * Stable child identity within the foreground run. Pair with Details.runId for
	 * cross-run correlation. This is assigned in launch order, remains stable across
	 * partial progress snapshots and the final result, and is independent of the
	 * result row's array position.
	 */
	index: number;
	// ...
	agent: string;
	task: string;
	/** Resolved launch context for this child. */
	context?: "fresh" | "fork";
	exitCode: number;
	// ...
	timedOut?: boolean;
	stopped?: boolean;
	turnBudget?: TurnBudgetState;
	turnBudgetExceeded?: boolean;
	toolBudget?: ToolBudgetState;
	toolBudgetBlocked?: boolean;
	messages?: Message[];
	usage: Usage;
	// ...
	error?: string;
	// ...
	finalOutput?: string;
	// ...
}
```

Background concurrency has an optional session-wide active-run cap (`maxActiveAsyncRunsPerSession`), default unlimited (`docs/configuration.md:488-504`). It reserves a slot before creating artifacts or starting children, and holds the slot through queued/running/paused terminal states. The implementation’s `resolveMaxActiveAsyncRunsPerSession()` turns `0` into "unlimited" (`src/runs/background/active-async-capacity.ts:97-100`):

```ts
export function resolveMaxActiveAsyncRunsPerSession(value: unknown): number | undefined {
	if (typeof value !== "number" || !Number.isInteger(value) || value < 0) return undefined;
	return value === 0 ? undefined : value;
}
```

There is also a cumulative spawn budget, default 64 logical child admissions per top-level run tree (`src/shared/types.ts:3068-3078`), and per-run workflow limits. These are guards against runaway recursion and cost, not just concurrency limits.

Timeouts and cancellation:

- `timeoutMs` for a foreground or plain single-agent async run is a run-level deadline; on timeout the child result is marked `timedOut`, the attempt gets a failure exit code, and partial output is preserved (`src/runs/foreground/execution.ts:1618-1626`). Composite async runs have no default top-level deadline (`docs/tool-reference.md:108`).
- Parent `AbortSignal` is forwarded into the child session; the abort path calls `session.abort()` and then a hard settle timer (`src/runs/foreground/execution.ts:1397-1412`).
- `bg_wait` has a separate report-only wait window: when it elapses, the tool returns a non-error `window_elapsed` result and the child keeps running (`docs/configuration.md:396`). This is an important distinction to preserve in a reimplementation: a child deadline and a parent wait deadline are different clocks.
- `interrupt` is distinct from `stop`: interrupt pauses the run and returns a result ("Run paused after interrupt..."), while stop settles the child as stopped.

### 4.2 `tintinweb/pi-subagents`

Background is the default (`src/index.ts:1318-1323`):

```ts
// What an unqualified top-level spawn means. Defaults to background,
// following Claude Code; `backgroundByDefault: false` restores the previous
// foreground default. Nested spawns ignore this — see nested-tools.ts.
let backgroundByDefault = true;
function getBackgroundByDefault(): boolean { return backgroundByDefault; }
function setBackgroundByDefault(b: boolean) { backgroundByDefault = b; }
```

There are two independent pools (`src/agent-manager.ts:1-15`):

```ts
/**
 * agent-manager.ts — Tracks agents, background execution, resume support.
 *
 * There are two independent concurrency pools, never one:
 *
 * - Background (`maxConcurrent`, default 10) bounds detached agents.
 * - Foreground (`maxConcurrentForeground`, default 0 = unlimited) bounds
 *   agents a caller is blocking on inline — `spawnAndWait`.
 *
 * Independent by design: a foreground agent blocks the parent anyway, so
 * charging it to the background pool would let a saturated pool starve the main
 * session of work it could have done itself. Excess agents in either pool are
 * queued and auto-started as slots free up. Nested children take no slot in
 * either — see `occupiesPoolSlot` / `occupiesForegroundSlot`.
 */
```

The queue is shared and drained by pool (`src/agent-manager.ts:408-422`):

```ts
private queue: { id: string; pool: Pool; start: () => Promise<void>; release: () => void }[] = [];
/** Number of currently running background agents. */
private runningBackground = 0;
/** Number of currently running foreground (blocking) agents. */
private runningForeground = 0;
```

`spawnAndWait()` is the blocking path (`src/agent-manager.ts:1082-1131`):

```ts
async spawnAndWait(
  pi: ExtensionAPI,
  ctx: ExtensionContext,
  type: SubagentType,
  prompt: string,
  options: Omit<SpawnOptions, "isBackground">,
  onSpawned?: (id: string) => void,
): Promise<{ id: string; record: AgentRecord }> {
  const id = this.spawn(pi, ctx, type, prompt, {
    ...options,
    isBackground: false,
    blocking: true,
    onSpawned,
  });
  const record = this.agents.get(id)!;

  // Queued: nothing to await yet — the promise appears when the drain starts
  // it. The gate resolves (never rejects) on every path out of the queue,
  // start and abort alike, so a rejection can never escape into the caller's
  // tool `execute` and take down pi's whole Promise.all tool batch.
  if (record.status === "queued") await record.startGate;

  // The run promise only exists once startup is past its awaited repo copy —
  // without this the call would return before the agent had started at all.
  await this.awaitStartup(id);

  // undefined when it was aborted while queued, or stopped mid-copy, and so
  // never ran — the record is already terminal with a completedAt, which is
  // what the caller renders.
  if (record.promise) await record.promise;

  // A record that ended "error" without ever getting a promise never ran ...
  if (record.promise === undefined && record.status === "error") {
    throw new Error(record.error ?? "Agent failed to start");
  }
  return { id, record };
}
```

If queued, it waits on `startGate`; if running, it waits on the run promise. The queue is drained when a slot frees (`src/agent-manager.ts:994-1014`), and `settleRun()` releases exactly the pool that was charged, then calls `drainQueue()`.

Parent cancellation is wired before/while the child runs (`src/agent-manager.ts:792-797`):

```ts
// Wire parent abort signal to stop the subagent when the parent is interrupted.
// A queued spawn can start minutes after the caller handed us its signal,
// by which time it may already be aborted — and `addEventListener` would
// never fire, leaving a child the parent can no longer reach.
if (options.signal?.aborted) this.abort(id);
else this.armParentAbort(record, options.signal);
```

That means a foreground child is stopped if the parent turn is cancelled. Background spawns deliberately omit the signal so a parent Esc does not kill detached work (the code comments in `src/index.ts:1467-1470` state this for background resumes; the same principle applies to background spawns). This is a deliberate semantic difference between "blocking child owned by this turn" and "detached child".

`max_turns` is a turn budget, not a wall-clock timeout. `observeRun()` steers once at the limit and hard-aborts after a grace period (`src/agent-runner.ts:662-680`):

```ts
run.unsubscribe = session.subscribe((event: AgentSessionEvent) => {
  if (event.type === "turn_end") {
    turnCount++;
    options.onTurnEnd(turnCount);
    if (options.maxTurns != null) {
      if (!run.steered && turnCount >= options.maxTurns) {
        run.steered = true;
        session.steer("You have reached your turn limit. Wrap up immediately — provide your final answer now.");
      } else if (run.steered && turnCount >= options.maxTurns + graceTurns) {
        run.aborted = true;
        session.abort();
      }
    }
  }
});
```

### 4.3 `championswimmer/pi-subagent-manager`

This is not a one-shot dispatcher; it is a thread manager with a small admission controller.

`spawn()` either backgrounds or waits, based on `wait` (`src/orch/manager.ts:368-371`):

```ts
record.async = args.wait === false;
this.start(record, args.task);
if (args.timeoutMs !== undefined) this.armTimeout(record, args.timeoutMs);
return args.wait === false ? this.view(record) : this.wait(caller, path, undefined, signal);
```

Admission limits:

- `maxConcurrent: 16` active threads (`src/prefs/settings.ts:74-77`);
- `maxThreads: 64` total retained threads;
- `maxLevels: 3` including L1.

Unlike tintinweb, there is **no queue** for concurrency. `spawn()` calls `assertCapacity()` and throws (`src/orch/manager.ts:327-329`, `1041-1046`):

```ts
if (this.records.size >= (this.options.maxThreads ?? DEFAULT_MANAGER_SETTINGS.maxThreads))
  throw new Error("Total thread limit reached");
this.assertCapacity();
```

```ts
private assertCapacity(): void {
  if (
    [...this.records.values()].filter((record) => active(record.view)).length >=
    (this.options.maxConcurrent ?? DEFAULT_MANAGER_SETTINGS.maxConcurrent)
  )
    throw new Error("Concurrent thread limit reached; wait for a thread to settle");
}
```

The caller gets a failed tool result; it must retry or wait. This is a deliberate "fail fast" policy, not a scheduler queue.

Timeouts are **report-only**. `armTimeout()` does not stop the child; it emits a `timeout` event with recent output (`src/orch/manager.ts:570-584`):

```ts
private armTimeout(record: Record, timeoutMs: number): void {
  this.clearTimeout(record);
  record.timeoutTimer = setTimeout(() => {
    record.timeoutTimer = undefined;
    if (this.disposed || !active(record.view) || record.stopRequested) return;
    this.emit({
      kind: "timeout",
      thread: this.view(record),
      recipient: record.view.parent ?? record.view.owner,
      timeoutMs,
      recentOutput: this.recentOutput(record),
    });
  }, timeoutMs);
  record.timeoutTimer.unref?.();
}
```

The tool description says this explicitly (`src/orch/tools.ts:95-99`): "If the child is still running after that many ms, a status event is sent to you; the child keeps running." A separate `agent_stop` performs cooperative cancellation (`src/orch/manager.ts:483-505`).

`wait()` is cancellable without stopping the child (`src/orch/manager.ts:423-454`):

```ts
async wait(
  caller: string,
  path: string,
  timeoutMs?: number,
  signal?: AbortSignal,
): Promise<ThreadView> {
  path = canonicalPath(path, caller);
  this.assertAccess(caller, path);
  const record = this.record(path);
  if (!active(record.view)) return this.view(record);
  // ...
  if (signal?.aborted) throw new Error("Waiting cancelled; thread remains running");
  await new Promise<void>((resolve, reject) => {
    // ...
    const cancel = () => finish(new Error("Waiting cancelled; thread remains running"));
    // ...
  });
  return this.view(record);
}
```

This is the cleanest expression of the distinction the chapter should teach: **waiting is not owning**. Cancelling a wait must not cancel the child.

### 4.4 Dispatch comparison

| Question | `nicobailon/pi-subagents` | `tintinweb/pi-subagents` | `pi-subagent-manager` |
|---|---|---|---|
| Default | background (`asyncByDefault` true) | background (`backgroundByDefault` true) | caller chooses `wait`; `agent_spawn` default waits |
| Foreground | blocking tool call; one blocking spawn per turn | `spawnAndWait`; multiple blocking calls allowed, optional foreground pool | `wait: true`; blocking wait on that thread |
| Background | detached runner process + result files | in-process manager record + completion notification | in-process thread + completion message |
| Concurrency queue | optional active async capacity; foreground guard | yes, two pools, queue and drain | no queue; reject at `maxConcurrent` / `maxThreads` |
| Foreground/background pools | separate concepts; foreground one-at-a-time | separate counts and queues | one active-thread count |
| Timeout | run-level deadline (terminal); `bg_wait` window is report-only | turn budget + abort; no wall-clock tool timeout in the core path | report-only event; child keeps running |
| Cancellation | parent signal aborts child; detached background survives parent turn | parent signal aborts foreground child; background survives | `agent_stop` cooperative; wait cancellation does not stop child |
| Nested children | no pool slot; depth cap | no pool slot; depth cap | same active-thread count; depth cap |

---

## 5. Result placement: how the child's output becomes visible

### 5.1 `nicobailon/pi-subagents`

**Foreground result.** `runSync()` returns a `SingleResult`; the executor wraps it in a tool result. The success path is (`src/runs/foreground/subagent-executor.ts:4637-4646`):

```ts
if (r.exitCode !== 0)
  return {
    content: [{ type: "text", text: `${formatFailedSingleRunOutput(r, finalizedOutput.displayOutput)}${worktreeSuffix}` }],
    details,
    isError: true,
  };
return {
  content: [{ type: "text", text: `${capInlineOutput(finalizedOutput.displayOutput, { artifactPath: r.artifactPaths?.outputPath, savedPath: finalizedOutput.savedPath }) || "(no output)"}${worktreeSuffix}` }],
  details,
};
```

`details` contains the structured envelope, including the `results` array. It is compacted before return: `messages` and `progress` are dropped from completed results, and tool-call summaries are retained (`src/runs/foreground/subagent-executor.ts:4578-4591`, `src/shared/utils.ts:413-432`):

```ts
const details = compactForegroundDetails(compactOptional<Details>({
  mode: "single",
  runId,
  timeoutMs: data.timeoutMs,
  results: [r],
  ...(effectiveToolBudget.toolBudget ? { toolBudget: effectiveToolBudget.toolBudget } : {}),
  progress: params.includeProgress ? allProgress : undefined,
  artifacts: allArtifactPaths.length ? { dir: artifactsDir, files: allArtifactPaths } : undefined,
  truncation: r.truncation,
  totalChildUsage: sumResultsUsage([r]),
  totalCost,
  usageBudget: usageBudgetState(data.usageBudget, totalCost),
  ...(worktreeHandoff?.reference ? { parallelHandoff: worktreeHandoff.reference } : {}),
}));
```

So the parent gets:

- `content[0].text`: the child's final text (or a failure headline), capped for inline display;
- `details.results[0]`: a compacted `SingleResult` (status fields, usage, session file, budget states, errors, artifacts). `compactForegroundDetails()` drops `messages` and `progress` and replaces them with tool-call summaries; the child transcript remains available through `sessionFile` / artifact paths (`src/shared/utils.ts:413-432`).
- `isError: true` on failure.

At the registered tool boundary, `finalizeToolResult()` converts a logical error into a thrown error (`src/extension/tool-result.ts:1-20`):

```ts
export function finalizeToolResult<T>(result: AgentToolResult<T>): AgentToolResult<T> {
	if (result.isError !== true) return result;

	const message = result.content
		.flatMap((item) => item.type === "text" && typeof item.text === "string" ? [item.text] : [])
		.join("\n")
		.trim();

	throw new Error(message || "pi-subagents reported a logical tool failure.");
}
```

This is an important distinction: internally the extension builds a structured `isError` result; at the pi boundary it throws, and pi produces a canonical failed `tool_result`. The parent still gets exactly one tool result for the call. The throw path carries the text message; whether the host preserves the internal `details` is host-defined, so the extension treats this as a deliberate tradeoff rather than a guarantee.

Cancellation and interruption paths are also explicit:

- Interrupted run (`src/runs/foreground/subagent-executor.ts:4630-4635`):

```ts
if (r.interrupted) {
  return {
    content: [{ type: "text", text: `Run paused after interrupt (${params.agent}). Waiting for explicit next action.${worktreeSuffix}` }],
    details,
  };
}
```

- Timeout: `r.exitCode !== 0`, so it returns an `isError` result whose text includes the timeout message and partial output.
- Budget exhaustion: `turnBudgetExceeded` / `toolBudgetBlocked` are fields on `SingleResult`; a terminal outcome can be `partial` with reason `budget_exhausted` (`src/runs/foreground/subagent-executor.ts:4770-4773`).
- Detached child: the immediate result is a receipt, not the final answer (`src/runs/foreground/subagent-executor.ts:4616-4627`):

```ts
if (r.detached) {
  const statusRecovery = `subagent({ action: "status", id: "${runId}" }) to recover the result; ...`;
  const blockingRecovery = `bg_wait({ id: "${runId}" }). Use ${statusRecovery}`;
  const message = ...
    : `Detached before task completion: ${params.agent}. Wait with ${blockingRecovery}`;
  return {
    content: [{ type: "text", text: `${message}${worktreeSuffix}` }],
    details,
  };
}
```

**Background result.** The initial `subagent` call returns a launched receipt. When the detached runner completes, the parent receives a custom message via `pi.sendMessage()` (`src/runs/background/notify.ts:673-713`):

```ts
function sendCompletion(pi: Pick<ParentWake, "sendMessage" | "isHeld">, items: PendingCompletion[], unstartedWakes: string[], unansweredCompletions: Map<string, { reminded: boolean }>, updateSettleSubscription: () => void, queuedNotices: Map<string, SubagentNotifyMessage>): boolean {
	if (items.length === 0) return true;
	const details = items.map((item) => item.details);
	const triggerTurn = items.some((item) => item.triggerTurn);
	const formatted = details.length === 1 ? formatSingleCompletion(details[0]!) : formatGroupedCompletion(details);
	const held = pi.isHeld();
	const content = triggerTurn ? `${formatted}\n\nParent action: ${held ? HELD_COMPLETION_ACTION : COMPLETION_ACTION}` : formatted;
	const display = details.some((detail) => detail.source === "foreground" || detail.status !== "completed" || detail.scheduleOrigin !== undefined);
	// A held notice starts no wake, so the silent-wake reminder must not later tell the model to resume the stopped task.
	const tracked = triggerTurn && !held;
	const message: SubagentNotifyMessage = {
		customType: "subagent-notify",
		content,
		display,
		// Pi does not send details to the model. Hosts match runs by id; the text previews stay in content only.
		details: { runs: details.map(notifyRunDetails) },
	};
	// Pi can queue an accepted wake behind the current turn or past agent_settled.
	// Recorded before sending in case Pi starts the message synchronously.
	if (tracked) {
		unstartedWakes.push(content);
		queuedNotices.set(content, message);
		unansweredCompletions.set(content, { reminded: false });
		updateSettleSubscription();
	}
	try {
		const appended = pi.sendMessage(message, { triggerTurn });
		// A notice only appended (held, or joining another extension's starting wake) gets no message_start.
		if (appended && tracked) unstartedWakes.splice(unstartedWakes.lastIndexOf(content), 1);
		if (tracked && !unstartedWakes.includes(content)) queuedNotices.delete(content);
		return true;
	} catch {
		if (tracked) {
			unstartedWakes.splice(unstartedWakes.lastIndexOf(content), 1);
			if (!unstartedWakes.includes(content)) queuedNotices.delete(content);
			unansweredCompletions.delete(content);
			updateSettleSubscription();
		}
		return false;
	}
}
```

The notification contains a result preview, status, and run metadata. Full output is retrieved through `subagent({ action: "status" })` / `bg_wait`, and the run's artifacts (`sessionFile`, output path) are in `details`. Crucially, the background notification is a **custom message**, not a second tool result for the original `subagent` call. The original tool call already got its receipt.

### 5.2 `tintinweb/pi-subagents`

**Foreground result.** The `Agent` tool returns a text result with stats and the child's final output (`src/index.ts:2486-2502`):

```ts
if (record.status === "error") {
  // Error headline + any partial output the run produced before failing.
  return textResult(`${fallbackNote}Agent failed: ${record.error}${partialOutputSuffix(record)}`, details);
}

const durationMs = (record.completedAt ?? Date.now()) - record.startedAt;
const statsParts = [`${record.toolUses} tool uses`];
if (tokenText) statsParts.push(tokenText);
if (showCost) {
  const costText = formatCost(getLifetimeCost(record.lifetimeUsage));
  if (costText) statsParts.push(costText);
}
return textResult(
  `${fallbackNote}Agent completed in ${formatMs(durationMs)} (${statsParts.join(", ")})${getForegroundOutcomeNote(record.status)}.\n\n` +
  (record.result?.trim() || "No output."),
  details,
);
```

Notice: the child failure path returns `textResult(...)` **without** `isError: true`. The failure is encoded in the text ("Agent failed: ...") and in `details.status`, not in the pi protocol error flag. This is a deliberate UX choice: the model reads a normal result that says the agent failed, rather than a tool-level error. For the course, this is a good example of why "result placement" and "error signaling" are separate decisions.

Non-normal terminal states get an explicit note (`src/status-note.ts:22-34`, `69-80`):

```ts
export function getStatusNote(status: string): string {
  switch (status) {
    case "stopped":
      return " (STOPPED BY THE USER before completion — output is partial; the task was NOT finished)";
    case "aborted":
      return " (aborted — hit the turn limit before completion; output may be incomplete)";
    case "steered":
      return " (wrapped up at the turn limit — output may be partial)";
    default:
      return "";
  }
}
```

```ts
export function getForegroundOutcomeNote(status: string): string {
  switch (status) {
    case "stopped":
      return " (STOPPED BY THE USER — everything the agent produced is above; the task is unfinished)";
    case "aborted":
      return " (aborted at the turn limit — everything the agent produced is above; the task is unfinished)";
    case "steered":
      return " (wrapped up at the turn limit — everything the agent produced is above; the task may be unfinished)";
    default:
      return "";
  }
}
```

**Background result.** The initial `Agent` call returns a receipt with the agent id (`src/index.ts:2237-2250`):

```ts
const backgroundResult = (id: string, record: AgentRecord | undefined) => {
  const isQueued = record?.status === "queued";
  return textResult(
    `${fallbackNote}Agent ${isQueued ? "queued" : "started"} in background.\n` +
    `Agent ID: ${id}\n` +
    `Type: ${displayName}\n` +
    `Description: ${params.description}\n` +
    (record?.outputFile ? `Output file: ${record.outputFile}\n` : "") +
    (isQueued ? `Position: queued (max ${manager.getMaxConcurrent()} concurrent)\n` : "") +
    `\nYou will be notified when this agent completes.\n` +
    `Use get_subagent_result to retrieve full results, or steer_subagent to send it messages.\n` +
    `Do not duplicate this agent's work.`,
    { ...detailBaseFor(record), toolUses: 0, tokens: "", durationMs: 0, status: "background" as const, agentId: id },
  );
};
```

Completion notifications go through an outbox and are sent as one message at a turn boundary (`src/completion-outbox.ts:1-20` and `src/index.ts:464-477`):

```ts
const outbox = new CompletionOutbox(
  (message) => pi.sendMessage<NotificationDetails>({
    customType: "subagent-notification",
    content: message.content,
    display: true,
    details: message.details,
  }, { deliverAs: "steer", triggerTurn: true }),
  () => currentCtx?.signal !== undefined,
);
```

`get_subagent_result` returns the full result and cancels the pending notification (`src/index.ts:2996-3037`):

```ts
if (params.wait && (record.status === "running" || record.status === "queued")) {
  while (record.status === "queued") {
    await abortable(
      new Promise<void>((resolve) => setTimeout(resolve, QUEUE_WAIT_POLL_MS)),
      signal,
    );
  }
  if (record.promise) await abortable(record.promise, signal);
}
// ...
if (record.status === "running") {
  output += "Agent is still running. Use wait: true or check back later.";
} else if (record.status === "error") {
  output += `Error: ${record.error}${partialOutputSuffix(record)}`;
} else {
  output += record.result?.trim() || "No output.";
}

// Mark result as consumed — suppresses the completion notification
if (record.status !== "running" && record.status !== "queued") {
  record.resultConsumed = true;
  outbox.cancel(record.id);
}
```

So the parent has two valid ways to see a background result: wait for the notification and then fetch the full result, or call `get_subagent_result` directly. Reading the result consumes the notification, preventing duplicates.

### 5.3 `championswimmer/pi-subagent-manager`

**Synchronous spawn.** The tool returns the `ThreadView` as JSON (`src/orch/tools.ts:102`):

```ts
async (params, signal) => result(compact(await threads().spawn(params, signal))),
```

`result()` is (`src/orch/tools.ts:14-19`):

```ts
function result(value: unknown) {
  return {
    content: [{ type: "text" as const, text: JSON.stringify(value, null, 2) }],
    details: value,
  };
}
```

`compact()` truncates `output` to 16,000 characters (`src/orch/tools.ts:10-13`). The parent therefore sees a JSON object with `path`, `state`, `status`, `output`, `error`, usage, and `sessionFile` — not a plain final-answer string. `agent_wait` returns the same shape after the thread settles. `agent_output` pages through the output separately (`src/orch/tools.ts:169-187`).

**Failure.** A failed child is a normal JSON result with `state: "failed"` and `error` set; it is not a thrown tool error. The tool result remains well-formed. An invalid spawn (bad path, depth limit, concurrency limit) throws and becomes a failed tool result, because `spawn()` throws before any child exists.

**Cancellation.** `agent_stop` returns a JSON view with `state: "stopped"` and no output (`src/orch/manager.ts:483-505`). Cancelling `agent_wait` rejects that tool call with "Waiting cancelled; thread remains running"; the child is unaffected.

**Background.** `agent_spawn wait:false` returns immediately with a `ThreadView` in state `starting`/`running`. Completion, failure, stop, and timeout are delivered as messages, not tool results. For a root recipient, the message is persisted in the root mailbox and delivered when the parent is idle (`src/index.ts:477-505`):

```ts
if (event.recipient === "/root") {
  const recap = ...
  const notification: RootNotification = {
    rootSessionId: requireContext().sessionManager.getSessionId(),
    content: message,
    finalRecap: recap,
    trigger:
      !suspendingSummaries &&
      (event.kind === "timeout" || (event.kind === "settled" && event.async && !recap)),
    details: {
      mailboxId: randomUUID(),
      path: thread.path,
      state: thread.state,
    },
  };
  // Persist first; defer delivery until the root is idle and the plugin is enabled.
  pi.appendEntry(ROOT_MAILBOX_ENTRY, notification);
  restoreRootMailbox(requireContext());
} else {
  void requireManager().deliver(event.recipient, message).catch(warnDelivery);
  ...
}
```

For a non-root parent, the notification is injected into that parent's retained session via `driver.sendUpdate()` (`src/orch/manager.ts:607-610`). This is the "message event" model, not the "tool result" model.

### 5.4 Result placement comparison

| Aspect | `nicobailon/pi-subagents` | `tintinweb/pi-subagents` | `pi-subagent-manager` |
|---|---|---|---|
| Foreground payload | final text + `details.results[]` | final text + stats + `details` | JSON `ThreadView` (incl. output) |
| Failure signaling | `isError` + throw at tool boundary | text status, not `isError` | `state: "failed"` + `error`, not thrown |
| Cancel signaling | interrupted result text / stopped status | status note in text | `state: "stopped"` view |
| Background receipt | launched receipt | agent id + "you will be notified" | thread view |
| Background completion | custom `subagent-notify` message | custom `subagent-notification` message | root mailbox / parent `sendUpdate` message |
| Full output retrieval | `status`/`bg_wait` + artifacts | `get_subagent_result` | `agent_output` / `agent_wait` |
| Parent tool pairing | one result per foreground call; background receipt already paired | same | same |

The key invariant all three preserve: **the original parent `tool_use` is answered exactly once.** Later background notifications are not `tool_result`s for that call. None of the projects inject a late child result into the parent transcript as a second result for the same tool call.

---

## 6. Manager state: `pi-subagent-manager` in detail

`pi-subagent-manager` is the outlier: it is not primarily a one-shot child dispatcher. It is a **persistent hierarchical thread manager with a UI and a wake scheduler**. The answer to "scheduler, UI, or both?" is **both**, but "scheduler" here means turn delivery / wake scheduling, not cron. There is no recurring-job scheduler in this repo.

### 6.1 Runtime state

`ThreadManager` holds an in-memory `Map<string, Record>` keyed by canonical path (`src/orch/manager.ts:93-110`). A `Record` contains (`src/orch/manager.ts:22-50`):

```ts
interface Record {
  view: ThreadView;
  definition: AgentType;
  inherited: AgentMessage[];
  contextReady?: Promise<void>;
  contextPending?: boolean;
  startup: AbortController;
  driver?: AgentDriver;
  initializing?: Promise<AgentDriver>;
  run?: Promise<void>;
  started?: Promise<void>;
  sessionLeafId?: string | null;
  /** Last authoritative cumulative usage applied from the current driver. */
  usageCursor?: { input: number; output: number; costUsd?: number };
  // ...
  /** Detached (spawned with wait:false or resumed): completion is reported as an event. */
  async?: boolean;
  timeoutTimer?: ReturnType<typeof setTimeout>;
  pauseRequested: boolean;
  stopRequested: boolean;
  observationRevision?: number;
  driverGeneration?: number;
  transcriptSource?: TranscriptSnapshot;
  transcriptListeners?: Set<TranscriptListener>;
  detachTranscript?: () => void;
  observationError?: string;
  /** Increments on every start/resume; guards summaries against stale runs. */
  runId?: number;
  /** Bounded recent activity lines for the optional housekeeping summarizer. */
  recentActivity?: string[];
}
```

The public `ThreadView` is a durable-ish projection with `path`, `parent`, `owner`, `type`, `state`, `task`, `status`, `output`, `error`, `sessionFile`, timing, token/cost totals, and an optional one-line housekeeping summary (`src/types.ts:50-98`).

### 6.2 State machine

`ThreadState` is (`src/types.ts:44`):

```ts
export type ThreadState = "starting" | "running" | "paused" | "completed" | "failed" | "stopped";
```

The normal transition is implemented in `start()` (`src/orch/manager.ts:635-702`):

```ts
private start(record: Record, message: string): void {
  this.freezeElapsed(record);
  record.pauseRequested = false;
  record.stopRequested = false;
  record.startup = new AbortController();
  record.view.state = "starting";
  record.view.status = "Starting";
  // ...
  record.run = Promise.resolve().then(async () => {
    try {
      const contextReady = this.prepareInheritedContext(record);
      if (contextReady) await abortable(contextReady, record.startup.signal);
      record.startup.signal.throwIfAborted();
      const driver = await this.ensureDriver(record);
      if (record.stopRequested || this.disposed) {
        record.view.state = "stopped";
        return;
      }
      const task = driver.prompt(message);
      record.view.state = "running";
      record.view.status = "Working";
      this.touch(record);
      markStarted();
      await task;
      record.view.state = record.stopRequested
        ? "stopped"
        : record.pauseRequested
          ? "paused"
          : "completed";
      if (record.view.state === "completed") {
        record.view.output = driver.output();
        record.view.status = "Completed; session retained";
      } else if (record.view.state === "stopped")
        record.view.status = "Stopped; session retained";
    } catch (error) {
      record.view.state = record.stopRequested ? "stopped" : "failed";
      record.view.error = errorText(error);
      record.view.status = record.stopRequested ? "Stopped; session retained" : record.view.error;
    } finally {
      this.clearTimeout(record);
      this.freezeElapsed(record);
      // ...
      markStarted();
      this.touch(record);
      if (!this.disposed && epoch === this.epoch)
        this.emit({
          kind: "settled",
          thread: this.view(record),
          recipient: record.view.parent ?? record.view.owner,
          async: record.async === true,
        });
    }
  });
  this.touch(record);
}
```

Transitions:

| From | Event | To | Notes |
|---|---|---|---|
| `starting` | driver ready, prompt accepted | `running` | `started` promise resolves |
| `running` | prompt resolves, no stop/pause requested | `completed` | output retained; session retained |
| `running` | `pause()` called | `paused` (after settle) | no final answer; session retained |
| `running` | `stop()` called | `stopped` | abort driver, await run, clear output |
| `starting`/`running` | driver/prompt throws | `failed` (or `stopped` if stop requested) | error stored in view |
| `paused`/`completed`/`failed`/`stopped` | `agent_steer` with message | `starting` → `running` | same session, same path, fresh `runId` |

`pause()` does not immediately change state; it sets `pauseRequested` and waits for the SDK to settle (`src/orch/manager.ts:473-482`). `stop()` is cooperative: it aborts the driver and awaits the run, then forces `state = "stopped"` (`src/orch/manager.ts:483-505`).

### 6.3 Persistence

There are two persistence layers.

**Child transcripts.** Each child is a normal pi JSONL session, usually in the main session directory, with a `parentSession` header. `createDriverFactory()` opens or creates the session file (`src/orch/runtime.ts:300-348`). `ThreadView.sessionFile` exposes it. Resuming a child reopens the same session; the registry stores `sessionLeafId` so a branch can be restored.

**Registry.** The parent session stores a custom entry, `pi-subagent:registry:v1`, containing `SavedThread[]` (`src/index.ts:278-289`):

```ts
const persist = () => {
  if (!manager) return;
  const threads = manager.saved();
  const signature = JSON.stringify(threads);
  if (signature !== persistenceSignature) {
    pi.appendEntry(REGISTRY_ENTRY, {
      version: 1,
      rootSessionId: requireContext().sessionManager.getSessionId(),
      threads,
    });
    persistenceSignature = signature;
  }
};
```

`ThreadManager.saved()` writes an explicit durable schema (`src/orch/manager.ts:200-238`): `view`, `definition`, and `inherited` only if there is no session file yet. On restore, active threads are converted to paused (`src/orch/manager.ts:240-291`):

```ts
if (active(view)) {
  view.state = "paused";
  view.status = "Interrupted by reload; send input to resume";
  delete view.output;
}
```

The parent restores the registry only when the `rootSessionId` matches its own session (`src/index.ts:674-690`):

```ts
const entries = ctx.sessionManager.getBranch();
const entry = [...entries]
  .reverse()
  .find((item) => item.type === "custom" && item.customType === REGISTRY_ENTRY);
if (entry?.type === "custom") {
  try {
    const data = entry.data as {
      version: number;
      rootSessionId: string;
      threads: SavedThread[];
    };
    if (data.version !== 1 || !Array.isArray(data.threads))
      throw new Error("Invalid registry format");
    // Forked parents own a new registry; they must not share writable child transcripts.
    if (data.rootSessionId === ctx.sessionManager.getSessionId())
      instance.restore(data.threads);
    else persist();
  } catch (error) {
    // ...
  }
}
```

This is a deliberate anti-corruption rule: a forked main session must not overwrite the original parent's child registry.

### 6.4 Root mailbox and wake scheduler

Completion/timeout/progress events are delivered through a mailbox, not directly through tool results. For `/root`, the notification is appended to the parent session as `pi-subagent:root-mailbox:v1` before delivery (`src/index.ts:477-505`). The mailbox entry carries a `mailboxId`, path, state, and content. Delivery is deferred while the root is busy; the code has a `rootTurnRunning` reservation and re-evaluates pending work at boundaries.

The UI's own docs describe the scheduler accurately (`docs/async-lifecycle.md`), and the code matches:

- Completion and reporting timeouts are separate signals.
- One automatic turn at a time; results arriving during a turn remain pending for a follow-up.
- A timeout wake does not stop the child.
- Session changes cancel scheduled work and change the generation; callbacks recheck generation.
- Persisted mailbox entries allow branch-local recovery after reload.

The scheduler is **not** a cron scheduler. It does not run jobs at future wall-clock times; it chooses when to deliver already-produced events and whether to trigger a parent turn. The repository's `schedule.ts` (in tintinweb, not this repo) is a different feature.

### 6.5 Multiple children in the UI

Children are rendered as a tree, keyed by path. `ThreadView.path` is the identity; `parent` gives the lexical parent. `src/ui/thread-tree.ts` builds status rows and sorts branches by most recent start. `src/ui/ui.ts:431-475` renders either a full tree or a compact summary into the above-editor widget. `src/ui/agent-navigation-editor.ts` installs arrow-key navigation from the prompt into the tree. `src/ui/live-agent-view.ts` is the per-thread conversation overlay.

The widget is not a log of results; it is a live process view:

```ts
const render = mode === "minimal" || settled ? renderAgentSummary : renderAgentTree;
```

(`src/ui/ui.ts:446-448`)

A settled tree remains visible while the main turn or any subagent is active; otherwise it collapses. This is a very different UI model from a one-shot tool result card.

### 6.6 Manager-state comparison

| Aspect | `nicobailon/pi-subagents` | `tintinweb/pi-subagents` | `pi-subagent-manager` |
|---|---|---|---|
| Primary state | run records + async job tracker + result files | `AgentManager` records + pools + outbox | `ThreadManager` records + paths + registry + mailbox |
| Child identity | run id, child index | agent id, handle/alias | canonical path |
| Persistence | child session files, async dirs, result files | child session files, `subagents:record` entries | child session files + parent registry entry + root mailbox |
| Resume | yes, from stored session/run artifacts | yes, from stored session | yes, same path and session; state paused after reload |
| UI | fleet widget, inspector, markdown renderer | above-editor widget, fleet list, conversation viewer | tree widget, navigation editor, live view, cost footer |
| Scheduler | background completion notifications / wait | completion outbox at turn boundary | root mailbox + wake reservation + final recap batching |

---

## 7. Divergence: the two projects named `pi-subagents`

### 7.1 Identity

They are not forks. They have different npm package names (`pi-subagents` vs `@tintinweb/pi-subagents`), different authors, different directory layouts, different tool names (`subagent` vs `Agent`), and different agent config schemas. The only shared thing is the pi extension API and the general idea of delegation.

### 7.2 Concrete differences

| Dimension | `nicobailon/pi-subagents` | `tintinweb/pi-subagents` |
|---|---|---|
| Tool name | `subagent` | `Agent` |
| Default context | fresh | fresh |
| "Inherit" implementation | real session branch/fork | text transcript prefix to the first user prompt |
| Parent system prompt on inherit | inherited for fork | not by `inherit_context`; `replace` default, `append` can inherit parent prompt |
| Parent tool results on inherit | included in the branch | explicitly skipped |
| Tool input | `agent` + `task`, or `tasks[]`, `chain[]`, workflow | `prompt` + `subagent_type`, plus many options |
| Default execution | background (`asyncByDefault` true) | background (`backgroundByDefault` true) |
| Foreground concurrency | one blocking `subagent` call per turn | multiple blocking calls; optional foreground pool |
| Background execution | detached runner process + result files | in-process manager record + outbox notification |
| Nested delegation | `allowNestedSubagents` / `tools: subagent` + `allowedAgents`; depth 2 | `allowed_subagents`; depth 2; child-safe `Agent`/result/steer tools |
| Tool scope enforcement | live `beforeToolCall` gate + plan | live `beforeToolCall` gate + plan |
| Failure result | `isError` then throw at boundary | text status ("Agent failed") without `isError` |
| Orchestration extras | chains, workflows, missions, watchdogs, external CLI adapters, worktrees | workflows, scheduler, fleet UI, worktrees, Claude compatibility |
| Code size | ~103k nonblank source lines | ~21k nonblank source lines |
| Docs/code mismatch | generally close | README says "fork" for `inherit_context`; code is textual |
| Best reference for | isolation and production failure envelopes | minimal child-loop shape and explicit context handoff |

### 7.3 Which is the better reference?

For **this Go course**, `tintinweb/pi-subagents` is the better primary reference, for four reasons:

1. Its context constructor is one readable function (`buildParentContext`) with a clear policy: text only and no tool results. System-prompt inheritance is a separate, explicit `prompt_mode` choice, which is a useful separation to teach. A student can reproduce the text builder in Go in ~20 lines.
2. Its core dispatch is a single `Agent` tool with a foreground branch (`spawnAndWait`) and a background branch (`spawn` + notification). The result placement is easy to see: final text in the foreground result, receipt + notification for background.
3. Its failure/terminal-state vocabulary (`completed`, `error`, `aborted`, `stopped`, `steered`) is small and is surfaced in text, which is easy to grade with a scripted provider.
4. It is ~5x smaller than `nicobailon/pi-subagents`, and its extra features are separable from the core.

`nicobailon/pi-subagents` is the better **advanced** reference: if the chapter wants to teach true context isolation, it is the only one of the two that actually forks a session. Its `SingleResult` envelope, budget fields, and cancellation/timeout handling are also excellent source material for "what a production envelope needs." But copying its structure into a minimal course would drown the child loop in workflow, mission, inspector, external-runner, and worktree machinery.

Recommended teaching sequence:

1. Build the fresh-child loop from the `tintinweb` shape.
2. Add the `SingleResult`-style envelope from `nicobailon`.
3. Add true fork only as an advanced box: branch a persisted session, and explain why a text prefix is not a fork.
4. Add the `pi-subagent-manager` path/state model only when teaching long-lived, steerable children; it is a different product.

---

## 8. Minimal teaching model in Go

The goal is the smallest design that is still honest:

- a real child agent loop, with its own message list and tool loop;
- isolated context by default;
- an explicit tool allowlist enforced at execution time;
- a result envelope that distinguishes success, failure, cancellation, timeout, and budget exhaustion;
- ordered, non-interleaved results for concurrent children;
- one parent tool result per parent tool call.

What is deliberately dropped is listed after the code.

### 8.1 Types

```go
package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// AgentSpec is a validated, immutable agent definition.
// The slices are treated as owned by the registry and are cloned on lookup.
type AgentSpec struct {
	Name         string
	Description  string
	SystemPrompt string
	Model        string
	Tools        []string // exact allowlist; empty means "no tools"
	MaxTurns     int      // 0 means a small hard default, never unlimited
}

type Registry struct {
	byName map[string]AgentSpec
}

func NewRegistry(specs ...AgentSpec) (*Registry, error) {
	r := &Registry{byName: make(map[string]AgentSpec, len(specs))}
	for _, s := range specs {
		if s.Name == "" {
			return nil, errors.New("agent name is required")
		}
		if _, exists := r.byName[s.Name]; exists {
			return nil, fmt.Errorf("duplicate agent %q", s.Name)
		}
		s.Tools = slices.Clone(s.Tools)
		r.byName[s.Name] = s
	}
	return r, nil
}

// Lookup returns a value copy. A child can never mutate the registry.
func (r *Registry) Lookup(name string) (AgentSpec, bool) {
	s, ok := r.byName[name]
	if !ok {
		return AgentSpec{}, false
	}
	s.Tools = slices.Clone(s.Tools)
	return s, true
}

type Role string

const (
	RoleSystem Role = "system"
	RoleUser   Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool   Role = "tool"
)

type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
	ToolCallID string     `json:"toolCallId,omitempty"` // set only on RoleTool
}

type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolResult struct {
	ToolCallID string `json:"toolCallId"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	IsError    bool   `json:"isError,omitempty"`
}

// Tool is a minimal pi-like tool. The child loop only ever sees the
// restricted Toolbox it was given, never the parent's full registry.
type Tool interface {
	Name() string
	Spec() ToolSpec
	Run(ctx context.Context, call ToolCall) ToolResult
}

type Toolbox struct {
	byName map[string]Tool
	order  []string
}

func NewToolbox(tools ...Tool) *Toolbox {
	b := &Toolbox{byName: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		if _, exists := b.byName[t.Name()]; exists {
			panic("duplicate tool " + t.Name())
		}
		b.byName[t.Name()] = t
		b.order = append(b.order, t.Name())
	}
	return b
}

// Restrict returns a new toolbox containing exactly the named tools.
// Unknown names are an error, not a silent omission: a typo in an agent
// definition must fail the spawn, not quietly widen or narrow capability.
func (b *Toolbox) Restrict(names []string) (*Toolbox, error) {
	out := &Toolbox{byName: make(map[string]Tool, len(names))}
	for _, name := range names {
		t, ok := b.byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool %q", name)
		}
		out.byName[name] = t
		out.order = append(out.order, name)
	}
	return out, nil
}

func (b *Toolbox) Get(name string) (Tool, bool) {
	t, ok := b.byName[name]
	return t, ok
}

func (b *Toolbox) Specs() []ToolSpec {
	out := make([]ToolSpec, 0, len(b.order))
	for _, name := range b.order {
		out = append(out, b.byName[name].Spec())
	}
	return out
}

type ProviderRequest struct {
	Model    string
	System   string
	Messages []Message
	Tools    []ToolSpec
}

type ProviderResponse struct {
	Text      string
	ToolCalls []ToolCall
}

// Provider is the only model boundary. A scripted provider is a valid
// implementation for offline grading.
type Provider interface {
	Complete(ctx context.Context, req ProviderRequest) (ProviderResponse, error)
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	Turns        int
}

type Status string

const (
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
	StatusTimeout   Status = "timeout"
	StatusBudget    Status = "budget_exhausted"
)

// ChildResult is the durable contract. A failed child still returns a result.
type ChildResult struct {
	Index    int
	Agent    string
	Status   Status
	Output   string // final text, or partial text on failure/cancel
	Error    string // populated for failed/timeout/budget; empty for completed
	Turns    int
	Usage    Usage
	ToolUses int
}

type Envelope struct {
	Results []ChildResult
	Started time.Time
	Ended   time.Time
}
```

### 8.2 Context builder

The minimal model supports **fresh** context and an explicit **bounded handoff**. It deliberately does not support a true fork. The absence of a `ParentMessages` field is the design: a fresh child cannot accidentally inherit history because the type has nowhere to put it.

```go
type ContextMode int

const (
	Fresh ContextMode = iota
	Handoff
)

// ParentView contains only what the caller explicitly chose to hand over.
// It intentionally has no []Message field: "fork the whole transcript" is
// a separate, advanced feature, not a default.
type ParentView struct {
	Summary string // bounded, caller-authored; may be empty
}

type ChildContext struct {
	System   string
	Messages []Message
}

func BuildChildContext(spec AgentSpec, parent ParentView, mode ContextMode, task string) ChildContext {
	user := task
	if mode == Handoff && parent.Summary != "" {
		user = "Parent handoff (reference only; not a live thread):\n\n" +
			parent.Summary + "\n\n---\n\nTask:\n" + task
	}
	return ChildContext{
		System: spec.SystemPrompt,
		Messages: []Message{{
			Role:    RoleUser,
			Content: user,
		}},
	}
}
```

If the course later adds a real fork, the safe rule is: clone the parent messages, drop every unmatched tool call/result, and never share the slice. The `pi-subagent-manager` implementation above is a compact example of that filter.

### 8.3 Child loop

```go
func (d *Dispatcher) runChild(
	ctx context.Context,
	spec AgentSpec,
	parent ParentView,
	mode ContextMode,
	task string,
	depth int,
) (result ChildResult) {
	result.Agent = spec.Name
	result.Status = StatusFailed

	// A panic in a child must become a failed child result, not a dead process.
	defer func() {
		if r := recover(); r != nil {
			result.Status = StatusFailed
			result.Error = fmt.Sprintf("panic: %v", r)
		}
	}()

	tools, err := d.Tools.Restrict(spec.Tools)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	child := BuildChildContext(spec, parent, mode, task)
	messages := child.Messages

	maxTurns := spec.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 8
	}

	for turn := 0; turn < maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			result.Status = statusFromContext(ctx)
			result.Output = lastAssistantText(messages)
			result.Error = err.Error()
			result.Turns = turn
			return result
		}

		resp, err := d.Provider.Complete(ctx, ProviderRequest{
			Model:    spec.Model,
			System:   child.System,
			Messages: messages,
			Tools:    tools.Specs(),
		})
		if err != nil {
			result.Status = statusFromContext(ctx)
			result.Error = err.Error()
			result.Output = lastAssistantText(messages)
			result.Turns = turn + 1
			return result
		}

		result.Usage.Turns++
		result.Usage.OutputTokens += len(resp.Text) // token accounting is provider-specific

		assistant := Message{Role: RoleAssistant, Content: resp.Text, ToolCalls: resp.ToolCalls}
		messages = append(messages, assistant)

		if len(resp.ToolCalls) == 0 {
			result.Status = StatusCompleted
			result.Output = resp.Text
			result.Turns = turn + 1
			return result
		}

		for _, call := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				// The assistant message above already contains this call.
				// A terminal result must still account for it, so append an
				// explicit cancellation result message.
				messages = append(messages, Message{
					Role:       RoleTool,
					ToolCallID: call.ID,
					Content:    "canceled before execution",
				})
				result.Status = statusFromContext(ctx)
				result.Error = err.Error()
				result.Output = lastAssistantText(messages)
				result.Turns = turn + 1
				return result
			}

			tool, ok := tools.Get(call.Name)
			if !ok {
				// Refusal is a tool result, not a panic and not a silent skip.
				tr := ToolResult{
					ToolCallID: call.ID,
					Name:       call.Name,
					Content:    fmt.Sprintf("tool %q is not allowed for agent %q", call.Name, spec.Name),
					IsError:    true,
				}
				messages = append(messages, Message{
					Role: RoleTool, ToolCallID: call.ID,
					Content: tr.Content,
				})
				result.ToolUses++
				continue
			}

			tr := tool.Run(ctx, call)
			if tr.ToolCallID == "" {
				tr.ToolCallID = call.ID
			}
			result.ToolUses++
			messages = append(messages, Message{
				Role: RoleTool, ToolCallID: call.ID,
				Content: contentOrError(tr),
			})
		}
	}

	result.Status = StatusBudget
	result.Error = fmt.Sprintf("turn budget exhausted after %d turns", maxTurns)
	result.Output = lastAssistantText(messages)
	result.Turns = maxTurns
	return result
}
```

`statusFromContext` maps `context.Canceled` to `StatusCanceled` and `context.DeadlineExceeded` to `StatusTimeout`. Do not use `ctx.Err() != nil` alone; the status matters to the parent.

The small helpers referenced above are:

```go
func statusFromContext(ctx context.Context) Status {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return StatusTimeout
	case errors.Is(ctx.Err(), context.Canceled):
		return StatusCanceled
	default:
		return StatusFailed
	}
}

func lastAssistantText(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleAssistant && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

func contentOrError(tr ToolResult) string {
	if tr.IsError {
		return "ERROR: " + tr.Content
	}
	return tr.Content
}
```

### 8.4 Dispatcher and tool boundary

```go
type SpawnRequest struct {
	Agent string
	Task  string
}

// SpawnOptions are chosen by the caller, never by model-supplied JSON.
type SpawnOptions struct {
	Mode   ContextMode
	Parent ParentView
}

type Dispatcher struct {
	Registry    *Registry
	Provider    Provider
	Tools       *Toolbox
	MaxDepth    int
	MaxParallel int
}

func (d *Dispatcher) Spawn(ctx context.Context, depth int, req SpawnRequest, opts SpawnOptions) (ChildResult, error) {
	if req.Task == "" {
		return ChildResult{}, errors.New("task is required")
	}
	if d.MaxDepth > 0 && depth >= d.MaxDepth {
		// A request error is a Go error. A *child* failure is a ChildResult.
		return ChildResult{}, fmt.Errorf("depth %d reached; max %d", depth, d.MaxDepth)
	}
	spec, ok := d.Registry.Lookup(req.Agent)
	if !ok {
		return ChildResult{}, fmt.Errorf("unknown agent %q", req.Agent)
	}
	return d.runChild(ctx, spec, opts.Parent, opts.Mode, req.Task, depth), nil
}

// SpawnMany preserves request order without a shared append.
func (d *Dispatcher) SpawnMany(ctx context.Context, depth int, reqs []SpawnRequest, opts SpawnOptions) Envelope {
	started := time.Now()
	results := make([]ChildResult, len(reqs))

	maxParallel := d.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 1
	}
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, req := range reqs {
		i, req := i, req
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = ChildResult{
					Index: i, Agent: req.Agent,
					Status: statusFromContext(ctx), Error: ctx.Err().Error(),
				}
				return
			}

			r, err := d.Spawn(ctx, depth, req, opts)
			if err != nil {
				results[i] = ChildResult{
					Index: i, Agent: req.Agent, Status: StatusFailed, Error: err.Error(),
				}
				return
			}
			r.Index = i
			results[i] = r
		}()
	}
	wg.Wait()
	return Envelope{Results: results, Started: started, Ended: time.Now()}
}

// SpawnTool is the only place a child result becomes a parent tool result.
// One Execute call => exactly one ToolResult, on every path.
type SpawnTool struct {
	Dispatcher *Dispatcher
	Depth      int
	Options    SpawnOptions
}

func (t *SpawnTool) Execute(ctx context.Context, call ToolCall) ToolResult {
	var args SpawnRequest
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    "invalid spawn arguments: " + err.Error(),
			IsError:    true,
		}
	}

	child, err := t.Dispatcher.Spawn(ctx, t.Depth, args, t.Options)
	if err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    err.Error(),
			IsError:    true,
		}
	}

	if child.Status == StatusCompleted {
		return ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    child.Output,
		}
	}

	// A failed/canceled/timed-out child is still a well-formed tool result.
	// Keep the structured status; do not return a Go error and lose it.
	body, _ := json.Marshal(child)
	return ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    string(body),
		IsError:    true,
	}
}
```

The parent harness is responsible for storing the returned `ToolResult` as the `tool_result` message paired with `call.ID`. Do not also append it to the parent transcript manually.

If the parent issues one spawn tool call per child, each call gets one `SpawnTool.Execute` and one result. If it issues a single batch call, `SpawnMany` returns one `Envelope` and the harness still produces one parent `tool_result` containing N `ChildResult`s. The count of child results is not the count of parent tool results.

### 8.5 What was dropped, and why

Dropped from the teaching model:

- **True session fork.** Replaced with an explicit `ParentView.Summary`. This is the single biggest simplification. A real fork needs persisted session files, branch creation, and tool-pair sanitization; it is a chapter of its own. The `nicobailon` code is the reference when the course wants it.
- **Background execution.** The model is synchronous. Background execution requires a durable job record, a result channel, a completion notification, and a retrieval tool. Adding it before the synchronous loop is correct teaches the wrong shape.
- **Resume / steer / pause.** The model is one-shot per child. These require persistent child sessions and a state machine; `pi-subagent-manager` shows the cost.
- **UI.** No widget, fleet, or transcript viewer.
- **Worktree isolation, remote runners, MCP, extension tools, skills, memory, structured output, acceptance reports, usage budgets, spawning budgets, compaction, prompt caching.**
- **Model-specific message sanitization** (signed thinking blocks, provider compatibility). The minimal model assumes a plain text/tool-call transcript.

What remains is still honest: a child has a real system prompt, a real model loop, a real tool allowlist, a real failure envelope, and a real cancellation path. The course can add the other features one at a time without changing the core contract.

---

## 9. Failure modes of a naive Go reimplementation

These are the bugs a competent Go developer is likely to ship when translating the pi design. Each one is specific enough to become a test case.

### 9.1 Child inherits parent history by accident

**Symptom:** the child's first request contains the parent's user/assistant/tool messages, so the child answers the parent's old question, repeats work, or blows the context window.

**Typical cause:** the spawn code passes `parent.Messages` directly, or `BuildChildContext` has a `ParentMessages []Message` field that is populated "just in case."

**Why it matters:** fresh is the default in both projects named `pi-subagents`. `tintinweb` explicitly defaults `inherit_context` to false. `championswimmer` is different: it defaults to a filtered snapshot of the lexical parent, and only an independent root starts with no history. A child that inherits by default is not isolated, no matter what its system prompt says — which is why the default deserves an explicit test.

**Guard:** make the fresh context type structurally incapable of carrying parent messages. If a handoff is needed, require an explicit `ParentView` with a bounded `Summary`, and test that the child request contains exactly one user message.

### 9.2 Shared slice aliasing corrupts the parent transcript

**Symptom:** a child appends to `parent.Messages`, and the parent's next request contains the child's tool calls or results. Concurrent children race on the same backing array.

**Typical cause:** `messages := parent.Messages` then `messages = append(messages, ...)`; if capacity is available, the append writes into the parent's backing array. Even without append, a child that mutates a `Message` value can affect the parent.

**Guard:** deep-copy the message slice and each mutable field (`ToolCalls`, content blocks). `pi-subagent-manager` uses `structuredClone` before filtering. In Go, write a `CloneMessages` helper and use it at every boundary.

### 9.3 Two children share a mutable registry or tool list

**Symptom:** one child's tool policy changes another's; a child sees `bash` even though its spec only allowed `read`; results become flaky under `-race`.

**Typical cause:** a global `map[string]AgentSpec` is read and mutated in place, or `Toolbox.Restrict` returns a view of the same map/slice.

**Guard:** build an immutable registry snapshot per session; return value copies from `Lookup`; `Restrict` returns a new `Toolbox` with its own map. Never let a child mutate the parent's registry.

### 9.4 Parent blocks forever on a cancelled child

**Symptom:** the user cancels the parent turn, but the tool call never returns; or a timeout fires, but the goroutine waits on a child that ignores context.

**Typical cause:** the child loop uses `context.Background()` or a fresh `context.TODO()` instead of the parent `ctx`; or the dispatcher waits on a `chan Result` with no `select` on `ctx.Done()`; or it calls `wg.Wait()` after cancelling without ensuring children can observe cancellation.

**Guard:** every child gets the parent context (or a derived one); every blocking wait is `select { case <-ctx.Done(): ... case r := <-ch: ... }`; the child loop checks `ctx.Err()` before each provider call and each tool call. A cancelled child must still produce a `ChildResult`.

### 9.5 Zero or two tool results for one parent tool call

**Symptom:** the provider rejects the next request because a `tool_use` has no result, or because there are two results; or the UI shows a duplicate result card.

**Typical cause:** the spawn tool returns a receipt and later injects the child result as a second `tool_result`; or it returns early on cancellation without a result while the harness also synthesizes an error result; or it panics in a goroutine and never returns.

**Guard:** one `execute()` call returns exactly one result, on every path, including invalid args, unknown agent, depth rejection, child failure, cancellation, and timeout. Background/async mode, if added, returns a receipt result immediately and uses a separate notification channel. Never append a second result for the same `toolUseID`.

### 9.6 Interleaved child result breaks tool_use/tool_result pairing

**Symptom:** the parent transcript has `assistant(tool_use A)`, then a user message containing child B's result, then `tool_result A`. The provider rejects the request or the model misattributes the result.

**Typical cause:** the spawn tool starts a goroutine, returns, and the goroutine appends a "child finished" message to the parent's message list whenever it completes — including in the middle of a tool batch.

**Guard:** do not mutate the parent transcript from a child goroutine. In synchronous mode, return the result from `execute()`. In asynchronous mode, deliver notifications only at a turn boundary (pi's `turn_end` / `agent_settled`), and keep them as custom messages, not `tool_result`s. `tintinweb`'s `CompletionOutbox` is the reference: it batches at `turn_end`, not inside the tool call.

### 9.7 No recursion guard

**Symptom:** a child spawns a child, which spawns a child, until the process runs out of file descriptors, money, or time.

**Typical cause:** every child gets the same tool registry, including `spawn`; there is no depth parameter.

**Guard:** pass `depth` explicitly; reject at `depth >= maxDepth`; only include the spawn tool in the child's toolbox when the agent's allowlist grants it; cap total spawns per run. All three projects do at least two of these.

### 9.8 Timeout and cancellation are conflated

**Symptom:** a report-only timeout kills a child that was supposed to keep running; or a user cancellation is reported as a normal failure with no partial output.

**Typical cause:** `context.WithTimeout` is used for both the wait and the child run, or `ctx.Err()` is mapped to a generic `failed` status.

**Guard:** separate the child's run context from the waiter's context. A report-only timer should emit an event and leave the child running (as in `championswimmer`'s spawn timer and nicobailon's `bg_wait` window). A wait cancellation should reject only the wait (as in `championswimmer`). Map `context.Canceled` to `canceled`, `context.DeadlineExceeded` to `timeout`, and preserve partial output.

### 9.9 Results are appended in completion order

**Symptom:** the parent asked for `[reviewer, tester, researcher]`, but got results in the order they finished; the wrong result is attributed to the wrong task.

**Typical cause:** `results = append(results, r)` inside each goroutine, guarded by a mutex.

**Guard:** preallocate `results := make([]ChildResult, len(reqs))` and write `results[i]` only. Do not append. This is what `nicobailon`'s `index` field and `tintinweb`'s per-agent IDs make explicit. If the parent API is one tool call per child, the host's `toolCallId` is the ordering key, not completion time.

### 9.10 Child failure panics the process

**Symptom:** a tool or provider panics in a child goroutine; the whole harness exits instead of returning a failed child result.

**Typical cause:** no `recover()` around the child loop, and the goroutine is not managed by an `errgroup` that can convert panics.

**Guard:** `defer recover()` inside `runChild`, convert the panic into `StatusFailed` with an error string, and still return a result. A panic in a child is a child failure, not a process failure.

### 9.11 Child output is dumped unbounded into the parent

**Symptom:** the parent context overflows after one child, or the child's entire tool log is pasted into the model-facing result.

**Typical cause:** returning `messages` or the full tool transcript as `content`.

**Guard:** return a bounded final answer in `content`, keep structured details (session id, usage, artifact path, status) in `details`, and page large output through a separate retrieval tool. All three bound the background notification preview; `championswimmer` also truncates the `ThreadView.output` passed in tool results at 16,000 characters, and `nicobailon` caps inline result text with an artifact fallback. `tintinweb`'s foreground `Agent` result path does not visibly cap `record.result`; only its notification preview is capped (`src/index.ts:190-194`), so a foreground child can still return a very large string.

### 9.12 Agent registry is reloaded under a running child

**Symptom:** a child's tool policy changes mid-run because someone edited an agent file or reloaded the registry; results differ between runs.

**Typical cause:** each tool call re-reads the global registry instead of using the snapshot captured at spawn.

**Guard:** capture the `AgentSpec` and `Toolbox` at spawn time; the child uses that snapshot for its whole run. `championswimmer` clones the type (`structuredClone(this.options.getType(args.type))`), and `tintinweb` resolves config before the run. Reloading should affect future spawns, not running ones.

---

## 10. Gradable invariants for a scripted provider

These are designed for an offline harness with a **scripted provider**: a `Provider` implementation that records every `ProviderRequest` and returns a preprogrammed sequence of `ProviderResponse`s, optionally blocking on a channel for cancellation/timeout tests.

Recommended harness capabilities:

```go
type ScriptedProvider struct {
	mu       sync.Mutex
	Requests []ProviderRequest
	Script   []ProviderResponse
	// Hooks let a test block, cancel, or fail a specific call.
	Before func(ctx context.Context, req ProviderRequest, call int) error
}

func (p *ScriptedProvider) Complete(ctx context.Context, req ProviderRequest) (ProviderResponse, error) {
	p.mu.Lock()
	p.Requests = append(p.Requests, cloneRequest(req))
	call := len(p.Requests) - 1
	p.mu.Unlock()
	if p.Before != nil {
		if err := p.Before(ctx, req, call); err != nil {
			return ProviderResponse{}, err
		}
	}
	if call >= len(p.Script) {
		return ProviderResponse{}, fmt.Errorf("script exhausted at call %d", call)
	}
	return p.Script[call], nil
}
```

### I1. A fresh child request contains no parent turns

**Setup:** parent provider script answers a trivial parent turn; spawn `researcher` with `ContextFresh` and task `"find X"`. Capture the child's `ProviderRequest`.

**Assertion:**

- child request has exactly one message: `role=user`, content contains `"find X"`;
- no parent user text, assistant text, tool calls, or tool results appear anywhere in the child request;
- child `system` equals the agent's `SystemPrompt`, not the parent's.

**Why it catches bugs:** the most common naive bug is passing `parent.Messages` or a shared slice. This test fails immediately and locally.

**Variant:** if the course implements `Handoff`, assert the handoff text is inside the single user message and that the parent's raw messages are still absent.

### I2. A disallowed tool is refused, not executed

**Setup:**

- register two tools: `read` (records calls) and `bash` (records calls);
- agent spec allows only `read`;
- scripted provider returns a tool call `bash` on turn 1, then a final text on turn 2.

**Assertion:**

- the child request's `Tools` list contains `read` but not `bash`;
- `bash` execution count is 0;
- the child transcript contains a `tool` message for the `bash` tool call ID, with an error/refusal text;
- the final child status is `completed` (the model can recover) or `failed` if the harness treats refusal as fatal, but the refusal must be a well-formed tool result, not a panic;
- the parent receives one result.

**Why it catches bugs:** hiding a tool from the schema is not enough; a model can still emit the name. The runtime check must be enforced at execution.

### I3. Exactly one tool result per spawn call, and one per child tool call

**Setup:** four independent spawn calls/scripts:

1. success: provider returns text immediately;
2. failure: provider returns an error;
3. timeout: provider blocks until the child's deadline;
4. cancellation: provider blocks until the parent context is cancelled.

Drive each through the spawn tool. Inspect the parent transcript/event log.

**Assertion:**

- `count(tool_result where toolCallId == call.ID) == 1` for each parent spawn call;
- inside each child transcript, every assistant `toolCall` id has exactly one `RoleTool` message, and every `RoleTool` message matches a prior call id;
- the result has a non-empty `content` for all four;
- success has `isError=false`; failure/timeout/cancel have `isError=true` (or a structured `status` field, per the harness contract);
- no result is a duplicate of another;
- the parent transcript has `assistant(tool_use ID)` immediately followed by the matching `tool_result` in the tool-result slot (no interleaved user message).

**Why it catches bugs:** it forces the implementation to handle the ugly paths before the happy path hides them. It also catches the "return early and inject later" bug.

### I4. A cancelled child still yields a well-formed result with partial output

**Setup:**

- provider call 1 returns an assistant message with text `"partial finding"` and a tool call to `slow_tool`;
- `slow_tool` blocks until the context is cancelled, then records that it observed cancellation;
- cancel the parent context after the tool starts.

**Assertion:**

- the child result status is `canceled`, not `failed` and not a bare Go error;
- `Output` contains `"partial finding"` (or a documented partial-output field is set);
- the slow tool observed cancellation (its `Run` returned after `ctx.Done()`);
- the parent still has exactly one `tool_result` for the spawn call, with `isError=true` and the canceled status;
- no goroutine remains blocked (use `goleak` or a `WaitGroup` with a timeout).

**Why it catches bugs:** cancellation is the path most likely to deadlock or lose the result. It also tests that a canceled child is not silently reported as success.

### I5. Concurrent children never cross-contaminate, and results are ordered by request index

**Setup:**

- spawn three children in one `SpawnMany` call: `A` task `"alpha"`, `B` task `"beta"`, `C` task `"gamma"`;
- the scripted provider returns a response determined by the task text, with deliberate random or reversed delays (e.g., C finishes first, A last);
- each child calls a `record` tool with its own name, and the tool records the caller's child ID if available.

**Assertion:**

- `Envelope.Results[0].Agent == "A"` and `Output` contains `"alpha"`; index 1 is `"beta"`; index 2 is `"gamma"`;
- each child's captured request contains only its own task, not the sibling tasks;
- the set of `record` tool invocations is exactly one per child, and no invocation is attributed to the wrong child;
- running the test with `-race` reports no data race;
- running it 100 times yields the same index mapping despite randomized completion order.

**Why it catches bugs:** append-in-completion-order and shared mutable state are the two most common concurrency bugs. This test catches both without relying on timing.

### I6. A child at the depth cap cannot create a grandchild

**Setup:**

- `MaxDepth = 1` (main at depth 0; child at depth 1 is at the cap);
- child agent's tool allowlist includes `spawn` (so the model can try);
- scripted provider for the child emits a `spawn` tool call with a valid task;
- the spawn tool is the same type used by the parent, with `Depth: 1`.

**Assertion:**

- the child's `spawn` tool call returns a `tool_result` with an error/refusal, not a new child;
- the dispatcher's child count remains 1 (only the first child);
- no grandchild provider request is made;
- the parent receives one well-formed result for the original spawn call.

**Why it catches bugs:** a depth check that only hides the tool is not enough if the model can still call the name. The refusal must be enforced at the tool boundary, and it must not panic or hang.

**Variant:** if the course hides the spawn tool at the cap, assert the child request's `Tools` list has no `spawn`, and separately unit-test the dispatcher's depth rejection.

### Invariant coverage matrix

| Invariant | Fresh context | Tool scoping | Pairing | Cancellation | Concurrency | Recursion |
|---|---|---|---|---|---|---|
| I1 | ● |  |  |  |  |  |
| I2 |  | ● | ● |  |  |  |
| I3 |  |  | ● | ● |  |  |
| I4 |  |  | ● | ● |  |  |
| I5 | ● |  |  |  | ● |  |
| I6 |  | ● | ● |  |  | ● |

---

## 11. Could not determine

These are open questions after reading the source. They are deliberately listed rather than guessed.

1. **Exact pi host behavior when `execute()` throws vs returns `isError: true`.** The docs say a throw produces a failed tool result, and `AgentToolResult` keeps `details`/`structuredContent` for non-throwing errors. We did not run a live pi host to verify whether `details` survive a thrown error, whether the host adds its own error text, or how a thrown error interacts with `tool_result` handlers. The extension code assumes the throw path is canonical for logical failure (`nicobailon`), while `tintinweb` avoids it for child failures. A live test should settle this.
2. **Whether a fork at tool-execution time includes the currently executing spawn tool call.** `nicobailon` branches the parent session at its current leaf while the spawn tool is running. Depending on when pi persists the assistant `tool_use`, the fork may or may not contain the spawn call. The code does not explicitly remove a self-referential spawn call. We did not instrument a live fork to see the exact branch contents. This matters for the "no parent turns" invariant if the course implements fork.
3. **The exact contents of `ctx.sessionManager.getBranch()` during a tool batch.** `tintinweb`'s `buildParentContext` and `pi-subagent-manager`'s `rootSnapshot` both read the branch during tool execution. The source assumes the branch is the settled conversation, but we did not run a live session to confirm whether the current assistant call is present, whether a pending tool result is absent, or how compaction entries are represented at that moment.
4. **Provider-specific message/tool declaration compatibility.** `nicobailon` has extensive fork sanitization for Anthropic thinking blocks, and `tintinweb` relies on pi's session/tool machinery. We did not run a provider to verify how each project's inherited messages/tools are serialized across providers, or which providers accept mid-conversation system/tool changes. The code's own comments acknowledge these constraints.
5. **The full behavior of `nicobailon`'s async runner under process death.** The code has boot ids, PID namespaces, process-terminal proofs, abandoned-slot policies, and result-file reconciliation. We read the top-level logic but did not exercise crash/restart scenarios. The `maxActiveAsyncRunsPerSession` and abandoned-slot rules are more complex than the summary here.
6. **`tintinweb`'s `inherit_context` with a compacted branch.** The code includes compaction summaries and skips tool results, but we did not test how a branch with a compaction entry plus post-compaction messages is rendered, or whether the text can exceed the child's context window. The implementation has no explicit size cap in `buildParentContext`.
7. **`pi-subagent-manager`'s persistence across real `/reload`, `/resume`, and branch navigation.** The code has a durable registry entry, a root mailbox, session-file ownership checks, and restore-to-paused behavior. We read the paths but did not execute the TUI lifecycle. The exact interaction between a forked main session, a restored child session, and the registry's `rootSessionId` check is only partially covered by comments.
8. **All test suites.** None were run for this note. The repositories contain substantial tests (for example, `nicobailon` has hundreds of test files), and running them would be the next verification step. The claims here are source-reading claims, not test-run claims.
9. **Exact LOC comparability.** The numbers are nonblank source lines, not `cloc`; generated code, tests, and examples are excluded. They are useful for relative size, not for a precise complexity metric.

---

## Appendix A: verification notes

Commands used for the snapshot:

```bash
mkdir -p /tmp/sa-survey
cd /tmp/sa-survey
git clone --depth 1 https://github.com/nicobailon/pi-subagents nicobailon_pi-subagents
git clone --depth 1 https://github.com/tintinweb/pi-subagents tintinweb-pi-subagents
git clone --depth 1 https://github.com/championswimmer/pi-subagent-manager championswimmer_pi-subagent-manager

# source size (nonblank .ts under src)
for d in nicobailon_pi-subagents tintinweb-pi-subagents championswimmer_pi-subagent-manager; do
  (cd "$d" && find src -name '*.ts' -print0 | xargs -0 cat | awk 'NF{n++} END{print n+0}')
done
```

Line numbers were taken from the commits in the table at the top. The pi API references are from the installed package at:

- `/home/darkoptimus/.npm-global/lib/node_modules/@earendil-works/pi-coding-agent/docs/extensions.md`
- `/home/darkoptimus/.npm-global/lib/node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts`
- `/home/darkoptimus/.npm-global/lib/node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-agent-core/dist/types.d.ts`

No files outside `docs/research/` were modified.
