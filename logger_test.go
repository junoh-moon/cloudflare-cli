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

type testLogger struct {
	*Logger
	stdout bytes.Buffer
	stderr bytes.Buffer
	path   string
}

func newTestLogger(t *testing.T) *testLogger {
	t.Helper()
	l := &testLogger{path: filepath.Join(t.TempDir(), ".dns.log")}
	l.Logger = NewLogger(&l.stdout, &l.stderr, l.path)
	t.Cleanup(l.close)
	return l
}

func TestLoggerWritesInfoLineToStdoutAndFile(t *testing.T) {
	logger := newTestLogger(t)

	logger.info("music.example.com", `{"success":true}`)

	want := regexp.MustCompile(`^` + timestampPattern + ` INFO  music\.example\.com - \{"success":true\}\n$`)
	if !want.MatchString(logger.stdout.String()) {
		t.Errorf("stdout: got %q", logger.stdout.String())
	}
	if got := readFile(t, logger.path); got != logger.stdout.String() {
		t.Errorf("file: got %q, want same as stdout %q", got, logger.stdout.String())
	}
	if logger.stderr.Len() != 0 {
		t.Errorf("stderr: got %q, want nothing", logger.stderr.String())
	}
}

func TestLoggerWritesWarnLine(t *testing.T) {
	logger := newTestLogger(t)

	logger.warn("music.example.com", "Update failed: zone not found")

	want := regexp.MustCompile(`^` + timestampPattern + ` WARN  music\.example\.com - Update failed: zone not found\n$`)
	if !want.MatchString(logger.stdout.String()) {
		t.Errorf("got %q", logger.stdout.String())
	}
}

func TestLoggerWritesErrorLineToStderrAndFile(t *testing.T) {
	logger := newTestLogger(t)

	logger.error("example.com", "IP lookup failed")

	want := regexp.MustCompile(`^` + timestampPattern + ` ERROR example\.com - IP lookup failed\n$`)
	if !want.MatchString(logger.stderr.String()) {
		t.Errorf("stderr: got %q", logger.stderr.String())
	}
	if got := readFile(t, logger.path); got != logger.stderr.String() {
		t.Errorf("file: got %q, want same as stderr %q", got, logger.stderr.String())
	}
	if logger.stdout.Len() != 0 {
		t.Errorf("stdout: got %q, want nothing", logger.stdout.String())
	}
}

func TestLoggerKeepsMessageOnOneLine(t *testing.T) {
	logger := newTestLogger(t)

	logger.info("example.com", "{\r\n  \"success\": true\n}\n")

	if !strings.HasSuffix(logger.stdout.String(), " - {  \"success\": true}\n") {
		t.Errorf("got %q", logger.stdout.String())
	}
}

func TestLoggerAppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".dns.log")
	if err := os.WriteFile(path, []byte("previous line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logger := NewLogger(&bytes.Buffer{}, &bytes.Buffer{}, path)
	defer logger.close()

	logger.info("example.com", "new")

	got := readFile(t, path)
	if !strings.HasPrefix(got, "previous line\n") || !strings.HasSuffix(got, " - new\n") {
		t.Errorf("got %q", got)
	}
}

func TestLoggerStillWritesToStdoutWhenFileCannotBeOpened(t *testing.T) {
	var stdout bytes.Buffer
	path := filepath.Join(t.TempDir(), "missing-dir", ".dns.log")
	logger := NewLogger(&stdout, &bytes.Buffer{}, path)
	defer logger.close()

	logger.info("example.com", "hello")

	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), stdout.String())
	}
	if !strings.Contains(lines[0], " WARN  "+path+" - cannot open log file") {
		t.Errorf("first line: got %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], " INFO  example.com - hello") {
		t.Errorf("second line: got %q", lines[1])
	}
}
