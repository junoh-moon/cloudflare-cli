package main

import (
	"strings"

	"golang.org/x/text/width"
)

const horizontalLine = "─"

// Table is a single-column, right-aligned text table. It reproduces the part
// of the cjk-table library (kr.sixtyfive.Table) that the Kotlin version used.
type Table struct {
	rows        []string
	lineIndices map[int]bool // row index -> draw a line before that row
}

func NewTable() *Table {
	return &Table{lineIndices: map[int]bool{}}
}

func (t *Table) addRow(row string) *Table {
	t.rows = append(t.rows, row)
	return t
}

// addLine draws a horizontal line at the current position.
func (t *Table) addLine() *Table {
	t.lineIndices[len(t.rows)] = true
	return t
}

func (t *Table) render() string {
	if len(t.rows) == 0 {
		return ""
	}

	maxWidth := 0
	for _, row := range t.rows {
		maxWidth = max(maxWidth, displayWidth(row))
	}
	line := strings.Repeat(horizontalLine, maxWidth) + "\n"

	var sb strings.Builder
	for i, row := range t.rows {
		if t.lineIndices[i] {
			sb.WriteString(line)
		}
		sb.WriteString(strings.Repeat(" ", maxWidth-displayWidth(row)))
		sb.WriteString(row)
		sb.WriteString("\n")
	}
	if t.lineIndices[len(t.rows)] {
		sb.WriteString(line)
	}
	return sb.String()
}

// displayWidth counts terminal columns: CJK wide characters take two.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			n += 2
		default:
			n++
		}
	}
	return n
}
