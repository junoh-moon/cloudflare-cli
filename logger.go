package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Logger writes every line to the terminal and to a log file that is only
// ever appended to. INFO and WARN go to stdout, ERROR to stderr.
//
// Line format: "2006-01-02 15:04:05.000 LEVEL subject - message"
type Logger struct {
	stdout io.Writer
	stderr io.Writer
	file   *os.File // nil when the log file could not be opened
}

func NewLogger(stdout, stderr io.Writer, path string) *Logger {
	logger := &Logger{stdout: stdout, stderr: stderr}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		logger.warn(path, "cannot open log file: "+err.Error())
		return logger
	}
	logger.file = file
	return logger
}

func (l *Logger) info(subject, message string) {
	l.write(l.stdout, "INFO", subject, message)
}

func (l *Logger) warn(subject, message string) {
	l.write(l.stdout, "WARN", subject, message)
}

func (l *Logger) error(subject, message string) {
	l.write(l.stderr, "ERROR", subject, message)
}

func (l *Logger) close() {
	if l.file != nil {
		l.file.Close()
	}
}

var lineBreaks = strings.NewReplacer("\r", "", "\n", "")

func (l *Logger) write(terminal io.Writer, level, subject, message string) {
	line := fmt.Sprintf("%s %-5s %s - %s\n",
		time.Now().Format("2006-01-02 15:04:05.000"),
		level,
		subject,
		lineBreaks.Replace(message),
	)

	io.WriteString(terminal, line)
	if l.file != nil {
		io.WriteString(l.file, line)
	}
}
