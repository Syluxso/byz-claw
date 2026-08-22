package core

import "testing"

func TestParseSkillMarkdown(t *testing.T) {
	raw := `---
id: daily-note
name: Daily note
tools: [workspace_read, workspace_write, memory_write]
---
When the user asks to jot a note, write markdown under workspace/notes/.
`
	sk, err := ParseSkillMarkdown(raw)
	if err != nil {
		t.Fatal(err)
	}
	if sk.ID != "daily-note" || sk.Name != "Daily note" {
		t.Fatalf("%+v", sk)
	}
	if len(sk.Tools) != 3 {
		t.Fatalf("tools=%v", sk.Tools)
	}
	if sk.Body == "" {
		t.Fatal("empty body")
	}
}
