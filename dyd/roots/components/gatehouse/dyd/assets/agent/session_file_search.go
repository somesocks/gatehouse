package agent

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"gatehouse/sessionsearch"
)

const (
	sessionFileSearchMaximumQueryBytes = 512
	sessionFileSearchMaximumMatches    = 4
	sessionFileSearchPreviewBytes      = 96
)

type sessionFileSearchOpen func(offset int64) (error, io.ReadCloser)

type sessionFileSearchRune struct {
	value       rune
	offset, end int64
}

func sessionFileSearch(open sessionFileSearchOpen, fileID, fingerprint, query, cursor string) (error, SessionFileSearchResult) {
	err, terms := sessionFileSearchTerms(query)
	if err != nil {
		return err, SessionFileSearchResult{}
	}
	err, offset, skip := sessionFileSearchCursor(fileID, fingerprint, query, cursor)
	if err != nil {
		return err, SessionFileSearchResult{}
	}
	matches, nextOffset, nextSkip, more, err := sessionFileSearchMatches(open, offset, skip, terms)
	if err != nil {
		return err, SessionFileSearchResult{}
	}
	for index := range matches {
		previewOffset, preview, err := sessionFileSearchPreview(open, matches[index].Offset)
		if err != nil {
			return err, SessionFileSearchResult{}
		}
		matches[index].PreviewOffset = previewOffset
		matches[index].Preview = preview
	}
	result := SessionFileSearchResult{Terms: terms, Matches: matches}
	if more {
		result.NextCursor = sessionFileSearchNextCursor(fileID, fingerprint, query, nextOffset, nextSkip)
	}
	return nil, result
}

func sessionFileSearchTerms(query string) (error, []string) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > sessionFileSearchMaximumQueryBytes {
		return fmt.Errorf("session file search query is invalid"), nil
	}
	if !strings.HasPrefix(query, "(") {
		return nil, []string{query}
	}
	err, expression := sessionsearch.Parse(query)
	if err != nil {
		return err, nil
	}
	terms := []string{}
	seen := map[string]bool{}
	var collect func(sessionsearch.Expression) error
	collect = func(expression sessionsearch.Expression) error {
		if expression.Operator == "" {
			if !seen[expression.Text] {
				seen[expression.Text] = true
				terms = append(terms, expression.Text)
			}
			return nil
		}
		if expression.Operator != "or" {
			return fmt.Errorf("session file search expressions support only or")
		}
		for _, child := range expression.Children {
			if err := collect(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := collect(expression); err != nil {
		return err, nil
	}
	return nil, terms
}

func sessionFileSearchMatches(open sessionFileSearchOpen, offset int64, skip int, terms []string) ([]SessionFileSearchMatch, int64, int, bool, error) {
	err, source := open(offset)
	if err != nil {
		return nil, 0, 0, false, err
	}
	defer source.Close()
	reader := bufio.NewReaderSize(source, 64*1024)
	patterns := make([][]rune, len(terms))
	maximumRunes := 0
	for index, term := range terms {
		patterns[index] = []rune(term)
		maximumRunes = max(maximumRunes, len(patterns[index]))
	}
	window := make([]sessionFileSearchRune, 0, maximumRunes)
	position := offset
	eof := false
	read := func() error {
		if eof {
			return nil
		}
		value, size, err := reader.ReadRune()
		if err == io.EOF {
			eof = true
			return nil
		}
		if err != nil {
			return err
		}
		if value == utf8.RuneError && size == 1 {
			return fmt.Errorf("session file search requires valid UTF-8")
		}
		window = append(window, sessionFileSearchRune{value: value, offset: position, end: position + int64(size)})
		position += int64(size)
		return nil
	}
	for len(window) < maximumRunes && !eof {
		if err := read(); err != nil {
			return nil, 0, 0, false, err
		}
	}
	matches := make([]SessionFileSearchMatch, 0, sessionFileSearchMaximumMatches)
	for len(window) > 0 {
		group := make([]SessionFileSearchMatch, 0, len(patterns))
		for index, pattern := range patterns {
			if len(pattern) > len(window) {
				continue
			}
			values := make([]rune, len(pattern))
			for position := range pattern {
				values[position] = window[position].value
			}
			if strings.EqualFold(string(values), terms[index]) {
				group = append(group, SessionFileSearchMatch{Offset: window[0].offset, Length: window[len(pattern)-1].end - window[0].offset, TermIndex: index})
			}
		}
		if skip > len(group) {
			return nil, 0, 0, false, fmt.Errorf("session file search cursor is invalid")
		}
		remaining := group[skip:]
		available := sessionFileSearchMaximumMatches - len(matches)
		if len(remaining) > available {
			matches = append(matches, remaining[:available]...)
			return matches, window[0].offset, skip + available, true, nil
		}
		matches = append(matches, remaining...)
		skip = 0
		nextOffset := window[0].end
		window = window[1:]
		if len(matches) == sessionFileSearchMaximumMatches {
			return matches, nextOffset, 0, true, nil
		}
		for len(window) < maximumRunes && !eof {
			if err := read(); err != nil {
				return nil, 0, 0, false, err
			}
		}
	}
	return matches, 0, 0, false, nil
}

func sessionFileSearchPreview(open sessionFileSearchOpen, offset int64) (int64, string, error) {
	start := max(int64(0), offset-sessionFileSearchPreviewBytes/2)
	err, source := open(start)
	if err != nil {
		return 0, "", err
	}
	defer source.Close()
	contents, err := io.ReadAll(io.LimitReader(source, sessionFileSearchPreviewBytes))
	if err != nil {
		return 0, "", err
	}
	for len(contents) > 0 && !utf8.RuneStart(contents[0]) {
		contents = contents[1:]
		start++
	}
	for len(contents) > 0 && !utf8.Valid(contents) {
		contents = contents[:len(contents)-1]
	}
	if len(contents) > 0 && !utf8.Valid(contents) {
		return 0, "", fmt.Errorf("session file search requires valid UTF-8")
	}
	return start, string(contents), nil
}

func sessionFileSearchNextCursor(fileID, fingerprint, query string, offset int64, skip int) string {
	digest := sha256.Sum256([]byte(fileID + "\x00" + fingerprint + "\x00" + query))
	value := strconv.FormatInt(offset, 10) + ":" + strconv.Itoa(skip) + ":" + fmt.Sprintf("%x", digest)
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func sessionFileSearchCursor(fileID, fingerprint, query, cursor string) (error, int64, int) {
	if cursor == "" {
		return nil, 0, 0
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return fmt.Errorf("session file search cursor is invalid"), 0, 0
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 {
		return fmt.Errorf("session file search cursor is invalid"), 0, 0
	}
	offset, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || offset < 0 {
		return fmt.Errorf("session file search cursor is invalid"), 0, 0
	}
	skip, err := strconv.Atoi(parts[1])
	if err != nil || skip < 0 {
		return fmt.Errorf("session file search cursor is invalid"), 0, 0
	}
	expected := sessionFileSearchNextCursor(fileID, fingerprint, query, offset, skip)
	if cursor != expected {
		return fmt.Errorf("session file search cursor is for a different file or query"), 0, 0
	}
	return nil, offset, skip
}
