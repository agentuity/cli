package tools

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSReadListEdit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// list_files
	lf := FSList(root)
	out, err := lf.Exec(json.RawMessage(`{}`))
	if err != nil || !strings.Contains(out, "a.txt") {
		t.Fatalf("list_files failed: %v, %s", err, out)
	}

	// read_file
	rf := FSRead(root)
	out, err = rf.Exec(json.RawMessage(`{"path":"a.txt"}`))
	if err != nil || !strings.Contains(out, "hello") {
		t.Fatalf("read_file failed: %v, %s", err, out)
	}

	// edit_file (append)
	ef := FSEdit(root)
	payload := `{"path":"a.txt","old_str":"hello","new_str":"hi"}`
	_, err = ef.Exec(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("edit_file exec: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if !strings.Contains(string(data), "hi") {
		t.Fatalf("edit failed, content: %s", data)
	}
}

func TestGrep(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "b.go"), []byte("package main\n// TODO: fix"), 0644)
	grep := Grep(root)
	out, err := grep.Exec(json.RawMessage(`{"pattern":"TODO"}`))
	if err != nil || !strings.Contains(out, "b.go") {
		t.Fatalf("grep failed: %v, %s", err, out)
	}
}

func TestGitDiff(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	cmd := exec.Command("git", "-C", root, "init")
	cmd.Run()
	os.WriteFile(filepath.Join(root, "c.txt"), []byte("x"), 0644)
	diffTool := GitDiff(root)
	out, err := diffTool.Exec(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("git diff exec: %v", err)
	}
	if !strings.Contains(out, "diff") {
		t.Fatalf("unexpected diff output: %s", out)
	}
}
