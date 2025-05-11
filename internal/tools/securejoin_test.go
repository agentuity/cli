package tools

import "testing"

func TestSecureJoin(t *testing.T) {
	base := "/tmp/project"
	good, err := secureJoin(base, "sub/file.txt")
	if err != nil || good != "/tmp/project/sub/file.txt" {
		t.Fatalf("expected valid join, got %s, err %v", good, err)
	}

	_, err = secureJoin(base, "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal not received")
	}
}
