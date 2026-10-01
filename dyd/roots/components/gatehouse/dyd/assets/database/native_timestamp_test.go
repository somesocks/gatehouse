package database

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/model"
	"github.com/jackc/pgx/v5/pgtype"
)

type nativeTimestampField struct {
	oid   uint32
	value any
}

type nativeTimestampRow []nativeTimestampField

// Exercise pgx's actual binary codecs without a server or database connection.
func (row nativeTimestampRow) Scan(dest ...any) error {
	if len(dest) != len(row) {
		return fmt.Errorf("scan destination count differs from field count")
	}
	types := pgtype.NewMap()
	for index, field := range row {
		var encoded []byte
		var err error
		if field.value != nil {
			encoded, err = types.Encode(field.oid, pgtype.BinaryFormatCode, field.value, nil)
			if err != nil {
				return fmt.Errorf("encode field %d: %w", index, err)
			}
		}
		if err := types.Scan(field.oid, pgtype.BinaryFormatCode, encoded, dest[index]); err != nil {
			return fmt.Errorf("scan field %d: %w", index, err)
		}
	}
	return nil
}

func TestSessionEventDecodesNativeTimestamp(t *testing.T) {
	stamp := time.Date(2026, 10, 1, 19, 11, 56, 278_000_000, time.UTC)
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "wsp_00000000000000000000000000"}, Id: "ses_00000000000000000000000000"}
	row := nativeTimestampRow{
		{pgtype.TextOID, "sev_00000000000000000000000001"},
		{pgtype.TextOID, nil},
		{pgtype.TextOID, model.SessionEventKindThinkingRequest},
		{pgtype.TextOID, "prn_00000000000000000000000000"},
		{pgtype.TextOID, "alice"},
		{pgtype.TextOID, "Alice"},
		{pgtype.BoolOID, true},
		{pgtype.TextOID, nil},
		{pgtype.TextOID, nil},
		{pgtype.JSONBOID, json.RawMessage(`{"turn":0}`)},
		{pgtype.JSONBOID, nil},
		{pgtype.TimestamptzOID, stamp},
	}
	err, event := sessionEventFromRow(row, session)
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != model.SessionEventKindThinkingRequest || event.AuthorPrincipal == nil {
		t.Fatalf("decoded native event = %#v", event)
	}
	assertNativeTimestamp(t, event.CreatedAt, stamp)
}

func TestAgentContextDecodesNativeTimestamps(t *testing.T) {
	stamp := time.Date(2026, 10, 1, 19, 11, 56, 278_000_000, time.UTC)
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "wsp_00000000000000000000000000"}, Id: "ses_00000000000000000000000000"}
	root := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_00000000000000000000000000"}
	store := &Store{kind: config.DatabaseKindPostgres}
	t.Run("current", func(t *testing.T) {
		err, value := store.agentContextGet(root, func(string, ...any) sessionEventRow {
			return nativeTimestampRow{
				{pgtype.TextOID, agent.Id}, {pgtype.TextOID, "profile"},
				{pgtype.JSONBOID, json.RawMessage(`{}`)}, {pgtype.TimestamptzOID, stamp},
			}
		})
		if err != nil || value == nil || value.Model != agent {
			t.Fatalf("current native context = (%#v, %v)", value, err)
		}
		assertNativeTimestamp(t, value.UpdatedAt, stamp)
	})
	t.Run("previous", func(t *testing.T) {
		err, value := store.agentContextPreviousGet(session, agent, root.Id, func(string, ...any) sessionEventRow {
			return nativeTimestampRow{
				{pgtype.TextOID, root.Id}, {pgtype.TextOID, "profile"},
				{pgtype.JSONBOID, json.RawMessage(`{}`)}, {pgtype.TimestamptzOID, stamp},
			}
		})
		if err != nil || value == nil || value.Root != root {
			t.Fatalf("previous native context = (%#v, %v)", value, err)
		}
		assertNativeTimestamp(t, value.UpdatedAt, stamp)
	})
}

func assertNativeTimestamp(t *testing.T, encoded string, want time.Time) {
	t.Helper()
	got, err := time.Parse(time.RFC3339Nano, encoded)
	if err != nil || !got.Equal(want) {
		t.Fatalf("native timestamp = (%q, %v), want instant %s", encoded, err, want.Format(time.RFC3339Nano))
	}
}
