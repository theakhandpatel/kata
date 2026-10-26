// Package hints is course content: the three-rung hint ladder for every graded
// case.
//
// Rung 1 names the mechanism (a nudge). Rung 2 points at the specific API or
// line to look at. Rung 3 states the answer. The ladder exists so that being
// stuck costs you a nudge, not the exercise.
//
// Hints live here rather than beside each check so that `assay hint` can print
// them without parsing Go source. The type is a fixed [3]string rather than a
// slice because a missing rung must be a compile error, not an invisible gap in
// the ladder.
package hints

// Table maps chapter number -> case id -> the three rungs.
var Table = map[string]map[string][3]string{
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
			"Return *harness.HTTPError{Status: resp.StatusCode, Body: <up to 4 KiB read>}.",
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
