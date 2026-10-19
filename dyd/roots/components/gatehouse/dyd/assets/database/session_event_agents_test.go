package database_test

import (
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestSessionEventAgentsHydrateReferencedIdentities(t *testing.T) {
	testSessionEventAgentsHydrateReferencedIdentities(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresSessionEventAgentsHydrateReferencedIdentities(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	testSessionEventAgentsHydrateReferencedIdentities(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func testSessionEventAgentsHydrateReferencedIdentities(t *testing.T, configuration config.DatabaseConfig) {
	f := newChatGroupFixture(t, configuration)
	chat := f.chat("alice")
	ref := model.WorkspaceAgentRef{Workspace: f.workspace, Id: f.bots["fin"]}
	if _, err := f.store.Exec(`UPDATE gatehouse_workspace_agents SET enabled = FALSE, label = 'Archived finance assistant' WHERE workspace_id = `+f.p(1)+` AND id = `+f.p(2), f.workspace.Id, ref.Id); err != nil {
		t.Fatal(err)
	}
	events := []model.SessionEvent{
		{Ref: model.SessionEventRef{Session: chat, Id: newTypedID(t, typed_id.SessionEvent)}, Kind: model.SessionEventKindAgentSuccess, AuthorAgent: &ref},
		{Ref: model.SessionEventRef{Session: chat, Id: newTypedID(t, typed_id.SessionEvent)}, Kind: model.SessionEventKindAgentRequest, Payload: map[string]interface{}{"agent": ref.Id}},
		{Ref: model.SessionEventRef{Session: chat, Id: newTypedID(t, typed_id.SessionEvent)}, Kind: model.SessionEventKindMessageText, Payload: map[string]interface{}{"agents": []string{ref.Id, f.bots["foreign"]}}},
	}
	err, agents := f.store.SessionEventAgentsGet(f.ctx, chat, events)
	if err != nil || len(agents) != 1 {
		t.Fatalf("referenced bot metadata = %#v %v", agents, err)
	}
	agent := agents[ref]
	if agent.Enabled || agent.Alias != "fin" || agent.Label == nil || *agent.Label != "Archived finance assistant" {
		t.Fatalf("disabled, non-participating author lost its identity: %#v", agent)
	}
	if err, active := f.store.SessionAgentParticipantGet(f.ctx, chat, ref); err != nil || active {
		t.Fatalf("metadata created participation: %t %v", active, err)
	}
	if err, empty := f.store.SessionEventAgentsGet(f.ctx, chat, nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty page loaded identities: %#v %v", empty, err)
	}
	wrong := events[0]
	wrong.Ref.Session.Id = newTypedID(t, typed_id.Session)
	if err, _ := f.store.SessionEventAgentsGet(f.ctx, chat, []model.SessionEvent{wrong}); err == nil {
		t.Fatal("accepted an event from another chat")
	}
	wrong = events[0]
	foreign := ref
	foreign.Workspace.Id = newTypedID(t, typed_id.Workspace)
	wrong.AuthorAgent = &foreign
	if err, _ := f.store.SessionEventAgentsGet(f.ctx, chat, []model.SessionEvent{wrong}); err == nil {
		t.Fatal("accepted an author reference from another workspace")
	}
	// Enough distinct authors to exercise bounded SQL parameter batches. No
	// unrelated workspace identities should enter the returned projection.
	err, binding := f.store.SystemWorkspaceAgentGet(f.ctx, f.workspace.Id, f.bots["eng"])
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.store.BeginTx(f.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	page := []model.SessionEvent{}
	for range 205 {
		id := newTypedID(t, typed_id.WorkspaceAgent)
		if _, err := tx.ExecContext(f.ctx, `INSERT INTO gatehouse_workspace_agents (workspace_id, id, alias, model_id, revision, "default", enabled) VALUES (`+f.p(1)+`, `+f.p(2)+`, `+f.p(3)+`, `+f.p(4)+`, 1, FALSE, FALSE)`, f.workspace.Id, id, "archived-"+id, binding.ModelID); err != nil {
			t.Fatal(err)
		}
		page = append(page, model.SessionEvent{Ref: model.SessionEventRef{Session: chat, Id: newTypedID(t, typed_id.SessionEvent)}, Kind: model.SessionEventKindAgentSuccess, AuthorAgent: &model.WorkspaceAgentRef{Workspace: f.workspace, Id: id}})
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	err, agents = f.store.SessionEventAgentsGet(f.ctx, chat, page)
	if err != nil || len(agents) != len(page) {
		t.Fatalf("batched page metadata = %d identities, %v", len(agents), err)
	}
	for _, event := range page {
		if _, exists := agents[*event.AuthorAgent]; !exists {
			t.Fatalf("batch omitted author %s", event.AuthorAgent.Id)
		}
	}
}
