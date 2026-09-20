// Package sessionsearch defines the constrained session event search language.
package sessionsearch

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"gatehouse/lisp"
	"gatehouse/model"
)

const (
	MaxSourceBytes = 4 << 10
	MaxDepth       = 16
	MaxLeaves      = 32
	PreviewBytes   = 160
)

// Expression is a positive, constrained Lisp search expression.
type Expression struct {
	Text     string
	Operator string
	Children []Expression
}

// Range identifies bytes in an event's readable body.
type Range struct {
	Offset int64
	Length int64
}

// EventResult describes one searchable session event.
type EventResult struct {
	ID      string
	Kind    string
	Size    int64
	Preview string
	Matches []Range
}

func Parse(source string) (error, Expression) {
	if len(source) > MaxSourceBytes {
		return fmt.Errorf("session event search expression exceeds %d bytes", MaxSourceBytes), Expression{}
	}
	readErr, value := lisp.Read(source)
	if readErr != nil {
		return fmt.Errorf("read session event search expression: %w", readErr), Expression{}
	}
	leaves := 0
	err, expression := parse(value, 1, &leaves)
	if err != nil {
		return err, Expression{}
	}
	return nil, expression
}

func parse(value lisp.Expr, depth int, leaves *int) (error, Expression) {
	if depth > MaxDepth {
		return fmt.Errorf("session event search expression exceeds depth %d", MaxDepth), Expression{}
	}
	if stringErr, text := lisp.RequireString(value); stringErr == nil {
		if text == "" {
			return fmt.Errorf("session event search strings must not be empty"), Expression{}
		}
		(*leaves)++
		if *leaves > MaxLeaves {
			return fmt.Errorf("session event search expression exceeds %d strings", MaxLeaves), Expression{}
		}
		return nil, Expression{Text: text}
	}
	values, ok := values(value)
	if !ok || len(values) == 0 {
		return fmt.Errorf("session event search expressions must be strings or forms beginning with and/or"), Expression{}
	}
	symbolErr, operator := lisp.RequireSymbol(values[0])
	if symbolErr != nil || (operator != "and" && operator != "or") {
		return fmt.Errorf("session event search forms must start with a supported operator"), Expression{}
	}
	if len(values) == 1 {
		return fmt.Errorf("session event search %s forms must not be empty", operator), Expression{}
	}
	expression := Expression{Operator: operator, Children: make([]Expression, 0, len(values)-1)}
	for _, child := range values[1:] {
		err, parsed := parse(child, depth+1, leaves)
		if err != nil {
			return err, Expression{}
		}
		expression.Children = append(expression.Children, parsed)
	}
	return nil, expression
}

func values(value lisp.Expr) ([]lisp.Expr, bool) {
	result := []lisp.Expr{}
	for {
		if lisp.IsNull(value) {
			return result, true
		}
		first, rest, ok := lisp.DeconstructPair(value)
		if !ok {
			return nil, false
		}
		result = append(result, first)
		value = rest
	}
}

// NextCursor encodes an event position bound to the exact source expression.
func NextCursor(source, id string) string {
	digest := sha256.Sum256([]byte(source))
	return base64.RawURLEncoding.EncodeToString([]byte(id + ":" + hex.EncodeToString(digest[:])))
}

// Cursor validates a query-bound cursor and returns its event position.
func Cursor(source, value string) (error, string) {
	if value == "" {
		return nil, ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("invalid session event search cursor"), ""
	}
	id, digest, ok := strings.Cut(string(decoded), ":")
	if !ok || id == "" || len(digest) != sha256.Size*2 {
		return fmt.Errorf("invalid session event search cursor"), ""
	}
	expected := sha256.Sum256([]byte(source))
	if digest != hex.EncodeToString(expected[:]) {
		return fmt.Errorf("session event search cursor is for a different expression"), ""
	}
	return nil, id
}

// EventReadableBody returns the exact bytes exposed by session/events/read.
func EventReadableBody(event model.SessionEvent) (string, bool) {
	switch event.Kind {
	case "message.text", "agent.reply":
		output, ok := event.Payload["text"].(string)
		return output, ok
	case "tool.request":
		callID, callIDOK := event.Payload["call_id"].(string)
		name, nameOK := event.Payload["name"].(string)
		code, codeOK := event.Payload["code"].(string)
		reason, reasonOK := event.Payload["reason"].(string)
		if !callIDOK || callID == "" || !nameOK || name != "lisp" || !codeOK || strings.TrimSpace(code) == "" || !reasonOK || strings.TrimSpace(reason) == "" {
			return "", false
		}
		return code + "\n" + reason, true
	case "tool.success", "tool.failure":
		output, ok := event.Payload["output"].(string)
		return output, ok
	case "approval.request":
		description, ok := event.Payload["description"].(string)
		return description, ok
	default:
		encoded, err := json.Marshal(event.Payload)
		if err != nil {
			return "", false
		}
		return string(encoded), true
	}
}

// EventSearchBody returns explicitly searchable current-session event text.
func EventSearchBody(event model.SessionEvent) (string, bool) {
	switch event.Kind {
	case "message.text", "agent.reply", "tool.request", "tool.success", "tool.failure", "approval.request":
		return EventReadableBody(event)
	default:
		return "", false
	}
}

// Result returns ranges for the first matching occurrence of each positive string.
func Result(event model.SessionEvent, expression Expression) (EventResult, bool) {
	body, ok := EventSearchBody(event)
	if !ok {
		return EventResult{}, false
	}
	matches, ok := match(body, expression)
	if !ok {
		return EventResult{}, false
	}
	terms := expression.terms(nil)
	ranges := make([]Range, 0, len(terms))
	seen := map[string]bool{}
	for _, term := range terms {
		if seen[term] {
			continue
		}
		seen[term] = true
		if match, exists := matches[term]; exists {
			ranges = append(ranges, match)
		}
	}
	if len(ranges) == 0 {
		return EventResult{}, false
	}
	return EventResult{ID: event.Ref.Id, Kind: event.Kind, Size: int64(len(body)), Preview: preview(body, ranges[0]), Matches: ranges}, true
}

func (expression Expression) terms(result []string) []string {
	if expression.Operator == "" {
		return append(result, expression.Text)
	}
	for _, child := range expression.Children {
		result = child.terms(result)
	}
	return result
}

func match(body string, expression Expression) (map[string]Range, bool) {
	if expression.Operator == "" {
		offset, length, ok := index(body, expression.Text)
		if !ok {
			return nil, false
		}
		return map[string]Range{expression.Text: {Offset: offset, Length: length}}, true
	}
	all := map[string]Range{}
	matched := expression.Operator == "and"
	for _, child := range expression.Children {
		childMatches, childMatched := match(body, child)
		if expression.Operator == "and" && !childMatched {
			return nil, false
		}
		if expression.Operator == "or" && childMatched {
			matched = true
		}
		for term, location := range childMatches {
			all[term] = location
		}
	}
	return all, matched
}

func index(body, term string) (int64, int64, bool) {
	characters := []rune(body)
	target := []rune(term)
	if len(target) == 0 || len(target) > len(characters) {
		return 0, 0, false
	}
	offsets := make([]int, len(characters)+1)
	position := 0
	for index, character := range characters {
		offsets[index] = position
		position += utf8.RuneLen(character)
	}
	offsets[len(characters)] = len(body)
	for start := 0; start+len(target) <= len(characters); start++ {
		end := start + len(target)
		if strings.EqualFold(string(characters[start:end]), term) {
			return int64(offsets[start]), int64(offsets[end] - offsets[start]), true
		}
	}
	return 0, 0, false
}

func preview(body string, location Range) string {
	start := int(location.Offset) - PreviewBytes/2
	if start < 0 {
		start = 0
	}
	end := start + PreviewBytes
	if end > len(body) {
		end = len(body)
		start = end - PreviewBytes
		if start < 0 {
			start = 0
		}
	}
	for start < end && !utf8.RuneStart(body[start]) {
		start++
	}
	for end > start && end < len(body) && !utf8.RuneStart(body[end]) {
		end--
	}
	result := body[start:end]
	if start > 0 {
		result = "..." + result
	}
	if end < len(body) {
		result += "..."
	}
	return result
}
