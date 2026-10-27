// Package hints is course content: the three-rung hint ladder for every graded
// case.
//
// Rung 1 names the mechanism (a nudge). Rung 2 points at the specific API or
// line to look at. Rung 3 states the answer. The ladder exists so that being
// stuck costs you a nudge, not the exercise.
//
// Hints live here rather than beside each check so that `kata hint` can print
// them without parsing Go source. The type is a fixed [3]string rather than a
// slice because a missing rung must be a compile error, not an invisible gap in
// the ladder.
package hints

import "sort"

// Table maps chapter number -> case id -> the three rungs.
var Table = map[string]map[string][3]string{
	"19": {
		"creates-a-session": {
			"A conversation needs an identity before it can be listed.",
			"Open a database, create the tables, and insert a session row on the first run.",
			"OpenStore runs the schema with CREATE TABLE IF NOT EXISTS. Create inserts the id, title and two timestamps.",
		},
		"lists-sessions": {
			"A directory of files named by hand is a puzzle.",
			"Print a table of sessions, most recently used first.",
			"SELECT id, title, created, updated and a COUNT of messages, ORDER BY updated DESC, then print one line each.",
		},
		"title-from-first-prompt": {
			"A list of timestamps is unreadable.",
			"Use the first prompt as the title, trimmed to a readable length.",
			"TitleFromPrompt collapses whitespace and cuts at 60 runes. Set it when the session is created.",
		},
		"resumes-by-id": {
			"The point of storage is returning to an old conversation.",
			"Add a -resume flag that loads a session by id before the loop runs.",
			"SELECT the messages for that id ORDER BY ordinal, and prepend them to the history after the system message.",
		},
		"continue-uses-latest": {
			"Typing an id every time is the chore.",
			"Add -continue that picks the most recently updated session.",
			"SELECT id FROM sessions ORDER BY updated DESC LIMIT 1, then resume that id. Touch updated on every append, or the ordering never changes.",
		},
		"tool-calls-round-trip": {
			"Both halves of a tool pair have to survive the database.",
			"Store the tool calls as JSON on the assistant row, and the id on the tool row.",
			"Marshal m.ToolCalls into a text column, and read it back with json.Unmarshal. Keep tool_call_id and name as their own columns.",
		},
		"two-sessions-do-not-mix": {
			"A store with no session column is one long thread.",
			"Key every message row by session_id, and filter on it when loading.",
			"messages(session_id, ordinal) with a UNIQUE constraint, and every SELECT filtered by session_id.",
		},
		"unknown-id-is-reported": {
			"Starting a new conversation under a mistyped id loses the thread.",
			"Check the id exists before loading, and exit non-zero when it does not.",
			"SELECT 1 FROM sessions WHERE id = ?, and return an error like 'no such session: <id>' when there are no rows.",
		},
	},
	"18": {
		"required-fields-always-present": {
			"A server needs all three to answer.",
			"Build the body from the request, with model, messages and stream always set.",
			"Start from a map with model, messages and stream: req.Stream, then add the rest only when they are set.",
		},
		"unset-optionals-absent": {
			"Zero is a value, not an absence.",
			"Add max_tokens and temperature only when they are greater than zero.",
			`A provider reads "max_tokens": 0 as produce nothing and "temperature": 0 as a deliberate choice. Omit the key entirely when the caller did not set it.`,
		},
		"set-optionals-present": {
			"Omitting unset fields only matters if the chosen ones still arrive.",
			"Add both keys when the values are positive.",
			"The check reads raw numbers from the body: max_tokens must equal 128 and temperature must equal 0.4.",
		},
		"messages-keep-their-order": {
			"The conversation is a sequence.",
			"Convert the messages in order, keeping the role and content of each.",
			"Walk the slice and append {role, content, tool_calls, tool_call_id} without sorting or grouping. The system message must stay at index 0.",
		},
		"headers-decide-the-response": {
			"Content-Type says what you sent. Accept says what you can read.",
			"Set both headers before the request goes out.",
			`Content-Type: application/json and Accept: text/event-stream. Without the second, a server may answer with one buffered response and the parser waits forever.`,
		},
		"base-url-trailing-slash": {
			"A doubled slash is a 404 that looks like a wrong route.",
			"Trim the trailing slash from the base URL before joining the path.",
			"strings.TrimRight(base, \"/\") + \"/chat/completions\" works for both http://x/v1 and http://x/v1/.",
		},
		"empty-tool-list-is-sent": {
			"A server distinguishes no tools from a request that never mentioned them.",
			"Send the tools key whenever the request has a tool slice, even an empty one.",
			"If req.Tools is not nil, build the array and set it. Only omit the key when the caller passed no slice at all.",
		},
	},
	"17": {
		"project-outranks-user": {
			"A repository's skills are about that repository.",
			"Give each scope a priority, and let the higher one win a collision.",
			"Scope{Root, Priority: 20} for project and Priority: 10 for user. On a collision keep the higher priority, and record the loser.",
		},
		"losing-copy-is-not-expanded": {
			"Naming a skill is not enough; the body that runs has to be the chosen one.",
			"Expand against the resolved catalogue, not against the directories.",
			"Build the expansion from the winning skills only, so the shadowed body never reaches a prompt.",
		},
		"both-scopes-contribute": {
			"Precedence applies to collisions, not to everything.",
			"Merge every scope's skills, then resolve duplicates.",
			"Load each scope, add each skill to a map keyed by name, and let the later priority replace the earlier. Distinct names all survive.",
		},
		"missing-scope-is-skipped": {
			"Most machines have no user skills directory.",
			"Treat a missing directory as an empty scope.",
			"If os.ReadDir returns os.ErrNotExist, continue to the next scope. The project skills still load.",
		},
		"no-scopes-at-all": {
			"An empty catalogue must produce no section at all.",
			"Return an empty string from Contribute when nothing was found.",
			`If len(chosen) == 0, return ("" , nil) so BuildSystemPrompt writes no skills heading.`,
		},
		"malformed-file-in-one-scope": {
			"One bad file must not take the catalogue with it.",
			"Skip parse errors inside a scope and keep the other skills from that same scope.",
			"The loader already skips a file without a name and description. Check that the other scope still contributes.",
		},
	},
	"16": {
		"ceiling-respected": {
			"Schemas are paid for on every request.",
			"Advertise at most the ceiling, in registration order.",
			"Take the first N names from the registry and return only their defs. A registry smaller than the ceiling advertises everything.",
		},
		"hidden-tools-are-named": {
			"A tool that can only be loaded by guessing is never loaded.",
			"Put the hidden names in the system message, next to the loader's name.",
			"Append one line: 'Other tools you can load with load_tool: write, bash, ...'. Emit nothing when the list is empty.",
		},
		"load-tool-is-always-advertised": {
			"The loader cannot be subject to the limit it enforces.",
			"Keep a list of names that are never hidden.",
			"Always(name) records a name that goes into every advertised set, before the ceiling is applied.",
		},
		"loading-a-tool-makes-it-callable": {
			"Naming a hidden tool is only useful if the next turn carries its schema.",
			"Record the loaded name, and include it in the advertised set from then on.",
			"Mount(name) checks the registry and adds the name to an extra set. Mounted() returns always-on names, then the ceiling, then anything explicitly mounted.",
		},
		"unknown-tool-load-is-a-result": {
			"The name comes from the model.",
			"Look it up before mounting, and return a message when it is missing.",
			`Return a tool error like "no such tool: <name>" so the registry records an IsError result and the run continues.`,
		},
		"no-ceiling-advertises-everything": {
			"The cap is a ceiling, not a rule.",
			"Apply the limit only when the registry is larger than it.",
			"Mounted() breaks out of the loop only when the count reaches the ceiling, so a registry of five with a ceiling of fifty returns all five.",
		},
		"schemas-still-count-against-budget": {
			"The two budgets are related.",
			"Count the schemas actually advertised, not the ones in the registry.",
			"Estimate from req.Tools, which the view already filtered. Advertising two instead of nine must lower the estimate.",
		},
	},
	"15": {
		"small-result-passes-through": {
			"A limit is a ceiling, not a rule for everything.",
			"Return the content unchanged when it is under the limit.",
			"If len(content) <= limit, return it with no notice and no file. Moving a short result costs a round trip for nothing.",
		},
		"large-result-moved": {
			"Cutting a result keeps the budget and loses the information.",
			"Write the whole thing to disk and send a summary with a handle.",
			`The notice needs three facts: how many bytes and lines exist, a handle, and the exact call that fetches more. Keep the first slice inline so the model can start working.`,
		},
		"result-written-to-disk": {
			"A summary that cannot be followed up is a deletion.",
			"Store the content under a name derived from it, so the same content reuses the same file.",
			"Hash the content with sha256, take the first 8 bytes as hex, and write it to <dir>/<handle>.txt.",
		},
		"read-result-fetches-a-slice": {
			"The handle is only useful if a second call returns bytes.",
			"Add a read_result tool that takes a handle, an offset and a length.",
			"Read with f.ReadAt(buf, int64(offset)), return the slice with a note naming the byte range, and include the next offset when there is more.",
		},
		"unknown-handle-is-a-result": {
			"The handle is model input, and models mistype.",
			"Return a message naming the missing handle rather than an error that stops the run.",
			`os.IsNotExist becomes a result like 'no result stored under handle "<x>"', so the model can try another.`,
		},
		"offset-past-end-is-a-result": {
			"A model that guesses an offset will overshoot.",
			"Compare the offset with the file size and say what the size is.",
			"If offset >= total, return a result naming both numbers, so the model learns the real length.",
		},
		"handle-is-not-a-path": {
			"The handle arrives as a model argument, so it is untrusted.",
			"Accept only plain hex and build the path yourself.",
			"Reject any rune outside 0-9a-f, then join with the directory. A handle of ../../etc/passwd never reaches the filesystem.",
		},
	},
	"14": {
		"retries-a-429": {
			"A 429 means busy, not wrong.",
			"Wrap the send in a loop that tries again when the status is 429.",
			"Return the error from the send, check Retryable, wait, and call Do again. The parser should never see the 429.",
		},
		"retries-a-500": {
			"The 5xx family is the server's problem, so the same request may work next time.",
			"Treat every 5xx except 501 as retryable.",
			"Retryable returns true for StatusTooManyRequests and for status >= 500, and false for 501, which means the server will never support it.",
		},
		"does-not-retry-a-400": {
			"The request is wrong, and repeating it repeats the mistake.",
			"Retry only the statuses that mean busy or broken.",
			"A 400, 401, 403, 404 and 422 all mean the request is wrong. Return the error on the first attempt and exit 1.",
		},
		"honours-retry-after": {
			"The server tells you when to come back.",
			"Read the Retry-After header into the error, and use it instead of your own delay.",
			"parseRetryAfter reads the header as a number of seconds. When it is non-zero, wait that long instead of the computed backoff.",
		},
		"stops-after-the-limit": {
			"A server that is down stays down.",
			"Count attempts, and return the last error when the count runs out.",
			"withRetry loops from 1 to MaxAttempts, returns on success, returns immediately when the error is not retryable, and otherwise waits and tries again.",
		},
		"no-retry-on-success": {
			"Retrying a success doubles the cost and prints the answer twice.",
			"Only enter the wait path when the attempt failed.",
			"If the first Do returns 200, withRetry returns immediately. Check that one request reaches the server.",
		},
		"does-not-retry-a-cancellation": {
			"Ctrl-C has to mean stop.",
			"Select on the context while waiting, and treat a cancellation as final.",
			"In the wait, select on ctx.Done() and return ctx.Err(). Retryable also returns false for context.Canceled, so a cancelled request is never retried.",
		},
	},
	"13": {
		"mcp-tools-advertised": {
			"A tool the model cannot see is a tool it will never call.",
			"After initialize and tools/list, wrap each server tool and add it to the registry the loop sends.",
			"Merge the server's tools with the built-in ones before NewRegistry, so the request advertises both.",
		},
		"mcp-initialize-handshake": {
			"MCP requires a handshake before any other request.",
			"Send initialize, then the notifications/initialized notification, then tools/list.",
			"initialize is a request with an id and a response; notifications/initialized has no id and expects no reply. Send it anyway: strict servers wait for it.",
		},
		"mcp-tool-call": {
			"The adapter is only worth having if calls cross the process boundary in both directions.",
			"Send tools/call with the original tool name and the arguments as an object.",
			"On the response, collect content blocks of type text and join them; return that string from Execute so the registry makes it a tool result.",
		},
		"mcp-content-blocks-joined": {
			"Content comes back as a list of blocks.",
			"Join the text blocks with a newline; ignore types you cannot use.",
			"Taking only content[0] loses most answers; taking only type text avoids pasting base64 into the context.",
		},
		"mcp-name-collision": {
			"A server may expose a tool with the same name as a built-in.",
			"Namespace every server tool with the server's reported name.",
			"Name() returns server + \".\" + tool, so demo.read and read are different entries. Sanitise the server name to lowercase and dashes.",
		},
		"mcp-error-is-a-result": {
			"A refusal from the server is information, not a crash.",
			"Turn a JSON-RPC error object into a Go error so the registry records an IsError result.",
			"Return fmt.Errorf from Execute with the error message; the loop continues and the model can read the refusal.",
		},
		"mcp-unavailable-is-survivable": {
			"An optional extension must not take the agent down with it.",
			"Warn on stderr and continue with the built-in tools when the server cannot start.",
			"Print the failure with fmt.Fprintf(os.Stderr, ...), skip the merge, and keep the exit code 0.",
		},
		"mcp-no-call-when-unused": {
			"Discovery is not usage.",
			"tools/list is the only request a plain run should make after the handshake.",
			"If the model asked for no tool, no tools/call should appear in the server's log.",
		},
	},
	"12": {
		"task-tool-advertised": {
			"Delegation has to be a tool the model can call.",
			"Register a task tool whose schema takes the full instruction for the child.",
			"Parameters: description (optional label) and prompt (required), with the description saying the child cannot see this conversation.",
		},
		"child-gets-fresh-context": {
			"Isolation is the reason to delegate at all.",
			"Build the child history from the system message plus the task prompt, and nothing else.",
			"[]Message{{Role: RoleSystem, Content: system}, {Role: RoleUser, Content: args.Prompt}}. Do not pass the parent history.",
		},
		"child-cannot-recurse": {
			"Recursion is cheap to allow and expensive to debug.",
			"Build the child registry before the task tool exists, and never add the task tool to it.",
			"baseTools := []Tool{read, write, bash}; childTools := NewRegistry(baseTools...); tools := NewRegistry(append(baseTools, task)...).",
		},
		"result-returned-to-parent": {
			"The parent asked one call and needs one answer.",
			"Return the child's final text from Execute, so the registry turns it into the tool result.",
			"Run the child loop, take res.Message.Content, and return it. The parent's next request carries it as the tool message for the call id.",
		},
		"exactly-one-result": {
			"Two results for one call is an invalid conversation.",
			"Return exactly once from Execute: either an answer or an error, never both.",
			`On failure return ("", err) so Registry.Run produces a single IsError result; do not also return a success text.`,
		},
		"child-uses-tools": {
			"A child that cannot read a file cannot summarise a repository.",
			"Give the child the real tools, and let its loop run with the same provider.",
			"TaskTool needs the provider and a registry containing read, write and bash; the child's RunLoop then works exactly like the parent's.",
		},
		"child-failure-is-a-result": {
			"The parent is blocked on one call, so every outcome must resolve it.",
			"A child that hits its step cap returns an error, and the registry records it as a failed result.",
			"Wrap the child's error with %w and mention the partial answer; Registry.Run converts it to IsError and the parent loop continues.",
		},
		"parent-continues-after-task": {
			"Delegation happens inside the parent's loop.",
			"Return from Execute so the registry appends the result, and let the parent loop decide whether to continue.",
			"Nothing special is needed beyond a correct result: the parent loop keeps going while there are tool calls, including further task calls.",
		},
	},
	"11": {
		"loads-body-on-invoke": {
			"Invocation is when the body enters the request.",
			"Scan the prompt for /name and replace the token with that skill's body.",
			"Expand before the prompt becomes history, so the request and the session agree.",
		},
		"substitutes-arguments": {
			"Placeholders are how a skill takes input.",
			"Replace $ARGUMENTS first, then positional $0, $1, ... from the whitespace-split arguments.",
			`$ARGUMENTS is the whole argument string; $0 is the first word. Replace with a regex over \$(\d+) so a missing argument becomes empty rather than literal.`,
		},
		"stacks-skills": {
			"One prompt can name several skills, and order is meaning.",
			"Split the prompt at each slash command; each skill's arguments run to the next command.",
			"Walk matches left to right, write the body for a known name, and advance past its arguments.",
		},
		"no-invocation-no-body": {
			"Ordinary text is the common case, and expansion must be a no-op.",
			"Return the prompt unchanged when no slash command matches.",
			"Trim only what you inserted. Do not trim or reformat the user's own words.",
		},
		"unknown-skill-preserved": {
			"A typo must not delete the person's words.",
			"On an unknown name, copy the original text through and continue.",
			`Write prompt[last:argsEnd] rather than expanding, and keep going with the next match.`,
		},
		"frontmatter-not-leaked": {
			"The header is metadata for the agent, not instructions for the model.",
			"Parse the frontmatter, keep the body, and substitute into the body only.",
			"Split on the closing '---' fence and take what follows; the body must not start with '---' or contain 'description:'.",
		},
	},
	"10": {
		"advertises-skills": {
			"A model cannot invoke what it has not been told about.",
			"Read skills/*/SKILL.md, and add a section to the system prompt naming each skill and its description.",
			"Implement the Contributor interface: render '# Skills', a line on how to invoke, then '- name: description' per skill.",
		},
		"body-not-advertised": {
			"The catalogue's value is that it is small.",
			"Advertise name and description only; keep Body out of the system prompt.",
			"If the system prompt contains the body of an uninvoked skill, chapter 07's budget pays for work nobody asked for.",
		},
		"malformed-skill-skipped": {
			"A file that cannot be parsed is not a skill.",
			"Treat a SKILL.md without a usable name and description as absent, not as a fatal error.",
			"If ParseSkill returns an error, skip that directory and keep loading the rest; the agent still has to start.",
		},
		"missing-skills-dir": {
			"Most projects have no skills directory.",
			"Return an empty catalogue when skills/ does not exist, and do not add a section to the system prompt.",
			"If os.ReadDir returns os.ErrNotExist, return nil, nil; Contribute then renders nothing and the base prompt is sent alone.",
		},
	},
	"09": {
		"steer-continues-after-done": {
			"A steered message is a reason to keep going, not a new run.",
			"After each turn, drain the queue; if anything arrived, append it and loop again even when the model asked for no tools.",
			"Track a steered flag per turn and stop only when !askedForTools && !steered.",
		},
		"steer-enters-after-turn": {
			"Order in the message list is meaning.",
			"Append the steered line after the assistant message, as a new user message.",
			"The request must be system, user(prompt), assistant(reply), user(steer). Not merged into the first user message.",
		},
		"steer-not-duplicated": {
			"A queue that is drained but not cleared replays the same instruction forever.",
			"Drain means take-and-clear, under a lock.",
			"out := s.queue; s.queue = nil; return out.",
		},
		"no-steer-single-request": {
			"Watching stdin must not change a run that has nothing to say.",
			"Only loop again when the drain actually returned messages.",
			"A nil or empty drain leaves steered false, so the turn ends normally after one request.",
		},
		"steer-event-reported": {
			"The person needs to see that their message landed.",
			"Emit an event for each injected message, with the text.",
			"emit(Event{Type: EventSteer, Turn: turn, Text: m.Content}) before appending it to history.",
		},
		"abort-keeps-partial": {
			"An interrupted answer is still an answer.",
			"Return the accumulated message alongside the error instead of an empty one.",
			"In readSSE, on scanner error return Message{Role: RoleAssistant, Content: content.String()} with the error, and let main print it.",
		},
		"abort-emits-event": {
			"A consumer must tell 'finished' from 'stopped'.",
			"Both end with agent_end; the reason field differs.",
			"If errors.Is(err, context.Canceled) then emit agent_end with reason 'aborted' before returning.",
		},
		"abort-persists-session": {
			"An interrupted run still produced work worth keeping.",
			"Persist before deciding what to return.",
			"Append the new history to the session first, then handle the cancellation and exit.",
		},
	},
	"08": {
		"budget-event-reported": {
			"A budget nobody can see is a number in a comment.",
			"Emit one event per request carrying the estimate and the configured ceiling.",
			"After fitting, emit {type: budget, turn, tokens: EstimateTokens(fitted, tools), budget, pruned}.",
		},
		"estimate-counts-tools": {
			"Tool schemas travel with every request and are often bigger than the conversation.",
			"Count the bytes of name, description and the JSON schema for each tool, then divide by roughly four.",
			"Sum len(t.Name) + len(t.Description) + len(json.Marshal(t.Parameters)) from the request's tools, not from the registry you built.",
		},
		"estimate-grows-with-history": {
			"Monotonicity is the property pruning depends on.",
			"Give every message a cost, even an empty one, so adding messages cannot lower the total.",
			"len(role) + len(content) + a fixed envelope, divided by four at the end. Keep the division at the end, not per message.",
		},
		"ceiling-respected": {
			"A threshold that fires eventually is not a ceiling.",
			"Fit the messages to the budget on every request, not once at start-up.",
			"Wrap the provider: before delegating, replace req.Messages with the fitted list. The loop then cannot bypass it.",
		},
		"prunes-when-over": {
			"Reporting a number and sending everything is the easy way to look correct.",
			"Walk from the newest message backwards and stop when the budget is reached.",
			"Count how many were dropped and report it in the event, so the number can be checked against the request.",
		},
		"prune-marker-present": {
			"A model that does not know history was removed will answer confidently about what it cannot see.",
			"Insert exactly one record where the gap begins.",
			"Prepend a system message such as '[context pruned: N earlier messages omitted]' before the kept tail.",
		},
		"no-orphaned-tool-pairs": {
			"Pruning at a boundary is where conversations become invalid.",
			"If the oldest kept message is a tool result, its call was dropped.",
			"Drop leading tool messages after fitting, and count them as pruned. Never send a tool result whose call is absent.",
		},
		"keeps-system-and-newest": {
			"Two things are not negotiable: the instructions and the newest question.",
			"Handle the system message separately, and fit only the conversation after it.",
			"Keep msgs[0] when it is a system message, walk backwards over the rest, and always keep at least the last message even if it alone exceeds the budget.",
		},
	},
	"07": {
		"creates-session-file": {
			"A session path is a promise that the conversation outlives the process.",
			"Open the file for append, creating it and its parent directory when absent.",
			"os.MkdirAll(filepath.Dir(path)) then os.OpenFile(path, O_CREATE|O_WRONLY|O_APPEND, 0o644).",
		},
		"session-is-jsonl": {
			"One JSON object per line, header first.",
			"Write a header with a type and a version, then one record per message.",
			"Marshal each record and append a newline; the header lets a reader reject an incompatible file instead of misreading it.",
		},
		"resumes-history": {
			"The second run has to see what the first one said.",
			"Load the recorded messages, then place them between the system message and the new user message.",
			"history = [system] + prior + [user]. Order matters: the provider pairs calls and results in sequence.",
		},
		"appends-not-overwrites": {
			"Rewriting the file loses everything before the last crash.",
			"Open for append and never truncate.",
			"Use O_APPEND and keep the header only for a new file. Check that the first run's question appears exactly once after two runs.",
		},
		"no-duplicate-turns": {
			"The classic resume bug is loading the history and then recording it again.",
			"Record only what this run added, not the history it loaded.",
			"Slice the run's history from len(prior)+1: that skips the system message and the loaded messages.",
		},
		"system-not-duplicated": {
			"Conversation is data; the system prompt is policy for the current run.",
			"Never store the system message, and rebuild it on every start.",
			"The session holds user, assistant and tool messages only. Prepend one fresh system message at index 0.",
		},
		"tool-history-persisted": {
			"A tool result without its call is rejected by the provider.",
			"Record assistant messages with their tool calls as well as the tool results.",
			"Persist every message the loop appended, including assistant messages whose ToolCalls slice is non-empty.",
		},
		"missing-session-is-created": {
			"The first run has no file yet.",
			"Treat a missing file as a new session rather than an error, and create parent directories.",
			"Check errors.Is(err, os.ErrNotExist) before opening, write the header only when the file is new.",
		},
	},
	"06": {
		"system-message-first": {
			"The system message is assembled before the loop starts, and it comes first.",
			"Build the string, then prepend a message with role system to the history you pass to the loop.",
			"history := []Message{{Role: RoleSystem, Content: system}, {Role: RoleUser, Content: prompt}} and keep the base instruction containing 'coding agent'.",
		},
		"agents-md-included": {
			"Project instructions are a file the agent reads before it starts.",
			"Find AGENTS.md from the working directory upward, read it, and add it to the system message with its path.",
			"Emit a section headed by '# Project instructions' and a '## <absolute path>' line, then the file body.",
		},
		"agents-md-from-parent": {
			"Rules often live above the directory you run in.",
			"Walk upward from the working directory, collecting AGENTS.md at each level.",
			"Loop on filepath.Dir until it stops changing, bounded by a depth limit, and stat AGENTS.md at each level.",
		},
		"agents-md-order": {
			"Two files can disagree, so order is the resolution rule.",
			"The nearest file must come last, which means discovery collects outward first and then reverses.",
			"Collect from the working directory upward, then reverse the slice before rendering.",
		},
		"no-agents-md": {
			"Discovery must succeed when there is nothing to find.",
			"Return an empty section rather than an error or an empty heading.",
			"Track how many files were included and emit the heading only for the first one.",
		},
		"empty-agents-md": {
			"A file that exists and contains nothing is not instructions.",
			"Trim the body and skip it when there is nothing left.",
			`If strings.TrimSpace(body) == "" then continue, before writing any heading.`,
		},
		"system-prompt-stable": {
			"Anything that varies between runs defeats the provider's prefix cache.",
			"Keep the base prompt constant and render files in a deterministic order.",
			"No timestamps, no random ids, no map iteration. Sort or preserve discovery order, and test two runs byte for byte.",
		},
		"user-message-preserved": {
			"Assembly adds a system message; it does not edit the user's text.",
			"The user message is appended unchanged after the system message.",
			"Messages: {system, prompt}. Do not trim, prefix, or wrap the prompt.",
		},
	},
	"05": {
		"loops-until-done": {
			"A one-hop program handles one tool. A loop handles a task.",
			"Wrap the request and the tool batch in a for loop; decide to continue from the reply, not from a counter.",
			"Continue while the assistant message has tool calls. Append the message, append one result per call, then ask again.",
		},
		"preserves-result-order": {
			"Two calls in one message are one batch, and the conversation must mirror it.",
			"Append results in the same order as the calls, each carrying its own id.",
			"Iterate msg.ToolCalls twice: once to announce, once to execute and append. Sorting or parallel collection loses the order.",
		},
		"step-cap-stops": {
			"A model can ask for tools forever, so the loop needs a ceiling.",
			"Count requests, and when the count reaches the limit, stop and return an error.",
			"Return an error wrapping a step-limit sentinel after N turns, and make main exit 1 so the failure is visible to a caller.",
		},
		"events-sequence": {
			"An event stream is a sequence with rules, not a set of logs.",
			"Emit turn_start, then the turn's events, then turn_end; emit agent_end exactly once, last.",
			"Announce every tool_call before executing, then emit one tool_result per call with the same id, then turn_end, then agent_end.",
		},
		"events-text-deltas": {
			"Collecting a reply into one message hides the stream.",
			"Pass a callback into the provider and emit an event for every delta.",
			"Stream(ctx, req, func(text string) { emit(Event{Type: EventText, Text: text}) }) and keep Chat as a wrapper that passes nil.",
		},
		"single-turn-no-tools": {
			"A reply without tool calls is the end of the run.",
			"Check for tool calls before looping again, not after.",
			"If len(msg.ToolCalls) == 0, emit turn_end and agent_end, then return the message.",
		},
		"continues-after-tool-error": {
			"A failed tool is information the model can act on.",
			"Keep looping after an error result; only a transport failure ends the run.",
			"Registry.Run already converts tool errors into results, so the loop needs no special case. Do not return on IsError.",
		},
		"turn-count-matches-requests": {
			"The event stream makes a claim about what happened; the request log can check it.",
			"Count one turn per model request, including the last one.",
			"Emit agent_end with Turns set to the number of requests made, and compare it with the recorded requests.",
		},
	},
	"04": {
		"advertises-three-tools": {
			"Registering a tool and advertising it are two steps, and only the second one is visible to the model.",
			"Build the registry with all three tools and pass Registry.Defs() into every request.",
			"NewRegistry(&ReadTool{...}, &WriteTool{...}, &BashTool{...}) and send the same Defs() on the first and the follow-up request.",
		},
		"writes-a-file": {
			"The effect of write is a file, not a sentence.",
			"Resolve the path, create the parent directories, then write the bytes.",
			"os.MkdirAll(filepath.Dir(full), 0o755) then os.WriteFile(full, content, 0o644), and report the byte count in the result.",
		},
		"write-creates-parents": {
			"The model names paths that do not exist yet.",
			"Create the missing directories before opening the file.",
			"MkdirAll on the parent path, not on the file path. It is a no-op when the directories already exist.",
		},
		"write-refuses-escape": {
			"write shares the escape rule with read, and the consequence is worse.",
			"Resolve the path against the root and refuse before creating anything.",
			"Check filepath.Rel(root, resolved) for '..' or a leading '../' and return an error naming the escape.",
		},
		"bash-runs-a-command": {
			"The command must see the workspace, not the source tree.",
			"Run sh -c with cmd.Dir set to the workspace root, and return combined output.",
			`exec.CommandContext(ctx, "sh", "-c", args.Command) with cmd.Dir = Root, then CombinedOutput().`,
		},
		"bash-reports-exit-code": {
			"A non-zero exit is a normal outcome for a shell command.",
			"Inspect the error; an *exec.ExitError carries the code.",
			"On *exec.ExitError return a result containing 'exit code: N' and the output, with no Go error.",
		},
		"bash-truncates-output": {
			"Output can be unbounded, so the result has a ceiling.",
			"Cap the combined output after the command finishes, or stream with a limit.",
			"If len(out) > limit, return out[:limit] plus a marker naming how much was shown.",
		},
		"bash-timeout-is-a-result": {
			"A command that never returns must not wedge the agent.",
			"Bound the run with a context, and treat that deadline as a result.",
			"context.WithTimeout plus exec.CommandContext. When runCtx hits DeadlineExceeded, return 'command timed out after ...' as the result, while a cancelled parent ctx still returns an error. The bound comes from the -tool-timeout flag.",
		},
	},
	"02": {
		"advertises-tools": {
			"The request carries the list of tools the model may ask for.",
			"Send a tools array, even when it is empty, and keep the field name exact.",
			`The body needs "tools": [] in this chapter. Chapter 03 fills it with the read tool.`,
		},
		"unknown-tool-is-a-result": {
			"The model can name a tool that does not exist, and it will.",
			"Look the name up, and when it is missing return a result instead of an error.",
			`Return a ToolResult with IsError set and text like "no such tool: <name>", so the model reads it and tries something else.`,
		},
		"invalid-arguments": {
			"The arguments are text the model wrote, and text can be malformed.",
			"Check that the arguments are valid JSON before decoding them.",
			`json.Valid([]byte(args)) fails; return a result saying the arguments are not valid JSON instead of calling the tool.`,
		},
		"folds-arguments": {
			"One call arrives as several fragments.",
			"Group fragments by their index, not by their id, and add the argument text together.",
			"Only the first fragment for a call has an id and a name. Map by index, append function.arguments, then emit in index order.",
		},
		"result-carries-call-id": {
			"A result and its call are two halves of one pair.",
			"Copy the id from the call into the tool message.",
			`The message needs role "tool", content, and tool_call_id equal to the call's id.`,
		},
		"tool-list-comes-from-the-registry": {
			"Do not advertise a tool nothing can run.",
			"Build the tool list from the registry instead of writing schemas by hand.",
			"Registry.Defs() walks the registered tools. A name in the request that is not in the registry is a tool the model will call and lose a turn on.",
		},
	},
	"03": {
		"advertises-read": {
			"A tool the model cannot see is a tool it will never call.",
			"Add the read tool to the registry and send its schema in the request.",
			"The schema needs name read, a description, and parameters {type:object, properties:{path:{type:string}}, required:[path]}.",
		},
		"reads-a-file": {
			"The result of a call is a message, not a return value.",
			"Append a message with role tool, the file contents, and the id of the call it answers.",
			"Read the file named by the path argument, return the text, and let the registry turn it into a tool message with the right id.",
		},
		"missing-file": {
			"A tool that fails is information, not a fatal error.",
			"Return the failure text as a tool result and let the model continue.",
			"os.IsNotExist becomes a result like 'no such file: <path>' with IsError set, and the program still exits 0 after the model's next reply.",
		},
		"refuses-escape": {
			"The model's path argument is untrusted input.",
			"Resolve the path against the workspace root and check the result, not the string.",
			"Compare filepath.Rel(root, resolved): reject when it is '..' or starts with '../'. Absolute paths are refused too.",
		},
		"truncates-large-file": {
			"Context is the scarce resource, so a file read is bounded.",
			"Read at most N bytes plus one, and if there is a byte left, cut and mark the cut.",
			"Return the first 20000 bytes followed by a marker such as '... [truncated: N of M bytes shown]'.",
		},
		"text-only-single-request": {
			"Most replies are not tool calls.",
			"Check the assistant message for tool calls before making another request.",
			"If len(msg.ToolCalls) == 0, print the content and return. One request, one answer.",
		},
	},
	"01": {
		"request-shape": {
			"Log the http.Request you build before you send it.",
			"The base URL already ends in /v1; you append /chat/completions. The model goes in the JSON body, the key in an Authorization header.",
			`Body: {"model": <model>, "messages": [{"role":..., "content":...}], "stream": true}. Header: "Authorization: Bearer " + APIKey.`,
		},
		"request-envelope": {
			"The request is an envelope, not just a body: method, path, three headers, and a JSON object with exactly the required keys.",
			"Set Content-Type: application/json and Accept: text/event-stream as well as Authorization. The body needs model, messages and stream; max_tokens and temperature only when they are non-zero.",
			"Use a pointer for optional numbers (*int, *float64) so zero means unset. With a plain int, `max_tokens: 0` is sent and a real provider reads it as 'produce nothing'.",
		},
		"optional-zero-values": {
			"Zero is not the same as unset.",
			"Use a pointer for optional numbers so a zero value can be omitted from the JSON entirely.",
			"`*int` with `omitempty`: when the caller leaves MaxTokens at 0, the key must not appear in the body at all.",
		},
		"prints-reply": {
			"The answer is printed, not returned, and it arrives in pieces.",
			"Keep adding each new piece of text, then print once at the end. If stdout is empty, check where the text is read from: the wire puts it inside choices[0].delta, not at the top level.",
			`Assigning instead of appending keeps only the last fragment. Use strings.Builder and write it out after the stream ends.`,
		},
		"debug-usage": {
			"stdout carries the answer only; diagnostics go to stderr.",
			"Read usage from the final chunk, then write it with fmt.Fprintf(os.Stderr, ...).",
			"Print a line containing prompt=, completion= and total= on stderr when HARNESS_DEBUG is set, and keep stdout to the answer plus one newline.",
		},
		"http-error": {
			"Check resp.StatusCode before you touch the body.",
			"A 429 is not a parse failure: wrap it in a typed error so callers can retry.",
			"Return *agent.HTTPError{Status: resp.StatusCode, Body: <up to 4 KiB read>}.",
		},
		"truncated-stream": {
			"bufio.Scanner stops on error and remembers it in scanner.Err().",
			"Check scanner.Err() after the loop, before returning success.",
			"An ignored scanner.Err() turns a dropped connection into a short but 'successful' answer.",
		},
		"request-timeout": {
			"The request must be cancellable; that only happens if it carries the context.",
			"Pass the ctx from run() into http.NewRequestWithContext, not context.Background().",
			"With a cancelled ctx, client.Do returns an error wrapping context.DeadlineExceeded. Return it so main exits 1.",
		},
		"sse-framing": {
			"SSE is a line protocol with more than data: lines in it.",
			"Skip lines that start with ':' (comments) and any field that is not 'data:'. Trim whitespace after the prefix, which also removes the CR in CRLF streams.",
			"Do not split the stream on a blank line as a unit: scan lines, skip everything that is not data:, and let blank lines be skipped like any other non-data line.",
		},
	},
}

// CasesFor returns the case ids that have a ladder in a chapter, sorted.
//
// The CLI uses it to turn a mistyped case id into a list of the real ones instead of
// a blank ladder.
func CasesFor(chapter string) []string {
	cases, ok := Table[chapter]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(cases))
	for id := range cases {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Get returns the hint ladder for a case. Unknown cases return the zero value,
// which the grader treats as "no ladder" and a completeness test treats as a bug.
func Get(chapter, caseID string) [3]string {
	if c, ok := Table[chapter]; ok {
		if h, ok := c[caseID]; ok {
			return h
		}
	}
	return [3]string{}
}
