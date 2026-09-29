package sessionsearch

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"gatehouse/model"
)

func TestParseAndResult(t *testing.T) {
	err, expression := Parse(`(and "Conroe" (or "ordinance" "zoning"))`)
	if err != nil {
		t.Fatal(err)
	}
	event := model.SessionEvent{Ref: model.SessionEventRef{Id: "sev_00000000000000000000000000"}, Kind: "tool.success", Payload: map[string]interface{}{"output": "Conroe zoning ordinance"}}
	result, matched := Result(event, expression)
	if !matched || result.ID != event.Ref.Id || result.Size != int64(len("Conroe zoning ordinance")) || len(result.Matches) != 3 {
		t.Fatalf("Result() = (%#v, %t)", result, matched)
	}
	for _, match := range result.Matches {
		if match.Offset < 0 || match.Length < 1 {
			t.Fatalf("Result() range = %#v", match)
		}
	}
	if !strings.Contains(result.Preview, "Conroe") {
		t.Fatalf("Result() preview = %q", result.Preview)
	}
}

func TestResultUsesCanonicalRequestAndApprovalText(t *testing.T) {
	err, expression := Parse(`"\"Conroe\""`)
	if err != nil {
		t.Fatal(err)
	}
	request := model.SessionEvent{Ref: model.SessionEventRef{Id: "sev_00000000000000000000000000"}, Kind: "tool.request", Payload: map[string]interface{}{"name": "lisp", "call_id": "call", "code": `say "Conroe"`, "reason": "Explain the quote."}}
	result, matched := Result(request, expression)
	if !matched || result.Size != int64(len("say \"Conroe\"\nExplain the quote.")) || len(result.Matches) != 1 || result.Matches[0] != (Range{Offset: 4, Length: 8}) {
		t.Fatalf("request Result() = (%#v, %t)", result, matched)
	}
	approval := model.SessionEvent{Ref: model.SessionEventRef{Id: "sev_00000000000000000000000001"}, Kind: "approval.request", Payload: map[string]interface{}{"description": `Approve "Conroe"`}}
	result, matched = Result(approval, expression)
	if !matched || result.Size != int64(len(`Approve "Conroe"`)) || len(result.Matches) != 1 || result.Matches[0] != (Range{Offset: 8, Length: 8}) {
		t.Fatalf("approval Result() = (%#v, %t)", result, matched)
	}
}

func TestResultUsesAgentSuccessText(t *testing.T) {
	err, expression := Parse(`"reply"`)
	if err != nil {
		t.Fatal(err)
	}
	reply := model.SessionEvent{Ref: model.SessionEventRef{Id: "sev_00000000000000000000000000"}, Kind: model.SessionEventKindAgentSuccess, Payload: map[string]interface{}{"text": "Final reply text"}}
	body, readable := EventReadableBody(reply)
	result, matched := Result(reply, expression)
	if !readable || body != "Final reply text" || !matched || result.Size != int64(len(body)) || result.Preview != body || len(result.Matches) != 1 || result.Matches[0] != (Range{Offset: 6, Length: 5}) {
		t.Fatalf("agent reply Result() = (%q, %#v, %t)", body, result, matched)
	}
	invalid := model.SessionEvent{Kind: model.SessionEventKindAgentSuccess, Payload: map[string]interface{}{"text": float64(42)}}
	if _, readable := EventReadableBody(invalid); readable {
		t.Fatalf("EventReadableBody() accepted malformed agent reply")
	}
	if _, matched := Result(invalid, expression); matched {
		t.Fatalf("Result() matched malformed agent reply")
	}
}

func TestParseRejectsUnsupportedExpressions(t *testing.T) {
	for _, source := range []string{
		`(not "Conroe")`,
		`(and)`,
		`""`,
		`(and "Conroe" 1)`,
	} {
		if err, _ := Parse(source); err == nil {
			t.Fatalf("Parse(%q) accepted an invalid expression", source)
		}
	}
}

func TestCursorBindsExpression(t *testing.T) {
	cursor := NextCursor(`"Conroe"`, "sev_00000000000000000000000000")
	err, id := Cursor(`"Conroe"`, cursor)
	if err != nil || id != "sev_00000000000000000000000000" {
		t.Fatalf("Cursor() = (%q, %v)", id, err)
	}
	if err, _ := Cursor(`"zoning"`, cursor); err == nil {
		t.Fatal("Cursor() accepted a different expression")
	}
}

func TestCursorHandlesDigestNUL(t *testing.T) {
	source := ""
	for index := 0; index < 10_000; index++ {
		candidate := fmt.Sprintf(`"cursor %d"`, index)
		digest := sha256.Sum256([]byte(candidate))
		if bytes.IndexByte(digest[:], 0) >= 0 {
			source = candidate
			break
		}
	}
	if source == "" {
		t.Fatal("could not generate a digest containing NUL")
	}
	cursor := NextCursor(source, "sev_00000000000000000000000000")
	err, id := Cursor(source, cursor)
	if err != nil || id != "sev_00000000000000000000000000" {
		t.Fatalf("Cursor() = (%q, %v)", id, err)
	}
}
