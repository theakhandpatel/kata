package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

// MCPProtocolVersion is the version this client speaks. A server may answer with a
// different one; we accept it and continue, because the two versions in use differ
// in features, not in framing.
const MCPProtocolVersion = "2024-11-05"

// MCPClient is a client for a Model Context Protocol server over stdio.
//
// The transport is the simplest part of MCP and the part worth teaching: a
// subprocess, one JSON object per line, requests with ids and responses carrying the
// same id. No HTTP, no sockets, no SDK.
type MCPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	serverName string
	tools      []MCPToolInfo
}

// MCPToolInfo is one tool as the server describes it.
type MCPToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpResponse struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// StartMCP launches a server and returns a client talking to it.
//
// The command is run through `sh -c`, so a flag value can carry arguments. The
// server's stderr is discarded: stdout is the protocol and must stay clean.
func StartMCP(ctx context.Context, command string) (*MCPClient, error) {
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("mcp command is empty")
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mcp server: %w", err)
	}

	return &MCPClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReaderSize(stdout, 1<<20),
	}, nil
}

// ServerName is the name the server reported, sanitised for use in tool names.
func (c *MCPClient) ServerName() string { return c.serverName }

// Tools returns the server's tools wrapped as agent tools, namespaced by server name.
func (c *MCPClient) Tools() []Tool {
	out := make([]Tool, 0, len(c.tools))
	for _, info := range c.tools {
		out = append(out, &mcpTool{client: c, info: info, server: c.serverName})
	}
	return out
}

// Initialize performs the MCP handshake: initialize, then the initialized
// notification.
func (c *MCPClient) Initialize(ctx context.Context) error {
	result, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": MCPProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "miyagi-agent", "version": "1.0"},
	})
	if err != nil {
		return err
	}

	var init struct {
		ServerInfo struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	_ = json.Unmarshal(result, &init)
	c.serverName = sanitizeName(init.ServerInfo.Name)
	if c.serverName == "" {
		c.serverName = "mcp"
	}

	// A notification: no id, no response. Sending it is required by the protocol and
	// easy to forget, because nothing fails immediately when you skip it.
	return c.notify("notifications/initialized", nil)
}

// ListTools fetches and caches the server's tool list.
func (c *MCPClient) ListTools(ctx context.Context) error {
	result, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return err
	}
	var list struct {
		Tools []MCPToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		return fmt.Errorf("decode tools/list: %w", err)
	}
	c.tools = list.Tools
	return nil
}

// CallTool invokes one tool on the server and joins its text content blocks.
func (c *MCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var arguments any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return "", fmt.Errorf("mcp arguments: %w", err)
		}
	}
	result, err := c.call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		return "", err
	}

	var call struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &call); err != nil {
		return "", fmt.Errorf("decode tools/call: %w", err)
	}

	var parts []string
	for _, block := range call.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	text := strings.Join(parts, "\n")
	if text == "" {
		text = "(the server returned no text content)"
	}
	if call.IsError {
		return "", errors.New(text)
	}
	return text, nil
}

// Close shuts the server down, then closes its stdin so it exits.
func (c *MCPClient) Close() error {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
	return nil
}

// call sends one request and reads until the matching response arrives.
//
// Notifications may arrive between requests; anything whose id does not match is
// skipped. Requests are serialised: one in flight at a time, which is enough for a
// harness that calls tools in order.
func (c *MCPClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := nextMCPID()

	payload := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		payload["params"] = params
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(c.stdin, "%s\n", body); err != nil {
		return nil, fmt.Errorf("write %s: %w", method, err)
	}

	type reply struct {
		result json.RawMessage
		err    error
	}
	done := make(chan reply, 1)

	go func() {
		for {
			line, err := c.stdout.ReadBytes('\n')
			if err != nil {
				done <- reply{err: err}
				return
			}
			var resp mcpResponse
			if json.Unmarshal(line, &resp) != nil {
				continue
			}
			if resp.ID == nil || *resp.ID != id {
				continue // a notification, or an answer to a request we abandoned
			}
			if resp.Error != nil {
				done <- reply{err: fmt.Errorf("mcp %s: %s (%d)", method, resp.Error.Message, resp.Error.Code)}
				return
			}
			done <- reply{result: resp.Result}
			return
		}
	}()

	select {
	case r := <-done:
		return r.result, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("mcp %s: %w", method, ctx.Err())
	}
}

func (c *MCPClient) notify(method string, params any) error {
	payload := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		payload["params"] = params
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.stdin, "%s\n", body); err != nil {
		return fmt.Errorf("write %s: %w", method, err)
	}
	return nil
}

var mcpID = 0

func nextMCPID() int {
	mcpID++
	return mcpID
}

var unsafeName = regexp.MustCompile(`[^a-z0-9-]+`)

// sanitizeName makes a server name safe for use inside a tool name.
func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = unsafeName.ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}

// mcpTool adapts one server tool to the agent's Tool interface.
type mcpTool struct {
	client *MCPClient
	info   MCPToolInfo
	server string
}

// Name is namespaced. A server that exposes `read` must not shadow the built-in
// `read`, and the only reliable way to prevent that is to include the server.
func (t *mcpTool) Name() string { return t.server + "." + t.info.Name }

func (t *mcpTool) Description() string {
	if t.info.Description == "" {
		return "Tool from the " + t.server + " MCP server."
	}
	return t.info.Description
}

func (t *mcpTool) Parameters() map[string]any {
	if t.info.InputSchema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return t.info.InputSchema
}

func (t *mcpTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	return t.client.CallTool(ctx, t.info.Name, input)
}
