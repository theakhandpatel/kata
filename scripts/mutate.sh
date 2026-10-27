#!/usr/bin/env bash
# Mutation test: break one thing, see whether the grader notices.
#
# Each mutation is a plausible mistake a learner could make, not a random edit. A mutation
# that survives means the chapter's checks do not grade that behaviour.
#
# Usage: ./scripts/mutate.sh [chapter ...]

set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

KATA="${KATA:-./kata}"
[ -x "$KATA" ] || { echo "build it first: go build -o kata ./cmd/kata"; exit 1; }

# work in a scratch copy so the real tree is never touched
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

cp -r . "$WORK/tree"
cd "$WORK/tree"
rm -rf .git .kata site

survived=0
killed=0
skipped=0
total=0

run_mutation() {
	local chapter="$1" name="$2" file="$3" from="$4" to="$5"
	local path="agent/$file"

	# The state "after chapter NN" is the starter plus chapters 1..NN. Applying every
	# overlay would leave the last chapter's main.go in charge, and the mutation would then
	# be tested against a different program than the chapter grades.
	rm -rf agent
	cp -r starter/agent agent
	for i in $(seq -w 1 21); do
		[ "$i" \> "$chapter" ] && break
		[ -d "reference/ch$i" ] && cp -r "reference/ch$i/." .
	done

	if [ ! -f "$path" ]; then
		echo "  SKIP  $chapter $name (no $path)"
		return
	fi

	if ! grep -qF "$from" "$path"; then
		echo "  SKIP  $chapter $name (pattern not found in $path)"
		skipped=$((skipped + 1))
		return
	fi

	# apply the mutation
	python3 - "$path" "$from" "$to" <<-'PY'
		import sys
		path, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
		s = open(path).read()
		open(path, "w").write(s.replace(old, new, 1))
	PY

	total=$((total + 1))
	if timeout 240 "$KATA" submit "$chapter" > /tmp/mut.$chapter.$name.log 2>&1; then
		echo "  SURVIVED  $chapter  $name   <- the checks did not notice"
		survived=$((survived + 1))
	else
		echo "  killed    $chapter  $name"
		killed=$((killed + 1))
	fi
}

echo
echo "mutation testing the grader"
echo

# Each block: chapter, label, file, original text, mutated text.
run_mutation 01 append-to-assign provider_openai.go \
	'content.WriteString(choice.Delta.Content)' 'content.Reset(); content.WriteString(choice.Delta.Content)'
run_mutation 01 drop-scanner-err provider_openai.go \
	'if err := scanner.Err(); err != nil {' 'if err := scanner.Err(); false && err != nil {'
run_mutation 02 keep-call-by-id provider_openai.go \
	'acc := calls[tc.Index]' 'acc := calls[0]'
run_mutation 02 accept-any-arguments tool.go \
	'if !json.Valid([]byte(call.Arguments)) {' 'if false {'

run_mutation 03 no-truncation tools.go \
	'if n > limit {' 'if false {'
run_mutation 03 allow-escape tools.go \
	'if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {' 'if false {'

run_mutation 05 no-step-cap cmd/agent/main.go \
	'flag.Int("max-steps", agent.DefaultMaxSteps,' 'flag.Int("max-steps", 1<<30,'
run_mutation 05 results-sorted loop.go \
	'history = append(history, Message{
				Role:       RoleTool,' 'history = append([]Message{}, Message{
				Role:       RoleTool,'

run_mutation 06 unstable-prefix prompt.go \
	'b.WriteString(strings.TrimRight(base, "\n"))' 'b.WriteString(strings.TrimRight(base, "\n") + time.Now().String())'
run_mutation 06 nearest-last prompt.go \
	'for i, j := 0, len(found)-1; i < j; i, j = i+1, j-1 {
		found[i], found[j] = found[j], found[i]
	}' ''

run_mutation 07 duplicate-on-resume cmd/agent/main.go \
	'res.History[len(prior)+1:]' 'res.History[len(prior):]'

run_mutation 08 no-ceiling budget.go \
	'if EstimateTokens(msgs, tools) <= budget {
		return msgs, 0
	}' 'return msgs, 0'
run_mutation 08 orphan-tool-result budget.go \
	'for len(kept) > 0 && kept[0].Role == RoleTool {
		kept = kept[1:]
		dropped++
	}' ''

run_mutation 09 abort-exits-1 cmd/agent/main.go \
	'if errors.Is(err, context.Canceled) || interrupted {' 'if false {'
run_mutation 09 steer-in-prompt loop.go \
	'history = append(history, m)
				steered = true' 'steered = false'

# Chapter 11 ships its own skill.go, which the overlay copies after chapter 10. The mutation
# therefore has to be applied to that later copy to survive the overlay.
run_mutation 10 body-advertised skill.go \
	'fmt.Fprintf(&b, "\n- %s: %s\n", s.Name, s.Description)' 'fmt.Fprintf(&b, "\n- %s: %s\n%s\n", s.Name, s.Description, s.Body)'
run_mutation 11 no-argument-substitution skill.go \
	'out := strings.ReplaceAll(body, "$ARGUMENTS", args)' 'out := body'

run_mutation 12 child-inherits-history subagent.go \
	'history := []Message{
		{Role: RoleSystem, Content: t.System},
		{Role: RoleUser, Content: args.Prompt},
	}' 'history := append(parentHistory, Message{Role: RoleUser, Content: args.Prompt})'

run_mutation 14 retry-everything retry.go \
	'case he.Status >= 500:' 'case he.Status >= 400:'
run_mutation 15 no-handle spill.go \
	'if len(content) <= limit {
		return content, false, nil
	}' 'return content, false, nil'

run_mutation 16 no-ceiling mount.go \
	'if len(out) >= m.max {
			break
		}' ''
run_mutation 17 user-beats-project scoped.go \
	'Priority: 20,' 'Priority: 5,'

run_mutation 18 send-zero-optionals request.go \
	'if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}' 'body["max_tokens"] = req.MaxTokens'

run_mutation 19 reset-ordinal store.go \
	'SELECT COALESCE(MAX(ordinal)+1, 0) FROM messages WHERE session_id = ?' 'SELECT 0 FROM messages WHERE session_id = ?'
run_mutation 20 wrong-price usage.go \
	'InputMicros: 150, OutputMicros: 600, CacheMicros: 15' 'InputMicros: 1, OutputMicros: 1, CacheMicros: 1'

run_mutation 21 flag-loses-to-file config.go \
	'applyEnv(&cfg, env)
	applyFile(&cfg, flags, "flag")' 'applyFile(&cfg, flags, "flag")
	applyEnv(&cfg, env)'

echo
echo "  $killed killed, $survived survived, $skipped skipped, $total applied"
echo

[ "$survived" -eq 0 ] || exit 1
