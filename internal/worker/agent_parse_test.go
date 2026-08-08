package worker

import (
	"encoding/json"
	"testing"
)

func TestParseActionJSON(t *testing.T) {
	reply := `{"tool":"file_read","args":{"path":"main.go"}}`
	act, err := parseAction(reply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Tool != "file_read" {
		t.Fatalf("expected tool file_read, got %q", act.Tool)
	}
	path, ok := act.Args["path"].(string)
	if !ok || path != "main.go" {
		t.Fatalf("expected args.path=main.go, got %v", act.Args)
	}
}

func TestParseActionJSONWithProse(t *testing.T) {
	reply := `Sure, I'll read the file for you.
` + "```json\n" + `{"tool":"file_read","args":{"path":"main.go"}}` + "\n```"
	act, err := parseAction(reply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Tool != "file_read" {
		t.Fatalf("expected tool file_read, got %q", act.Tool)
	}
}

func TestParseActionXMLFallback(t *testing.T) {
	reply := `<function=file_read>
<parameter=path>main.go</parameter>
</function>`
	act, err := parseAction(reply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Tool != "file_read" {
		t.Fatalf("expected tool file_read, got %q", act.Tool)
	}
	path, ok := act.Args["path"].(string)
	if !ok || path != "main.go" {
		t.Fatalf("expected args.path=main.go, got %v", act.Args)
	}
}

func TestParseActionXMLMultipleParams(t *testing.T) {
	reply := `<function=file_write>
<parameter=path>main.go</parameter>
<parameter=content>package main</parameter>
</function>`
	act, err := parseAction(reply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Tool != "file_write" {
		t.Fatalf("expected tool file_write, got %q", act.Tool)
	}
	if act.Args["path"] != "main.go" {
		t.Fatalf("expected path main.go, got %v", act.Args["path"])
	}
	if act.Args["content"] != "package main" {
		t.Fatalf("expected content, got %v", act.Args["content"])
	}
}

func TestParseActionXMLMultilineContent(t *testing.T) {
	want := "line1\nline2\nline3"
	reply := `<function=file_write>
<parameter=path>main.go</parameter>
<parameter=content>` + want + `</parameter>
</function>`
	act, err := parseAction(reply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Args["content"] != want {
		t.Fatalf("multiline content corrupted:\nwant %q\ngot  %q", want, act.Args["content"])
	}
}

func TestParseActionJSONPreferredOverXML(t *testing.T) {
	reply := `{"tool":"file_read","args":{"path":"main.go"}}` + `
<function=other>
<parameter=x>y</parameter>
</function>`
	act, err := parseAction(reply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Tool != "file_read" {
		t.Fatalf("expected JSON action to win, got %q", act.Tool)
	}
}

func TestParseActionInvalid(t *testing.T) {
	reply := `this is just prose with no tool call`
	if _, err := parseAction(reply); err == nil {
		t.Fatal("expected error for prose-only reply")
	}
}

func TestParseActionDoneJSON(t *testing.T) {
	reply := `{"done":true,"summary":"completed"}`
	act, err := parseAction(reply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !act.Done {
		t.Fatal("expected done=true")
	}
	if act.Summary != "completed" {
		t.Fatalf("expected summary completed, got %q", act.Summary)
	}
}

func TestDecodeActionFlattenedArgs(t *testing.T) {
	raw := []byte(`{"tool":"file_write","path":"main.go","content":"package main"}`)
	act, err := decodeAction(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Args["path"] != "main.go" {
		t.Fatalf("expected flattened path, got %v", act.Args)
	}
	if act.Args["content"] != "package main" {
		t.Fatalf("expected flattened content, got %v", act.Args)
	}
}

func TestDecodeActionNestedJSON(t *testing.T) {
	// JSON containing nested braces inside a string value must parse correctly.
	payload := map[string]string{"nested": `{"a": {"b": 1}}`}
	raw, _ := json.Marshal(map[string]any{
		"tool": "file_write",
		"args": map[string]any{
			"path":    "main.go",
			"content": payload["nested"],
		},
	})
	act, err := parseAction(string(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if act.Args["content"] != payload["nested"] {
		t.Fatalf("nested JSON string corrupted: %v", act.Args["content"])
	}
}
