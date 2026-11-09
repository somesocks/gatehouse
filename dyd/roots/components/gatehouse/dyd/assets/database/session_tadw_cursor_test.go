package database

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestTADWHistoryCursorEveryEarlyStopPrefix(t *testing.T) {
	store := tadwCursorStore(t)
	nodes := tadwCursorBoundary(t, store)
	want := tadwCursorOracle(nodes, 11)
	for limit := 0; limit <= len(want)+1; limit++ {
		tx, err := store.Begin()
		if err != nil {
			t.Fatal(err)
		}
		err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(11))
		if err != nil {
			t.Fatal(err)
		}
		tadwCursorInstrument(cursor)
		var got []string
		for len(got) < limit && cursor.Next() {
			got = append(got, cursor.Event().Event.Ref.Id)
		}
		if err := cursor.Err(); err != nil {
			t.Fatal(err)
		}
		if err := cursor.Close(); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want[:min(limit, len(want))]) {
			t.Fatalf("limit=%d got=%v", limit, got)
		}
		if count := tadwCursorWork.payloads.Load(); count != int64(len(got)) {
			t.Fatalf("limit=%d payload reads=%d", limit, count)
		}
		if limit == 0 && tadwCursorWork.nodes.Load() != 0 {
			t.Fatal("zero consumption did work")
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTADWHistoryCursorPayloadErrorClosesReader(t *testing.T) {
	store := tadwCursorStore(t)
	tadwCursorBoundary(t, store)
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id=?`, tadwCursorID(12)); err != nil {
		t.Fatal(err)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(11))
	if err != nil {
		t.Fatal(err)
	}
	if cursor.Next() || cursor.Err() == nil || !cursor.closed {
		t.Fatal("decode error did not close reader", cursor.Err())
	}
	if cursor.Next() {
		t.Fatal("iteration resumed after error")
	}
	var one int
	if err := tx.QueryRow(`SELECT 1`).Scan(&one); err != nil {
		t.Fatal("reader failure ended caller transaction", err)
	}
}

func TestTADWHistoryCursorOutcomeOnAnchorAncestry(t *testing.T) {
	store := tadwCursorStore(t)
	tadwCursorInsert(t, store.DB, tadwCursorScope(),
		tadwCursorFixtureNode{1, 0, "message.text", "00"},
		tadwCursorFixtureNode{2, 1, "agent.request", "01"},
		tadwCursorFixtureNode{3, 2, "agent.success", "02"},
		tadwCursorFixtureNode{4, 3, "message.text", "03"})
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	got, err := tadwCursorCollect(t, store, tx, tadwCursorRef(4))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{tadwCursorID(4), tadwCursorID(3), tadwCursorID(2), tadwCursorID(1)}
	if !slices.Equal(tadwCursorIDs(got), want) {
		t.Fatalf("ancestral outcome duplicated: %v", tadwCursorIDs(got))
	}
}

func TestTADWHistoryCursorRejectsCyclicAncestry(t *testing.T) {
	for _, self := range []bool{false, true} {
		store := tadwCursorStore(t)
		nodes := []tadwCursorFixtureNode{{1, 2, "message.text", "00"}, {2, 1, "message.text", "01"}}
		if self {
			nodes = []tadwCursorFixtureNode{{1, 1, "message.text", "00"}}
		}
		tadwCursorInsert(t, store.DB, tadwCursorScope(), nodes...)
		tx, err := store.Begin()
		if err != nil {
			t.Fatal(err)
		}
		_, err = tadwCursorCollect(t, store, tx, tadwCursorRef(1))
		tx.Rollback()
		if err == nil || !strings.Contains(err.Error(), "cyclic") {
			t.Fatalf("cyclic ancestry accepted: %v", err)
		}
	}
}

func TestTADWHistoryQueryPlans(t *testing.T) {
	store := tadwCursorStore(t)
	tadwCursorBoundary(t, store)
	scope := []any{sql.Named("workspace", "w"), sql.Named("session", "s")}
	plan := func(query string, args []any) string {
		t.Helper()
		rows, err := store.Query(`EXPLAIN QUERY PLAN `+query, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var details []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			details = append(details, detail)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return strings.Join(details, "\n")
	}
	seeds := plan(tadwHistorySeedsSQL, append(slices.Clone(scope), sql.Named("anchor", tadwCursorID(11))))
	for _, fragment := range []string{"CO-ROUTINE branch_walk", "USING COVERING INDEX gatehouse_session_events_thread_order", "created_at=? AND id<?"} {
		if !strings.Contains(seeds, fragment) {
			t.Fatalf("missing %q in plan:\n%s", fragment, seeds)
		}
	}
	if strings.Contains(seeds, "MATERIALIZE branch_walk") || strings.Contains(seeds, "USE TEMP B-TREE FOR ORDER BY") {
		t.Fatalf("eager history plan:\n%s", seeds)
	}
	args := slices.Clone(scope)
	for index := 0; index < tadwHistoryFrontierKeys; index++ {
		var id any
		if index == 0 {
			id = tadwCursorID(4)
		}
		args = append(args, sql.Named(fmt.Sprintf("p%d", index), id))
	}
	children := plan(tadwHistoryChildrenSQL, args)
	if count := strings.Count(children, "USING COVERING INDEX gatehouse_session_events_agent_terminals"); count != 2 {
		t.Fatalf("expected indexed outer and inner outcome lookups, got %d:\n%s", count, children)
	}
}
