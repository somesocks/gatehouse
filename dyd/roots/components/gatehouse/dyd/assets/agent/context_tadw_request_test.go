package agent

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"

	"gatehouse/config"
)

func TestTADWRequestRendersEveryAttempt(t *testing.T) {
	store, initial, limits, request, _ := tadwToolingDB(t)
	selected := tadwProviderDatabaseAgent(t, store, request)
	raw, err := json.Marshal(config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: limits.ContextMaxBytes, HistoryMaxBytes: limits.HistoryMaxBytes, HistoryMinBytes: limits.HistoryMinBytes}})
	if err != nil {
		t.Fatal(err)
	}
	selected.Compaction = string(raw)
	choice, err := selectedAgentContext(selected)
	if err != nil {
		t.Fatal(err)
	}
	scope := tadwPreparationScope(t, request.Session, selected, false)
	choice.Compatibility, err = tadwRequestCompatibility(selected.Ref, scope, limits)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &SessionEventReplyRuntime{store: store}
	state := &tadwRequestContext{request: request, choice: choice, historyIDs: append([]string(nil), initial.Window.EventIDs...)}
	ids := append([]string(nil), state.historyIDs...)
	err, thinking := store.SessionEventGet(context.Background(), tadwToolingThinking(request, 40))
	if err != nil {
		t.Fatal(err)
	}
	update := func(text, name string) {
		if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload=json_object('text',?) WHERE id=?`, text, tadwToolingThinking(request, 1).Id); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload=json_object('code','cancelled','output',?) WHERE id=?`, text+"_TOOL", tadwToolingThinking(request, 31).Id); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Exec(`UPDATE gatehouse_session_files SET name=?`, name); err != nil {
			t.Fatal(err)
		}
	}
	update("AFTER_DECISION", "after-decision.txt")
	first, err := runtime.renderRequestTADW(context.Background(), state, *thinking, selected, false)
	if err != nil {
		t.Fatal(err)
	}
	firstXML, _ := tadwProviderUserContent(t, first)
	if !strings.Contains(firstXML, "AFTER_DECISION_TOOL") || !strings.Contains(firstXML, `name="after-decision.txt"`) || strings.Contains(firstXML, "résumé") {
		t.Fatal("provider content does not match current event payloads and file metadata")
	}
	update("NEXT_ATTEMPT", "next-attempt.txt")
	second, err := runtime.renderRequestTADW(context.Background(), state, *thinking, selected, false)
	if err != nil {
		t.Fatal(err)
	}
	secondXML, _ := tadwProviderUserContent(t, second)
	if !strings.Contains(secondXML, "NEXT_ATTEMPT_TOOL") || !strings.Contains(secondXML, `name="next-attempt.txt"`) || strings.Contains(secondXML, "AFTER_DECISION") || strings.Contains(secondXML, "after-decision.txt") {
		t.Fatal("retry content does not match updated event payloads and file metadata")
	}
	unchanged, _ := tadwProviderUserContent(t, first)
	if unchanged != firstXML || !reflect.DeepEqual(state.historyIDs, ids) {
		t.Fatal("render mutated another attempt or recorded IDs")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	failed, err := runtime.renderRequestTADW(canceled, state, *thinking, selected, false)
	if !errors.Is(err, context.Canceled) || failed.body() != nil {
		t.Fatal("cancelled render must return cancellation and an empty body", err)
	}
	if _, err := store.Exec(`DELETE FROM gatehouse_session_events WHERE id=?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	failed, err = runtime.renderRequestTADW(context.Background(), state, *thinking, selected, false)
	if err == nil || !strings.Contains(err.Error(), "event not found") || failed.body() != nil {
		t.Fatal("missing history must return event-not-found and an empty body", err)
	}
}

func TestReplyExecutionConsumesDeclaredStepResults(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		tree, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(tree, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok && (selector.Sel.Name == "GetWorkflowSteps" || selector.Sel.Name == "GetStepID") {
				t.Errorf("%s: workflow execution must consume declared step results, not inspect checkpoint records", name)
			}
			return true
		})
		for _, table := range []string{"operation_outputs", "transaction_completion"} {
			if strings.Contains(string(source), table) {
				t.Errorf("%s: application execution must not read DBOS checkpoint tables", name)
			}
		}
	}
}
