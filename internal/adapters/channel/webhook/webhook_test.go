package webhook

import "testing"

func TestAssertBindSafe(t *testing.T) {
	if err := assertBindSafe("127.0.0.1:8743", false); err != nil {
		t.Fatal(err)
	}
	if err := assertBindSafe("0.0.0.0:8743", false); err == nil {
		t.Fatal("expected public bind without allow_public to fail")
	}
	if err := assertBindSafe("0.0.0.0:8743", true); err != nil {
		t.Fatal(err)
	}
}
