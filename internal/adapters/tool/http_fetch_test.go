package tool

import "testing"

func TestSSRFBlocks(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/",
		"http://10.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost/",
		"file:///etc/passwd",
	}
	for _, u := range cases {
		if err := validatePublicURL(u); err == nil {
			t.Fatalf("expected block for %s", u)
		}
	}
}

func TestSSRFAllowsExample(t *testing.T) {
	// May fail offline; only check parser accepts public hostname shape without resolving if needed.
	// Use a hostname that resolves publicly when network available; skip on DNS fail.
	err := validatePublicURL("https://example.com/")
	if err != nil && !contains(err.Error(), "dns") {
		t.Fatal(err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()))
}
