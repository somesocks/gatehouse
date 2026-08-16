package typed_id

import "testing"

func TestNew(t *testing.T) {
	for _, kind := range []string{Workspace, Principal, Identity, Group, Tool, Resource, AgentProvider, AgentModel, StorageProvider, Session, SessionEvent, ActivityEvent} {
		id, err := New(kind)
		if err != nil || !Valid(kind, id) {
			t.Fatalf("New(%q) = (%q, %v)", kind, id, err)
		}
	}
}

func TestDerive(t *testing.T) {
	first, err := Derive(SessionEvent, "stable input")
	if err != nil || !Valid(SessionEvent, first) {
		t.Fatalf("Derive() = (%q, %v)", first, err)
	}
	second, err := Derive(SessionEvent, "stable input")
	if err != nil || first != second {
		t.Fatalf("Derive() = (%q, %v), want %q", second, err, first)
	}
}

func TestValid(t *testing.T) {
	for _, test := range []struct {
		kind, value string
		valid       bool
	}{
		{Workspace, "wsp_aaisem2ekvthpcezvk54zxpo74", true},
		{Workspace, "wsp_AAISEM2EKVTHPCEZVK54ZXPO74", false},
		{Workspace, "grp_aaisem2ekvthpcezvk54zxpo74", false},
		{Workspace, "wsp_aaisem2ekvthpcezvk54zxpo7", false},
		{Workspace, "wsp_aaisem2ekvthpcezvk54zxpo70", false},
	} {
		if got := Valid(test.kind, test.value); got != test.valid {
			t.Errorf("Valid(%q, %q) = %t, want %t", test.kind, test.value, got, test.valid)
		}
	}
}
