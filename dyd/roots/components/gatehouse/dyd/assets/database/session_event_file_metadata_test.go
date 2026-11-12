package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"gatehouse/config"
	"gatehouse/model"
)

const tadwFileID = "sfi_00000000000000000000000001"

func tadwMetadataFixture(t *testing.T) (*Store, model.SessionEvent) {
	t.Helper()
	store := tadwCursorStore(t)
	if _, err := store.Exec(`CREATE TABLE gatehouse_storage_objects(id TEXT PRIMARY KEY,size INTEGER);
 CREATE TABLE gatehouse_session_files(workspace TEXT,session TEXT,id TEXT,storage_object TEXT,name TEXT,media_type TEXT,enabled INTEGER,PRIMARY KEY(workspace,session,id));
 INSERT INTO gatehouse_storage_objects VALUES('object',9007199254740993);
 INSERT INTO gatehouse_session_files VALUES('w','s','sfi_00000000000000000000000001','object','original',NULL,0);`); err != nil {
		t.Fatal(err)
	}
	event := model.SessionEvent{Ref: tadwCursorRef(1), Kind: "message.text", Payload: map[string]interface{}{"text": "body", "attachments": []interface{}{tadwFileID, tadwFileID}}}
	return store, event
}
func TestSessionEventFilesHydrateInTransaction(t *testing.T) {
	store, event := tadwMetadataFixture(t)
	before, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err, projected := store.SessionEventFilesHydrateInTransaction(context.Background(), tx, event)
	if err != nil {
		t.Fatal(err)
	}
	files := projected.Payload["attachments"].([]interface{})
	if len(files) != 2 {
		t.Fatal("changed attachment order/membership")
	}
	file := files[0].(map[string]interface{})
	if file["size"] != int64(9007199254740993) || file["name"] != "original" || file["id"] != tadwFileID || len(file) != 3 {
		t.Fatal("unexpected projection", file)
	}
	after, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("mutated canonical event")
	}
	projected.Payload["text"] = "changed"
	if event.Payload["text"] != "body" {
		t.Fatal("payload aliases input")
	}
	// Canonical output_file uses the same exact integer metadata projection.
	output := event
	output.Kind = "tool.success"
	output.Payload = map[string]interface{}{"output_file": model.SessionFileRef{Session: event.Ref.Session, Id: tadwFileID}}
	err, result := store.SessionEventFilesHydrateInTransaction(context.Background(), tx, output)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Payload["output_file"], file) {
		t.Fatal(result.Payload)
	}
	if _, present, err := SessionEventOutputFile(output); err != nil || !present {
		t.Fatal("mutated output reference", err)
	}
}

func TestSessionEventFilesMetadataSharesReadSnapshot(t *testing.T) {
	store, event := tadwMetadataFixture(t)
	path := storePathForTADWTest(t, store)
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err, before := store.SessionEventFilesHydrateInTransaction(context.Background(), tx, event)
	if err != nil {
		t.Fatal(err)
	}
	err, writer := Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.Exec(`UPDATE gatehouse_session_files SET name='new name'`); err != nil {
		t.Fatal(err)
	}
	err, during := store.SessionEventFilesHydrateInTransaction(context.Background(), tx, event)
	if err != nil || !reflect.DeepEqual(before, during) {
		t.Fatal("metadata escaped snapshot", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Rollback()
	err, after := store.SessionEventFilesHydrateInTransaction(context.Background(), fresh, event)
	if err != nil || reflect.DeepEqual(before, after) {
		t.Fatal("fresh snapshot missed new metadata", err)
	}
}

func TestSessionEventFilesMetadataScopeAndErrors(t *testing.T) {
	store, event := tadwMetadataFixture(t)
	// Deliberate ID collisions ensure lookups are scoped, not global ID reads.
	if _, err := store.Exec(`INSERT INTO gatehouse_session_files VALUES('other','s','sfi_00000000000000000000000001','object','foreign',NULL,1);
 INSERT INTO gatehouse_session_files VALUES('w','other','sfi_00000000000000000000000001','object','foreign',NULL,1);`); err != nil {
		t.Fatal(err)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, payload := range []map[string]interface{}{
		{"attachments": 42}, {"attachments": []string{"invalid"}},
		{"attachments": []string{"sfi_00000000000000000000000002"}},
		{"output_file": model.SessionFileRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: "other"}, Id: "s"}, Id: tadwFileID}},
	} {
		bad := event
		bad.Payload = payload
		err, projection := store.SessionEventFilesHydrateInTransaction(context.Background(), tx, bad)
		if err == nil || !reflect.DeepEqual(projection, model.SessionEvent{}) {
			t.Fatal("invalid reference returned partial projection", err)
		}
	}
	err, scoped := store.SessionEventFilesHydrateInTransaction(context.Background(), tx, event)
	if err != nil {
		t.Fatal(err)
	}
	if scoped.Payload["attachments"].([]interface{})[0].(map[string]interface{})["name"] != "original" {
		t.Fatal("scope leak")
	}
	if err, _ := store.SessionEventFilesHydrateInTransaction(context.Background(), nil, event); err == nil {
		t.Fatal("nil transaction accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err, _ := store.SessionEventFilesHydrateInTransaction(ctx, tx, event); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionEventFilesHydrateInTransaction(context.Background(), tx, event); !errors.Is(err, sql.ErrTxDone) {
		t.Fatal(err)
	}
}
