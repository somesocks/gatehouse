package database_test

import (
	"context"
	"crypto/sha256"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
	"testing"
	"time"
)

func TestRecordSearchUsesVisibleValuesAndReferences(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}}, Principals: []config.Principal{{Alias: "alice", Enabled: true}, {Alias: "bob", Enabled: true}}, StorageProviders: []config.StorageProvider{{Alias: "embedded", Revision: 1, Protocol: "embedded", Enabled: true}}, WorkspaceStorageProviders: []config.WorkspaceStorageProvider{{WorkspaceID: "engineering", ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true}}}
	if err := migrateState(ctx, store, configuration, withTestWorkspaceMembers(state)); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice := principalRef(t, ctx, store, "alice")
	sequence := 0
	id := func(kind string) string {
		sequence++
		value, err := typed_id.NewAt(kind, time.Date(2026, 1, 1, 0, 0, sequence, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	project := model.ProjectRef{Workspace: workspace, Id: id(typed_id.Project)}
	if err, _ := store.ProjectsCreate(ctx, model.Project{Ref: project, Enabled: true}, alice, nil); err != nil {
		t.Fatal(err)
	}
	createSchema := func(name string) model.ProjectRecordSchema {
		err, schema := store.ProjectRecordSchemaCreate(ctx, model.ProjectRecordSchema{Ref: model.ProjectRecordSchemaRef{Project: project, Id: id(typed_id.ProjectRecordSchema)}, Name: name, Label: name}, alice, database.ProjectRecordAuthor{})
		if err != nil {
			t.Fatal(err)
		}
		return schema
	}
	schema := createSchema("records")
	target := createSchema("targets")
	field := func(owner model.ProjectRecordSchema, name, kind, display string, targetSchema *model.ProjectRecordSchemaRef) model.ProjectRecordAttributeRef {
		ref := model.ProjectRecordAttributeRef{Schema: owner.Ref, Id: id(typed_id.ProjectRecordAttribute)}
		if err, _ := store.ProjectRecordAttributeCreate(ctx, model.ProjectRecordAttribute{Ref: ref, Name: name, Label: name, Type: kind, Cardinality: "one", Uniqueness: "none", Display: display, TargetSchema: targetSchema}, alice, database.ProjectRecordAuthor{}); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	text := field(schema, "title", "text", "primary", nil)
	number := field(schema, "number", "number", "secondary", nil)
	boolean := field(schema, "approved", "boolean", "secondary", nil)
	datetime := field(schema, "date", "datetime", "secondary", nil)
	file := field(schema, "file", "file", "secondary", nil)
	reference := field(schema, "related", "record", "secondary", &target.Ref)
	hidden := field(schema, "hidden", "text", "none", nil)
	targetTitle := field(target, "title", "text", "primary", nil)
	create := func(owner model.ProjectRecordSchema, values ...database.ProjectRecordValueCreate) model.ProjectRecord {
		err, record, _ := store.ProjectRecordCreate(ctx, model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: owner.Ref, Id: id(typed_id.ProjectRecord)}}, alice, database.ProjectRecordAuthor{}, values)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	targetRecord := create(target, database.ProjectRecordValueCreate{Attribute: targetTitle, Value: "Lovelace"})
	fileRef := model.ProjectFileRef{Project: project, Id: id(typed_id.ProjectFile)}
	object := id(typed_id.StorageObject)
	if err, _, _ := store.ProjectFileCreate(ctx, model.ProjectFile{Ref: fileRef, Name: "report.pdf", Enabled: true}, object, alice); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("report"))
	if err := store.StorageObjectStoreIntegrity(ctx, object, digest[:], 6); err != nil {
		t.Fatal(err)
	}
	if err := store.StorageObjectMarkSuccess(ctx, object); err != nil {
		t.Fatal(err)
	}
	if err, _, _ := store.ProjectFileFinish(ctx, fileRef, alice); err != nil {
		t.Fatal(err)
	}
	first := create(schema, database.ProjectRecordValueCreate{Attribute: text, Value: "needle %_ alpha"}, database.ProjectRecordValueCreate{Attribute: number, Value: 123.5}, database.ProjectRecordValueCreate{Attribute: boolean, Value: true}, database.ProjectRecordValueCreate{Attribute: datetime, Value: "2026-10-12T10:00:00.000Z"}, database.ProjectRecordValueCreate{Attribute: file, Value: fileRef.Id}, database.ProjectRecordValueCreate{Attribute: reference, Value: targetRecord.Ref.Id}, database.ProjectRecordValueCreate{Attribute: hidden, Value: "invisible"})
	create(schema, database.ProjectRecordValueCreate{Attribute: text, Value: "unrelated"})
	second := create(schema, database.ProjectRecordValueCreate{Attribute: text, Value: "needle beta"})
	for _, search := range []string{"123.5", "Yes", "true", "2026-10-12", "report.pdf", "Lovelace", "%_", first.Ref.Id} {
		err, rows := store.ProjectRecordsGet(ctx, schema.Ref, alice, 50, "", search)
		if err != nil || len(rows) != 1 || rows[0].Ref != first.Ref {
			t.Fatalf("search %q: %#v, %v", search, rows, err)
		}
	}
	for _, search := range []string{"invisible", "no match"} {
		err, rows := store.ProjectRecordsGet(ctx, schema.Ref, alice, 50, "", search)
		if err != nil || len(rows) != 0 {
			t.Fatalf("search %q: %#v, %v", search, rows, err)
		}
	}
	err, rows := store.ProjectRecordsGet(ctx, schema.Ref, alice, 1, "", "needle")
	if err != nil || len(rows) != 1 || rows[0].Ref != second.Ref {
		t.Fatalf("first page: %#v, %v", rows, err)
	}
	err, rows = store.ProjectRecordsGet(ctx, schema.Ref, alice, 1, second.Ref.Id, "needle")
	if err != nil || len(rows) != 1 || rows[0].Ref != first.Ref {
		t.Fatalf("second page: %#v, %v", rows, err)
	}
	err, rows = store.ProjectRecordsGet(ctx, schema.Ref, principalRef(t, ctx, store, "bob"), 50, "", "needle")
	if err != nil || len(rows) != 0 {
		t.Fatalf("unauthorized search: %#v, %v", rows, err)
	}
}
