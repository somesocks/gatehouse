package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/model"
)

func tadwToolingFixture(t *testing.T) (*Store, model.SessionEventRef, model.WorkspaceAgentRef, model.SessionEventRef) {
	t.Helper()
	store := tadwCursorStore(t)
	tadwCursorInsert(t, store.DB, tadwCursorScope(),
		tadwCursorFixtureNode{1, 0, "message.text", "00"},
		tadwCursorFixtureNode{2, 1, "agent.request", "02"},
		tadwCursorFixtureNode{10, 1, "agent.request", "10"},
		tadwCursorFixtureNode{15, 10, "thinking.request", "15"},
		tadwCursorFixtureNode{20, 10, "tool.request", "20"},
		tadwCursorFixtureNode{25, 10, "tool.request", "20"},
		tadwCursorFixtureNode{30, 10, "agent.request", "30"},
		tadwCursorFixtureNode{31, 30, "tool.request", "31"},
		tadwCursorFixtureNode{35, 10, "thinking.success", "35"},
		tadwCursorFixtureNode{36, 35, "tool.request", "36"},
		tadwCursorFixtureNode{40, 20, "tool.success", "40"},
		tadwCursorFixtureNode{45, 2, "tool.request", "45"},
		tadwCursorFixtureNode{50, 25, "tool.failure", "50"},
		tadwCursorFixtureNode{55, 10, "approval.request", "55"},
		tadwCursorFixtureNode{56, 55, "input.request", "56"},
		tadwCursorFixtureNode{57, 56, "tool.success", "57"},
		tadwCursorFixtureNode{60, 10, "thinking.request", "60"},
		tadwCursorFixtureNode{70, 10, "tool.request", "05"}, // Backdated timestamp, but after boundary ID.
		tadwCursorFixtureNode{71, 70, "tool.success", "71"},
		tadwCursorFixtureNode{80, 10, "thinking.request", "80"},
	)
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='{"agent":"agent"}' WHERE kind='agent.request';
	 UPDATE gatehouse_session_events SET author_principal=NULL,author_agent='agent' WHERE kind<>'agent.request' AND kind<>'message.text';
	 UPDATE gatehouse_session_events SET author_agent='other' WHERE id=?;
	 UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id IN (?,?);`, tadwCursorID(35), tadwCursorID(15), tadwCursorID(55)); err != nil {
		t.Fatal(err)
	}
	return store, tadwCursorRef(10), model.WorkspaceAgentRef{Workspace: tadwCursorScope().Workspace, Id: "agent"}, tadwCursorRef(60)
}

func collectTADWToolingCursor(t *testing.T, cursor *TADWToolingCursor) []model.SessionEvent {
	t.Helper()
	var events []model.SessionEvent
	for cursor.Next() {
		events = append(events, cursor.Event())
	}
	if err := cursor.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func TestTADWToolingCursorSelectionOrderAndBoundary(t *testing.T) {
	store, request, agent, thinking := tadwToolingFixture(t)
	// Deliberate scoped ID collisions must not become metadata or payload reads.
	other := tadwCursorScope()
	other.Id = "other"
	tadwCursorInsert(t, store.DB, other, tadwCursorFixtureNode{20, 10, "tool.request", "01"})
	other.Id, other.Workspace.Id = "s", "other"
	tadwCursorInsert(t, store.DB, other, tadwCursorFixtureNode{20, 10, "tool.request", "01"})
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err, cursor := store.SessionTADWToolingCursor(context.Background(), tx, request, agent, thinking)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Close()
	events := collectTADWToolingCursor(t, cursor)
	var ids []string
	for _, event := range events {
		if event.Ref.Session != request.Session {
			t.Fatal("scope leaked", event)
		}
		ids = append(ids, event.Ref.Id)
	}
	want := []string{tadwCursorID(10), tadwCursorID(20), tadwCursorID(25), tadwCursorID(40), tadwCursorID(50), tadwCursorID(57)}
	if !slices.Equal(ids, want) {
		t.Fatal("wrong membership or (created_at,id) order", ids)
	}
	if events[3].Parent == nil || events[3].Parent.Id != tadwCursorID(20) || events[5].Parent.Id != tadwCursorID(56) {
		t.Fatal("actual parents lost across omitted bookkeeping")
	}
	if cursor.Next() || cursor.Close() != nil || !reflect.DeepEqual(cursor.Event(), model.SessionEvent{}) {
		t.Fatal("cursor exhaustion/close contract")
	}
}

func TestTADWToolingCursorValidationAndResources(t *testing.T) {
	for _, name := range []string{"nil-tx", "empty-scope", "wrong-agent-workspace", "wrong-session", "bad-id", "early-boundary", "missing-root", "wrong-root-kind", "wrong-target", "missing-thinking", "wrong-thinking-kind", "wrong-thinking-parent", "wrong-thinking-author", "cancelled", "closed-tx"} {
		t.Run(name, func(t *testing.T) {
			store, request, agent, thinking := tadwToolingFixture(t)
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var mutation string
			switch name {
			case "nil-tx":
				tx.Rollback()
				tx = nil
			case "empty-scope":
				request.Session.Id = ""
			case "wrong-agent-workspace":
				agent.Workspace.Id = "other"
			case "wrong-session":
				thinking.Session.Id = "other"
			case "bad-id":
				thinking.Id = "not-an-event"
			case "early-boundary":
				thinking = request
			case "missing-root":
				request.Id = tadwCursorID(9)
			case "wrong-root-kind":
				mutation = `UPDATE gatehouse_session_events SET kind='message.text' WHERE id='` + request.Id + `'`
			case "wrong-target":
				agent.Id = "other"
			case "missing-thinking":
				thinking.Id = tadwCursorID(61)
			case "wrong-thinking-kind":
				mutation = `UPDATE gatehouse_session_events SET kind='tool.request' WHERE id='` + thinking.Id + `'`
			case "wrong-thinking-parent":
				mutation = `UPDATE gatehouse_session_events SET parent='` + tadwCursorID(2) + `' WHERE id='` + thinking.Id + `'`
			case "wrong-thinking-author":
				mutation = `UPDATE gatehouse_session_events SET author_agent='other' WHERE id='` + thinking.Id + `'`
			case "cancelled":
				cancel()
			case "closed-tx":
				tx.Rollback()
			}
			if mutation != "" {
				if _, err := tx.Exec(mutation); err != nil {
					t.Fatal(err)
				}
			}
			if err, cursor := store.SessionTADWToolingCursor(ctx, tx, request, agent, thinking); err == nil || cursor != nil {
				t.Fatal("invalid cursor accepted", cursor, err)
			}
		})
	}
	store, request, agent, thinking := tadwToolingFixture(t)
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err, cursor := store.SessionTADWToolingCursor(ctx, tx, request, agent, thinking)
	if err != nil || !cursor.Next() {
		t.Fatal(err)
	}
	cancel()
	if cursor.Next() || cursor.Err() == nil || cursor.Close() != nil {
		t.Fatal("mid-cursor cancellation not reported")
	}
	// Closing the cursor must not close its caller's transaction.
	var one int
	if err := tx.QueryRow(`SELECT 1`).Scan(&one); err != nil {
		t.Fatal(err)
	}
}

func TestTADWToolingCursorSnapshotAndLazyPayloads(t *testing.T) {
	store, request, agent, thinking := tadwToolingFixture(t)
	err, writer := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: storePathForTADWTest(t, store)})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err, cursor := store.SessionTADWToolingCursor(context.Background(), tx, request, agent, thinking)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Close()
	if _, err := writer.Exec(`UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id=?`, tadwCursorID(20)); err != nil {
		t.Fatal(err)
	}
	if events := collectTADWToolingCursor(t, cursor); len(events) != 6 {
		t.Fatal(events)
	}
	tx.Rollback()
	tx, err = store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err, cursor = store.SessionTADWToolingCursor(context.Background(), tx, request, agent, thinking)
	if err != nil || !cursor.Next() {
		t.Fatal("later corrupt payload eagerly decoded", err)
	}
	if cursor.Next() || cursor.Err() == nil {
		t.Fatal("corrupt payload not decoded when requested")
	}
}

func TestTADWToolingCursorAndFileMetadataShareSnapshot(t *testing.T) {
	store, request, agent, thinking := tadwToolingFixture(t)
	if _, err := store.Exec(`CREATE TABLE gatehouse_storage_objects(id TEXT PRIMARY KEY,size INTEGER);
	 CREATE TABLE gatehouse_session_files(workspace TEXT,session TEXT,id TEXT,storage_object TEXT,name TEXT,media_type TEXT,enabled INTEGER,PRIMARY KEY(workspace,session,id));
	 INSERT INTO gatehouse_storage_objects VALUES('object',9007199254740993);
	 INSERT INTO gatehouse_session_files VALUES('w','s','sfi_00000000000000000000000001','object','original',NULL,0);`); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"output_file": model.SessionFileRef{Session: request.Session, Id: tadwFileID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload=? WHERE id=?`, string(payload), tadwCursorID(40)); err != nil {
		t.Fatal(err)
	}
	err, writer := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: storePathForTADWTest(t, store)})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err, cursor := store.SessionTADWToolingCursor(context.Background(), tx, request, agent, thinking)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Close()
	if _, err := writer.Exec(`UPDATE gatehouse_session_files SET name='renamed'`); err != nil {
		t.Fatal(err)
	}
	found := false
	for cursor.Next() {
		event := cursor.Event()
		if event.Ref.Id == tadwCursorID(40) {
			found = true
			err, projected := store.SessionEventFilesHydrateInTransaction(context.Background(), tx, event)
			if err != nil {
				t.Fatal(err)
			}
			file := projected.Payload["output_file"].(map[string]any)
			if file["name"] != "original" || file["size"] != int64(9007199254740993) {
				t.Fatal("file metadata escaped the tooling transaction", file)
			}
		}
	}
	if cursor.Err() != nil || !found {
		t.Fatal(found, cursor.Err())
	}
	tx.Rollback()
	var fresh string
	if err := store.QueryRow(`SELECT name FROM gatehouse_session_files`).Scan(&fresh); err != nil || fresh != "renamed" {
		t.Fatal(fresh, err)
	}
}

func TestTADWToolingCursorRetainedBoundariesAndReverse(t *testing.T) {
	store, request, agent, _ := tadwToolingFixture(t)
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id=?`, tadwCursorID(20)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		from     int
		reverse  bool
		thinking int
		want     []int
	}{
		{25, false, 60, []int{10, 25, 40, 50, 57}},
		{25, true, 60, []int{57, 50, 40, 25, 10}},
		{40, false, 60, []int{10, 40, 50, 57}},
		// A newer-ID event with an older timestamp stays before this cutoff;
		// its newer result is retained, so do not prune ancestors at the cutoff.
		{40, false, 80, []int{10, 40, 50, 57, 71}},
	} {
		tx, err := store.Begin()
		if err != nil {
			t.Fatal(err)
		}
		err, cursor := store.SessionTADWToolingCursor(context.Background(), tx, request, agent, tadwCursorRef(test.thinking), TADWToolingReadOptions{FromEventID: tadwCursorID(test.from), Reverse: test.reverse})
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if cursor.RequestEvent().Ref != request {
			t.Fatal("validated request unavailable")
		}
		events := collectTADWToolingCursor(t, cursor)
		var ids, want []string
		for _, event := range events {
			ids = append(ids, event.Ref.Id)
		}
		for _, id := range test.want {
			want = append(want, tadwCursorID(id))
		}
		if !slices.Equal(ids, want) {
			t.Fatal(ids, want)
		}
		if cursor.RequestEvent().Ref.Id != "" {
			t.Fatal("closed cursor retains request")
		}
		tx.Rollback()
	}
}

func TestTADWToolingCursorRejectsForeignOrMissingRetainedBoundary(t *testing.T) {
	store, request, agent, thinking := tadwToolingFixture(t)
	for _, id := range []int{1, 10, 15, 31, 36, 45, 60, 70, 999} {
		tx, err := store.Begin()
		if err != nil {
			t.Fatal(err)
		}
		err, cursor := store.SessionTADWToolingCursor(context.Background(), tx, request, agent, thinking, TADWToolingReadOptions{FromEventID: tadwCursorID(id)})
		if err == nil || cursor != nil {
			t.Fatal("accepted invalid retained boundary", id, err)
		}
		tx.Rollback()
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err, _ := store.SessionTADWToolingCursor(context.Background(), tx, request, agent, thinking, TADWToolingReadOptions{FromEventID: "bad"}); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if err, _ := store.SessionTADWToolingCursor(context.Background(), tx, request, agent, thinking, TADWToolingReadOptions{}, TADWToolingReadOptions{}); err == nil {
		t.Fatal("multiple options accepted")
	}
}

func TestTADWToolingQueryScopedSeeks(t *testing.T) {
	store, request, agent, thinking := tadwToolingFixture(t)
	rows, err := store.Query(`EXPLAIN QUERY PLAN `+tadwToolingIDsSQL,
		sql.Named("request", request.Id), sql.Named("workspace", request.Session.Workspace.Id), sql.Named("session", request.Session.Id), sql.Named("thinking", thinking.Id), sql.Named("agent", agent.Id))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	childSeek, eventSeek := false, false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		childSeek = childSeek || strings.Contains(detail, "SEARCH child") && strings.Contains(detail, "parent=?")
		eventSeek = eventSeek || strings.Contains(detail, "SEARCH events") && strings.Contains(detail, "id=?")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !childSeek || !eventSeek {
		t.Fatalf("missing scoped child/ID seeks: child=%v event=%v", childSeek, eventSeek)
	}
}
