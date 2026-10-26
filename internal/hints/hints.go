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

// Table maps chapter number -> case id -> the three rungs.
var Table = map[string]map[string][3]string{
	"05": {
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
	"04": {
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
	"03": {
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
		"advertises-read": {
			"A tool the model cannot see is a tool it will never call.",
			"Send the schema list in the request body, with name, description and a JSON Schema for the parameters.",
			"Registry.Defs() must return the read tool with parameters {type:object, properties:{path:{type:string}}, required:[path]}.",
		},
		"reads-a-file": {
			"The result of a call is a message, not a return value.",
			"Append a message with role tool, the file contents, and the id of the call it answers.",
			"tool_call_id must equal the id from the assistant message. Order the results the same way as the calls.",
		},
		"folds-arguments": {
			"Argument text arrives in pieces, and only the first piece names the call.",
			"Key the accumulator by the index field, not by the id.",
			"Append every fragment's function.arguments into one buffer per index, then join the calls in index order.",
		},
		"missing-file": {
			"A tool that fails is information, not a fatal error.",
			"Return the failure text as a tool result and let the model continue.",
			"os.IsNotExist becomes a result like 'no such file: <path>', with IsError set; the program still exits 0 after the model's next reply.",
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
		"invalid-arguments": {
			"The model writes the arguments as text, and text can be malformed.",
			"Validate before decoding, and turn a malformed call into an error result.",
			"json.Valid([]byte(args)) fails; return a result saying the arguments are not valid JSON instead of calling the tool.",
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
			"The answer is printed, not returned. text arrives in pieces.",
			"Accumulate every delta, then print once at the end with fmt.Println.",
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
