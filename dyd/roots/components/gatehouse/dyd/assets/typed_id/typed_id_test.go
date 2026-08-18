package typed_id

import (
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
)

func TestNew(t *testing.T) {
	for _, kind := range []string{Workspace, Principal, Identity, Group, Tool, Resource, AgentProvider, AgentModel, StorageProvider, StorageObject, SessionFile, ProjectFile, ProjectNote, SessionNote, Gateway, Project, Session, SessionEvent, ActivityEvent} {
		id, err := New(kind)
		if err != nil || !Valid(kind, id) {
			t.Fatalf("New(%q) = (%q, %v)", kind, id, err)
		}
	}
}

func TestNewAt(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC)
	value, err := NewAt(SessionEvent, at)
	if err != nil || !Valid(SessionEvent, value) {
		t.Fatalf("NewAt() = (%q, %v)", value, err)
	}
	id, err := ulid.ParseStrict(strings.ToUpper(strings.TrimPrefix(value, SessionEvent+"_")))
	if err != nil || !id.Timestamp().Equal(at) {
		t.Fatalf("NewAt() timestamp = (%v, %v), want %v", id.Timestamp(), err, at)
	}
}

func TestTimestamp(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC)
	value, err := NewAt(SessionEvent, at)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Timestamp(SessionEvent, value)
	if err != nil || !got.Equal(at) {
		t.Fatalf("Timestamp() = (%v, %v), want %v", got, err, at)
	}
	if _, err := Timestamp(ActivityEvent, value); err == nil {
		t.Fatal("Timestamp() accepted an ID with the wrong kind")
	}
}

func TestValid(t *testing.T) {
	for _, test := range []struct {
		kind, value string
		valid       bool
	}{
		{Workspace, "wsp_01arz3ndektsv4rrffq69g5fav", true},
		{Workspace, "wsp_01ARZ3NDEKTSV4RRFFQ69G5FAV", false},
		{Workspace, "grp_01arz3ndektsv4rrffq69g5fav", false},
		{Workspace, "wsp_01arz3ndektsv4rrffq69g5fa", false},
		{Workspace, "wsp_01arz3ndektsv4rrffq69g5fai", false},
	} {
		if got := Valid(test.kind, test.value); got != test.valid {
			t.Errorf("Valid(%q, %q) = %t, want %t", test.kind, test.value, got, test.valid)
		}
	}
}
