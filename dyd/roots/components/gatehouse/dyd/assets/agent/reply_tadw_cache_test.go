package agent_test

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
)

type tadwCacheTestTransport func(*http.Request) (*http.Response, error)

func (rt tadwCacheTestTransport) RoundTrip(req *http.Request) (*http.Response, error) { return rt(req) }

func TestTADWExplicitCacheToolLoopAndFork(t *testing.T) {
	for _, tc := range []struct{ protocol, path string }{
		{"openai-responses", "/v1/responses"},
		{"openai-chat-completions", "/v1/chat/completions"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			testTADWExplicitCacheToolLoopAndFork(t, tc.protocol, "https://api.openai.com"+tc.path)
		})
	}
}

func testTADWExplicitCacheToolLoopAndFork(t *testing.T, protocol, endpoint string) {
	t.Helper()
	var calls atomic.Int32
	bodies := make(chan []byte, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		select {
		case bodies <- body:
		default:
			t.Error("unexpected extra provider call")
		}
		tool := 0
		if calls.Add(1) == 1 {
			tool = 1
		}
		liveProviderReply(w, protocol, tool)
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	http.DefaultTransport = tadwCacheTestTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != endpoint {
			return nil, fmt.Errorf("unexpected outbound request to %s", req.URL)
		}
		proxy := req.Clone(req.Context())
		proxy.URL, proxy.Host = target, target.Host
		return original.RoundTrip(proxy)
	})
	t.Cleanup(func() { http.DefaultTransport = original })

	f := newTADWLiveFixture(t, filepath.Join(t.TempDir(), "explicit.sqlite"), endpoint, protocol, config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: 16384, HistoryMaxBytes: 8192, HistoryMinBytes: 1}})
	if _, err := f.store.Exec(`UPDATE gatehouse_agent_models SET model='gpt-5.6-luna'`); err != nil {
		t.Fatal(err)
	}
	captureLiveTADWHintWrites(t, f.store)
	agents := slices.Clone(f.message.Payload["agents"].([]string))
	f.submit(t, true)
	first := f.outcome(t)
	if first.Kind != model.SessionEventKindAgentSuccess || first.Parent == nil {
		t.Fatal(first.Payload)
	}
	var children []string
	for _, text := range []string{"first reply to the answer", "sibling reply to the same answer"} {
		follow := *f
		follow.message.Ref.Id, err = typed_id.New(typed_id.SessionEvent)
		if err != nil {
			t.Fatal(err)
		}
		follow.message.Parent = &first.Ref
		follow.message.Payload = map[string]any{"text": text, "agents": agents}
		follow.submit(t, false)
		if outcome := follow.outcome(t); outcome.Kind != model.SessionEventKindAgentSuccess {
			t.Fatal(outcome.Payload)
		}
		children = append(children, follow.message.Ref.Id)
	}
	if calls.Load() != 4 || len(bodies) != 4 || liveTADWHintWrites(t, f.store) != 4 {
		t.Fatal("expected four sends and four post-commit publications", calls.Load())
	}

	initial := explicitTADWInput(t, <-bodies, protocol)
	if len(initial.events) != 2 || !slices.Equal(initial.marked, []string{f.message.Ref.Id, first.Parent.Id}) {
		t.Fatal("initial call must mark history and owner", initial)
	}
	afterTool := explicitTADWInput(t, <-bodies, protocol)
	if len(afterTool.events) != 4 || !slices.Equal(initial.events, afterTool.events[:2]) ||
		!slices.Equal(afterTool.marked, []string{f.message.Ref.Id, first.Parent.Id, afterTool.ids[3]}) {
		t.Fatal("tool round must retain previous breakpoint and mark the tool result separately", afterTool)
	}
	child, sibling := explicitTADWInput(t, <-bodies, protocol), explicitTADWInput(t, <-bodies, protocol)
	if len(child.events) <= len(afterTool.events) || !slices.Equal(afterTool.events, child.events[:len(afterTool.events)]) ||
		!slices.Equal(child.marked, []string{afterTool.ids[3], first.Ref.Id, children[0], child.ids[len(child.ids)-1]}) {
		t.Fatal("thread reply must mark the previous send's endpoint, branch parent, current history and owner", child)
	}
	anchor := slices.Index(child.ids, first.Ref.Id)
	siblingAnchor := slices.Index(sibling.ids, first.Ref.Id)
	if anchor < 0 || anchor != siblingAnchor || !slices.Equal(child.events[:anchor+1], sibling.events[:siblingAnchor+1]) ||
		!slices.Contains(sibling.marked, first.Ref.Id) || !slices.Contains(sibling.marked, children[1]) {
		t.Fatal("sibling reply lost its marked common-prefix boundary", sibling)
	}
}

type explicitTADWEvents struct{ events, ids, marked []string }

func explicitTADWInput(t *testing.T, body []byte, protocol string) explicitTADWEvents {
	t.Helper()
	type item struct {
		Role    string
		Content json.RawMessage
	}
	var wire struct {
		Options  struct{ Mode string } `json:"prompt_cache_options"`
		Input    []item
		Messages []item
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	items, blockType := wire.Input, "input_text"
	if protocol == "openai-chat-completions" {
		items, blockType = wire.Messages, "text"
		if len(items) > 0 && items[0].Role == "system" {
			items = items[1:]
		}
	}
	if wire.Options.Mode != "explicit" || len(items) < 2 {
		t.Fatal("wire request missing explicit mode or input")
	}
	var result explicitTADWEvents
	for i, item := range items {
		var content []struct {
			Type, Text string
			Breakpoint *struct{ Mode string } `json:"prompt_cache_breakpoint"`
		}
		if err := json.Unmarshal(item.Content, &content); err != nil {
			t.Fatal(err)
		}
		if item.Role != "user" || len(content) != 1 || content[0].Type != blockType {
			t.Fatal("event must be one user text block", i)
		}
		block := content[0]
		var event struct {
			XMLName xml.Name
			ID      string `xml:"id,attr"`
		}
		if err := xml.Unmarshal([]byte(block.Text), &event); err != nil {
			t.Fatal(err)
		}
		if i == len(items)-1 {
			if event.XMLName.Local != "hints" || block.Breakpoint != nil {
				t.Fatal("hints must be separate and unmarked")
			}
			break
		}
		if event.ID == "" {
			t.Fatal("invalid event block", i)
		}
		result.ids = append(result.ids, event.ID)
		result.events = append(result.events, block.Text)
		if block.Breakpoint != nil {
			if block.Breakpoint.Mode != "explicit" {
				t.Fatal("invalid marker", i)
			}
			result.marked = append(result.marked, event.ID)
		}
	}
	if len(result.marked) == 0 || len(result.marked) > 4 || result.marked[len(result.marked)-1] != result.ids[len(result.ids)-1] {
		t.Fatal("invalid breakpoint count or trailing event not marked", result.marked)
	}
	return result
}
