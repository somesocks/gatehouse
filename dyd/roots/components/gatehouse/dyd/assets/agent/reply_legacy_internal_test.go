package agent

import (
	"context"
	"time"

	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/storage"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// Register a reference-only admission and the original round step sequence for
// recovery tests. The recovering process registers the normal runtime instead.
func RegisterLegacyReplyRoundForTest(root dbos.Context, store *database.Store, ring *keychain.Keyring) (func(model.SessionEventRef) error, error) {
	source, err := dbos.NewDataSource(root, store.DB, dbos.WithDataSourceName("gatehouse"))
	if err != nil {
		return nil, err
	}
	runtime := &SessionEventReplyRuntime{store: store, keyring: ring, storage: storage.NewClient(store, ring), dbos: root, dataSource: source, pollingInterval: 10 * time.Millisecond}
	dbos.RegisterWorkflow(root, runtime.legacyRoundForTest, dbos.WithInstance(runtime), dbos.WithWorkflowName("gatehouse.session-agent-round"))
	return func(ref model.SessionEventRef) error {
		input := SessionAgentRoundInput{AgentRequest: ref}
		_, err := dbos.RunWorkflow(root, runtime.legacyRoundForTest, input, dbos.WithRunInstance(runtime), dbos.WithWorkflowID(sessionAgentRoundWorkflowID(input)))
		return err
	}, nil
}

func (runtime *SessionEventReplyRuntime) legacyRoundForTest(ctx dbos.Context, input SessionAgentRoundInput) (model.SessionEventRef, error) {
	ref, err := dbos.RunAsTransaction(ctx, runtime.dataSource, func(step context.Context, tx dbos.Tx) (model.SessionEventRef, error) {
		admission, err := runtime.agentRoundAdmit(step, tx, input)
		return admission.SessionEventRef, err
	}, dbos.WithStepName("gatehouse.agent-round-prepare"))
	if err != nil {
		return model.SessionEventRef{}, err
	}
	return runtime.runAdmittedRound(ctx, input, ref, agentContextSelection{Algorithm: "mcmtr"}, nil, false)
}
