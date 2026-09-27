package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func inputResponse(t *testing.T, store *database.Store, request model.SessionEventRef, id, kind string, payload map[string]interface{}) model.SessionEvent {
	t.Helper()
	principal := principalRef(t, context.Background(), store, "alice")
	return model.SessionEvent{
		Ref: model.SessionEventRef{Session: request.Session, Id: id}, Parent: &request,
		Kind: kind, AuthorPrincipal: &model.Principal{Ref: principal, Enabled: true}, Payload: payload,
	}
}

func TestSessionInputSubmitUsesValidatedStoredDraft(t *testing.T) {
	ctx := context.Background()
	store, request := inputDraftFixture(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	response := inputResponse(t, store, request, "sev_00000000000000000000000003", "input.success", nil)
	err, _ := store.SessionInputResponseCreate(ctx, response)
	if !errors.Is(err, database.ErrSessionInputInvalidDraft) || !strings.Contains(err.Error(), `"name" is required`) {
		t.Fatalf("submit incomplete draft error = %v", err)
	}
	if err, draft := store.SessionInputDraftGet(ctx, request); err != nil || draft != nil {
		t.Fatalf("failed submit left a draft row = (%#v, %v)", draft, err)
	}
	if err, _ := store.SessionInputDraftSet(ctx, request, []string{"name"}, json.RawMessage(`""`)); err != nil {
		t.Fatal(err)
	}
	err, _ = store.SessionInputResponseCreate(ctx, response)
	if !errors.Is(err, database.ErrSessionInputInvalidDraft) || !strings.Contains(err.Error(), "text length is out of bounds") {
		t.Fatalf("submit short name error = %v", err)
	}
	if err, draft := store.SessionInputDraftGet(ctx, request); err != nil || draft == nil || string(draft.Values) != `{"name":""}` {
		t.Fatalf("invalid submit changed draft = (%#v, %v)", draft, err)
	}
	for _, patch := range []struct {
		path  []string
		value string
	}{
		{[]string{"name"}, `"Ada"`},
		{[]string{"count"}, `9007199254740993`},
	} {
		if err, _ := store.SessionInputDraftSet(ctx, request, patch.path, json.RawMessage(patch.value)); err != nil {
			t.Fatal(err)
		}
	}
	err, stored := store.SessionInputResponseCreate(ctx, response)
	if err != nil || stored.Ref != response.Ref || stored.Kind != "input.success" || stored.AuthorPrincipal == nil {
		t.Fatalf("submit stored draft = (%#v, %v)", stored, err)
	}
	err, text := store.SessionInputResponseResultGet(ctx, stored.Ref)
	if err != nil || text != `{"count":9007199254740993,"name":"Ada"}` {
		t.Fatalf("stored result = (%q, %v)", text, err)
	}
	if err, draft := store.SessionInputDraftGet(ctx, request); err != nil || draft != nil {
		t.Fatalf("successful submit retained draft = (%#v, %v)", draft, err)
	}
	err, tasks := store.SessionInputResponseTasksGet(ctx, 10)
	if err != nil || len(tasks) != 1 || tasks[0].Input != request || tasks[0].Response != stored.Ref {
		t.Fatalf("undelivered response tasks = (%#v, %v)", tasks, err)
	}
	if err, _ := store.SessionInputDraftSet(ctx, request, []string{"name"}, json.RawMessage(`"Grace"`)); !errors.Is(err, database.ErrSessionInputResolved) {
		t.Fatalf("late draft update error = %v", err)
	}
	if err, _ := store.SessionInputDraftRemove(ctx, request, []string{"name"}); !errors.Is(err, database.ErrSessionInputResolved) {
		t.Fatalf("late draft removal error = %v", err)
	}
	second := inputResponse(t, store, request, "sev_00000000000000000000000004", "input.failure", map[string]interface{}{"code": "cancelled"})
	if err, _ := store.SessionInputResponseCreate(ctx, second); !errors.Is(err, database.ErrSessionInputResolved) {
		t.Fatalf("duplicate resolution error = %v", err)
	}
	if err, event := store.SessionEventGet(ctx, second.Ref); err != nil || event != nil {
		t.Fatalf("duplicate response event = (%#v, %v)", event, err)
	}
	if err := store.SessionInputResponseTaskDelivered(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err, tasks := store.SessionInputResponseTasksGet(ctx, 10); err != nil || len(tasks) != 0 {
		t.Fatalf("delivered response tasks = (%#v, %v)", tasks, err)
	}
}

func TestSessionInputFailureAndInvalidResponse(t *testing.T) {
	ctx := context.Background()
	store, request := inputDraftFixture(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	response := inputResponse(t, store, request, "sev_00000000000000000000000003", "input.failure", map[string]interface{}{"code": " "})
	if err, _ := store.SessionInputResponseCreate(ctx, response); err == nil {
		t.Fatal("accepted a blank failure code")
	}
	response.Payload = map[string]interface{}{"code": "cancelled"}
	err, stored := store.SessionInputResponseCreate(ctx, response)
	if err != nil || stored.Payload["code"] != "cancelled" {
		t.Fatalf("cancel input = (%#v, %v)", stored, err)
	}
	if err, draft := store.SessionInputDraftGet(ctx, request); err != nil || draft != nil {
		t.Fatalf("cancel retained draft = (%#v, %v)", draft, err)
	}
	if err, _ := store.SessionInputResponseCreate(ctx, inputResponse(t, store, request, "sev_00000000000000000000000004", "input.success", nil)); !errors.Is(err, database.ErrSessionInputResolved) {
		t.Fatalf("success after cancellation error = %v", err)
	}
}

func TestSessionInputPatchAndSubmitSerializeAcrossStores(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "input.sqlite")}
	first, request := inputDraftFixture(t, configuration)
	err, second := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err, _ := first.SessionInputDraftSet(ctx, request, []string{"name"}, json.RawMessage(`"Ada"`)); err != nil {
		t.Fatal(err)
	}
	response := inputResponse(t, first, request, "sev_00000000000000000000000003", "input.success", nil)
	var wait sync.WaitGroup
	start := make(chan struct{})
	patchErr := make(chan error, 1)
	responseErr := make(chan error, 1)
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		err, _ := first.SessionInputDraftSet(ctx, request, []string{"name"}, json.RawMessage(`"Grace"`))
		patchErr <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		err, _ := second.SessionInputResponseCreate(ctx, response)
		responseErr <- err
	}()
	close(start)
	wait.Wait()
	if err := <-responseErr; err != nil {
		t.Fatal(err)
	}
	err, result := second.SessionInputResponseResultGet(ctx, response.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-patchErr; err == nil {
		if result != `{"name":"Grace"}` {
			t.Fatalf("accepted patch was not included in terminal result: %s", result)
		}
	} else if !errors.Is(err, database.ErrSessionInputResolved) || result != `{"name":"Ada"}` {
		t.Fatalf("late patch error = %v, terminal result = %s", err, result)
	}
}

func TestSessionInputStateReadIsConsistentWithResolution(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "input-state.sqlite")}
	first, request := inputDraftFixture(t, configuration)
	err, initial := first.SessionInputStateGet(ctx, request)
	if err != nil || initial == nil || initial.Resolved || initial.UpdatedAt != nil || string(initial.Draft) != `{}` {
		t.Fatalf("initial input state = (%#v, %v)", initial, err)
	}
	if err, draft := first.SessionInputDraftSet(ctx, request, []string{"name"}, json.RawMessage(`"Ada"`)); err != nil || draft == nil {
		t.Fatalf("seed input draft = (%#v, %v)", draft, err)
	}
	err, second := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	response := inputResponse(t, first, request, "sev_00000000000000000000000003", "input.success", nil)
	start := make(chan struct{})
	stateRead := make(chan struct {
		state *database.SessionInputState
		err   error
	}, 1)
	responseErr := make(chan error, 1)
	go func() {
		<-start
		err, state := first.SessionInputStateGet(ctx, request)
		stateRead <- struct {
			state *database.SessionInputState
			err   error
		}{state, err}
	}()
	go func() {
		<-start
		err, _ := second.SessionInputResponseCreate(ctx, response)
		responseErr <- err
	}()
	close(start)
	read := <-stateRead
	if read.err != nil || read.state == nil {
		t.Fatalf("concurrent input state = (%#v, %v)", read.state, read.err)
	}
	if err := <-responseErr; err != nil {
		t.Fatal(err)
	}
	if read.state.Resolved {
		if string(read.state.Draft) != `{}` || read.state.UpdatedAt != nil {
			t.Fatalf("resolved input exposed stale draft: %#v", read.state)
		}
	} else if string(read.state.Draft) != `{"name":"Ada"}` || read.state.UpdatedAt == nil {
		t.Fatalf("pending input lost its draft: %#v", read.state)
	}
	err, final := first.SessionInputStateGet(ctx, request)
	if err != nil || final == nil || !final.Resolved || final.UpdatedAt != nil || string(final.Draft) != `{}` {
		t.Fatalf("resolved input state = (%#v, %v)", final, err)
	}
}
