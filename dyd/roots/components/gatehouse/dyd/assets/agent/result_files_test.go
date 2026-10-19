package agent_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"gatehouse/agent"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/lisp"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/storage"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

type resultFileFixture struct {
	store     *database.Store
	runtime   *agent.SessionEventReplyRuntime
	storage   *storage.Client
	ctx       dbos.Context
	session   model.SessionRef
	principal model.PrincipalRef
	agent     model.WorkspaceAgentRef
	parent    model.SessionEventRef
	next      int
}

func testToolResultOutput(t *testing.T, ctx context.Context, store *database.Store, ring *keychain.Keyring, event model.SessionEvent, principal model.PrincipalRef) string {
	t.Helper()
	if output, ok := event.Payload["output"].(string); ok {
		return output
	}
	ref, present, err := database.SessionEventOutputFile(event)
	if err != nil || !present {
		t.Fatalf("result has no valid body: %#v %v", event, err)
	}
	err, file, object := store.SessionFileGet(ctx, ref, principal)
	if err != nil || file == nil || object == nil {
		t.Fatalf("result file unavailable: %v", err)
	}
	err, reader := storage.NewClient(store, ring).Get(ctx, object.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func newResultFileFixture(t *testing.T, cutoff int, endpoint string) *resultFileFixture {
	t.Helper()
	return newResultFileFixtureWithDatabase(t, cutoff, endpoint, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
}

func newResultFileFixtureWithDatabase(t *testing.T, cutoff int, endpoint string, configuration config.DatabaseConfig) *resultFileFixture {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	t.Setenv("GATEHOUSE_RESULT_TEST_KEY", "test-passphrase")
	t.Setenv("GATEHOUSE_RESULT_TEST_SECRET", "test-secret")
	keys := []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_RESULT_TEST_KEY"}}}
	err, ring := keychain.NewKeyring(store, keys, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ring.Close() })
	state := config.State{
		Keychains:       keys,
		Workspaces:      []config.Workspace{{Alias: "test", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "test", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"done"}`, ResultInlineBytes: cutoff, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "test", Alias: "test", Revision: 1, ModelAlias: "test", Default: true, Enabled: true}},
	}
	if endpoint != "" {
		region, bucket, access := "us-east-1", "results", "test-access"
		state.StorageProviders = []config.StorageProvider{
			{Alias: "external", Revision: 1, Protocol: "s3", Endpoint: &endpoint, Region: &region, Bucket: &bucket, AccessKeyID: &access, SecretKeySources: []config.StorageProviderSecretKeySource{"env:GATEHOUSE_RESULT_TEST_SECRET"}, Enabled: true},
			{Alias: "local", Revision: 1, Protocol: "embedded", Enabled: true},
		}
		state.WorkspaceStorageProviders = []config.WorkspaceStorageProvider{
			{WorkspaceID: "test", ProviderAlias: "external", Revision: 1, Priority: 10, Enabled: true},
			{WorkspaceID: "test", ProviderAlias: "local", Revision: 1, Priority: 1, Enabled: true},
		}
	}
	err, set := migrations.Build(configuration, state, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		t.Fatal(err)
	}
	f := &resultFileFixture{store: store, session: model.SessionRef{Id: "ses_00000000000000000000000000"}, next: 2}
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspaces WHERE alias = 'test'`).Scan(&f.session.Workspace.Id); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'alice'`).Scan(&f.principal.Id); err != nil {
		t.Fatal(err)
	}
	f.agent.Workspace = f.session.Workspace
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'test'`).Scan(&f.agent.Id); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: f.session, AuthorPrincipal: &f.principal, Enabled: true}, f.principal); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionParticipantGrantCreateSubject(ctx, f.session, f.principal, nil, nil, &f.agent.Id, authz.Contributor); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: f.session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: f.principal, Enabled: true}, Payload: map[string]interface{}{"text": "test"}}
	if err, _ := store.SessionEventsCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	f.parent = model.SessionEventRef{Session: f.session, Id: "sev_00000000000000000000000001"}
	if err, _ := store.SessionEventsCreate(ctx, model.SessionEvent{Ref: f.parent, Parent: &message.Ref, Kind: "agent.request", AuthorPrincipal: message.AuthorPrincipal, Payload: map[string]interface{}{"agent": f.agent.Id}}); err != nil {
		t.Fatal(err)
	}
	f.ctx, err = dbos.NewContext(ctx, dbos.Config{AppName: "result-files-test", ApplicationVersion: "results-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	err, f.runtime = agent.NewSessionEventReplyRuntime(f.ctx, store, ring)
	if err != nil {
		t.Fatal(err)
	}
	f.storage = storage.NewClient(store, ring)
	return f
}

func (f *resultFileFixture) request(t *testing.T, kind, code string, reasons ...string) model.SessionEvent {
	t.Helper()
	f.next++
	event := model.SessionEvent{Ref: model.SessionEventRef{Session: f.session, Id: fmt.Sprintf("sev_%026d", f.next)}, Parent: &f.parent, Kind: kind, AuthorAgent: &f.agent}
	if kind == "tool.request" {
		reason := "test result"
		if len(reasons) > 0 {
			reason = reasons[0]
		}
		event.Payload = map[string]interface{}{"name": "lisp", "call_id": "call-" + event.Ref.Id, "code": code, "reason": reason}
	} else {
		event.Payload = map[string]interface{}{"turn": 0}
	}
	err, stored := f.store.SessionEventsCreate(f.ctx, event)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestLargeResultPublicationAndRetrieval(t *testing.T) {
	f := newResultFileFixture(t, 0, "") // Default cutoff; no configured storage or bindings.
	work := agent.ResultFileTestToolCall(f.runtime)
	dbos.RegisterWorkflow(f.ctx, work, dbos.WithWorkflowName("test.result-tool"))
	if err := dbos.Launch(f.ctx); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(f.ctx, time.Second)
	for _, test := range []struct {
		name, code, reason string
		spilled            bool
	}{
		{"empty", `""`, "", false},
		{"below", strconv.Quote(strings.Repeat("x", 4093)), "", false},
		{"at", strconv.Quote(strings.Repeat("x", 4094)), "", false},
		{"above", strconv.Quote(strings.Repeat("x", 4095)), "Retrieve the complete repository list from the downloaded API response for analysis", true},
		{"long-reason", strconv.Quote(strings.Repeat("x", 4095)), strings.Repeat("a", 200), true},
		{"utf8", strconv.Quote(strings.Repeat("é", 2048)), "", true},
		{"failure", `(error/throw ` + strconv.Quote(strings.Repeat("diagnostic", 1000)) + `)`, "", true},
		{"redacted", `(taint/secret/mark ` + strconv.Quote(strings.Repeat("secret", 1000)) + `)`, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := f.request(t, "tool.request", test.code, test.reason)
			evalErr, value := lisp.Evaluate(test.code, lisp.EvalOptions{})
			want := ""
			if evalErr != nil {
				want = evalErr.Error()
			} else {
				want = value.String()
			}
			handle, err := dbos.RunWorkflow(f.ctx, work, request.Ref, dbos.WithWorkflowID("result:"+request.Ref.Id))
			if err != nil {
				t.Fatal(err)
			}
			ref, err := handle.GetResult()
			if err != nil {
				t.Fatal(err)
			}
			err, event := f.store.SessionEventGet(f.ctx, ref)
			if err != nil || event == nil {
				t.Fatalf("result event: %v", err)
			}
			fileRef, present, err := database.SessionEventOutputFile(*event)
			if err != nil || present != test.spilled {
				t.Fatalf("spilled=%v, want %v: %v %#v", present, test.spilled, err, event.Payload)
			}
			if !present {
				if event.Payload["output"] != want {
					t.Fatalf("inline output was changed: %#v", event.Payload)
				}
				return
			}
			if _, exists := event.Payload["output"]; exists {
				t.Fatal("file-backed event retained full output")
			}
			err, file, object := f.store.SessionFileGet(f.ctx, fileRef, f.principal)
			if err != nil || file == nil || object == nil || file.AuthorAgent == nil || *file.AuthorAgent != f.agent || file.AuthorPrincipal != nil || object.State != "success" {
				t.Fatalf("published result file: %#v %#v %v", file, object, err)
			}
			if test.name == "above" && file.Name != request.Ref.Id+"-retrieve-the-complete-repository-list-from-the-downloaded-api-response-for-analysis.txt" {
				t.Fatalf("result file name = %q, want full event ID followed by reason", file.Name)
			}
			if test.name == "long-reason" {
				wantName := request.Ref.Id + "-" + strings.Repeat("a", 128-len(request.Ref.Id)-5) + ".txt"
				if file.Name != wantName {
					t.Fatalf("long-reason file name = %q, want %q", file.Name, wantName)
				}
			}
			if len(file.Name) > 128 {
				t.Fatalf("result file name has %d characters; limit is 128", len(file.Name))
			}
			err, reader := f.storage.Get(f.ctx, object.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			contents, err := io.ReadAll(reader)
			reader.Close()
			if err != nil || string(contents) != want {
				t.Fatal("stored bytes differ from rendered result")
			}
			err, hydrated := f.store.SessionEventOutputFilesHydrate(f.ctx, []model.SessionEvent{*event})
			if err != nil {
				t.Fatal(err)
			}
			status := "success"
			if event.Kind == "tool.failure" {
				status = "failure"
			}
			rendered := agent.ResultFileTestTranscript(hydrated[0], status)
			if !strings.Contains(rendered, `<output-file id="`+fileRef.Id+`"`) || !strings.Contains(rendered, `size-bytes="`+strconv.Itoa(len(want))+`"`) || strings.Contains(rendered, want) {
				t.Fatalf("context must contain a compact file notice: %s", rendered)
			}
			err, attachments := f.store.SessionFilesByAuthorGet(f.ctx, f.session, "principal")
			if err != nil || len(attachments) != 0 {
				t.Fatal("result artifacts leaked into attachment listing")
			}
			if err, files := f.store.SessionFilesByIDs(f.ctx, f.session, []string{fileRef.Id}); err != nil || len(files) != 1 {
				t.Fatal("explicit result attachment was unavailable")
			}
		})
	}
	if err, files := f.store.SessionFilesByAuthorGet(f.ctx, f.session, "agent"); err != nil || len(files) != 4 {
		t.Fatalf("result listing: %#v %v", files, err)
	}
}

func TestResultProviderWriteFailureDoesNotFailOver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "write rejected", http.StatusForbidden) }))
	defer server.Close()
	f := newResultFileFixture(t, 4096, server.URL)
	work := agent.ResultFileTestToolCall(f.runtime)
	dbos.RegisterWorkflow(f.ctx, work, dbos.WithWorkflowName("test.result-write-failure"))
	if err := dbos.Launch(f.ctx); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(f.ctx, time.Second)
	request := f.request(t, "tool.request", strconv.Quote(strings.Repeat("large-body", 1000)))
	handle, err := dbos.RunWorkflow(f.ctx, work, request.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.GetResult(); err == nil {
		t.Fatal("storage write failure became a successful workflow")
	}
	if err, success := f.store.SessionEventChildGet(f.ctx, request.Ref, "tool.success"); err != nil || success != nil {
		t.Fatal("failed storage published a success event")
	}
	var protocol string
	var chunks int
	if err := f.store.QueryRow(`SELECT providers.protocol FROM gatehouse_storage_objects AS objects JOIN gatehouse_storage_providers AS providers ON providers.id = objects.provider JOIN gatehouse_session_files AS files ON files.storage_object = objects.id WHERE files.workspace = ? AND files.session = ? AND files.name LIKE ?`, f.session.Workspace.Id, f.session.Id, request.Ref.Id+"-%").Scan(&protocol); err != nil || protocol != "s3" {
		t.Fatalf("provider selection changed after failure: %q %v", protocol, err)
	}
	if err := f.store.QueryRow(`SELECT count(*) FROM gatehouse_embedded_storage_object_chunks`).Scan(&chunks); err != nil || chunks != 0 {
		t.Fatal("provider failure wrote an embedded fallback")
	}
	err, failed := f.store.SessionEventChildGet(f.ctx, request.Ref, "tool.failure")
	if err != nil || failed == nil || len(failed.Payload["output"].(string)) > 256 || strings.Contains(failed.Payload["output"].(string), "large-body") {
		t.Fatalf("storage failure retained large original output: %#v %v", failed, err)
	}
}

func TestResultPublicationLeavesUnreferencedUpload(t *testing.T) {
	f := newResultFileFixture(t, 32, "")
	body := strconv.Quote(strings.Repeat("new", 100))
	orphanBody := strconv.Quote(strings.Repeat("old", 100))
	request := f.request(t, "tool.request", body)
	err, file, object := f.store.SessionResultFileCreate(f.ctx, request.Ref, f.principal)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.storage.Put(f.ctx, object.ID, strings.NewReader(orphanBody), int64(len(orphanBody))); err != nil {
		t.Fatal(err)
	}
	if err := f.storage.Finish(f.ctx, object.ID); err != nil {
		t.Fatal(err)
	}
	// Simulate a completed upload whose terminal event transaction never committed.
	work := agent.ResultFileTestToolCall(f.runtime)
	dbos.RegisterWorkflow(f.ctx, work, dbos.WithWorkflowName("test.result-publication-recovery"))
	if err := dbos.Launch(f.ctx); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(f.ctx, time.Second)
	handle, err := dbos.RunWorkflow(f.ctx, work, request.Ref, dbos.WithWorkflowID("result-recovery"))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := handle.GetResult()
	if err != nil {
		t.Fatal(err)
	}
	err, event := f.store.SessionEventGet(f.ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	got, present, err := database.SessionEventOutputFile(*event)
	if err != nil || !present || got == file.Ref {
		t.Fatalf("publication reused an unreferenced upload: %#v %v", got, err)
	}
	err, published, _ := f.store.SessionFileGet(f.ctx, got, f.principal)
	if err != nil || published == nil {
		t.Fatalf("published file: %v", err)
	}
	err, reader := f.storage.Get(f.ctx, published.StorageObject.Id, 0)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(contents) != body {
		t.Fatal("published file contains the old attempt's body")
	}
	var count int
	if err := f.store.QueryRow(`SELECT count(*) FROM gatehouse_session_files`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("expected orphan plus published file: %d %v", count, err)
	}
}

func TestLargeProviderDiagnosticsUseResultFile(t *testing.T) {
	f := newResultFileFixture(t, 4096, "")
	diagnostic := "provider returned HTTP 400: " + strings.Repeat("diagnostic", 1000)
	request := f.request(t, "thinking.request", "")
	work := agent.ResultFileTestDiagnostic(f.runtime, diagnostic)
	dbos.RegisterWorkflow(f.ctx, work, dbos.WithWorkflowName("test.result-provider-diagnostic"))
	if err := dbos.Launch(f.ctx); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(f.ctx, time.Second)
	handle, err := dbos.RunWorkflow(f.ctx, work, request.Ref)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := handle.GetResult()
	if err != nil {
		t.Fatal(err)
	}
	err, event := f.store.SessionEventGet(f.ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	fileRef, present, err := database.SessionEventOutputFile(*event)
	if err != nil || !present || event.Kind != "thinking.failure" || len(event.Payload["message"].(string)) > 256 {
		t.Fatalf("provider diagnostic event: %#v %v", event, err)
	}
	err, file, _ := f.store.SessionFileGet(f.ctx, fileRef, f.principal)
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != request.Ref.Id+"-provider-diagnostic.txt" {
		t.Fatalf("provider result file name = %q, want full event ID followed by diagnostic label", file.Name)
	}
	err, reader := f.storage.Get(f.ctx, file.StorageObject.Id, 0)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(contents) != diagnostic {
		t.Fatal("full provider diagnostic was lost")
	}
}

func TestResultReferenceRequiresPublishedSameSessionFile(t *testing.T) {
	f := newResultFileFixture(t, 4096, "")
	request := f.request(t, "tool.request", `"test"`)
	err, file, object := f.store.SessionResultFileCreate(f.ctx, request.Ref, f.principal)
	if err != nil {
		t.Fatal(err)
	}
	newEvent := func(output *string, ref *model.SessionFileRef) model.SessionEvent {
		payload, err := database.SessionEventPayloadFrom(model.ToolSuccessPayload{Output: output, OutputFile: ref})
		if err != nil {
			t.Fatal(err)
		}
		return model.SessionEvent{Ref: model.SessionEventRef{Session: f.session, Id: "sev_00000000000000000000000020"}, Parent: &request.Ref, Kind: "tool.success", AuthorAgent: &f.agent, Payload: payload}
	}
	if err, _ := f.store.SessionEventsCreate(f.ctx, newEvent(nil, &file.Ref)); err == nil {
		t.Fatal("pending artifact was referenced by a committed event")
	}
	if err := f.storage.Put(f.ctx, object.ID, strings.NewReader("test"), 4); err != nil {
		t.Fatal(err)
	}
	if err := f.storage.Finish(f.ctx, object.ID); err != nil {
		t.Fatal(err)
	}
	output := "test"
	if err, _ := f.store.SessionEventsCreate(f.ctx, newEvent(&output, &file.Ref)); err == nil {
		t.Fatal("result retained inline output and file reference")
	}
	if err, _ := f.store.SessionEventsCreate(f.ctx, newEvent(nil, nil)); err == nil {
		t.Fatal("result omitted both output forms")
	}
	wrong := file.Ref
	wrong.Session.Id = "ses_00000000000000000000000001"
	if err, _ := f.store.SessionEventsCreate(f.ctx, newEvent(nil, &wrong)); err == nil {
		t.Fatal("result referenced another session's file")
	}
	event := newEvent(nil, &file.Ref)
	if err, _ := f.store.SessionEventsCreate(f.ctx, event); err != nil {
		t.Fatal(err)
	}
	if err, removed := f.store.SessionFileRemove(f.ctx, file.Ref, f.principal); err != nil || !removed {
		t.Fatalf("remove result file: %v", err)
	}
	if err, projected := f.store.SessionEventOutputFilesHydrate(f.ctx, []model.SessionEvent{event}); err != nil || len(projected) != 1 {
		t.Fatalf("removing result content broke event rendering: %v", err)
	}
}

func TestEventSearchRangesReadTheSameResultRepresentation(t *testing.T) {
	f := newResultFileFixture(t, 4096, "")
	module := agent.ResultFileTestEvents(f.runtime, f.ctx, f.parent, f.agent, f.principal)
	for _, kind := range []string{model.SessionEventKindToolSuccess, model.SessionEventKindToolFailure} {
		for _, spilled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/spilled=%t", kind, spilled), func(t *testing.T) {
				request := f.request(t, model.SessionEventKindToolRequest, `"unused"`)
				term := "inline-search-needle-" + request.Ref.Id
				payload := map[string]interface{}{"output": "Unicode prefix: é ☃ " + term}
				var file *model.SessionFile
				if spilled {
					err, created, object := f.store.SessionResultFileCreate(f.ctx, request.Ref, f.principal)
					if err != nil {
						t.Fatal(err)
					}
					file = created
					body := strings.Repeat("body-only-text ", 500)
					if err := f.storage.Put(f.ctx, object.ID, strings.NewReader(body), int64(len(body))); err != nil {
						t.Fatal(err)
					}
					if err := f.storage.Finish(f.ctx, object.ID); err != nil {
						t.Fatal(err)
					}
					payload, err = database.SessionEventPayloadFrom(model.ToolSuccessPayload{OutputFile: &file.Ref})
					if err != nil {
						t.Fatal(err)
					}
					term = file.Ref.Id
				}
				if kind == model.SessionEventKindToolFailure {
					payload["code"], payload["message"] = "execution_failed", "concise failure summary"
				}
				id, err := typed_id.New(typed_id.SessionEvent)
				if err != nil {
					t.Fatal(err)
				}
				event := model.SessionEvent{Ref: model.SessionEventRef{Session: f.session, Id: id}, Parent: &request.Ref, Kind: kind, AuthorAgent: &f.agent, Payload: payload}
				if err, _ := f.store.SessionEventsCreate(f.ctx, event); err != nil {
					t.Fatal(err)
				}
				check := func() {
					t.Helper()
					err, result := lisp.Evaluate(`(import (events @native:gatehouse/session/events/v1)
					  (let ((hits (list/assoc/require 'events (events/search query))))
					    (assert (= (list/length hits) 1))
					    (let ((hit (list/first hits)))
					      (assert (= (list/assoc/require 'id hit) expected-id))
					      (let ((range (list/first (list/assoc/require 'matches hit))))
					        (bytes/utf8/decode (events/read expected-id
					          (list/assoc/require 'offset range)
					          (list/assoc/require 'length range)))))))`, lisp.EvalOptions{
						HostModules: []lisp.HostModule{module},
						Bindings:    []lisp.Binding{{Name: "query", Value: lisp.String(strconv.Quote(term))}, {Name: "expected-id", Value: lisp.String(event.Ref.Id)}},
					})
					if err != nil {
						t.Fatal(err)
					}
					err, text := lisp.RequireString(result)
					if err != nil || text != term {
						t.Fatalf("search/read mismatch: %q, want %q (%v)", text, term, err)
					}
				}
				check()
				if spilled {
					if err, removed := f.store.SessionFileRemove(f.ctx, file.Ref, f.principal); err != nil || !removed {
						t.Fatalf("remove body file: %v", err)
					}
					check() // Reading the event's reference does not require the body file.
				}
			})
		}
	}
}
