package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gatehouse/internal/testdbos"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

func TestTADWHandoffRendersEveryAttempt(t *testing.T) {
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "tadw-handoff-render")
	store, _, limits, request, _ := tadwToolingDB(t)
	selected := tadwProviderDatabaseAgent(t, store, request)
	tadwCacheTestTable(t, store)
	root := tadwHandoffRoot(t, store)
	worker := tadwHandoffTestWorker(t, root, store)
	owner := func(ctx dbos.Context, request model.SessionEventRef) (tadwHistoryHandoff, error) {
		return worker.Prepare(ctx, request, tadwToolingThinking(request, 5), selected, false, limits)
	}
	roundWorkflow := func(ctx dbos.Context, handoff tadwHistoryHandoff) (bool, error) {
		thinking, err := dbos.RunAsStep(ctx, func(context.Context) (model.SessionEventRef, error) { return tadwToolingThinking(request, 40), nil }, dbos.WithStepName("gatehouse.agent-round-prepare"))
		if err != nil {
			return false, err
		}
		round, err := worker.Round(ctx, tadwRoundHandoff{History: handoff}, thinking, selected, false, limits)
		if err != nil {
			return false, err
		}
		ids := append([]string(nil), round.historyIDs...)
		// Each provider attempt renders current event payloads and file metadata
		// using the same recorded IDs and boundary.
		return dbos.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
			update := func(text, name string) error {
				if _, err := store.ExecContext(stepCtx, `UPDATE gatehouse_session_events SET payload=json_object('text',?) WHERE id=?`, text, tadwToolingThinking(request, 1).Id); err != nil {
					return err
				}
				if _, err := store.ExecContext(stepCtx, `UPDATE gatehouse_session_events SET payload=json_object('code','cancelled','output',?) WHERE id=?`, text+"_TOOL", tadwToolingThinking(request, 31).Id); err != nil {
					return err
				}
				_, err := store.ExecContext(stepCtx, `UPDATE gatehouse_session_files SET name=?`, name)
				return err
			}
			if err := update("AFTER_DECISION", "after-decision.txt"); err != nil {
				return false, err
			}
			first, err := worker.Render(stepCtx, round, selected, false)
			if err != nil {
				return false, err
			}
			firstXML, _ := tadwProviderUserContent(t, first)
			if !strings.Contains(firstXML, "AFTER_DECISION_TOOL") || !strings.Contains(firstXML, `name="after-decision.txt"`) || strings.Contains(firstXML, "résumé") {
				return false, fmt.Errorf("provider content does not match current event payloads and file metadata")
			}
			if err := update("NEXT_ATTEMPT", "next-attempt.txt"); err != nil {
				return false, err
			}
			second, err := worker.Render(stepCtx, round, selected, false)
			if err != nil {
				return false, err
			}
			secondXML, _ := tadwProviderUserContent(t, second)
			if !strings.Contains(secondXML, "NEXT_ATTEMPT_TOOL") || !strings.Contains(secondXML, `name="next-attempt.txt"`) || strings.Contains(secondXML, "after-decision.txt") || strings.Contains(secondXML, "AFTER_DECISION") || firstXML == secondXML {
				return false, fmt.Errorf("retry content does not match updated event payloads and file metadata")
			}
			unchanged, _ := tadwProviderUserContent(t, first)
			if unchanged != firstXML || !reflect.DeepEqual(round.historyIDs, ids) {
				return false, fmt.Errorf("render mutated another attempt or recorded IDs")
			}
			canceled, cancel := context.WithCancel(stepCtx)
			cancel()
			failed, err := worker.Render(canceled, round, selected, false)
			if !errors.Is(err, context.Canceled) || failed.body() != nil {
				return false, fmt.Errorf("cancelled render returned a body: %v", err)
			}
			if _, err := store.ExecContext(stepCtx, `DELETE FROM gatehouse_session_events WHERE id=?`, ids[0]); err != nil {
				return false, err
			}
			failed, err = worker.Render(stepCtx, round, selected, false)
			if err == nil || !strings.Contains(err.Error(), "event not found") || failed.body() != nil {
				return false, fmt.Errorf("missing history: want event-not-found error and empty body, got %v", err)
			}
			return true, nil
		}, dbos.WithStepName("test.handoff-render-provider"))
	}
	dbos.RegisterWorkflow(root, owner, dbos.WithWorkflowName("test.handoff-render-owner"))
	dbos.RegisterWorkflow(root, roundWorkflow, dbos.WithWorkflowName("test.handoff-render-round"))
	if err := testdbos.Launch(root); err != nil {
		t.Fatal(err)
	}
	handle, err := dbos.RunWorkflow(root, owner, request, dbos.WithWorkflowID(tadwHistoryOwnerWorkflowID(request)))
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := testdbos.GetResult(handle)
	if err != nil {
		t.Fatal(err)
	}
	round, err := dbos.RunWorkflow(root, roundWorkflow, handoff, dbos.WithWorkflowID("handoff-render-round"))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := testdbos.GetResult(round); err != nil || !ok {
		t.Fatal(ok, err)
	}
	assertTADWDBOSPayloadAbsent(t, store)
}
