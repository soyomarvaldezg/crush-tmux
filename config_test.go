package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPrintsTmuxSnippet(t *testing.T) {
	out := captureStdout(t, func() {
		configCmd()
	})

	if !strings.Contains(out, "set -g focus-events on") {
		t.Errorf("config output missing 'set -g focus-events on', got:\n%s", out)
	}
	if !strings.Contains(out, "status-right") {
		t.Errorf("config output missing status-right line, got:\n%s", out)
	}
	if !strings.Contains(out, "mark-viewed") {
		t.Errorf("config output missing mark-viewed hook, got:\n%s", out)
	}
	if !strings.Contains(out, "crush-switcher.sh") {
		t.Errorf("config output missing switcher reference, got:\n%s", out)
	}
}

func TestConfigUsesOwnBinaryPath(t *testing.T) {
	out := captureStdout(t, func() {
		configCmd()
	})

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() failed: %v", err)
	}
	if !strings.Contains(out, exe) {
		t.Errorf("config output should contain own binary path %q, got:\n%s", exe, out)
	}
}

func TestEnsureSwitcherScriptCreatesFile(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "crush-switcher.sh")

	err := ensureSwitcherScript(scriptPath)
	if err != nil {
		t.Fatalf("ensureSwitcherScript failed: %v", err)
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("reading switcher script: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "#!/usr/bin/env bash") {
		t.Error("switcher script missing shebang")
	}
	if !strings.Contains(content, "fzf") {
		t.Error("switcher script missing fzf invocation")
	}
	if !strings.Contains(content, "switch-client") {
		t.Error("switcher script missing switch-client command")
	}

	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat switcher script: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("switcher script is not executable")
	}
}

func TestEnsureSwitcherScriptIdempotent(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "crush-switcher.sh")

	if err := ensureSwitcherScript(scriptPath); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	first, _ := os.ReadFile(scriptPath)

	if err := ensureSwitcherScript(scriptPath); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	second, _ := os.ReadFile(scriptPath)

	if string(first) != string(second) {
		t.Error("ensureSwitcherScript is not idempotent — content changed on second call")
	}
}

func TestEnsureSwitcherScriptCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "subdir", "nested", "crush-switcher.sh")

	err := ensureSwitcherScript(scriptPath)
	if err != nil {
		t.Fatalf("ensureSwitcherScript with nested path failed: %v", err)
	}

	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Error("switcher script was not created in nested directory")
	}
}

// captureStdout redirects stdout to a pipe, runs fn, and returns captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	origStdout := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
			}
			if readErr != nil {
				break
			}
		}
		done <- b.String()
	}()

	fn()

	w.Close()
	os.Stdout = origStdout
	return <-done
}
