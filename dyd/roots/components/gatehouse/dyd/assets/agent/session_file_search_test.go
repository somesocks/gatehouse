package agent

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestSessionFileSearchReturnsAllLiteralOccurrencesAcrossPages(t *testing.T) {
	contents := []byte(strings.Repeat("match ", 7))
	open := sessionFileSearchTestOpen(contents)
	err, first := sessionFileSearch(open, "file", "sha256:test", "match", "")
	if err != nil || len(first.Matches) != sessionFileSearchMaximumMatches || first.NextCursor == "" {
		t.Fatalf("sessionFileSearch() first = (%#v, %v)", first, err)
	}
	for index, match := range first.Matches {
		if match.Offset != int64(index*6) || match.Length != 5 || match.TermIndex != 0 || !strings.Contains(match.Preview, "match") {
			t.Fatalf("first match %d = %#v", index, match)
		}
	}
	err, second := sessionFileSearch(open, "file", "sha256:test", "match", first.NextCursor)
	if err != nil || len(second.Matches) != 3 || second.Matches[0].Offset != 24 || second.Matches[2].Offset != 36 || second.NextCursor != "" {
		t.Fatalf("sessionFileSearch() second = (%#v, %v)", second, err)
	}
}

func TestSessionFileSearchMatchesAcrossStreamBlocks(t *testing.T) {
	contents := append(bytes.Repeat([]byte("x"), 64*1024-2), []byte("Needle")...)
	err, result := sessionFileSearch(sessionFileSearchTestOpen(contents), "file", "sha256:test", "needle", "")
	if err != nil || len(result.Matches) != 1 || result.Matches[0].Offset != 64*1024-2 || result.Matches[0].Length != 6 {
		t.Fatalf("sessionFileSearch() = (%#v, %v)", result, err)
	}
}

func TestSessionFileSearchReturnsNestedOrOccurrences(t *testing.T) {
	open := sessionFileSearchTestOpen([]byte("One two ONE three"))
	err, result := sessionFileSearch(open, "file", "sha256:test", `(or "one" (or "two" "three"))`, "")
	if err != nil || len(result.Terms) != 3 || len(result.Matches) != 4 {
		t.Fatalf("sessionFileSearch() = (%#v, %v)", result, err)
	}
	for index, want := range []struct {
		offset int64
		term   int
	}{{0, 0}, {4, 1}, {8, 0}, {12, 2}} {
		if result.Matches[index].Offset != want.offset || result.Matches[index].TermIndex != want.term {
			t.Fatalf("match %d = %#v, want (%d, %d)", index, result.Matches[index], want.offset, want.term)
		}
	}
}

func TestSessionFileSearchRejectsInvalidQueriesAndCursors(t *testing.T) {
	open := sessionFileSearchTestOpen([]byte("alpha beta"))
	for _, query := range []string{"", `(and "alpha" "beta")`} {
		if err, _ := sessionFileSearch(open, "file", "sha256:test", query, ""); err == nil {
			t.Fatalf("sessionFileSearch(%q) accepted an invalid query", query)
		}
	}
	err, result := sessionFileSearch(open, "file", "sha256:test", "alpha", "")
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("sessionFileSearch() = (%#v, %v)", result, err)
	}
	cursor := sessionFileSearchNextCursor("file", "sha256:test", "alpha", 0, 0)
	if err, _ := sessionFileSearch(open, "file", "sha256:changed", "alpha", cursor); err == nil {
		t.Fatal("sessionFileSearch accepted a cursor for a changed file")
	}
}

func TestSessionFileSearchRejectsInvalidUTF8(t *testing.T) {
	open := sessionFileSearchTestOpen([]byte{'m', 'a', 't', 'c', 'h', 0xff})
	if err, _ := sessionFileSearch(open, "file", "sha256:test", "match", ""); err == nil {
		t.Fatal("sessionFileSearch accepted invalid UTF-8")
	}
}

func sessionFileSearchTestOpen(contents []byte) sessionFileSearchOpen {
	return func(offset int64) (error, io.ReadCloser) {
		if offset < 0 || offset > int64(len(contents)) {
			return io.EOF, nil
		}
		return nil, io.NopCloser(bytes.NewReader(contents[offset:]))
	}
}
