package database_test

import (
	"context"
	"errors"
	"testing"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestSessionParticipantsPreserveExplicitPeopleAndGroups(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}, {Alias: "elsewhere", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Enabled: true}, {Alias: "bob", Enabled: true}, {Alias: "outsider", Enabled: true}, {Alias: "disabled", Enabled: false}},
		Groups: []config.Group{
			{WorkspaceID: "engineering", Alias: "developers", Enabled: true, Members: []config.GroupMember{{PrincipalID: "alice", Enabled: true}, {PrincipalID: "bob", Enabled: true}, {PrincipalID: "disabled", Enabled: false}}},
			{WorkspaceID: "elsewhere", Alias: "other", Enabled: true},
		},
	}
	state.WorkspaceGrants = []config.WorkspaceGrant{{WorkspaceID: "engineering", PrincipalID: stringPointer("alice"), Role: "guest", Enabled: true, Revision: 1}, {WorkspaceID: "engineering", PrincipalID: stringPointer("bob"), Role: "guest", Enabled: true, Revision: 1}}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	alice, bob := principalRef(t, ctx, store, "alice"), principalRef(t, ctx, store, "bob")
	outsider := principalRef(t, ctx, store, "outsider")
	group := groupID(t, ctx, store, "engineering", "developers")
	otherGroup := groupID(t, ctx, store, "elsewhere", "other")
	id, _ := typed_id.New(typed_id.Session)
	session := model.SessionRef{Workspace: workspace, Id: id}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	search := database.SessionParticipantSearch{Kind: "principal", Limit: 50}
	err, options, _ := store.SessionParticipantsSearch(ctx, session, alice, search, true)
	if err != nil || len(options) != 2 {
		t.Fatalf("workspace people options = %#v, %v", options, err)
	}
	if err, _ := store.SessionParticipantGrantCreate(ctx, session, alice, &outsider.Id, nil, authz.Guest); !errors.Is(err, database.ErrSessionParticipantNotFound) {
		t.Fatalf("outsider = %v", err)
	}
	if err, _ := store.SessionParticipantGrantCreate(ctx, session, alice, nil, &otherGroup, authz.Guest); !errors.Is(err, database.ErrSessionParticipantNotFound) {
		t.Fatalf("cross-workspace group = %v", err)
	}
	err, direct := store.SessionParticipantGrantCreate(ctx, session, alice, &bob.Id, nil, authz.Member)
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionParticipantGrantCreate(ctx, session, alice, &bob.Id, nil, authz.Member); !errors.Is(err, database.ErrSessionGrantExists) {
		t.Fatalf("duplicate = %v", err)
	}
	if err, _ := store.SessionParticipantGrantCreate(ctx, session, bob, nil, &group, authz.Guest); !errors.Is(err, database.ErrSessionParticipantForbidden) {
		t.Fatalf("contributor mutation = %v", err)
	}
	err, grouped := store.SessionParticipantGrantCreate(ctx, session, alice, nil, &group, authz.Guest)
	if err != nil {
		t.Fatal(err)
	}
	// Another direct role is a distinct grant, not a merged effective-access row.
	err, viewer := store.SessionParticipantGrantCreate(ctx, session, alice, &bob.Id, nil, authz.Guest)
	if err != nil {
		t.Fatal(err)
	}
	err, people, _ := store.SessionParticipantsSearch(ctx, session, alice, database.SessionParticipantSearch{Kind: "principal", Name: "bob", Limit: 1}, false)
	if err != nil || len(people) != 1 || len(people[0].Grants) != 2 {
		t.Fatalf("explicit bob roles = %#v, %v", people, err)
	}
	err, groups, _ := store.SessionParticipantsSearch(ctx, session, alice, database.SessionParticipantSearch{Kind: "group", Limit: 1}, false)
	if err != nil || len(groups) != 1 || groups[0].ID != group || groups[0].Grants[0].ID != grouped.ID {
		t.Fatalf("explicit groups = %#v, %v", groups, err)
	}
	search.Limit = 1
	seen := map[string]bool{}
	for {
		err, page, next := store.SessionParticipantsSearch(ctx, session, alice, search, false)
		if err != nil || len(page) != 1 || seen[page[0].ID] {
			t.Fatalf("page = %#v %s %v", page, next, err)
		}
		seen[page[0].ID] = true
		if next == "" {
			break
		}
		search.Cursor = next
	}
	if len(seen) != 2 {
		t.Fatalf("people count = %d", len(seen))
	}
	disabled := false
	if err, _ := store.SessionParticipantGrantUpdate(ctx, session, alice, direct.ID, nil, &disabled, 2); !errors.Is(err, database.ErrRevisionConflict) {
		t.Fatalf("stale = %v", err)
	}
	if err, _ := store.SessionParticipantGrantUpdate(ctx, session, alice, direct.ID, nil, &disabled, 1); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionParticipantGrantUpdate(ctx, session, alice, viewer.ID, nil, &disabled, 1); err != nil {
		t.Fatal(err)
	}
	if err, visible := store.SessionGet(ctx, session, bob); err != nil || visible == nil {
		t.Fatalf("group access after direct removal = %#v %v", visible, err)
	}
	err, restored := store.SessionParticipantGrantCreate(ctx, session, alice, &bob.Id, nil, authz.Member)
	if err != nil || restored.ID != direct.ID || restored.Revision != 3 {
		t.Fatalf("re-enable = %#v %v", restored, err)
	}
	err, all, _ := store.SessionParticipantsSearch(ctx, session, alice, database.SessionParticipantSearch{Kind: "principal", Limit: 50}, false)
	if err != nil {
		t.Fatal(err)
	}
	var manager database.SessionParticipantGrant
	for _, person := range all {
		if person.ID == alice.Id {
			manager = person.Grants[0]
		}
	}
	if err, _ := store.SessionParticipantGrantUpdate(ctx, session, alice, manager.ID, nil, &disabled, manager.Revision); !errors.Is(err, database.ErrSessionLastAdmin) {
		t.Fatalf("last manager = %v", err)
	}
	// Competing self-removals must not both succeed.
	err, secondManager := store.SessionParticipantGrantCreate(ctx, session, alice, &bob.Id, nil, authz.Admin)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, attempt := range []struct {
		actor model.PrincipalRef
		grant database.SessionParticipantGrant
	}{{alice, manager}, {bob, *secondManager}} {
		go func() {
			err, _ := store.SessionParticipantGrantUpdate(ctx, session, attempt.actor, attempt.grant.ID, nil, &disabled, attempt.grant.Revision)
			results <- err
		}()
	}
	succeeded, blocked := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			succeeded++
		} else if errors.Is(err, database.ErrSessionLastAdmin) {
			blocked++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || blocked != 1 {
		t.Fatalf("concurrent removals = %d success, %d blocked", succeeded, blocked)
	}
	var topics int
	if err := store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_event_topics WHERE topic = ?`, bob.Id+"/"+grouped.ID).Scan(&topics); err != nil || topics == 0 {
		t.Fatalf("group principal notifications = %d %v", topics, err)
	}
}
