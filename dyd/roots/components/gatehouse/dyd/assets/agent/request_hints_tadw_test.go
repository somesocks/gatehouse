package agent

import (
	"context"
	"encoding/xml"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"gatehouse/database"
)

func tadwTestHintsXML(t *testing.T, selected *database.WorkspaceAgentModel, hints tadwRequestHints) string {
	t.Helper()
	text, err := renderTADWRequestHints(selected, hints)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestTADWRequestHintsWireShapeAndEscaping(t *testing.T) {
	selected, prepared, transcript, limits, hints := tadwProviderFixture(t, "openai-responses")
	selected.Alias = "lúna<&\"🙂"
	prepared.Scope = tadwPreparationScope(t, prepared.Scope.Session, selected, false)
	suffix := tadwTestHintsXML(t, selected, hints)
	var decoded struct {
		Agent struct {
			Alias string `xml:"alias,attr"`
			ID    string `xml:"id,attr"`
		} `xml:"agent"`
		Request struct {
			ID      string `xml:"id,attr"`
			Message string `xml:"message,attr"`
		} `xml:"request"`
		ToolingStart struct {
			ID string `xml:"id,attr"`
		} `xml:"tooling-start"`
	}
	if err := xml.Unmarshal([]byte(suffix), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Agent.Alias != selected.Alias || decoded.Agent.ID != selected.Ref.Id || decoded.Request.ID != hints.Request.Id || decoded.Request.Message != hints.Message.Id || decoded.ToolingStart.ID != transcript.Tooling[0].ID {
		t.Fatalf("hints lost their actual identity or boundary: %#v", decoded)
	}
	if !strings.Contains(suffix, `alias="lúna&lt;&amp;&#34;🙂"`) || len(suffix) <= utf8.RuneCountInString(suffix) {
		t.Fatal("hints did not preserve escaped UTF-8", suffix)
	}
	if !strings.Contains(suffix, "Answer the identified message as this agent.") || !strings.Contains(suffix, "use its tool results as work already performed.") {
		t.Fatal("missing request/tool-result orientation", suffix)
	}
	prepared.History, limits = tadwTideBudget(prepared.History, transcript.ToolingBytes()+int64(len(suffix)))
	err, wire := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, limits, hints)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := tadwProviderUserContent(t, wire)
	if content != transcript.String()+suffix || int64(len(content)) != limits.ContextMaxBytes || strings.Count(content, "<hints>") != 1 {
		t.Fatal("hints must be a single, budgeted suffix after unchanged events", content)
	}
}

func TestTADWRequestHintsRejectInvalidReferences(t *testing.T) {
	for _, name := range []string{"missing-agent", "agent-id", "alias-utf8", "request-id", "message-id", "same-event", "session", "scope-session", "workspace", "tooling-start", "hint-budget"} {
		t.Run(name, func(t *testing.T) {
			selected, prepared, transcript, limits, hints := tadwProviderFixture(t, "openai-responses")
			switch name {
			case "missing-agent":
				selected = nil
			case "agent-id":
				selected.Ref.Id = "bad"
			case "alias-utf8":
				selected.Alias = "\xff"
			case "request-id":
				hints.Request.Id = "bad"
			case "message-id":
				hints.Message.Id = "bad"
			case "same-event":
				hints.Message = hints.Request
			case "session":
				hints.Message.Session.Id = "another-session"
			case "scope-session":
				hints.Request.Session.Id = "another-session"
				hints.Message.Session = hints.Request.Session
			case "workspace":
				hints.Request.Session.Workspace.Id = "another-workspace"
				hints.Message.Session = hints.Request.Session
			case "tooling-start":
				hints.Request.Id = "sev_00000000000000000000000099"
			case "hint-budget":
				prepared.History, limits = tadwTideBudget(prepared.History, int64(len(tadwTestHintsXML(t, selected, hints)))-1)
			}
			err, request := agentProviderRequestPrepareTADW(selected, false, prepared, transcript, limits, hints)
			if err == nil || !reflect.DeepEqual(request, agentProviderRequest{}) {
				t.Fatal("invalid hints must not produce a partial provider request", err)
			}
		})
	}
}

func TestLoadTADWRequestHintsUsesOwningRequestParent(t *testing.T) {
	store, _, _, request, _ := tadwToolingDB(t)
	selected := tadwProviderDatabaseAgent(t, store, request)
	hints, err := loadTADWRequestHints(context.Background(), store, request, selected.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if hints.Request != request || hints.Message != tadwToolingThinking(request, 5) {
		t.Fatal("hints did not use the owning request's actual parent", hints)
	}
	foreign := selected.Ref
	foreign.Id = "wag_00000000000000000000000002"
	if _, err := loadTADWRequestHints(context.Background(), store, request, foreign); err == nil {
		t.Fatal("accepted another agent's request")
	}
}
