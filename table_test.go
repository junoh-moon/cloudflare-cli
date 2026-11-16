package main

import "testing"

// Expected strings were captured by running the original cjk-table library
// (kr.sixtyfive.Table) with the same calls.

func TestTableRendersNothingWhenEmpty(t *testing.T) {
	if got := NewTable().render(); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestTableRendersNothingWhenOnlyLines(t *testing.T) {
	if got := NewTable().addLine().render(); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestTableRightAlignsRows(t *testing.T) {
	got := NewTable().
		addRow("music.example.com").
		addRow("example.com").
		addRow("videos.example.com").
		render()

	want := "" +
		" music.example.com\n" +
		"       example.com\n" +
		"videos.example.com\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTableDrawsLinesAroundRows(t *testing.T) {
	got := NewTable().addLine().
		addRow("example.com").addLine().
		addRow("a-much-longer-zone.example.org").addLine().
		render()

	want := "" +
		"──────────────────────────────\n" +
		"                   example.com\n" +
		"──────────────────────────────\n" +
		"a-much-longer-zone.example.org\n" +
		"──────────────────────────────\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTableDrawsRepeatedLineOnce(t *testing.T) {
	got := NewTable().addLine().
		addRow("example.com").addLine().addLine().
		render()

	want := "" +
		"───────────\n" +
		"example.com\n" +
		"───────────\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTableCountsWideCharactersAsTwoColumns(t *testing.T) {
	got := NewTable().
		addRow("한글.example.com").
		addRow("example.com").
		addRow("日本語テスト.example.com").
		addRow("é.example.com").
		render()

	want := "" +
		"        한글.example.com\n" +
		"             example.com\n" +
		"日本語テスト.example.com\n" +
		"           é.example.com\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
