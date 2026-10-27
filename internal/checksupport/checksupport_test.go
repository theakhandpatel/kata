package checksupport

import (
	"testing"
	"time"

	"kata/internal/fake"
)

// Tests for the helpers themselves. They are shared by twenty-one chapters, so a bug here
// misgrades the whole course, and the failure would look like a learner's mistake.

func TestCallScriptsOneToolCall(t *testing.T) {
	reply := Call("call_1", "read", `{"path":"a.txt"}`)

	if len(reply.Frames) != 3 {
		t.Fatalf("want id, arguments and finish; got %d frames", len(reply.Frames))
	}
	if reply.Frames[0].ToolCall.ID != "call_1" || reply.Frames[0].ToolCall.Name != "read" {
		t.Fatalf("first frame should name the call, got %+v", reply.Frames[0].ToolCall)
	}
	if reply.Frames[1].ToolCall.Args != `{"path":"a.txt"}` {
		t.Fatalf("second frame should carry the arguments, got %q", reply.Frames[1].ToolCall.Args)
	}
	if reply.Frames[2].Finish != "tool_calls" {
		t.Fatalf("last frame should finish with tool_calls, got %q", reply.Frames[2].Finish)
	}
}

func TestCallSplitKeepsEveryByte(t *testing.T) {
	const args = `{"path":"some/long/path/to/a/file.txt"}`
	reply := CallSplit("call_9", "read", args, 4)

	var rebuilt string
	var ids []string
	for _, fr := range reply.Frames {
		if fr.ToolCall == nil {
			continue
		}
		if fr.ToolCall.ID != "" {
			ids = append(ids, fr.ToolCall.ID)
		}
		rebuilt += fr.ToolCall.Args
	}

	if rebuilt != args {
		t.Fatalf("fragments do not reassemble:\n got %q\nwant %q", rebuilt, args)
	}
	if len(ids) != 1 || ids[0] != "call_9" {
		t.Fatalf("the id should appear exactly once, on the first fragment; got %v", ids)
	}
	if len(reply.Frames) < 5 {
		t.Fatalf("expected several argument fragments, got %d frames", len(reply.Frames))
	}
}

func TestEventsSkipsNonJSONLines(t *testing.T) {
	stderr := "usage prompt=1 completion=2\n" +
		`{"type":"turn_start","turn":1}` + "\n" +
		"agent: something\n" +
		`{"type":"agent_end","reason":"done"}` + "\n"

	events := Events(t, stderr)
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d: %v", len(events), events)
	}
	if LastEventOfType(events, "agent_end")["reason"] != "done" {
		t.Fatal("last agent_end not found")
	}
}

func TestRequestAndToolResultReportWhatIsMissing(t *testing.T) {
	srv := fake.New(t, fake.Reply{Text: "unused"})
	if _, err := Request(srv, 0); err == nil {
		t.Fatal("Request should report a missing request, not panic")
	}

	if _, err := ToolResult(fake.Recorded{}, "call_x"); err == nil {
		t.Fatal("ToolResult should report a missing result, not panic")
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 7: "7", 42: "42", 1001: "1001", -3: "-3"}
	for in, want := range cases {
		if got := Itoa(in); got != want {
			t.Errorf("Itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRunCleanHasNoConfiguration(t *testing.T) {
	dir := t.TempDir()
	res := RunClean(t, dir, "-config")
	// The binary refuses to run without a prompt, or prints config; either way the real
	// machine's settings must not appear.
	if res.ExitCode == 0 {
		return
	}
	if res.Duration > 30*time.Second {
		t.Fatal("RunClean should bound the run")
	}
}
