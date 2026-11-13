package database

import (
	"context"
	"reflect"
	"testing"

	"gatehouse/model"
)

func TestSessionEventGetInSQLTransaction(t *testing.T) {
	store := tadwCursorStore(t)
	ref := tadwCursorRef(1)
	tadwCursorInsert(t, store.DB, ref.Session, tadwCursorFixtureNode{1, 0, "message.text", "00"})
	other := ref.Session
	other.Workspace.Id = "other"
	tadwCursorInsert(t, store.DB, other, tadwCursorFixtureNode{1, 0, "agent.request", "01"})
	err, want := store.SessionEventGet(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err, got := store.SessionEventGetInSQLTransaction(context.Background(), tx, ref)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("ordinary event decoder not reused", got, err)
	}
	err, collision := store.SessionEventGetInSQLTransaction(context.Background(), tx, model.SessionEventRef{Session: other, Id: ref.Id})
	if err != nil || collision == nil || collision.Kind != "agent.request" || collision.Ref.Session != other {
		t.Fatal("cross-scope ID collision", collision, err)
	}
	missing := ref
	missing.Session.Id = "missing"
	if err, event := store.SessionEventGetInSQLTransaction(context.Background(), tx, missing); err != nil || event != nil {
		t.Fatal("missing event contract", event, err)
	}
	if err, _ := store.SessionEventGetInSQLTransaction(context.Background(), nil, ref); err == nil {
		t.Fatal("nil transaction accepted")
	}
	if err, _ := store.SessionEventGetInSQLTransaction(context.Background(), tx, model.SessionEventRef{}); err == nil {
		t.Fatal("empty scope accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err, _ := store.SessionEventGetInSQLTransaction(ctx, tx, ref); err == nil {
		t.Fatal("cancelled read accepted")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionEventGetInSQLTransaction(context.Background(), tx, ref); err == nil {
		t.Fatal("finished transaction accepted")
	}
}
