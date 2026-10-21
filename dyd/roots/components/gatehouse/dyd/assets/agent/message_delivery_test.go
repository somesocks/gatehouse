package agent_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

func TestTransactionalMessageDelivery(t *testing.T) {
	testTransactionalDelivery(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func TestPostgresTransactionalMessageDelivery(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	testTransactionalDelivery(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
}

func testTransactionalDelivery(t *testing.T, configuration config.DatabaseConfig) {
	t.Run("message and all workflows commit together without a dispatcher pass", func(t *testing.T) {
		f := newDeliveryFixture(t, configuration)
		message := f.message(t, f.bots...)
		if err, _ := f.runtime.CreateMessage(context.Background(), message); err != nil {
			t.Fatal(err)
		}
		var queued, titles int
		if err := f.store.QueryRow(`SELECT COUNT(*) FROM ` + f.system + `workflow_status WHERE status = 'ENQUEUED'`).Scan(&queued); err != nil || queued != 3 {
			t.Fatalf("message did not commit two replies and one title: %d (%v)", queued, err)
		}
		// Repeating a title enqueue must not abort this second message's
		// transaction or create another title workflow.
		if err, _ := f.runtime.CreateMessage(context.Background(), f.message(t, f.bots...)); err != nil {
			t.Fatal(err)
		}
		if err := f.store.QueryRow(`SELECT COUNT(*) FROM ` + f.system + `workflow_status WHERE name = 'gatehouse.session-name'`).Scan(&titles); err != nil || titles != 1 {
			t.Fatalf("title workflow was not deduplicated: %d (%v)", titles, err)
		}
		if err, _ := f.runtime.CreateMessage(context.Background(), f.message(t)); err != nil {
			t.Fatal(err)
		}
		if err := f.store.QueryRow(`SELECT COUNT(*) FROM ` + f.system + `workflow_status`).Scan(&queued); err != nil || queued != 5 {
			t.Fatalf("human-only message scheduled work: %d (%v)", queued, err)
		}
		f.launch(t)
		err, events := f.store.SessionEventsGet(context.Background(), f.session)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Kind != model.SessionEventKindAgentRequest {
				continue
			}
			handle, err := dbos.RetrieveWorkflow[model.SessionEventRef](f.ctx, "session-event-reply:"+event.Ref.Id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := handle.GetResult(dbos.WithHandleTimeout(15 * time.Second)); err != nil {
				t.Fatalf("durably enqueued reply did not execute: %v", err)
			}
		}
	})
	t.Run("failed title enqueue rolls back message replies activity and invitations", func(t *testing.T) {
		f := newDeliveryFixture(t, configuration)
		if configuration.Kind == config.DatabaseKindPostgres {
			_, err := f.store.Exec(`CREATE FUNCTION ` + f.system + `reject_title() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
				IF NEW.name = 'gatehouse.session-name' THEN RAISE EXCEPTION 'reject title enqueue'; END IF;
				RETURN NEW; END $$;
				CREATE TRIGGER reject_title BEFORE INSERT ON ` + f.system + `workflow_status FOR EACH ROW EXECUTE FUNCTION ` + f.system + `reject_title()`)
			if err != nil {
				t.Fatal(err)
			}
		} else if _, err := f.store.Exec(`CREATE TRIGGER reject_title BEFORE INSERT ON workflow_status
			WHEN NEW.name = 'gatehouse.session-name' BEGIN SELECT RAISE(ABORT, 'reject title enqueue'); END`); err != nil {
			t.Fatal(err)
		}
		message := f.message(t, f.bots...)
		if err, _ := f.runtime.CreateMessage(context.Background(), message); err == nil {
			t.Fatal("failed workflow enqueue accepted the message")
		}
		for _, query := range []string{
			`SELECT COUNT(*) FROM gatehouse_session_events WHERE session = $1`,
			`SELECT COUNT(*) FROM gatehouse_session_grants WHERE session = $1 AND agent IS NOT NULL`,
		} {
			var count int
			if err := f.store.QueryRow(query, f.session.Id).Scan(&count); err != nil || count != 0 {
				t.Fatalf("failed delivery left application writes: %d (%v)", count, err)
			}
		}
		var count int
		if err := f.store.QueryRow(`SELECT COUNT(*) FROM ` + f.system + `workflow_status`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed delivery left DBOS enqueues: %d (%v)", count, err)
		}
		if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_session_event = $1`, message.Ref.Id).Scan(&count); err != nil || count != 0 {
			t.Fatalf("failed delivery left activity: %d (%v)", count, err)
		}
	})
	t.Run("approval delivery rolls back without a destination and commits exactly once", func(t *testing.T) {
		f := newDeliveryFixture(t, configuration)
		root := f.message(t)
		bot := model.WorkspaceAgentRef{Workspace: f.session.Workspace, Id: f.bots[0]}
		tool := model.SessionEvent{Ref: f.eventRef(t), Parent: &root.Ref, Kind: model.SessionEventKindToolRequest, AuthorAgent: &bot, Payload: map[string]interface{}{}}
		approval := model.SessionEvent{Ref: f.eventRef(t), Parent: &tool.Ref, Kind: model.SessionEventKindApprovalRequest, AuthorAgent: &bot, Payload: map[string]interface{}{}}
		if err, _ := f.store.SessionEventsCreateBatch(context.Background(), []model.SessionEvent{root, tool, approval}); err != nil {
			t.Fatal(err)
		}
		response := model.SessionEvent{Ref: f.eventRef(t), Parent: &approval.Ref, Kind: model.SessionEventKindApprovalSuccess, AuthorPrincipal: &f.principal, Payload: map[string]interface{}{}}
		if err, _ := f.runtime.RespondToApproval(context.Background(), response); err == nil {
			t.Fatal("accepted approval without a destination workflow")
		}
		if err, stored := f.store.SessionEventGet(context.Background(), response.Ref); err != nil || stored != nil {
			t.Fatalf("failed send persisted approval: %#v (%v)", stored, err)
		}
		dbos.RegisterWorkflow(f.ctx, deliveryApprovalWait, dbos.WithWorkflowName("test.delivery-approval"))
		f.launch(t)
		handle, err := dbos.RunWorkflow(f.ctx, deliveryApprovalWait, struct{}{}, dbos.WithWorkflowID("session-approval:"+approval.Ref.Id))
		if err != nil {
			t.Fatal(err)
		}
		other := response
		other.Ref = f.eventRef(t)
		var results [2]error
		var wait sync.WaitGroup
		for index, event := range []model.SessionEvent{response, other} {
			wait.Add(1)
			go func() {
				defer wait.Done()
				results[index], _ = f.runtime.RespondToApproval(context.Background(), event)
			}()
		}
		wait.Wait()
		if !((results[0] == nil && errors.Is(results[1], database.ErrSessionApprovalResolved)) || (results[1] == nil && errors.Is(results[0], database.ErrSessionApprovalResolved))) {
			t.Fatalf("concurrent approval outcomes = %v", results)
		}
		got, err := handle.GetResult(dbos.WithHandleTimeout(10 * time.Second))
		if err != nil || (got != response.Ref && got != other.Ref) {
			t.Fatalf("approval message was not delivered: %#v (%v)", got, err)
		}
		var count int
		if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_session_events WHERE parent = $1 AND kind IN ('approval.success', 'approval.failure')`, approval.Ref.Id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("approval terminal count = %d (%v)", count, err)
		}
	})
}

func deliveryApprovalWait(ctx dbos.Context, _ struct{}) (model.SessionEventRef, error) {
	return dbos.Recv[model.SessionEventRef](ctx, "response", time.Minute)
}

type deliveryFixture struct {
	store     *database.Store
	runtime   *agent.SessionEventReplyRuntime
	ctx       dbos.Context
	system    string
	session   model.SessionRef
	principal model.Principal
	bots      []string
}

func newDeliveryFixture(t *testing.T, configuration config.DatabaseConfig) *deliveryFixture {
	t.Helper()
	ctx := context.Background()
	err, store := database.OpenWithCrypto(ctx, configuration, config.Crypto{DefaultKDF: "pbkdf2-hmac-sha256-v0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	err, ring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ring.Close)
	id, err := typed_id.New(typed_id.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	alias := "delivery-" + id
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: alias, Enabled: true}},
		Principals:      []config.Principal{{Alias: alias, Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: alias, Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: alias, Revision: 1, ProviderAlias: alias, Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Delivered"}`, Compaction: defaultAgentModelCompaction, MaxTurns: 1, MaxOutputTokens: 1024, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: alias, Alias: "first", ModelAlias: alias, Revision: 1, Default: true, Enabled: true}, {WorkspaceID: alias, Alias: "second", ModelAlias: alias, Revision: 1, Default: true, Enabled: true}},
		WorkspaceGrants: []config.WorkspaceGrant{{WorkspaceID: alias, PrincipalID: &alias, Role: "contributor", Revision: 1, Enabled: true}},
	}
	err, registry := migrations.Build(configuration, state, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, registry); err != nil {
		t.Fatal(err)
	}
	err, workspace := store.WorkspaceRefGetByAlias(ctx, alias)
	if err != nil || workspace == nil {
		t.Fatalf("workspace = %#v (%v)", workspace, err)
	}
	f := &deliveryFixture{store: store}
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = $1`, alias).Scan(&f.principal.Ref.Id); err != nil {
		t.Fatal(err)
	}
	f.principal.Enabled = true
	for _, botAlias := range []string{"first", "second"} {
		var bot string
		if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id = $1 AND alias = $2`, workspace.Id, botAlias).Scan(&bot); err != nil {
			t.Fatal(err)
		}
		f.bots = append(f.bots, bot)
	}
	sessionID, err := typed_id.New(typed_id.Session)
	if err != nil {
		t.Fatal(err)
	}
	f.session = model.SessionRef{Workspace: *workspace, Id: sessionID}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: f.session, AuthorPrincipal: &f.principal.Ref, Enabled: true}, f.principal.Ref); err != nil {
		t.Fatal(err)
	}
	durableConfig := dbos.Config{AppName: alias, ApplicationVersion: "delivery-v1", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if configuration.Kind == config.DatabaseKindPostgres {
		durableConfig.SystemDBPool = store.PostgresPool()
		durableConfig.DatabaseSchema = "delivery_" + id
		f.system = durableConfig.DatabaseSchema + "."
	} else {
		durableConfig.SQLiteSystemDB = store.DB
	}
	f.ctx, err = dbos.NewContext(ctx, durableConfig)
	if err != nil {
		t.Fatal(err)
	}
	err, f.runtime = agent.NewSessionEventReplyRuntime(f.ctx, store, ring)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbos.Shutdown(f.ctx, 5*time.Second) })
	return f
}

func (f *deliveryFixture) launch(t *testing.T) {
	t.Helper()
	if err := dbos.Launch(f.ctx); err != nil {
		t.Fatal(err)
	}
}

func (f *deliveryFixture) eventRef(t *testing.T) model.SessionEventRef {
	t.Helper()
	id, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	return model.SessionEventRef{Session: f.session, Id: id}
}

func (f *deliveryFixture) message(t *testing.T, bots ...string) model.SessionEvent {
	return model.SessionEvent{Ref: f.eventRef(t), Kind: model.SessionEventKindMessageText, AuthorPrincipal: &f.principal, Payload: map[string]interface{}{"text": "Hello", "agents": bots}}
}
