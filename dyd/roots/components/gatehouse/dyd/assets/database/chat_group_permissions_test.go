package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

type chatGroupFixture struct {
	t         *testing.T
	ctx       context.Context
	store     *database.Store
	workspace model.WorkspaceRef
	people    map[string]model.PrincipalRef
	groups    map[string]string
	bots      map[string]string
	p         func(int) string
}

func newChatGroupFixture(t *testing.T, configuration config.DatabaseConfig) *chatGroupFixture {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	suffix := newTypedID(t, typed_id.Workspace)
	workspace, foreign, provider, agentModel := "chat-"+suffix, "foreign-"+suffix, "provider-"+suffix, "model-"+suffix
	aliases := map[string]string{}
	principals := []config.Principal{}
	for _, name := range []string{"alice", "bob", "bridge", "lonely", "disabled", "dana"} {
		aliases[name] = name + "-" + suffix
		principals = append(principals, config.Principal{Alias: aliases[name], Enabled: name != "disabled"})
	}
	members := func(names ...string) []config.GroupMember {
		result := []config.GroupMember{}
		for _, name := range names {
			result = append(result, config.GroupMember{PrincipalID: aliases[name], Enabled: name != "disabled", Revision: 1})
		}
		return result
	}
	botMembers := func(names ...string) []config.GroupAgent {
		result := []config.GroupAgent{}
		for _, name := range names {
			result = append(result, config.GroupAgent{AgentID: name, Enabled: true, Revision: 1})
		}
		return result
	}
	engineering := members("alice", "bridge", "disabled")
	engineering = append(engineering, config.GroupMember{PrincipalID: aliases["dana"], Enabled: false, Revision: 1})
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: workspace, Enabled: true}, {Alias: foreign, Enabled: true}},
		Principals:     principals,
		AgentProviders: []config.AgentProvider{{Alias: provider, Protocol: "builtin", Revision: 1, Enabled: true}},
		AgentModels:    []config.AgentModel{{Alias: agentModel, ProviderAlias: provider, Revision: 1, Model: "dummy.fixed-reply", Parameters: `{"text":"Done"}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		Groups: []config.Group{
			{WorkspaceID: workspace, Alias: "engineering", Enabled: true, Revision: 1, Members: engineering, Agents: botMembers("eng", "shared", "disabled")},
			{WorkspaceID: workspace, Alias: "finance", Enabled: true, Revision: 1, Members: members("bob", "bridge"), Agents: botMembers("fin", "shared")},
			{WorkspaceID: workspace, Alias: "disabled", Enabled: false, Revision: 1, Members: members("alice", "bob"), Agents: botMembers("hidden")},
			{WorkspaceID: workspace, Alias: "paused", Enabled: true, Revision: 1, Members: []config.GroupMember{{PrincipalID: aliases["alice"], Enabled: false, Revision: 1}, {PrincipalID: aliases["bob"], Enabled: true, Revision: 1}}, Agents: botMembers("paused")},
			{WorkspaceID: foreign, Alias: "foreign", Enabled: true, Revision: 1, Members: members("alice", "bob"), Agents: botMembers("foreign")},
		},
	}
	for _, name := range []string{"eng", "fin", "shared", "general", "hidden", "disabled", "paused"} {
		state.WorkspaceAgents = append(state.WorkspaceAgents, config.WorkspaceAgent{WorkspaceID: workspace, Alias: name, ModelAlias: agentModel, Revision: 1, Default: name == "general", Enabled: name != "disabled"})
	}
	state.WorkspaceAgents = append(state.WorkspaceAgents, config.WorkspaceAgent{WorkspaceID: foreign, Alias: "foreign", ModelAlias: agentModel, Revision: 1, Enabled: true})
	for _, alias := range aliases {
		state.WorkspaceGrants = append(state.WorkspaceGrants, config.WorkspaceGrant{WorkspaceID: workspace, PrincipalID: stringPointer(alias), Role: "contributor", Enabled: true, Revision: 1})
	}
	for _, name := range []string{"alice", "bob"} {
		state.WorkspaceGrants = append(state.WorkspaceGrants, config.WorkspaceGrant{WorkspaceID: foreign, PrincipalID: stringPointer(aliases[name]), Role: "member", Enabled: true, Revision: 1})
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	f := &chatGroupFixture{t: t, ctx: ctx, store: store, workspace: workspaceRef(t, ctx, store, workspace), people: map[string]model.PrincipalRef{}, groups: map[string]string{}, bots: map[string]string{}}
	f.p = func(index int) string {
		if configuration.Kind == config.DatabaseKindPostgres {
			return fmt.Sprintf("$%d", index)
		}
		return fmt.Sprintf("?%d", index)
	}
	for name, alias := range aliases {
		var principal model.PrincipalRef
		if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = `+f.p(1), alias).Scan(&principal.Id); err != nil {
			t.Fatal(err)
		}
		f.people[name] = principal
	}
	for _, group := range state.Groups {
		ref := f.workspace
		if group.WorkspaceID == foreign {
			ref = workspaceRef(t, ctx, store, foreign)
		}
		var id string
		if err := store.QueryRow(`SELECT id FROM gatehouse_groups WHERE workspace_id = `+f.p(1)+` AND alias = `+f.p(2), ref.Id, group.Alias).Scan(&id); err != nil {
			t.Fatal(err)
		}
		f.groups[group.Alias] = id
	}
	for _, bot := range state.WorkspaceAgents {
		ref := f.workspace
		if bot.WorkspaceID == foreign {
			ref = workspaceRef(t, ctx, store, foreign)
		}
		var id string
		if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = `+f.p(1)+` AND alias = `+f.p(2), ref.Id, bot.Alias).Scan(&id); err != nil {
			t.Fatal(err)
		}
		f.bots[bot.Alias] = id
	}
	return f
}

func (f *chatGroupFixture) chat(author string) model.SessionRef {
	f.t.Helper()
	actor := f.people[author]
	chat := model.SessionRef{Workspace: f.workspace, Id: newTypedID(f.t, typed_id.Session)}
	if err, _ := f.store.SessionsCreate(f.ctx, model.Session{Ref: chat, AuthorPrincipal: &actor, Enabled: true}, actor); err != nil {
		f.t.Fatal(err)
	}
	return chat
}

func (f *chatGroupFixture) invite(chat model.SessionRef, author, kind, id string, role authz.Role) *database.SessionParticipantGrant {
	f.t.Helper()
	var person, group, bot *string
	switch kind {
	case "principal":
		person = &id
	case "group":
		group = &id
	case "agent":
		bot = &id
	}
	err, grant := f.store.SessionParticipantGrantCreateSubject(f.ctx, chat, f.people[author], person, group, bot, role)
	if err != nil {
		f.t.Fatal(err)
	}
	return grant
}

func (f *chatGroupFixture) send(chat model.SessionRef, author string, refs ...model.MessageTargetRef) (error, model.SessionEvent) {
	targets := []model.MessageTarget{}
	for _, ref := range refs {
		targets = append(targets, model.MessageTarget{Ref: ref})
	}
	return f.store.SessionMessagesCreate(f.ctx, model.SessionEvent{Ref: model.SessionEventRef{Session: chat, Id: newTypedID(f.t, typed_id.SessionEvent)}, Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: f.people[author], Enabled: true}, Payload: map[string]interface{}{"text": "Hello", "targets": targets}})
}

func (f *chatGroupFixture) sharedChat() (model.SessionRef, *database.SessionParticipantGrant, *database.SessionParticipantGrant) {
	chat := f.chat("alice")
	engineering := f.invite(chat, "alice", "group", f.groups["engineering"], authz.Contributor)
	f.invite(chat, "alice", "principal", f.people["bridge"].Id, authz.Manager)
	finance := f.invite(chat, "bridge", "group", f.groups["finance"], authz.Contributor)
	return chat, engineering, finance
}

func assertChatIDs(t *testing.T, got []string, want ...string) {
	t.Helper()
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Fatalf("IDs = %#v, want %#v", got, want)
	}
}

func TestChatGroupPermissions(t *testing.T) {
	testChatGroupPermissions(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresChatGroupPermissions(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	testChatGroupPermissions(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func testChatGroupPermissions(t *testing.T, configuration config.DatabaseConfig) {
	t.Run("discovery and transactional invitations", func(t *testing.T) {
		f := newChatGroupFixture(t, configuration)
		chat := f.chat("alice")
		for kind, want := range map[string][]string{
			"principal": {f.people["alice"].Id, f.people["bridge"].Id},
			"group":     {f.groups["engineering"]},
			"agent":     {f.bots["eng"], f.bots["shared"], f.bots["general"]},
		} {
			err, options, _ := f.store.SessionParticipantsSearch(f.ctx, chat, f.people["alice"], database.SessionParticipantSearch{Kind: kind, Limit: 100}, true)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, option := range options {
				ids = append(ids, option.ID)
			}
			assertChatIDs(t, ids, want...)
			// Outside-recipient pagination uses the same eligibility predicate.
			ids, cursor := []string{}, ""
			for {
				err, page, next := f.store.SessionMessageTargetsSearch(f.ctx, chat, f.people["alice"], database.SessionParticipantSearch{Kind: kind, Outside: true, Limit: 1, Cursor: cursor})
				if err != nil {
					t.Fatal(err)
				}
				for _, target := range page {
					ids = append(ids, target.Ref.Id)
				}
				if next == "" {
					break
				}
				if len(ids) > 10 {
					t.Fatal("recipient pagination did not terminate")
				}
				cursor = next
			}
			outsideWant := want
			if kind == "principal" {
				outsideWant = []string{f.people["bridge"].Id}
			}
			assertChatIDs(t, ids, outsideWant...)
		}
		err, bots := f.store.WorkspaceAgentsGet(f.ctx, f.workspace, f.people["alice"])
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, bot := range bots {
			ids = append(ids, bot.ID)
		}
		assertChatIDs(t, ids, f.bots["eng"], f.bots["shared"], f.bots["general"])
		lonely := f.chat("lonely")
		for _, kind := range []string{"agent", "group"} {
			err, options, _ := f.store.SessionParticipantsSearch(f.ctx, lonely, f.people["lonely"], database.SessionParticipantSearch{Kind: kind, Limit: 100}, true)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, option := range options {
				ids = append(ids, option.ID)
			}
			if kind == "agent" {
				assertChatIDs(t, ids, f.bots["general"])
			} else if len(ids) != 0 {
				t.Fatalf("group-less user discovered groups: %#v", ids)
			}
		}
		f.invite(lonely, "lonely", "agent", f.bots["general"], authz.Contributor)
		for _, target := range []model.MessageTargetRef{
			{Kind: "principal", Id: f.people["bob"].Id}, {Kind: "principal", Id: f.people["disabled"].Id}, {Kind: "principal", Id: f.people["dana"].Id},
			{Kind: "group", Id: f.groups["finance"]}, {Kind: "group", Id: f.groups["disabled"]}, {Kind: "group", Id: f.groups["paused"]}, {Kind: "group", Id: f.groups["foreign"]},
			{Kind: "agent", Id: f.bots["fin"]}, {Kind: "agent", Id: f.bots["hidden"]}, {Kind: "agent", Id: f.bots["paused"]}, {Kind: "agent", Id: f.bots["disabled"]}, {Kind: "agent", Id: f.bots["foreign"]},
		} {
			var person, group, bot *string
			switch target.Kind {
			case "principal":
				person = &target.Id
			case "group":
				group = &target.Id
			case "agent":
				bot = &target.Id
			}
			if err, _ := f.store.SessionParticipantGrantCreateSubject(f.ctx, chat, f.people["alice"], person, group, bot, authz.Contributor); !errors.Is(err, database.ErrSessionParticipantNotFound) {
				t.Fatalf("explicit invitation %#v = %v", target, err)
			}
			if err, _ := f.send(chat, "alice", target); !errors.Is(err, database.ErrMessageTarget) {
				t.Fatalf("automatic invitation %#v = %v", target, err)
			}
		}
		// A permitted group earlier in the same request cannot launder another
		// recipient's eligibility, and a later failure rolls back the group invite.
		if err, _ := f.send(chat, "alice", model.MessageTargetRef{Kind: "group", Id: f.groups["engineering"]}, model.MessageTargetRef{Kind: "principal", Id: f.people["bob"].Id}); !errors.Is(err, database.ErrMessageTarget) {
			t.Fatalf("mixed invitation = %v", err)
		}
		for table, want := range map[string]int{"gatehouse_session_grants": 1, "gatehouse_session_events": 0, "gatehouse_agent_tasks__session_event_reply": 0} {
			var count int
			if err := f.store.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE workspace = `+f.p(1)+` AND session = `+f.p(2), f.workspace.Id, chat.Id).Scan(&count); err != nil || count != want {
				t.Fatalf("rejected invitation changed %s: %d, %v", table, count, err)
			}
		}
		for _, botFirst := range []bool{false, true} {
			groupChat := f.chat("alice")
			selected := []model.MessageTargetRef{{Kind: "group", Id: f.groups["engineering"]}, {Kind: "agent", Id: f.bots["eng"]}}
			if botFirst {
				selected[0], selected[1] = selected[1], selected[0]
			}
			err, message := f.send(groupChat, "alice", selected...)
			if err != nil {
				t.Fatal(err)
			}
			targets, err := database.SessionMessageTargets(message)
			if err != nil || len(targets) != 2 || targets[0].Ref != selected[0] || targets[1].Ref != selected[1] {
				t.Fatalf("group/bot target order changed: %#v %v", targets, err)
			}
			var direct int
			if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_grants WHERE workspace = `+f.p(1)+` AND session = `+f.p(2)+` AND agent IS NOT NULL`, f.workspace.Id, groupChat.Id).Scan(&direct); err != nil || direct != 0 {
				t.Fatalf("same-message group inheritance created direct bot grants: %d %v", direct, err)
			}
		}
		// Eligibility is checked at mutation time, not cached from an options lookup.
		if err := f.store.SystemGroupMemberUpdate(f.ctx, f.workspace.Id, f.groups["engineering"], f.people["alice"].Id, false, 1); err != nil {
			t.Fatal(err)
		}
		if err, _ := f.store.SessionParticipantGrantCreateSubject(f.ctx, chat, f.people["alice"], nil, nil, stringPointer(f.bots["eng"]), authz.Contributor); !errors.Is(err, database.ErrSessionParticipantNotFound) {
			t.Fatalf("revoked shared membership still invited bot: %v", err)
		}
	})

	t.Run("shared chat does not create new invitation relationships", func(t *testing.T) {
		f := newChatGroupFixture(t, configuration)
		chat, _, finance := f.sharedChat()
		for _, author := range []string{"alice", "bob"} {
			err, targets, _ := f.store.SessionMessageTargetsSearch(f.ctx, chat, f.people[author], database.SessionParticipantSearch{Kind: "agent", Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, target := range targets {
				ids = append(ids, target.Ref.Id)
			}
			assertChatIDs(t, ids, f.bots["eng"], f.bots["fin"], f.bots["shared"])
			if err, _ := f.send(chat, author, model.MessageTargetRef{Kind: "agent", Id: f.bots["eng"]}, model.MessageTargetRef{Kind: "agent", Id: f.bots["fin"]}); err != nil {
				t.Fatalf("existing participant interaction by %s: %v", author, err)
			}
		}
		err, roster, _ := f.store.SessionParticipantsSearch(f.ctx, chat, f.people["bob"], database.SessionParticipantSearch{Kind: "agent", Limit: 1}, false)
		if err != nil || len(roster) != 1 {
			t.Fatalf("effective roster page = %#v %v", roster, err)
		}
		err, roster, _ = f.store.SessionParticipantsSearch(f.ctx, chat, f.people["alice"], database.SessionParticipantSearch{Kind: "agent", Limit: 100}, false)
		if err != nil {
			t.Fatal(err)
		}
		sharedFound := false
		for _, bot := range roster {
			if bot.ID != f.bots["shared"] {
				continue
			}
			sharedFound = true
			if len(bot.Grants) != 0 || len(bot.InheritedFrom) != 2 {
				t.Fatalf("deduplicated shared bot = %#v", bot)
			}
			for _, source := range bot.InheritedFrom {
				if (source.Group != f.groups["engineering"] && source.Group != f.groups["finance"]) || source.Role != authz.Contributor {
					t.Fatalf("inherited participation lost provenance: %#v", source)
				}
			}
		}
		if !sharedFound {
			t.Fatal("effective roster omitted the bot shared by both groups")
		}
		var directBots int
		if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_grants WHERE workspace = `+f.p(1)+` AND session = `+f.p(2)+` AND agent IS NOT NULL`, f.workspace.Id, chat.Id).Scan(&directBots); err != nil || directBots != 0 {
			t.Fatalf("inherited bots became direct grants: %d %v", directBots, err)
		}
		if err, _ := f.send(chat, "alice", model.MessageTargetRef{Kind: "principal", Id: f.people["bob"].Id}, model.MessageTargetRef{Kind: "group", Id: f.groups["finance"]}); err != nil {
			t.Fatal(err)
		}
		separate := f.chat("alice")
		for _, target := range []model.MessageTargetRef{{Kind: "principal", Id: f.people["bob"].Id}, {Kind: "group", Id: f.groups["finance"]}, {Kind: "agent", Id: f.bots["fin"]}} {
			if err, _ := f.send(separate, "alice", target); !errors.Is(err, database.ErrMessageTarget) {
				t.Fatalf("shared chat enabled separate invitation %#v: %v", target, err)
			}
		}
		if err, _, _ := f.store.SessionParticipantsSearch(f.ctx, separate, f.people["bob"], database.SessionParticipantSearch{Kind: "agent", Limit: 10}, false); !errors.Is(err, database.ErrSessionParticipantNotFound) {
			t.Fatalf("outsider read a separate roster: %v", err)
		}
		if err, _, _ := f.store.SessionMessageTargetsSearch(f.ctx, chat, f.people["bob"], database.SessionParticipantSearch{Kind: "agent", Outside: true, Limit: 10}); !errors.Is(err, database.ErrSessionParticipantForbidden) {
			t.Fatalf("nonmanager browsed outside recipients: %v", err)
		}
		// Existing conversation history never broadens workspace discovery.
		disabled := false
		if err, _ := f.store.SessionParticipantGrantUpdate(f.ctx, chat, f.people["bridge"], finance.ID, nil, &disabled, finance.Revision); err != nil {
			t.Fatal(err)
		}
		err, directory := f.store.WorkspaceAgentsGet(f.ctx, f.workspace, f.people["alice"])
		if err != nil {
			t.Fatal(err)
		}
		for _, bot := range directory {
			if bot.ID == f.bots["fin"] {
				t.Fatal("historical interaction broadened workspace discovery")
			}
		}
	})

	t.Run("direct and inherited roster grants stay separate", func(t *testing.T) {
		f := newChatGroupFixture(t, configuration)
		chat, _, _ := f.sharedChat()
		assertSources := func(direct *database.SessionParticipantGrant) {
			t.Helper()
			err, roster, _ := f.store.SessionParticipantsSearch(f.ctx, chat, f.people["alice"], database.SessionParticipantSearch{Kind: "agent", Limit: 100}, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, bot := range roster {
				if bot.ID != f.bots["shared"] {
					continue
				}
				groups := []string{}
				for _, source := range bot.InheritedFrom {
					if source.Role != authz.Contributor {
						t.Fatalf("direct role edit changed inherited role: %#v", source)
					}
					groups = append(groups, source.Group)
				}
				assertChatIDs(t, groups, f.groups["engineering"], f.groups["finance"])
				if direct == nil {
					if len(bot.Grants) != 0 {
						t.Fatalf("inherited source exposed as editable bot grant: %#v", bot)
					}
				} else if len(bot.Grants) != 1 || bot.Grants[0].ID != direct.ID || bot.Grants[0].Role != direct.Role || bot.Grants[0].Agent == nil || *bot.Grants[0].Agent != bot.ID || bot.Grants[0].Group != nil {
					t.Fatalf("direct bot grant mixed with inherited sources: %#v", bot)
				}
				return
			}
			t.Fatal("effective roster omitted shared bot")
		}
		assertSources(nil)
		err, direct := f.store.SessionParticipantGrantCreateSubject(f.ctx, chat, f.people["alice"], nil, nil, stringPointer(f.bots["shared"]), authz.Contributor)
		if err != nil {
			t.Fatal(err)
		}
		assertSources(direct)
		role := authz.Manager
		err, direct = f.store.SessionParticipantGrantUpdate(f.ctx, chat, f.people["alice"], direct.ID, &role, nil, direct.Revision)
		if err != nil {
			t.Fatal(err)
		}
		assertSources(direct)
		disabled := false
		if err, _ := f.store.SessionParticipantGrantUpdate(f.ctx, chat, f.people["alice"], direct.ID, nil, &disabled, direct.Revision); err != nil {
			t.Fatal(err)
		}
		assertSources(nil)
		if err, active := f.store.SessionAgentParticipantGet(f.ctx, chat, model.WorkspaceAgentRef{Workspace: f.workspace, Id: f.bots["shared"]}); err != nil || !active {
			t.Fatalf("removing direct grant removed inherited participation: %t %v", active, err)
		}
	})

	t.Run("revocation and restoration use every participation path", func(t *testing.T) {
		f := newChatGroupFixture(t, configuration)
		chat, engineering, finance := f.sharedChat()
		request := func(bot string) {
			t.Helper()
			err, message := f.send(chat, "bob", model.MessageTargetRef{Kind: "agent", Id: f.bots[bot]})
			if err != nil {
				t.Fatal(err)
			}
			err, child := f.store.SessionEventChildGet(f.ctx, message.Ref, model.SessionEventKindAgentRequest)
			if err != nil || child == nil {
				t.Fatalf("bot request = %#v %v", child, err)
			}
		}
		request("eng")
		request("shared")
		active := func(bot string, expected bool) {
			t.Helper()
			if err, got := f.store.SessionAgentParticipantGet(f.ctx, chat, model.WorkspaceAgentRef{Workspace: f.workspace, Id: f.bots[bot]}); err != nil || got != expected {
				t.Fatalf("%s active = %t, want %t: %v", bot, got, expected, err)
			}
		}
		available := func(bot string, expected bool) {
			t.Helper()
			err, targets, _ := f.store.SessionMessageTargetsSearch(f.ctx, chat, f.people["bob"], database.SessionParticipantSearch{Kind: "agent", Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, target := range targets {
				if target.Ref.Id == f.bots[bot] {
					found = true
				}
			}
			if found != expected {
				t.Fatalf("%s available = %t, want %t", bot, found, expected)
			}
		}
		update := func(author string, grant *database.SessionParticipantGrant, enabled bool) *database.SessionParticipantGrant {
			t.Helper()
			err, changed := f.store.SessionParticipantGrantUpdate(f.ctx, chat, f.people[author], grant.ID, nil, &enabled, grant.Revision)
			if err != nil {
				t.Fatal(err)
			}
			return changed
		}
		direct := f.invite(chat, "alice", "agent", f.bots["shared"], authz.Contributor)
		active("eng", true)
		active("shared", true)
		engineering = update("alice", engineering, false)
		active("eng", false)
		active("shared", true)
		if err := f.store.SystemGroupAgentUpdate(f.ctx, f.workspace.Id, f.groups["finance"], f.bots["shared"], false, 1); err != nil {
			t.Fatal(err)
		}
		active("shared", true) // Direct grant survives the last group path.
		direct = update("alice", direct, false)
		active("shared", false)
		if err := f.store.SystemGroupAgentUpdate(f.ctx, f.workspace.Id, f.groups["finance"], f.bots["shared"], true, 2); err != nil {
			t.Fatal(err)
		}
		active("shared", true)
		finance = update("alice", finance, false)
		active("shared", false)
		enabled := true
		if err, _ := f.store.SessionParticipantGrantUpdate(f.ctx, chat, f.people["alice"], finance.ID, nil, &enabled, finance.Revision); !errors.Is(err, database.ErrSessionParticipantNotFound) {
			t.Fatalf("nonmember restored a group grant: %v", err)
		}
		if err, _ := f.store.SessionParticipantGrantCreate(f.ctx, chat, f.people["alice"], nil, stringPointer(f.groups["finance"]), authz.Contributor); !errors.Is(err, database.ErrSessionParticipantNotFound) {
			t.Fatalf("nonmember restored a group through POST: %v", err)
		}
		finance = update("bridge", finance, true)
		active("shared", true)
		engineering = update("alice", engineering, true)
		active("eng", true)
		disabled := false
		if err, _ := f.store.SystemGroupUpdate(f.ctx, f.workspace.Id, f.groups["engineering"], 1, database.SystemGroupInput{Enabled: &disabled}); err != nil {
			t.Fatal(err)
		}
		active("eng", false)
		active("shared", true) // Finance still supplies a path.
		if _, err := f.store.Exec(`UPDATE gatehouse_workspace_agents SET enabled = FALSE WHERE workspace_id = `+f.p(1)+` AND id = `+f.p(2), f.workspace.Id, f.bots["shared"]); err != nil {
			t.Fatal(err)
		}
		active("shared", true)
		available("shared", false)
		if _, err := f.store.Exec(`UPDATE gatehouse_workspace_agents SET enabled = TRUE WHERE workspace_id = `+f.p(1)+` AND id = `+f.p(2), f.workspace.Id, f.bots["shared"]); err != nil {
			t.Fatal(err)
		}
		active("shared", true)
		available("shared", true)
		for _, resource := range []struct{ table, selector string }{
			{"gatehouse_agent_models", `id = (SELECT model_id FROM gatehouse_workspace_agents WHERE id = ` + f.p(1) + `)`},
			{"gatehouse_agent_providers", `id = (SELECT models.provider_id FROM gatehouse_agent_models models JOIN gatehouse_workspace_agents bots ON bots.model_id = models.id WHERE bots.id = ` + f.p(1) + `)`},
		} {
			if _, err := f.store.Exec(`UPDATE `+resource.table+` SET enabled = FALSE WHERE `+resource.selector, f.bots["shared"]); err != nil {
				t.Fatal(err)
			}
			active("shared", true)
			available("shared", false)
			if _, err := f.store.Exec(`UPDATE `+resource.table+` SET enabled = TRUE WHERE `+resource.selector, f.bots["shared"]); err != nil {
				t.Fatal(err)
			}
			active("shared", true)
			available("shared", true)
		}
		if err, _ := f.store.SystemGroupUpdate(f.ctx, f.workspace.Id, f.groups["engineering"], 2, database.SystemGroupInput{Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
		engDirect := f.invite(chat, "alice", "agent", f.bots["eng"], authz.Contributor)
		if err := f.store.SystemGroupAgentUpdate(f.ctx, f.workspace.Id, f.groups["engineering"], f.bots["eng"], false, 1); err != nil {
			t.Fatal(err)
		}
		active("eng", true)
		engDirect = update("alice", engDirect, false)
		active("eng", false)
		if err := f.store.SystemGroupMemberUpdate(f.ctx, f.workspace.Id, f.groups["engineering"], f.people["alice"].Id, false, 1); err != nil {
			t.Fatal(err)
		}
		if err, _ := f.store.SessionParticipantGrantUpdate(f.ctx, chat, f.people["alice"], engDirect.ID, nil, &enabled, engDirect.Revision); !errors.Is(err, database.ErrSessionParticipantNotFound) {
			t.Fatalf("revoked actor restored a private bot: %v", err)
		}
		if err, _ := f.store.SessionParticipantGrantCreateSubject(f.ctx, chat, f.people["alice"], nil, nil, stringPointer(f.bots["eng"]), authz.Contributor); !errors.Is(err, database.ErrSessionParticipantNotFound) {
			t.Fatalf("revoked actor restored private bot through POST: %v", err)
		}
		person := f.invite(chat, "bridge", "principal", f.people["bob"].Id, authz.Contributor)
		person = update("alice", person, false)
		if err, _ := f.store.SessionParticipantGrantUpdate(f.ctx, chat, f.people["alice"], person.ID, nil, &enabled, person.Revision); !errors.Is(err, database.ErrSessionParticipantNotFound) {
			t.Fatalf("co-participation restored a new direct person grant: %v", err)
		}
		update("bridge", person, true)
		general := f.invite(chat, "alice", "agent", f.bots["general"], authz.Contributor)
		general = update("alice", general, false)
		update("alice", general, true) // Default bots remain eligible without a group.
	})
}
