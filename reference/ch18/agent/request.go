package agent

// BuildRequestBody turns a Request into the JSON object a provider expects.
//
// The translation is small and every line of it is a decision: which fields are always
// present, which are omitted when unset, and which order they appear in.
func BuildRequestBody(model string, req Request) map[string]any {
	body := map[string]any{
		"model":    model,
		"messages": toWireMessages(req.Messages),
		"stream":   req.Stream,
	}

	// Absent, not zero. A provider reads "max_tokens": 0 as "produce nothing" and
	// "temperature": 0 as a deliberate setting, so neither may be sent unless chosen.
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}

	// An empty tool list is still a tool list: a server distinguishes "no tools" from
	// "this field was not sent" and some reject the request without it.
	if req.Tools != nil {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		body["tools"] = tools
	}

	return body
}
