package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/internal/testdbos"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

func TestReplyCooperativelyObservesPersistedCancellation(t *testing.T) {
	for _, algorithm := range []string{"mcmtr", "tadw"} {
		t.Run(algorithm, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				liveProviderReply(w, "openai-responses", 0)
			}))
			defer server.Close()
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			configuration := defaultAgentModelCompaction
			if algorithm == "tadw" {
				configuration = config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: 8192, HistoryMaxBytes: 4096, HistoryMinBytes: 1024}}
			}
			f := newTADWLiveFixture(t, filepath.Join(t.TempDir(), "cooperative.sqlite"), server.URL, "openai-responses", configuration)
			f.submit(t, true)
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("provider was not reached")
			}
			err, request := f.store.SessionEventChildGet(context.Background(), f.message.Ref, model.SessionEventKindAgentRequest)
			if err != nil || request == nil {
				t.Fatal(request, err)
			}
			id, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				t.Fatal(err)
			}
			// Observe a committed request before engine cancellation is delivered.
			if err, _ := f.store.SessionEventsCreate(context.Background(), model.SessionEvent{Ref: model.SessionEventRef{Session: request.Ref.Session, Id: id}, Parent: &request.Ref, Kind: model.SessionEventKindCancelRequest, AuthorPrincipal: f.message.AuthorPrincipal, Payload: map[string]any{}}); err != nil {
				t.Fatal(err)
			}
			unblock()
			handle, err := dbos.RetrieveWorkflow[model.SessionEventRef](f.ctx, "session-event-reply:"+request.Ref.Id)
			if err != nil {
				t.Fatal(err)
			}
			result, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(10*time.Second))
			if err != nil || result.Id != "" {
				t.Fatal("cooperative cancellation must finish without a reply or workflow failure", result, err)
			}
			for _, kind := range []string{model.SessionEventKindAgentSuccess, model.SessionEventKindAgentFailure} {
				if err, event := f.store.SessionEventChildGet(context.Background(), request.Ref, kind); err != nil || event != nil {
					t.Fatal("cancellation finalization belongs to its owning workflow", kind, event, err)
				}
			}
		})
	}
}
