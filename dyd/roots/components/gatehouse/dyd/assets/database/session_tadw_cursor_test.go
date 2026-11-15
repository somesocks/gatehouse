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

func TestTADWHistoryCursorSingleReversalProducesPromptOrder(t *testing.T) {
	store := tadwCursorStore(t)
	tadwCursorBoundary(t, store)
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	got, err := tadwCursorCollect(t, store, tx, tadwCursorRef(11))
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(got)
	var want []string
	for _, id := range []int{1, 15, 2, 3, 4, 8, 5, 9, 6, 11, 12} {
		want = append(want, tadwCursorID(id))
	}
	if !slices.Equal(tadwCursorIDs(got), want) {
		t.Fatalf("whole-list reversal gave %v", tadwCursorIDs(got))
	}
	positions := map[string]int{}
	for index, item := range got {
		positions[item.Event.Ref.Id] = index
	}
	for index, item := range got {
		if item.Event.Parent != nil {
			if parent, exists := positions[item.Event.Parent.Id]; exists && parent >= index {
				t.Fatalf("parent follows child: %+v", item)
			}
		}
	}
	// A cutoff can fall inside a subtree. Keep actual parent identities even
	// when that parent falls outside the accepted suffix; do not regroup it.
	err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(11))
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Close()
	var prefix []TADWHistoryEvent
	for len(prefix) < 4 && cursor.Next() {
		prefix = append(prefix, cursor.Event())
	}
	if cursor.Err() != nil {
		t.Fatal(cursor.Err())
	}
	slices.Reverse(prefix)
	if prefix[0].Event.Ref.Id != tadwCursorID(9) || prefix[0].Event.Parent.Id != tadwCursorID(5) {
		t.Fatal("wrong partial-subtree suffix", prefix)
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
		tadwCursorFixtureNode{1, 0, "message.text", tadwCursorAt(0)},
		tadwCursorFixtureNode{2, 1, "agent.request", tadwCursorAt(1)},
		tadwCursorFixtureNode{3, 2, "agent.success", tadwCursorAt(2)},
		tadwCursorFixtureNode{4, 3, "message.text", tadwCursorAt(3)})
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
		nodes := []tadwCursorFixtureNode{{1, 2, "message.text", tadwCursorAt(0)}, {2, 1, "message.text", tadwCursorAt(1)}}
		if self {
			nodes = []tadwCursorFixtureNode{{1, 1, "message.text", tadwCursorAt(0)}}
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
	pointLookups := 0
	for _, line := range strings.Split(seeds, "\n") {
		if strings.HasPrefix(line, "SEARCH n ") {
			pointLookups++
			if !strings.Contains(line, "id=?") {
				t.Fatalf("recursive node lookup scans a scope instead of seeking by ID:\n%s", seeds)
			}
		}
		if strings.HasPrefix(line, "SEARCH p ") && !strings.Contains(line, "id=?") {
			t.Fatalf("parent/preview lookup is not an ID seek:\n%s", seeds)
		}
	}
	if pointLookups != 3 {
		t.Fatalf("expected three node ID seeks, got %d:\n%s", pointLookups, seeds)
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
