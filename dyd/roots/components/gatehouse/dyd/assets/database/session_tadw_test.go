package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"gatehouse/config"
	"gatehouse/model"
	"modernc.org/sqlite"
)

// Reduced schema preserves the production payload/created_at layout and index
// shapes, while permitting deliberate cross-scope ID collisions and bad rows.
const tadwCursorTestSchema = `
 CREATE TABLE gatehouse_principals(id TEXT PRIMARY KEY,alias TEXT,name TEXT,enabled INTEGER);
 INSERT INTO gatehouse_principals VALUES('user','alice','Alice',1);
 CREATE TABLE gatehouse_session_events(
 workspace TEXT,session TEXT,id TEXT,parent TEXT,kind TEXT,
 author_principal TEXT,author_agent TEXT,author_gateway TEXT,
 payload TEXT NOT NULL,created_at TEXT NOT NULL,metrics TEXT,
 PRIMARY KEY(workspace,session,id));
 CREATE INDEX gatehouse_session_events_by_session_order ON gatehouse_session_events(workspace,session,id);
 CREATE INDEX gatehouse_session_events_children ON gatehouse_session_events(workspace,session,parent,id);
 CREATE INDEX gatehouse_session_events_thread_order ON gatehouse_session_events(workspace,session,parent,created_at DESC,id DESC);
 CREATE INDEX gatehouse_session_events_agent_terminals ON gatehouse_session_events(workspace,session,parent,kind,created_at,id)
 WHERE kind IN ('agent.success','agent.failure');`

type tadwCursorFixtureNode struct {
	id, parent int
	kind, at   string
}

func tadwCursorID(id int) string { return fmt.Sprintf("sev_%026d", id) }
func tadwCursorScope() model.SessionRef {
	return model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}
}
func tadwCursorRef(id int) model.SessionEventRef {
	return model.SessionEventRef{Session: tadwCursorScope(), Id: tadwCursorID(id)}
}

func tadwCursorStore(t *testing.T) *Store {
	t.Helper()
	err, store := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "history.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.Exec(`PRAGMA journal_mode=WAL;` + tadwCursorTestSchema); err != nil {
		t.Fatal(err)
	}
	return store
}
func tadwCursorInsert(t *testing.T, db *sql.DB, scope model.SessionRef, nodes ...tadwCursorFixtureNode) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO gatehouse_session_events(workspace,session,id,parent,kind,author_principal,payload,created_at)
 VALUES(?,?,?,?,?,'user','{"text":"small inline value"}',?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	for _, node := range nodes {
		var parent any
		if node.parent != 0 {
			parent = tadwCursorID(node.parent)
		}
		if _, err := stmt.Exec(scope.Workspace.Id, scope.Id, tadwCursorID(node.id), parent, node.kind, node.at); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
func tadwCursorBoundary(t *testing.T, store *Store) []tadwCursorFixtureNode {
	t.Helper()
	nodes := []tadwCursorFixtureNode{
		{1, 0, "message.text", "00"}, {2, 1, "agent.request", "02"}, {3, 1, "message.text", "04"},
		{4, 3, "agent.request", "05"}, {5, 3, "agent.request", "05"}, {6, 3, "agent.request", "06"}, {7, 3, "agent.request", "07"},
		{8, 4, "agent.success", "20"}, {9, 5, "agent.failure", "21"}, {10, 7, "agent.failure", "22"},
		{11, 1, "message.text", "08"}, {12, 1, "message.text", "08"}, {13, 1, "message.text", "09"},
		{14, 0, "message.text", "01"}, {15, 0, "message.text", "00"},
		{16, 11, "agent.request", "09"}, {17, 16, "agent.success", "10"},
	}
	tadwCursorInsert(t, store.DB, tadwCursorScope(), nodes...)
	return nodes
}
func tadwCursorCollect(t *testing.T, store *Store, tx *sql.Tx, anchor model.SessionEventRef) ([]TADWHistoryEvent, error) {
	t.Helper()
	err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, anchor)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()
	var got []TADWHistoryEvent
	for cursor.Next() {
		got = append(got, cursor.Event())
	}
	return got, cursor.Err()
}
func tadwCursorIDs(events []TADWHistoryEvent) []string {
	var ids []string
	for _, event := range events {
		ids = append(ids, event.Event.Ref.Id)
	}
	return ids
}

func TestTADWHistoryCursorSelection(t *testing.T) {
	store := tadwCursorStore(t)
	nodes := tadwCursorBoundary(t, store)
	for _, reverse := range []string{"OFF", "ON"} {
		if _, err := store.Exec("PRAGMA reverse_unordered_selects=" + reverse); err != nil {
			t.Fatal(err)
		}
		tx, err := store.Begin()
		if err != nil {
			t.Fatal(err)
		}
		got, err := tadwCursorCollect(t, store, tx, tadwCursorRef(11))
		tx.Rollback()
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for _, id := range []int{12, 11, 6, 9, 5, 8, 4, 3, 2, 15, 1} {
			want = append(want, tadwCursorID(id))
		}
		if !slices.Equal(tadwCursorIDs(got), want) {
			t.Fatalf("reverse=%s got %v want %v", reverse, tadwCursorIDs(got), want)
		}
		byID := map[string]tadwCursorFixtureNode{}
		for _, node := range nodes {
			byID[tadwCursorID(node.id)] = node
		}
		for _, item := range got {
			node := byID[item.Event.Ref.Id]
			if item.Event.Kind != node.kind || item.Event.CreatedAt != node.at || item.Event.Ref.Session != tadwCursorScope() || item.Event.Payload["text"] != "small inline value" || item.Event.AuthorPrincipal == nil || item.Event.AuthorPrincipal.Ref.Id != "user" {
				t.Fatalf("not an actual decoded event: %+v", item)
			}
			if node.parent == 0 {
				if item.Event.Parent != nil {
					t.Fatal("invented root parent")
				}
			} else if item.Event.Parent == nil || item.Event.Parent.Id != tadwCursorID(node.parent) {
				t.Fatalf("lost parent: %+v", item)
			}
		}
		for _, index := range []int{2, 3, 4, 5, 6, 7} {
			if got[index].OwnerID != tadwCursorID(3) {
				t.Fatal("local tree lost its owner block")
			}
		}
	}
}

func TestTADWHistoryCursorScopesAndMissingAnchor(t *testing.T) {
	store := tadwCursorStore(t)
	tadwCursorBoundary(t, store)
	for _, scope := range []model.SessionRef{{Workspace: model.WorkspaceRef{Id: "other"}, Id: "s"}, {Workspace: model.WorkspaceRef{Id: "w"}, Id: "other"}} {
		tadwCursorInsert(t, store.DB, scope, tadwCursorFixtureNode{11, 0, "message.text", "99"}, tadwCursorFixtureNode{3, 11, "agent.request", "01"}, tadwCursorFixtureNode{18, 4, "agent.failure", "01"})
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	got, err := tadwCursorCollect(t, store, tx, tadwCursorRef(11))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range got {
		if item.Event.Ref.Session != tadwCursorScope() || item.Event.CreatedAt == "99" || item.Event.Ref.Id == tadwCursorID(18) {
			t.Fatal("scope leak", item)
		}
	}
	missing, err := tadwCursorCollect(t, store, tx, tadwCursorRef(999))
	if err != nil || len(missing) != 0 {
		t.Fatalf("missing anchor=%v err=%v", missing, err)
	}
}

func TestTADWHistoryCursorSnapshot(t *testing.T) {
	store := tadwCursorStore(t)
	tadwCursorBoundary(t, store)
	path := storePathForTADWTest(t, store)
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	before, err := tadwCursorCollect(t, store, tx, tadwCursorRef(11))
	if err != nil {
		t.Fatal(err)
	}
	err, writer := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	tadwCursorInsert(t, writer.DB, tadwCursorScope(), tadwCursorFixtureNode{18, 6, "agent.success", "99"}, tadwCursorFixtureNode{19, 1, "message.text", "08"})
	during, err := tadwCursorCollect(t, store, tx, tadwCursorRef(11))
	if err != nil || !reflect.DeepEqual(before, during) {
		t.Fatalf("concurrent events leaked: err=%v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Rollback()
	after, err := tadwCursorCollect(t, store, fresh, tadwCursorRef(11))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tadwCursorIDs(after), tadwCursorID(18)) || !slices.Contains(tadwCursorIDs(after), tadwCursorID(19)) {
		t.Fatal("fresh transaction missed concurrent events")
	}
}
func storePathForTADWTest(t *testing.T, store *Store) string {
	t.Helper()
	var seq int
	var name, path string
	if err := store.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTADWHistoryCursorErrorsAndClose(t *testing.T) {
	store := tadwCursorStore(t)
	tadwCursorBoundary(t, store)
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, anchor := range []model.SessionEventRef{{}, {Session: tadwCursorScope(), Id: "invalid"}} {
		if err, _ := store.SessionTADWHistoryCursor(context.Background(), tx, anchor); err == nil {
			t.Fatal("invalid anchor accepted")
		}
	}
	if err, _ := store.SessionTADWHistoryCursor(context.Background(), nil, tadwCursorRef(11)); err == nil {
		t.Fatal("nil transaction accepted")
	}
	other := &Store{kind: config.DatabaseKindPostgres}
	if err, _ := other.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(11)); err == nil {
		t.Fatal("PostgreSQL silently accepted")
	}
	err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(11))
	if err != nil {
		t.Fatal(err)
	}
	if err := cursor.Close(); err != nil || cursor.Next() || cursor.Err() != nil {
		t.Fatal("close before iteration failed", err)
	}
	if err := cursor.Close(); err != nil {
		t.Fatal(err)
	}
	var one int
	if err := tx.QueryRow("SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatal("cursor ended caller transaction", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	err, cursor = store.SessionTADWHistoryCursor(ctx, tx, tadwCursorRef(11))
	if err != nil {
		t.Fatal(err)
	}
	if !cursor.Next() {
		t.Fatal(cursor.Err())
	}
	cancel()
	if cursor.Next() || !errors.Is(cursor.Err(), context.Canceled) {
		t.Fatal("cancellation lost", cursor.Err())
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	err, cursor = store.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(11))
	if err != nil {
		t.Fatal(err)
	}
	if cursor.Next() || !errors.Is(cursor.Err(), sql.ErrTxDone) {
		t.Fatal("finished transaction accepted", cursor.Err())
	}
}

func TestTADWHistoryCursorRejectsDuplicateOutcomes(t *testing.T) {
	store := tadwCursorStore(t)
	tadwCursorBoundary(t, store)
	tadwCursorInsert(t, store.DB, tadwCursorScope(), tadwCursorFixtureNode{18, 4, "agent.failure", "30"})
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = tadwCursorCollect(t, store, tx, tadwCursorRef(11))
	if err == nil || !strings.Contains(err.Error(), "multiple terminal outcomes") {
		t.Fatal("duplicate outcomes not rejected", err)
	}
}

func TestTADWSelectedTreeUsesGenericParentLinks(t *testing.T) {
	meta := func(id, parent, kind string) tadwHistoryNode {
		return tadwHistoryNode{ID: id, Parent: sql.NullString{String: parent, Valid: parent != ""}, Kind: kind, CreatedAt: "00"}
	}
	request := meta("request", "outside", "agent.request")
	children := map[string][]tadwHistoryNode{
		"request": {meta("tool", "request", "tool.request"), meta("thinking", "request", "thinking.request")},
		"tool":    {meta("result", "tool", "tool.success")}, "thinking": {meta("trace", "thinking", "thinking.success")},
	}
	nodes, err := tadwSelectedTree(tadwHistorySeed{Owner: request}, children)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, node := range nodes {
		ids = append(ids, node.ID)
		if node.OwnerID != "request" {
			t.Fatal("owner changed")
		}
	}
	if !slices.Equal(ids, []string{"result", "tool", "trace", "thinking", "request"}) {
		t.Fatal(ids)
	}
	children["result"] = []tadwHistoryNode{meta("request", "result", "agent.request")}
	if _, err := tadwSelectedTree(tadwHistorySeed{Owner: request}, children); err == nil {
		t.Fatal("cycle accepted")
	}
}

// Work instrumentation exists only in the test binary, applied to the same
// query texts used by the cursor. It counts logical work, not physical I/O.
var tadwCursorWork struct{ nodes, lookups, previews, keys, candidates, children, payloads atomic.Int64 }

func init() {
	for name, counter := range map[string]*atomic.Int64{"gh_test_tadw_node": &tadwCursorWork.nodes, "gh_test_tadw_lookup": &tadwCursorWork.lookups, "gh_test_tadw_preview": &tadwCursorWork.previews, "gh_test_tadw_key": &tadwCursorWork.keys, "gh_test_tadw_candidate": &tadwCursorWork.candidates, "gh_test_tadw_child": &tadwCursorWork.children, "gh_test_tadw_payload": &tadwCursorWork.payloads} {
		counter := counter
		sqlite.MustRegisterScalarFunction(name, 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			counter.Add(1)
			return args[0], nil
		})
	}
}
func tadwCursorInstrument(cursor *TADWHistoryCursor) {
	for _, counter := range []*atomic.Int64{&tadwCursorWork.nodes, &tadwCursorWork.lookups, &tadwCursorWork.previews, &tadwCursorWork.keys, &tadwCursorWork.candidates, &tadwCursorWork.children, &tadwCursorWork.payloads} {
		counter.Store(0)
	}
	cursor.seedQuery = strings.ReplaceAll(cursor.seedQuery, "/* node */ n.id", "gh_test_tadw_node(n.id)")
	cursor.seedQuery = strings.ReplaceAll(cursor.seedQuery, "n.session = :session", "n.session = :session AND gh_test_tadw_lookup(n.id) IS NOT NULL")
	cursor.seedQuery = strings.ReplaceAll(cursor.seedQuery, "/* preview-scan */", "AND gh_test_tadw_preview(c.id) IS NOT NULL")
	cursor.childQuery = strings.Replace(cursor.childQuery, "SELECT id FROM inputs", "SELECT gh_test_tadw_key(id) FROM inputs", 1)
	cursor.childQuery = strings.Replace(cursor.childQuery, "SELECT e.id,", "SELECT gh_test_tadw_child(e.id),", 1)
	for _, alias := range []string{"e", "c"} {
		cursor.childQuery = strings.Replace(cursor.childQuery, alias+".parent=p.id", alias+".parent=p.id AND gh_test_tadw_candidate("+alias+".id) IS NOT NULL", 1)
	}
	cursor.eventQuery = strings.Replace(cursor.eventQuery, "SELECT events.id,", "SELECT gh_test_tadw_payload(events.id),", 1)
}

func TestTADWHistoryCursorStopsOnWideAndDeepHistory(t *testing.T) {
	for _, shape := range []string{"wide-pending", "wide-success", "deep"} {
		t.Run(shape, func(t *testing.T) {
			store := tadwCursorStore(t)
			var nodes []tadwCursorFixtureNode
			nodes = append(nodes, tadwCursorFixtureNode{1, 0, "message.text", "00"})
			for id := 2; id <= 10001; id++ {
				parent := 1
				if shape == "deep" {
					parent = id - 1
				}
				nodes = append(nodes, tadwCursorFixtureNode{id, parent, "message.text", "10"})
			}
			anchor := 5000
			if shape == "deep" {
				anchor = 10001
			}
			if strings.HasPrefix(shape, "wide") {
				nodes = append(nodes, tadwCursorFixtureNode{20000, 10001, "agent.request", "11"})
				for id := 20001; id <= 30000; id++ {
					nodes = append(nodes, tadwCursorFixtureNode{id, 20000, "tool.request", "12"})
				}
				if shape == "wide-success" {
					nodes = append(nodes, tadwCursorFixtureNode{40000, 20000, "agent.success", "13"})
				}
			}
			tadwCursorInsert(t, store.DB, tadwCursorScope(), nodes...)
			// A buffered but unconsumed event must not have its payload decoded.
			if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='not JSON' WHERE id=?`, tadwCursorID(10000)); err != nil {
				t.Fatal(err)
			}
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(anchor))
			if err != nil {
				t.Fatal(err)
			}
			tadwCursorInstrument(cursor)
			if tadwCursorWork.nodes.Load() != 0 {
				t.Fatal("constructor eagerly read history")
			}
			if !cursor.Next() {
				t.Fatal(cursor.Err())
			}
			firstID := 10001
			if shape == "wide-pending" {
				firstID = 20000
			}
			if shape == "wide-success" {
				firstID = 40000
			}
			if cursor.Event().Event.Ref.Id != tadwCursorID(firstID) {
				t.Fatal("wrong first event")
			}
			if err := cursor.Close(); err != nil {
				t.Fatal(err)
			}
			if n := tadwCursorWork.nodes.Load(); n != 5 {
				t.Fatalf("main work=%d", n)
			}
			if n := tadwCursorWork.lookups.Load(); n != 5 {
				t.Fatalf("node lookup candidates=%d", n)
			}
			if n := tadwCursorWork.previews.Load(); n > 15 {
				t.Fatalf("preview work=%d", n)
			}
			wantKeys := int64(0)
			if strings.HasPrefix(shape, "wide") {
				wantKeys = 1
			}
			if n := tadwCursorWork.keys.Load(); n != wantKeys {
				t.Fatalf("child keys=%d want=%d", n, wantKeys)
			}
			wantCandidates, wantChildren := int64(0), int64(0)
			if shape == "wide-success" {
				wantCandidates, wantChildren = 2, 1
			}
			if n := tadwCursorWork.candidates.Load(); n != wantCandidates {
				t.Fatalf("execution candidates visited=%d want=%d", n, wantCandidates)
			}
			if n := tadwCursorWork.children.Load(); n != wantChildren {
				t.Fatalf("selected execution children=%d want=%d", n, wantChildren)
			}
			if n := tadwCursorWork.payloads.Load(); n != 1 {
				t.Fatalf("payload fetches=%d", n)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(shape, "wide") {
				// Negative control: without index enforcement, the request must inspect its
				// execution children when the partial outcome index is absent.
				if _, err := store.Exec(`DROP INDEX gatehouse_session_events_agent_terminals`); err != nil {
					t.Fatal(err)
				}
				tx, err := store.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(anchor))
				if err != nil {
					t.Fatal(err)
				}
				cursor.childQuery = strings.ReplaceAll(cursor.childQuery, " INDEXED BY gatehouse_session_events_agent_terminals", "")
				tadwCursorInstrument(cursor)
				if !cursor.Next() {
					t.Fatal(cursor.Err())
				}
				cursor.Close()
				if n := tadwCursorWork.candidates.Load(); n < 10000 {
					t.Fatalf("negative control did not expose execution scans: %d", n)
				}
			}
		})
	}
}

func TestTADWHistoryCursorAgainstParentMapOracle(t *testing.T) {
	random := rand.New(rand.NewPCG(42, 17))
	for trial := 0; trial < 20; trial++ {
		store := tadwCursorStore(t)
		var nodes []tadwCursorFixtureNode
		for id := 1; id <= 40; id++ {
			parent := 0
			if id > 1 && random.IntN(5) != 0 {
				parent = random.IntN(id-1) + 1
			}
			nodes = append(nodes, tadwCursorFixtureNode{id, parent, "message.text", fmt.Sprintf("%02d", random.IntN(8))})
		}
		next := 100
		for owner := 1; owner <= 40; owner++ {
			for count := random.IntN(6); count > 0; count-- {
				next++
				request := next
				nodes = append(nodes, tadwCursorFixtureNode{request, owner, "agent.request", fmt.Sprintf("%02d", random.IntN(8))})
				if random.IntN(3) != 0 {
					next++
					kind := "agent.success"
					if random.IntN(2) == 0 {
						kind = "agent.failure"
					}
					nodes = append(nodes, tadwCursorFixtureNode{next, request, kind, fmt.Sprintf("%02d", random.IntN(8))})
				}
			}
		}
		tadwCursorInsert(t, store.DB, tadwCursorScope(), nodes...)
		anchor := random.IntN(40) + 1
		tx, err := store.Begin()
		if err != nil {
			t.Fatal(err)
		}
		got, err := tadwCursorCollect(t, store, tx, tadwCursorRef(anchor))
		tx.Rollback()
		if err != nil {
			t.Fatal(err)
		}
		want := tadwCursorOracle(nodes, anchor)
		if !reflect.DeepEqual(tadwCursorIDs(got), want) {
			t.Fatalf("trial=%d got=%v want=%v", trial, tadwCursorIDs(got), want)
		}
	}
}
func tadwCursorOracle(nodes []tadwCursorFixtureNode, anchor int) []string {
	byID := map[int]tadwCursorFixtureNode{}
	byParent := map[int][]tadwCursorFixtureNode{}
	less := func(a, b tadwCursorFixtureNode) bool {
		if a.at != b.at {
			return a.at < b.at
		}
		return a.id < b.id
	}
	for _, node := range nodes {
		byID[node.id] = node
		byParent[node.parent] = append(byParent[node.parent], node)
	}
	for _, children := range byParent {
		sort.Slice(children, func(a, b int) bool { return less(children[a], children[b]) })
	}
	var result []string
	var emit func(tadwCursorFixtureNode)
	emit = func(node tadwCursorFixtureNode) {
		result = append(result, tadwCursorID(node.id))
		if node.kind == "agent.request" {
			for _, child := range byParent[node.id] {
				if child.kind == "agent.success" || child.kind == "agent.failure" {
					emit(child)
				}
			}
		}
	}
	// Construct final prompt order independently: outer ancestry first, each
	// level oldest-first, and each subtree parent-first. Then reverse ONCE.
	var levels [][]string
	for root := anchor; root != 0; root = byID[root].parent {
		result = nil
		branch := byID[root]
		siblings := byParent[branch.parent]
		for _, owner := range siblings {
			if owner.at > branch.at {
				continue
			}
			emit(owner)
			if owner.id != root && owner.kind != "agent.request" {
				children := byParent[owner.id]
				for _, child := range children[:min(3, len(children))] {
					emit(child)
				}
			}
		}
		levels = append(levels, result)
	}
	result = nil
	for index := len(levels) - 1; index >= 0; index-- {
		result = append(result, levels[index]...)
	}
	slices.Reverse(result)
	return result
}
