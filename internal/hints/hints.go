// Package hints is course content: the three-tier hint ladder for every graded
// case.
//
// Tier 1 names the mechanism (a nudge). Tier 2 points at the specific API or
// line to look at. Tier 3 states the answer. The ladder exists so that being
// stuck costs you a nudge, not the exercise.
//
// Hints live here rather than beside each check so that `craft hint` can print
// them without parsing Go source.
package hints

// Table maps chapter number -> case id -> hint tiers.
var Table = map[string]map[string][]string{
	"01": {
		"request-shape": {
			"Log the http.Request you build before you send it.",
			"The base URL already ends in /v1; you append /chat/completions. The model goes in the JSON body, the key in an Authorization header.",
			`Body: {"model": <model>, "messages": [{"role":..., "content":...}], "stream": true}. Header: "Authorization: Bearer " + APIKey.`,
		},
		"text-reassembly": {
			"Each data: line is a partial update, not a full message.",
			"Accumulate with strings.Builder or []byte, then convert once at the end.",
			`Payloads end with "data: [DONE]"; that line is not JSON and must be skipped before decoding.`,
		},
		"usage": {
			"Usage rides on the last chunk only.",
			"Decode every chunk into the same struct; a later non-nil usage overwrites the zero value.",
			"Fields sit at the top level of the chunk: usage.prompt_tokens, usage.completion_tokens, usage.total_tokens.",
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
		"context-cancel": {
			"Pass the caller's ctx into the request, not context.Background().",
			"http.NewRequestWithContext is the only place the context enters.",
			"With a cancelled ctx, client.Do returns an error that wraps context.Canceled.",
		},
	},
}

// Get returns the hint ladder for a case, or nil if unknown.
func Get(chapter, caseID string) []string {
	if c, ok := Table[chapter]; ok {
		return c[caseID]
	}
	return nil
}
