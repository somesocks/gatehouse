package agent_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"gatehouse/internal/testdbos"
	"gatehouse/model"
)

func TestTADWNormalReplyCodex(t *testing.T) {
	var calls, unexpected atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		content := liveUserContent(t, body)
		if !strings.Contains(content, "Calculate 1 + 2") || r.Header.Get("Authorization") != "Bearer INFERENCE_ACCESS_SENTINEL" {
			t.Error("incorrect Codex context or credentials")
		}
		if calls.Add(1) == 1 {
			args, _ := json.Marshal(map[string]string{"code": "(int/add 1 2)", "reason": "Calculate"})
			codexWireReply(w, map[string]any{"type": "function_call", "call_id": "tadw-codex", "name": "lisp", "arguments": string(args)}, 20)
			return
		}
		if !strings.Contains(content, "<tool.success ") || !strings.Contains(content, ">3</tool.success>") {
			t.Error("missing XML tool result", content)
		}
		codexWireReply(w, codexWireMessage(`{"text":"The answer is 3.","attachments":[]}`), 30)
	}))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	original := http.DefaultTransport
	http.DefaultTransport = codexProxyTransport{target: target, base: original, unexpected: &unexpected}
	t.Cleanup(func() { http.DefaultTransport = original })
	f := newCodexAgentFixture(t, true)
	if _, err := f.store.Exec(`UPDATE gatehouse_agent_models SET compaction='{"algorithm":"tadw","context_max_bytes":8192,"history_max_bytes":4096,"history_min_bytes":1024}'`); err != nil {
		t.Fatal(err)
	}
	if err := testdbos.Launch(f.ctx); err != nil {
		t.Fatal(err)
	}
	events := f.await(t, model.SessionEventKindAgentSuccess)
	if calls.Load() != 2 || unexpected.Load() != 0 {
		t.Fatal("Codex calls or routing", calls.Load(), unexpected.Load())
	}
	for _, event := range events {
		if event.Kind == model.SessionEventKindAgentFailure || event.Kind == model.SessionEventKindThinkingFailure {
			t.Fatal(event.Payload)
		}
	}
	assertLiveTADWState(t, f.store, 2)
}
