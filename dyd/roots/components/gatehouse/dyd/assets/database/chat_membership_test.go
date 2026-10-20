package database_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestChatSendJoinsOutsideParticipantsAtomicallyAndChecksSender(t *testing.T) {
	testChatSendJoinsOutsideParticipants(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresChatSendJoinsOutsideParticipantsAtomicallyAndChecksSender(t *testing.T) {
	dsn := os.Getenv("GATEHOUSE_TEST_MEMBERSHIP_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires a disposable PostgreSQL server with CREATE DATABASE permission")
	}
	connection, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	control := stdlib.OpenDB(*connection)
	defer control.Close()
	id, err := typed_id.New(typed_id.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	name := "membership_" + strings.TrimPrefix(id, "wsp_")
	if _, err := control.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := control.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); err != nil {
			t.Errorf("clean up test database: %v", err)
		}
	}()
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("test database connection must be a PostgreSQL URL")
	}
	parsed.Path = "/" + name
	t.Setenv("GATEHOUSE_TEST_MEMBERSHIP_DATABASE_URL", parsed.String())
	testChatSendJoinsOutsideParticipants(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MEMBERSHIP_DATABASE_URL"})
}

func testChatSendJoinsOutsideParticipants(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := context.Background()
	p := func(index int) string {
		if configuration.Kind == config.DatabaseKindPostgres {
			return fmt.Sprintf("$%d", index)
		}
		return "?"
	}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}, {Alias: "other", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}, {Alias: "bob", Enabled: true}, {Alias: "carol", Enabled: true}},
		Groups:          []config.Group{{WorkspaceID: "engineering", Alias: "team", Enabled: true, Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true}, {PrincipalID: "bob", Enabled: true}}}, {WorkspaceID: "engineering", Alias: "contacts", Enabled: true, Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true}, {PrincipalID: "carol", Enabled: true}}}, {WorkspaceID: "other", Alias: "other-team", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Done."}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, withTestWorkspaceMembers(state)); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	person := func(alias string) model.PrincipalRef {
		t.Helper()
		var ref model.PrincipalRef
		if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = `+p(1), alias).Scan(&ref.Id); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	groupFor := func(workspaceAlias, alias string) string {
		t.Helper()
		var id string
		if err := store.QueryRow(`SELECT g.id FROM gatehouse_groups g JOIN gatehouse_workspaces w ON w.id = g.workspace_id WHERE w.alias = `+p(1)+` AND g.alias = `+p(2), workspaceAlias, alias).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	alice, bob, carol := person("alice"), person("bob"), person("carol")
	group := groupFor("engineering", "team")
	var bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionParticipantGrantCreate(ctx, session, alice, &bob.Id, nil, authz.Contributor); err != nil {
		t.Fatal(err)
	}
	send := func(actor model.PrincipalRef, refs []model.MessageTargetRef, invalidFile bool) error {
		id, _ := typed_id.New(typed_id.SessionEvent)
		targets := make([]model.MessageTarget, len(refs))
		for i, ref := range refs {
			targets[i] = model.MessageTarget{Ref: ref}
		}
		payload := map[string]interface{}{"text": "Hello", "targets": targets}
		if invalidFile {
			payload["attachments"] = []string{"sfi_00000000000000000000000000"}
		}
		err, _ := store.SessionMessagesCreate(ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: actor, Enabled: true}, Payload: payload})
		return err
	}
	botRef := model.MessageTargetRef{Kind: "agent", Id: bot}
	err, current, _ := store.SessionMessageTargetsSearch(ctx, session, alice, database.SessionParticipantSearch{Kind: "agent", Limit: 50})
	if err != nil || len(current) != 0 {
		t.Fatalf("nonparticipant bot in chat lookup: %#v %v", current, err)
	}
	err, outside, _ := store.SessionMessageTargetsSearch(ctx, session, alice, database.SessionParticipantSearch{Kind: "agent", Outside: true, Limit: 50})
	if err != nil || len(outside) != 1 {
		t.Fatalf("outside bot lookup: %#v %v", outside, err)
	}
	if err := send(bob, []model.MessageTargetRef{botRef}, false); !errors.Is(err, database.ErrSessionParticipantForbidden) {
		t.Fatalf("nonmanager invited bot: %v", err)
	}
	if err := send(alice, []model.MessageTargetRef{botRef}, true); err == nil {
		t.Fatal("invalid attachment send succeeded")
	}
	if err, active := store.SessionAgentParticipantGet(ctx, session, model.WorkspaceAgentRef{Workspace: workspace, Id: bot}); err != nil || active {
		t.Fatalf("failed send left bot joined: %t %v", active, err)
	}
	refs := []model.MessageTargetRef{botRef, {Kind: "principal", Id: carol.Id}, {Kind: "group", Id: group}}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- send(alice, refs, false) }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent send: %v", err)
		}
	}
	for _, ref := range refs {
		column := ref.Kind
		if column == "group" {
			column = `"group"`
		}
		var count int
		if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_grants WHERE workspace = `+p(1)+` AND session = `+p(2)+` AND `+column+` = `+p(3)+` AND role = 'contributor' AND enabled = TRUE`, workspace.Id, session.Id, ref.Id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("joined %#v = %d %v", ref, count, err)
		}
	}
	if err := send(bob, []model.MessageTargetRef{botRef}, false); err != nil {
		t.Fatalf("participant could not address joined bot: %v", err)
	}
	err, roster, _ := store.SessionParticipantsSearch(ctx, session, alice, database.SessionParticipantSearch{Kind: "agent", Limit: 50}, false)
	if err != nil || len(roster) != 1 || roster[0].Grants[0].Agent == nil {
		t.Fatalf("bot roster: %#v %v", roster, err)
	}
	disabled := false
	grant := roster[0].Grants[0]
	if err, _ := store.SessionParticipantGrantUpdate(ctx, session, alice, grant.ID, nil, &disabled, grant.Revision); err != nil {
		t.Fatal(err)
	}
	if err, active := store.SessionAgentParticipantGet(ctx, session, model.WorkspaceAgentRef{Workspace: workspace, Id: bot}); err != nil || active {
		t.Fatalf("removed bot still active: %t %v", active, err)
	}
	if err := send(bob, []model.MessageTargetRef{botRef}, false); !errors.Is(err, database.ErrSessionParticipantForbidden) {
		t.Fatalf("stale target bypassed removal: %v", err)
	}
	otherGroup := groupFor("other", "other-team")
	if err := send(alice, []model.MessageTargetRef{{Kind: "group", Id: otherGroup}}, false); !errors.Is(err, database.ErrMessageTarget) {
		t.Fatalf("cross-workspace invitation: %v", err)
	}
	for _, personFirst := range []bool{false, true} {
		for _, preserveDirect := range []bool{false, true} {
			t.Run(fmt.Sprintf("group-precedence/person-first=%t/direct=%t", personFirst, preserveDirect), func(t *testing.T) {
				id, err := typed_id.New(typed_id.Session)
				if err != nil {
					t.Fatal(err)
				}
				chat := model.SessionRef{Workspace: workspace, Id: id}
				if err, _ := store.SessionsCreate(ctx, model.Session{Ref: chat, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
					t.Fatal(err)
				}
				var direct *database.SessionParticipantGrant
				if preserveDirect {
					err, direct = store.SessionParticipantGrantCreate(ctx, chat, alice, &bob.Id, nil, authz.Member)
					if err != nil {
						t.Fatal(err)
					}
				}
				selected := []model.MessageTargetRef{{Kind: "group", Id: group}, {Kind: "principal", Id: bob.Id}}
				if personFirst {
					selected[0], selected[1] = selected[1], selected[0]
				}
				targets := make([]model.MessageTarget, len(selected))
				for index, ref := range selected {
					targets[index] = model.MessageTarget{Ref: ref}
				}
				messageID, err := typed_id.New(typed_id.SessionEvent)
				if err != nil {
					t.Fatal(err)
				}
				err, stored := store.SessionMessagesCreate(ctx, model.SessionEvent{
					Ref: model.SessionEventRef{Session: chat, Id: messageID}, Kind: "message.text",
					AuthorPrincipal: &model.Principal{Ref: alice, Enabled: true}, Payload: map[string]interface{}{"text": "For Bob and the team", "targets": targets},
				})
				if err != nil {
					t.Fatal(err)
				}
				saved, err := database.SessionMessageTargets(stored)
				if err != nil || len(saved) != len(selected) {
					t.Fatalf("saved targets: %#v, %v", saved, err)
				}
				for index, ref := range selected {
					if saved[index].Ref != ref {
						t.Fatalf("message recipient order changed: %#v", saved)
					}
				}
				var directCount int
				if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_grants WHERE workspace = `+p(1)+` AND session = `+p(2)+` AND principal = `+p(3), workspace.Id, chat.Id, bob.Id).Scan(&directCount); err != nil {
					t.Fatal(err)
				}
				wantDirect := 0
				if preserveDirect {
					wantDirect = 1
				}
				if directCount != wantDirect {
					t.Fatalf("direct grants = %d, want %d", directCount, wantDirect)
				}
				if direct != nil {
					var role string
					var revision int
					var enabled bool
					if err := store.QueryRow(`SELECT role, revision, enabled FROM gatehouse_session_grants WHERE id = `+p(1), direct.ID).Scan(&role, &revision, &enabled); err != nil {
						t.Fatal(err)
					}
					if role != string(direct.Role) || revision != direct.Revision || enabled != direct.Enabled {
						t.Fatalf("existing direct grant changed: %s %d %t", role, revision, enabled)
					}
				}
				err, groups, _ := store.SessionParticipantsSearch(ctx, chat, alice, database.SessionParticipantSearch{Kind: "group", Limit: 50}, false)
				if err != nil || len(groups) != 1 {
					t.Fatalf("group grant: %#v %v", groups, err)
				}
				groupGrant := groups[0].Grants[0]
				if err, _ := store.SessionParticipantGrantUpdate(ctx, chat, alice, groupGrant.ID, nil, &disabled, groupGrant.Revision); err != nil {
					t.Fatal(err)
				}
				err, access := store.SessionGet(ctx, chat, bob)
				if err != nil || (access != nil) != preserveDirect {
					t.Fatalf("access after group removal: %#v %v; direct=%t", access, err, preserveDirect)
				}
			})
		}
	}
}
