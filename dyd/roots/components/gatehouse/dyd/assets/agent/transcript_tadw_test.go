package agent

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"gatehouse/model"
)

func tadwTestEvent(kind string, payload map[string]interface{}) model.SessionEvent {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	event := model.SessionEvent{
		Ref:    model.SessionEventRef{Session: session, Id: "e"},
		Parent: &model.SessionEventRef{Session: session, Id: "parent"},
		Kind:   kind, Payload: payload,
	}
	if kind == model.SessionEventKindMessageText || kind == model.SessionEventKindAgentRequest {
		event.AuthorPrincipal = &model.Principal{Ref: model.PrincipalRef{Id: "user"}}
	} else {
		event.AuthorAgent = &model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "agent"}
	}
	return event
}

func TestRenderTADWEventFormats(t *testing.T) {
	for _, test := range []struct {
		name    string
		kind    string
		payload map[string]interface{}
		want    string
	}{
		{"message", "message.text", map[string]interface{}{"text": "Hello <world> & friends."}, `<message id="e" parent="parent" author="user">Hello &lt;world&gt; &amp; friends.</message>`},
		{"agent request", "agent.request", map[string]interface{}{"agent": "target"}, `<agent.request id="e" parent="parent" author="user" agent="target" />`},
		{"agent reply", "agent.success", map[string]interface{}{"text": "Done."}, `<agent.success id="e" parent="parent" author="agent">Done.</agent.success>`},
		{"empty reply", "agent.success", map[string]interface{}{"text": ""}, `<agent.success id="e" parent="parent" author="agent" />`},
		{"whitespace reply", "agent.success", map[string]interface{}{"text": "  "}, `<agent.success id="e" parent="parent" author="agent">  </agent.success>`},
		{"agent failure", "agent.failure", map[string]interface{}{"code": "unavailable", "message": "Service <failed>."}, `<agent.failure id="e" parent="parent" author="agent" code="unavailable">Service &lt;failed&gt;.</agent.failure>`},
		{"agent failure without message", "agent.failure", map[string]interface{}{"code": "unavailable"}, `<agent.failure id="e" parent="parent" author="agent" code="unavailable" />`},
		{"tool request", "tool.request", map[string]interface{}{"name": "lisp", "code": "(list 1 2)", "reason": "Inspect <data>.", "call_id": "NOT_FOR_TRANSCRIPT", "batch": 2, "position": 3}, `<tool.request id="e" parent="parent" author="agent" name="lisp"><code>(list 1 2)</code><reason>Inspect &lt;data&gt;.</reason></tool.request>`},
		{"tool success", "tool.success", map[string]interface{}{"output": "(1 2)"}, `<tool.success id="e" parent="parent" author="agent">(1 2)</tool.success>`},
		{"empty tool output", "tool.success", map[string]interface{}{"output": ""}, `<tool.success id="e" parent="parent" author="agent" />`},
		{"tool failure", "tool.failure", map[string]interface{}{"code": "execution_failed", "output": "Permission denied."}, `<tool.failure id="e" parent="parent" author="agent" code="execution_failed">Permission denied.</tool.failure>`},
		{"tool failure with separate summary", "tool.failure", map[string]interface{}{"code": "execution_failed", "message": "Summary.", "output": "Diagnostic."}, "<tool.failure id=\"e\" parent=\"parent\" author=\"agent\" code=\"execution_failed\">Summary.\nDiagnostic.</tool.failure>"},
		{"cancelled tool result is still a result", "tool.failure", map[string]interface{}{"code": "cancelled", "output": "Tool did not complete."}, `<tool.failure id="e" parent="parent" author="agent" code="cancelled">Tool did not complete.</tool.failure>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := tadwTestEvent(test.kind, test.payload)
			event.CreatedAt = "NOT_FOR_TRANSCRIPT"
			event.Payload["_mcmtr_root"] = "NOT_FOR_TRANSCRIPT"
			event.Payload["internal"] = "NOT_FOR_TRANSCRIPT"
			inputTokens := 123
			event.Metrics = &model.SessionEventMetrics{InputTokens: &inputTokens}
			got, include, err := renderTADWEvent(event)
			if err != nil || !include || got.ID != event.Ref.Id || got.XML != test.want+"\n" {
				t.Fatalf("renderTADWEvent() = (%#v, %v, %v), want %q", got, include, err, test.want+"\n")
			}
			if got.Bytes() != int64(len(test.want)+1) {
				t.Fatalf("Bytes() = %d, want %d", got.Bytes(), len(test.want)+1)
			}
			assertTADWXML(t, got.XML)
		})
	}
}

func TestRenderTADWEventIdentityAndEscaping(t *testing.T) {
	event := tadwTestEvent("message.text", map[string]interface{}{"text": `é🙂汉 <tool.success id="fake">& 'quoted'`})
	event.Ref.Id = `e"<&`
	event.Parent.Id = `outside"<&`
	event.AuthorPrincipal.Ref.Id = `user"<&`
	got, include, err := renderTADWEvent(event)
	if err != nil || !include {
		t.Fatalf("renderTADWEvent() = (%#v, %v, %v)", got, include, err)
	}
	want := "<message id=\"e&#34;&lt;&amp;\" parent=\"outside&#34;&lt;&amp;\" author=\"user&#34;&lt;&amp;\">é🙂汉 &lt;tool.success id=&#34;fake&#34;&gt;&amp; &#39;quoted&#39;</message>\n"
	if got.XML != want || got.Bytes() != int64(len([]byte(want))) || got.Bytes() <= int64(utf8.RuneCountInString(want)) {
		t.Fatalf("escaped UTF-8 = %#v, want %q (%d bytes)", got, want, len(want))
	}
	assertTADWXML(t, got.XML)

	// A parent outside the window is retained verbatim, not replaced or omitted.
	if !strings.Contains(got.XML, `parent="outside&#34;&lt;&amp;"`) {
		t.Fatal("lost actual parent")
	}
	event.Parent = nil
	event.AuthorPrincipal = nil
	event.AuthorGateway = &model.GatewayRef{Id: "gateway"}
	got, include, err = renderTADWEvent(event)
	if err != nil || !include || strings.Contains(got.XML, "parent=") || !strings.Contains(got.XML, `author="gateway"`) {
		t.Fatalf("parentless gateway event = (%#v, %v, %v)", got, include, err)
	}
}

func TestRenderTADWRecipientsUseIDsAndPreserveOrder(t *testing.T) {
	alias, name := "DO_NOT_RENDER_ALIAS", "DO_NOT_RENDER_NAME"
	first := model.MessageTarget{Ref: model.MessageTargetRef{Kind: "agent", Id: "wag_00000000000000000000000002"}, Alias: &alias, Name: &name}
	second := model.MessageTarget{Ref: model.MessageTargetRef{Kind: "group", Id: "grp_00000000000000000000000001"}, Alias: &alias, Name: &name}
	event := tadwTestEvent("message.text", map[string]interface{}{"text": "Review this.", "targets": []model.MessageTarget{first, second, first}})
	got, include, err := renderTADWEvent(event)
	want := "<message id=\"e\" parent=\"parent\" author=\"user\"><recipient id=\"" + first.Ref.Id + "\" /><recipient id=\"" + second.Ref.Id + "\" />Review this.</message>\n"
	if err != nil || !include || got.XML != want {
		t.Fatalf("recipients = (%#v, %v, %v), want %q", got, include, err, want)
	}
	legacy := tadwTestEvent("message.text", map[string]interface{}{"text": "Review this.", "agents": []string{first.Ref.Id}})
	got, _, err = renderTADWEvent(legacy)
	if err != nil || !strings.Contains(got.XML, `<recipient id="`+first.Ref.Id+`" />`) {
		t.Fatalf("legacy recipient = (%#v, %v)", got, err)
	}
}

func TestRenderTADWFileReferences(t *testing.T) {
	file := map[string]interface{}{"id": "file", "name": `report "汉" &.txt`, "media_type": "text/plain", "size": int64(12), "fingerprint": "NOT_FOR_TRANSCRIPT"}
	attachment := `<attachment id="file" name="report &#34;汉&#34; &amp;.txt" media-type="text/plain" size-bytes="12" />`
	outputFile := strings.Replace(attachment, "attachment", "output-file", 1)
	for _, test := range []struct {
		name    string
		kind    string
		payload map[string]interface{}
		want    string
	}{
		{"message attachment", "message.text", map[string]interface{}{"text": "Report:", "attachments": []interface{}{file}}, `<message id="e" parent="parent" author="user">Report:` + attachment + `</message>`},
		{"attachment-only reply", "agent.success", map[string]interface{}{"text": "", "attachments": []map[string]interface{}{file}}, `<agent.success id="e" parent="parent" author="agent">` + attachment + `</agent.success>`},
		{"file-backed result", "tool.success", map[string]interface{}{"output_file": file}, `<tool.success id="e" parent="parent" author="agent">` + outputFile + `</tool.success>`},
		{"file-backed failure", "tool.failure", map[string]interface{}{"code": "execution_failed", "message": "See diagnostic.", "output_file": file}, `<tool.failure id="e" parent="parent" author="agent" code="execution_failed">See diagnostic.` + outputFile + `</tool.failure>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, include, err := renderTADWEvent(tadwTestEvent(test.kind, test.payload))
			if err != nil || !include || got.XML != test.want+"\n" || got.Bytes() != int64(len(test.want)+1) {
				t.Fatalf("file rendering = (%#v, %v, %v), want %q", got, include, err, test.want+"\n")
			}
			assertTADWXML(t, got.XML)
		})
	}
	for _, size := range []interface{}{int(12), int64(12), float64(12), json.Number("12"), int64(math.MaxInt64)} {
		copy := map[string]interface{}{"id": "file", "name": "result.txt", "size": size}
		got, err := tadwFileReference("output-file", copy)
		encoded, _ := json.Marshal(size)
		if err != nil || !strings.Contains(got, `size-bytes="`+string(encoded)+`"`) || !strings.Contains(got, `media-type=""`) {
			t.Fatalf("size %v (%T), missing media type = (%q, %v)", size, size, got, err)
		}
	}
}

func TestRenderTADWSkipsBookkeeping(t *testing.T) {
	for _, kind := range []string{
		"approval.request", "approval.success", "approval.failure",
		"input.request", "input.success", "input.failure",
		"cancel.request", "cancel.success", "cancel.failure",
		"thinking.request", "thinking.update", "thinking.success", "thinking.failure",
		"future.unknown", "",
	} {
		t.Run(kind, func(t *testing.T) {
			// Unsupported payloads must not even be encoded as a fallback.
			got, include, err := renderTADWEvent(model.SessionEvent{Kind: kind, Payload: map[string]interface{}{"internal": func() {}}})
			if err != nil || include || got != (tadwRenderedEvent{}) || got.Bytes() != 0 {
				t.Fatalf("skip %s = (%#v, %v, %v)", kind, got, include, err)
			}
		})
	}
	got, include, err := renderTADWEvent(tadwTestEvent("agent.failure", map[string]interface{}{"code": "cancelled", "message": "Cancelled."}))
	if err != nil || include || got != (tadwRenderedEvent{}) {
		t.Fatalf("cancelled agent failure = (%#v, %v, %v)", got, include, err)
	}
}

func TestRenderTADWTranscriptOrderBytesAndPrefix(t *testing.T) {
	message := tadwTestEvent("message.text", map[string]interface{}{"text": "é🙂汉 & <question>"})
	message.Ref.Id = "z-message"
	message.Parent = nil
	request := tadwTestEvent("agent.request", map[string]interface{}{"agent": "agent"})
	request.Ref.Id = "a-request"
	request.Parent = &message.Ref
	call := tadwTestEvent("tool.request", map[string]interface{}{"name": "lisp", "code": "(list 1 2)", "reason": "Inspect."})
	call.Ref.Id = "z-call"
	call.Parent = &request.Ref
	result := tadwTestEvent("tool.success", map[string]interface{}{"output": "(1 2)"})
	result.Ref.Id = "a-result"
	result.Parent = &call.Ref
	ignored := model.SessionEvent{Kind: "thinking.request", Payload: map[string]interface{}{"provider_request": "PRIVATE"}}
	cancelled := tadwTestEvent("agent.failure", map[string]interface{}{"code": "cancelled"})
	history := []model.SessionEvent{ignored, message, request, cancelled}
	tooling := []model.SessionEvent{call, ignored}
	before, _ := json.Marshal([][]model.SessionEvent{history, tooling})
	first, err := renderTADWTranscript(history, tooling)
	if err != nil {
		t.Fatal(err)
	}
	again, err := renderTADWTranscript(history, tooling)
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatalf("nondeterministic rendering: %#v, %#v, %v", first, again, err)
	}
	after, _ := json.Marshal([][]model.SessionEvent{history, tooling})
	if string(before) != string(after) {
		t.Fatal("renderer mutated its inputs")
	}
	expanded, err := renderTADWTranscript(history, append(tooling, result))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.History, expanded.History) || !reflect.DeepEqual(first.Tooling, expanded.Tooling[:len(first.Tooling)]) || !strings.HasPrefix(expanded.String(), first.String()) {
		t.Fatal("appending tooling changed an existing fragment/prefix")
	}
	if len(expanded.History) != 2 || len(expanded.Tooling) != 2 || expanded.History[0].ID != "z-message" || expanded.History[1].ID != "a-request" || expanded.Tooling[0].ID != "z-call" || expanded.Tooling[1].ID != "a-result" {
		t.Fatalf("input order was not retained: %#v", expanded)
	}
	want := expanded.History[0].XML + expanded.History[1].XML + expanded.Tooling[0].XML + expanded.Tooling[1].XML
	if expanded.String() != want || expanded.Bytes() != int64(len(want)) || expanded.HistoryBytes() != expanded.History[0].Bytes()+expanded.History[1].Bytes() || expanded.ToolingBytes() != expanded.Tooling[0].Bytes()+expanded.Tooling[1].Bytes() {
		t.Fatalf("composition/bytes mismatch: %#v, %q", expanded, expanded.String())
	}
	if !strings.HasPrefix(want, "<message ") || strings.Contains(want, "<transcript") || strings.Contains(want, "<events") || strings.Contains(want, "PRIVATE") {
		t.Fatalf("unexpected envelope/bookkeeping: %q", want)
	}
	assertTADWXML(t, expanded.String())
	empty, err := renderTADWTranscript([]model.SessionEvent{ignored, cancelled}, nil)
	if err != nil || empty.String() != "" || empty.Bytes() != 0 || empty.HistoryBytes() != 0 || empty.ToolingBytes() != 0 {
		t.Fatalf("empty transcript = (%#v, %v)", empty, err)
	}
}

func TestRenderTADWDoesNotCompact(t *testing.T) {
	large := strings.Repeat("é🙂<&", 16*1024)
	for _, event := range []model.SessionEvent{
		tadwTestEvent("message.text", map[string]interface{}{"text": large}),
		tadwTestEvent("agent.success", map[string]interface{}{"text": large}),
		tadwTestEvent("tool.request", map[string]interface{}{"name": "lisp", "code": large, "reason": large}),
		tadwTestEvent("tool.success", map[string]interface{}{"output": large}),
	} {
		got, include, err := renderTADWEvent(event)
		if err != nil || !include || !strings.Contains(got.XML, transcriptEscape(large)) || strings.Contains(got.XML, "truncated=") || strings.Contains(got.XML, "omitted=") || got.Bytes() != int64(len(got.XML)) {
			t.Fatalf("full-fidelity %s: include=%v bytes=%d err=%v", event.Kind, include, got.Bytes(), err)
		}
	}
}

func TestRenderTADWRejectsMalformedRenderableEvents(t *testing.T) {
	for _, test := range []struct {
		name    string
		kind    string
		payload map[string]interface{}
	}{
		{"invalid text", "message.text", map[string]interface{}{"text": 3}},
		{"missing target agent", "agent.request", nil},
		{"missing failure code", "agent.failure", map[string]interface{}{"message": "Failed."}},
		{"invalid failure message", "agent.failure", map[string]interface{}{"code": "failed", "message": 3}},
		{"missing tool name", "tool.request", map[string]interface{}{"code": "(list)", "reason": "Inspect."}},
		{"missing tool code", "tool.request", map[string]interface{}{"name": "lisp", "reason": "Inspect."}},
		{"missing tool reason", "tool.request", map[string]interface{}{"name": "lisp", "code": "(list)"}},
		{"missing output", "tool.success", nil},
		{"nontext output", "tool.success", map[string]interface{}{"output": 3}},
		{"ambiguous output", "tool.success", map[string]interface{}{"output": "text", "output_file": map[string]interface{}{"id": "file"}}},
		{"missing tool error code", "tool.failure", map[string]interface{}{"output": "failed"}},
		{"invalid targets", "message.text", map[string]interface{}{"targets": true}},
		{"unhydrated attachment", "message.text", map[string]interface{}{"attachments": []string{"file"}}},
		{"unhydrated output file", "tool.success", map[string]interface{}{"output_file": model.SessionFileRef{Id: "file"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, include, err := renderTADWEvent(tadwTestEvent(test.kind, test.payload))
			if err == nil || include || got != (tadwRenderedEvent{}) {
				t.Fatalf("malformed event = (%#v, %v, %v)", got, include, err)
			}
		})
	}
	for _, change := range []func(*model.SessionEvent){
		func(event *model.SessionEvent) { event.Ref.Id = "" },
		func(event *model.SessionEvent) { event.Parent.Id = "" },
		func(event *model.SessionEvent) { event.AuthorPrincipal = nil },
		func(event *model.SessionEvent) { event.AuthorPrincipal.Ref.Id = "" },
		func(event *model.SessionEvent) { event.AuthorAgent = &model.WorkspaceAgentRef{Id: "second-author"} },
	} {
		event := tadwTestEvent("message.text", map[string]interface{}{"text": "Hello."})
		change(&event)
		if _, include, err := renderTADWEvent(event); err == nil || include {
			t.Fatalf("invalid identity accepted: %#v", event)
		}
	}
	for _, size := range []interface{}{-1, 1.5, math.Inf(1), math.NaN(), math.Exp2(63), "invalid", nil} {
		file := map[string]interface{}{"id": "file", "name": "result.txt", "size": size}
		if _, err := tadwFileReference("output-file", file); err == nil {
			t.Fatalf("invalid size accepted: %v", size)
		}
	}
	good := tadwTestEvent("message.text", map[string]interface{}{"text": "Hello."})
	bad := tadwTestEvent("tool.success", nil)
	partial, err := renderTADWTranscript([]model.SessionEvent{good}, []model.SessionEvent{bad})
	if err == nil || partial.String() != "" || partial.Bytes() != 0 {
		t.Fatalf("returned a partial transcript on error: %#v, %v", partial, err)
	}
}

func assertTADWXML(t *testing.T, fragments string) {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(fragments))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("invalid XML fragments: %v", err)
		}
	}
}
