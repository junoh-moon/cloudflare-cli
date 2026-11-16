package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const timestampPattern = `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}`

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestLoggerWritesInfoLineToStdoutAndFile(t *testing.T) {
	var out bytes.Buffer
	path := filepath.Join(t.TempDir(), ".dns.log")
	logger := NewLogger(&out, path)
	defer logger.close()

	logger.info("music.example.com", `{"success":true}`)

	want := regexp.MustCompile(`^` + timestampPattern + ` INFO  music\.example\.com - \{"success":true\}\n$`)
	if !want.MatchString(out.String()) {
		t.Errorf("stdout: got %q", out.String())
	}
	if got := readFile(t, path); got != out.String() {
		t.Errorf("file: got %q, want same as stdout %q", got, out.String())
	}
}

func TestLoggerWritesWarnLine(t *testing.T) {
	var out bytes.Buffer
	logger := NewLogger(&out, filepath.Join(t.TempDir(), ".dns.log"))
	defer logger.close()

	logger.warn("music.example.com", "Update failed: zone not found")

	want := regexp.MustCompile(`^` + timestampPattern + ` WARN  music\.example\.com - Update failed: zone not found\n$`)
	if !want.MatchString(out.String()) {
		t.Errorf("got %q", out.String())
	}
}

func TestLoggerKeepsMessageOnOneLine(t *testing.T) {
	var out bytes.Buffer
	logger := NewLogger(&out, filepath.Join(t.TempDir(), ".dns.log"))
	defer logger.close()

	logger.info("example.com", "{\r\n  \"success\": true\n}\n")

	if !strings.HasSuffix(out.String(), " - {  \"success\": true}\n") {
		t.Errorf("got %q", out.String())
	}
}

func TestLoggerAppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".dns.log")
	if err := os.WriteFile(path, []byte("previous line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logger := NewLogger(&bytes.Buffer{}, path)
	defer logger.close()

	logger.info("example.com", "new")

	got := readFile(t, path)
	if !strings.HasPrefix(got, "previous line\n") || !strings.HasSuffix(got, " - new\n") {
		t.Errorf("got %q", got)
	}
}

func TestLoggerStillWritesToStdoutWhenFileCannotBeOpened(t *testing.T) {
	var out bytes.Buffer
	path := filepath.Join(t.TempDir(), "missing-dir", ".dns.log")
	logger := NewLogger(&out, path)
	defer logger.close()

	logger.info("example.com", "hello")

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), out.String())
	}
	if !strings.Contains(lines[0], " WARN  "+path+" - cannot open log file") {
		t.Errorf("first line: got %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], " INFO  example.com - hello") {
		t.Errorf("second line: got %q", lines[1])
	}
}
