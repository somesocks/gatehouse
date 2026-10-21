package database_test

import (
	"errors"
	"os"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestWorkspaceAdmissionRevokesRetainedResourceAndGroupAccess(t *testing.T) {
	testWorkspaceAdmissionRevokesRetainedResourceAndGroupAccess(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresWorkspaceAdmissionRevokesRetainedResourceAndGroupAccess(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	testWorkspaceAdmissionRevokesRetainedResourceAndGroupAccess(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func testWorkspaceAdmissionRevokesRetainedResourceAndGroupAccess(t *testing.T, configuration config.DatabaseConfig) {
	f := newChatGroupFixture(t, configuration)
	chat, _, _ := f.sharedChat()
	alice := f.people["alice"]
	project := model.Project{Ref: model.ProjectRef{Workspace: f.workspace, Id: newTypedID(t, typed_id.Project)}, Enabled: true}
	if err, _ := f.store.ProjectsCreate(f.ctx, project, alice, nil); err != nil {
		t.Fatal(err)
	}
	linked := model.SessionRef{Workspace: f.workspace, Id: newTypedID(t, typed_id.Session)}
	if err, _ := f.store.SessionsCreate(f.ctx, model.Session{Ref: linked, Project: &project.Ref, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	err, message := f.send(chat, "alice", model.MessageTargetRef{Kind: "agent", Id: f.bots["eng"]})
	if err != nil {
		t.Fatal(err)
	}
	err, request := f.store.SessionEventChildGet(f.ctx, message.Ref, model.SessionEventKindAgentRequest)
	if err != nil || request == nil {
		t.Fatalf("queued request: %#v %v", request, err)
	}
	if err, removed := f.store.SystemWorkspacePrincipalGrantsRemove(f.ctx, f.workspace.Id, alice.Id); err != nil || !removed {
		t.Fatalf("remove admission: %t %v", removed, err)
	}
	countGroupSignals := func() int {
		t.Helper()
		var count int
		if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_event_topics WHERE topic = `+f.p(1), alice.Id+"/"+f.groups["engineering"]).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	beforeSignals := countGroupSignals()
	name := "Updated engineering"
	if err, _ := f.store.SystemGroupUpdate(f.ctx, f.workspace.Id, f.groups["engineering"], 1, database.SystemGroupInput{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if countGroupSignals() != beforeSignals {
		t.Fatal("revoked human continued receiving group activity through retained membership")
	}
	if err, roles := f.store.WorkspaceRolesGet(f.ctx, f.workspace, alice); err != nil || len(roles) != 0 {
		t.Fatalf("workspace roles survived: %#v %v", roles, err)
	}
	if err, updated := f.store.SessionNameSet(f.ctx, chat, "Unauthorized generated title", alice); err != nil || updated {
		t.Fatalf("revoked author published a generated title: %t %v", updated, err)
	}
	for _, ref := range []model.SessionRef{chat, linked} {
		if err, got := f.store.SessionGet(f.ctx, ref, alice); err != nil || got != nil {
			t.Fatalf("retained chat grant bypassed admission: %#v %v", got, err)
		}
		if err, roles := f.store.SessionRolesGet(f.ctx, ref, alice); err != nil || len(roles) != 0 {
			t.Fatalf("chat roles survived admission loss: %#v %v", roles, err)
		}
	}
	if err, got := f.store.ProjectGet(f.ctx, project.Ref, alice); err != nil || got != nil {
		t.Fatalf("direct project bypassed admission: %#v %v", got, err)
	}
	if err, got := f.store.ProjectsGetByRefs(f.ctx, f.workspace, alice, []model.ProjectRef{project.Ref}); err != nil || len(got) != 0 {
		t.Fatalf("reference lookup bypassed admission: %#v %v", got, err)
	}
	if err, got := f.store.WorkspaceGroupsGet(f.ctx, f.workspace, alice); err != nil || len(got) != 0 {
		t.Fatalf("retained membership exposed groups: %#v %v", got, err)
	}
	if err, allowed := f.store.SessionRequestAuthorizedGet(f.ctx, request.Ref); err != nil || allowed {
		t.Fatalf("queued request remained authorized: %t %v", allowed, err)
	}
	if err, active := f.store.SessionAgentParticipantGet(f.ctx, chat, model.WorkspaceAgentRef{Workspace: f.workspace, Id: f.bots["eng"]}); err != nil || !active {
		t.Fatalf("author admission affected bot participation: %t %v", active, err)
	}
	if err, _ := f.send(chat, "alice"); !errors.Is(err, database.ErrSessionParticipantForbidden) {
		t.Fatalf("revoked author sent message: %v", err)
	}
	err, recipients, _ := f.store.SessionMessageTargetsSearch(f.ctx, chat, f.people["bridge"], database.SessionParticipantSearch{Kind: "principal", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range recipients {
		if target.Ref.Id == alice.Id {
			t.Fatal("revoked human remained an effective recipient")
		}
	}
	err, options, _ := f.store.SessionParticipantsSearch(f.ctx, chat, f.people["bridge"], database.SessionParticipantSearch{Kind: "principal", Limit: 100}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range options {
		if target.ID == alice.Id {
			t.Fatal("revoked human remained invitable through shared group")
		}
	}
	var retained bool
	if err := f.store.QueryRow(`SELECT enabled FROM gatehouse_group_members WHERE workspace_id = `+f.p(1)+` AND group_id = `+f.p(2)+` AND principal_id = `+f.p(3), f.workspace.Id, f.groups["engineering"], alice.Id).Scan(&retained); err != nil || !retained {
		t.Fatalf("revocation rewrote group membership: %t %v", retained, err)
	}
	if err, _ := f.store.SystemWorkspacePrincipalGrantCreate(f.ctx, f.workspace.Id, alice.Id, authz.Contributor); err != nil {
		t.Fatal(err)
	}
	if err, got := f.store.ProjectGet(f.ctx, project.Ref, alice); err != nil || got == nil {
		t.Fatalf("restored admission lost project grant: %#v %v", got, err)
	}
	if err, got := f.store.SessionGet(f.ctx, chat, alice); err != nil || got == nil {
		t.Fatalf("restored admission lost group chat: %#v %v", got, err)
	}
	if err, authorized := f.store.SessionRequestAuthorizedGet(f.ctx, request.Ref); err != nil || !authorized {
		t.Fatalf("restored admission did not authorize retained request: %t %v", authorized, err)
	}
}
