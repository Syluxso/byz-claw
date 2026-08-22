package telegram

import "testing"

func TestAllowed(t *testing.T) {
	allow := []string{"12345", "@alice"}
	if !allowed(allow, "12345") {
		t.Fatal("chat id should allow")
	}
	if !allowed(allow, "", "999", "alice") {
		t.Fatal("username should allow")
	}
	if allowed(allow, "999", "bob") {
		t.Fatal("should deny")
	}
}
