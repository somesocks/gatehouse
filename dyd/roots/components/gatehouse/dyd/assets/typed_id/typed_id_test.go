package typed_id

import "testing"

func TestNew(t *testing.T) {
	for _, kind := range []string{Workspace, Principal, Identity, Group, Tool, Resource, AgentProvider, AgentModel} {
		id, err := New(kind)
		if err != nil || !Valid(kind, id) {
			t.Fatalf("New(%q) = (%q, %v)", kind, id, err)
		}
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
