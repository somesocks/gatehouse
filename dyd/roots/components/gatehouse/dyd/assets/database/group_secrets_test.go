package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestGroupSecrets(t *testing.T) {
	testGroupSecrets(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresGroupSecrets(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_GROUP_SECRETS_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	testGroupSecrets(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_GROUP_SECRETS_POSTGRES_URL"})
}

func testGroupSecrets(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil { t.Fatal(err) }
	defer store.Close()
	owner, alice, bob, outsider := "owner", "alice", "bob", "outsider"
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "team", Enabled: true}, {Alias: "other", Enabled: true}},
		Principals: []config.Principal{{Alias: owner, Enabled: true}, {Alias: alice, Enabled: true}, {Alias: bob, Enabled: true}, {Alias: outsider, Enabled: true}},
		WorkspaceGrants: []config.WorkspaceGrant{
			{WorkspaceID: "team", PrincipalID: &owner, Role: "manager", Revision: 1, Enabled: true},
			{WorkspaceID: "team", PrincipalID: &alice, Role: "member", Revision: 1, Enabled: true},
			{WorkspaceID: "team", PrincipalID: &bob, Role: "member", Revision: 1, Enabled: true},
			{WorkspaceID: "team", PrincipalID: &outsider, Role: "manager", Revision: 1, Enabled: true},
			{WorkspaceID: "other", PrincipalID: &owner, Role: "manager", Revision: 1, Enabled: true},
			{WorkspaceID: "other", PrincipalID: &alice, Role: "member", Revision: 1, Enabled: true},
		},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Protocol: "builtin", Revision: 1, Enabled: true}},
		AgentModels: []config.AgentModel{{Alias: "model", ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Done"}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Revision: 1, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "team", Alias: "assistant", ModelAlias: "model", Revision: 1, Enabled: true},
			{WorkspaceID: "team", Alias: "second", ModelAlias: "model", Revision: 1, Enabled: true},
			{WorkspaceID: "other", Alias: "assistant", ModelAlias: "model", Revision: 1, Enabled: true},
		},
	}
	for _, alias := range []string{"human", "bot", "shared", "neither", "disabled", "contacts", "foreign"} {
		workspace := "team"
		if alias == "foreign" { workspace = "other" }
		group := config.Group{WorkspaceID: workspace, Alias: alias, Enabled: true, Members: []config.GroupMember{{PrincipalID: owner, Role: "manager", Enabled: true}}}
		if alias == "human" || alias == "shared" || alias == "disabled" || alias == "contacts" || alias == "foreign" {
			group.Members = append(group.Members, config.GroupMember{PrincipalID: alice, Enabled: true})
		}
		if alias == "neither" || alias == "contacts" { group.Members = append(group.Members, config.GroupMember{PrincipalID: bob, Enabled: true}) }
		if alias == "bot" || alias == "shared" || alias == "disabled" || alias == "foreign" {
			group.Agents = []config.GroupAgent{{AgentID: "assistant", Enabled: true, Revision: 1}}
		}
		if alias == "neither" { group.Agents = []config.GroupAgent{{AgentID: "second", Enabled: true, Revision: 1}} }
		state.Groups = append(state.Groups, group)
	}
	if err := migrateState(ctx, store, configuration, state); err != nil { t.Fatal(err) }
	p := func(index int) string {
		if configuration.Kind == config.DatabaseKindPostgres { return fmt.Sprintf("$%d", index) }
		return fmt.Sprintf("?%d", index)
	}
	lookup := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := store.QueryRowContext(ctx, query, args...).Scan(&id); err != nil { t.Fatal(err) }
		return id
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.ExecContext(ctx, query, args...); err != nil { t.Fatalf("%s: %v", query, err) }
	}
	person := func(alias string) model.PrincipalRef { return model.PrincipalRef{Id: lookup(`SELECT id FROM gatehouse_principals WHERE alias = `+p(1), alias)} }
	manager, actor, otherPerson, administrator := person(owner), person(alice), person(bob), person(outsider)
	workspace, other := workspaceRef(t, ctx, store, "team"), workspaceRef(t, ctx, store, "other")
	bot := model.WorkspaceAgentRef{Workspace: workspace, Id: lookup(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = `+p(1)+` AND alias = 'assistant'`, workspace.Id)}
	foreignBot := model.WorkspaceAgentRef{Workspace: other, Id: lookup(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = `+p(1)+` AND alias = 'assistant'`, other.Id)}
	refs := map[string]model.GroupSecretRef{}
	for _, alias := range []string{"human", "bot", "shared", "neither", "disabled", "foreign"} {
		ownerWorkspace := workspace
		if alias == "foreign" { ownerWorkspace = other }
		group := lookup(`SELECT id FROM gatehouse_groups WHERE workspace_id = `+p(1)+` AND alias = `+p(2), ownerWorkspace.Id, alias)
		id, _ := typed_id.New(typed_id.GroupSecret)
		ref := model.GroupSecretRef{Workspace: ownerWorkspace, Group: model.GroupRef{Id: group}, Id: id}
		err, secret := store.GroupSecretCreate(ctx, model.GroupSecret{Ref: ref, Description: "Credential " + alias, Ciphertext: "cipher-" + alias}, manager)
		if err != nil || secret.Revision != 1 { t.Fatalf("create %s: %#v %v", alias, secret, err) }
		refs[alias] = ref
	}
	exec(`UPDATE gatehouse_groups SET enabled = FALSE WHERE id = `+p(1), refs["disabled"].Group.Id)
	id, _ := typed_id.New(typed_id.Session)
	session := model.SessionRef{Workspace: workspace, Id: id}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &actor, Enabled: true}, actor); err != nil { t.Fatal(err) }
	if err, _ := store.SessionParticipantGrantCreateSubject(ctx, session, actor, nil, nil, &bot.Id, authz.Contributor); err != nil { t.Fatal(err) }
	if err, _ := store.SessionParticipantGrantCreate(ctx, session, actor, &otherPerson.Id, nil, authz.Contributor); err != nil { t.Fatal(err) }
	assertAvailable := func(alias string, want bool) {
		t.Helper()
		err, detail := store.GroupSecretForBotGet(ctx, workspace, actor, bot, refs[alias].Id)
		if want {
			if err != nil || detail == nil || detail.Secret.Ciphertext != "cipher-"+alias { t.Fatalf("read %s: %#v %v", alias, detail, err) }
		} else if !errors.Is(err, database.ErrGroupSecretNotFound) { t.Fatalf("unauthorized read %s: %#v %v", alias, detail, err) }
	}
	assertList := func(want int) {
		t.Helper()
		err, items := store.GroupSecretsForBotGet(ctx, workspace, actor, bot)
		if err != nil || len(items) != want { t.Fatalf("shared-group list count: %d want %d (%v)", len(items), want, err) }
		seen := map[string]bool{}
		for _, item := range items {
			if item.Ref.Id != refs["shared"].Id { t.Fatalf("listed a secret outside the shared group: %s", item.Ref.Id) }
			if seen[item.Ref.Id] { t.Fatal("overlapping memberships duplicated a secret") }
			seen[item.Ref.Id] = true
		}
	}
	assertList(1)
	assertAvailable("shared", true)
	for _, alias := range []string{"human", "bot", "neither", "disabled", "foreign"} { assertAvailable(alias, false) }
	if err, items := store.GroupSecretsForBotGet(ctx, workspace, otherPerson, bot); err != nil || len(items) != 0 { t.Fatalf("another conversation participant's memberships expanded access: %#v %v", items, err) }
	if err, _ := store.GroupSecretForBotGet(ctx, workspace, actor, foreignBot, refs["human"].Id); !errors.Is(err, database.ErrGroupSecretNotFound) { t.Fatalf("foreign bot: %v", err) }
	if err, detail := store.GroupSecretGet(ctx, refs["human"], actor); err != nil || detail.Secret.Ciphertext != "" { t.Fatalf("human metadata loaded ciphertext: %#v %v", detail, err) }
	if err, _ := store.GroupSecretGet(ctx, refs["bot"], actor); !errors.Is(err, database.ErrGroupSecretNotFound) { t.Fatalf("human acquired bot metadata outside its memberships: %v", err) }
	for _, principal := range []model.PrincipalRef{actor, administrator} {
		description := "Denied"
		if err, _ := store.GroupSecretUpdate(ctx, refs["human"], principal, &description, nil, 1); !errors.Is(err, database.ErrGroupForbidden) { t.Fatalf("non-group-manager changed secret: %v", err) }
	}
	// Either membership loss must revoke both listing and direct-by-ID reads,
	// even while the other membership remains enabled.
	exec(`UPDATE gatehouse_group_members SET enabled = FALSE WHERE group_id = `+p(1)+` AND principal_id = `+p(2), refs["shared"].Group.Id, actor.Id)
	assertList(0)
	assertAvailable("shared", false)
	exec(`UPDATE gatehouse_group_members SET enabled = TRUE WHERE group_id = `+p(1)+` AND principal_id = `+p(2), refs["shared"].Group.Id, actor.Id)
	assertList(1)
	assertAvailable("shared", true)
	exec(`UPDATE gatehouse_group_agents SET enabled = FALSE WHERE group_id = `+p(1)+` AND agent_id = `+p(2), refs["shared"].Group.Id, bot.Id)
	assertList(0)
	assertAvailable("shared", false)
	exec(`UPDATE gatehouse_group_agents SET enabled = TRUE WHERE group_id = `+p(1)+` AND agent_id = `+p(2), refs["shared"].Group.Id, bot.Id)
	assertList(1)
	assertAvailable("shared", true)
	for _, change := range []struct{ table, condition string }{
		{"gatehouse_principals", "id = '"+actor.Id+"'"},
		{"gatehouse_workspace_agents", "id = '"+bot.Id+"'"},
		{"gatehouse_workspace_grants", "workspace = '"+workspace.Id+"' AND principal = '"+actor.Id+"'"},
	} {
		exec(`UPDATE `+change.table+` SET enabled = FALSE WHERE `+change.condition)
		assertList(0)
		assertAvailable("shared", false)
		exec(`UPDATE `+change.table+` SET enabled = TRUE WHERE `+change.condition)
	}
	assertList(1)
	// Round admission owns these checks; ordinary credential permissions must not
	// repeat them or terminate an already-admitted tool batch.
	for _, change := range []struct{ table, condition string }{
		{"gatehouse_session_grants", "session = '"+session.Id+"'"},
		{"gatehouse_sessions", "id = '"+session.Id+"'"},
		{"gatehouse_agent_models", "alias = 'model'"},
		{"gatehouse_agent_providers", "alias = 'builtin'"},
	} {
		exec(`UPDATE `+change.table+` SET enabled = FALSE WHERE `+change.condition)
		assertList(1)
		assertAvailable("shared", true)
		exec(`UPDATE `+change.table+` SET enabled = TRUE WHERE `+change.condition)
	}
	description := "Rotated metadata"
	err, updated := store.GroupSecretUpdate(ctx, refs["shared"], manager, &description, nil, 1)
	if err != nil || updated.Secret.Revision != 2 { t.Fatalf("update: %#v %v", updated, err) }
	assertAvailable("shared", true)
	if err, _ := store.GroupSecretUpdate(ctx, refs["shared"], manager, &description, nil, 1); !errors.Is(err, database.ErrRevisionConflict) { t.Fatalf("stale edit: %v", err) }
	results := make(chan error, 2)
	for range 2 { go func() { err, _ := store.GroupSecretUpdate(ctx, refs["shared"], manager, &description, nil, 2); results <- err }() }
	succeeded, conflicted := 0, 0
	for range 2 { err := <-results; if err == nil { succeeded++ } else if errors.Is(err, database.ErrRevisionConflict) { conflicted++ } else { t.Fatal(err) } }
	if succeeded != 1 || conflicted != 1 { t.Fatalf("competing writes: %d %d", succeeded, conflicted) }
	if err := store.GroupSecretRemove(ctx, refs["shared"], manager, 3); err != nil { t.Fatal(err) }
	assertList(0)
	assertAvailable("shared", false)
	var cipher string
	var enabled bool
	if err := store.QueryRowContext(ctx, `SELECT ciphertext, enabled FROM gatehouse_group_secrets WHERE id = `+p(1), refs["shared"].Id).Scan(&cipher, &enabled); err != nil || cipher != "" || enabled { t.Fatalf("removed value retained: %q %t %v", cipher, enabled, err) }
}
