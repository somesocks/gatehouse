package agent

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"

	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// These locators contain no event lists, boundary decisions or rendered context.
// StepID is also a row offset only after capture verifies a dense committed prefix.
type tadwCheckpointRef struct {
	WorkflowID string
	StepID     int
}
type tadwHistoryHandoff struct {
	Request       model.SessionEventRef
	Anchor        model.SessionEventRef
	Checkpoint    tadwCheckpointRef
	Compatibility string
}
type tadwToolingHandoff struct {
	Request    model.SessionEventRef
	Thinking   model.SessionEventRef
	Checkpoint tadwCheckpointRef
}
type tadwRoundHandoff struct {
	History  tadwHistoryHandoff
	Previous *tadwToolingHandoff
}

// Worker-only result. Never return this from a DBOS workflow/step.
type tadwRoundContext struct {
	Prepared tadwPreparedHistory
	Window   tadwToolingWindow
	Provider agentProviderRequest
	Handoff  tadwToolingHandoff
	prepare  func(context.Context) error
	ready    bool
}

// Call inside the existing provider step callback, not before RunAsStep. A
// completed provider step then needs neither XML reconstruction nor assembly on
// replay. Sequential retries reuse the successful worker-local preparation.
func (round *tadwRoundContext) Prepare(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if round.ready {
		return nil
	}
	if err := round.prepare(ctx); err != nil {
		return err
	}
	round.ready = true
	return nil
}

type tadwHistoryMemoKey struct {
	History tadwHistoryHandoff
	Limits  tadwWindowConfig
}
type tadwHistoryMemoEntry struct {
	done     chan struct{}
	history  tadwInitialHistory
	err      error
	position *list.Element
}

// Worker memory management, not a transcript-retention setting. Eviction only
// causes exact-ID recovery; it cannot change the request's history selection.
const tadwHandoffWorkerHistoryEntries = 32

// One instance per worker, not per child. No production workflow constructs it
// yet. Local Release is eager cleanup only: remote workers cannot depend on the
// parent running there. The LRU bounds idle entries on every worker.
type tadwHandoffWorker struct {
	root      dbos.Context
	store     *database.Store
	mu        sync.Mutex
	histories map[tadwHistoryMemoKey]*tadwHistoryMemoEntry
	order     list.List
}

func newTADWHandoffWorker(root dbos.Context, store *database.Store) (*tadwHandoffWorker, error) {
	if root == nil || store == nil {
		return nil, fmt.Errorf("TADW handoff requires a worker DBOS context and store")
	}
	if _, err := dbos.GetWorkflowID(root); err == nil {
		return nil, fmt.Errorf("TADW handoff reader requires a non-workflow DBOS context")
	}
	return &tadwHandoffWorker{root: root, store: store, histories: make(map[tadwHistoryMemoKey]*tadwHistoryMemoEntry)}, nil
}

func tadwHistoryOwnerWorkflowID(request model.SessionEventRef) string {
	return "tadw-history-owner:" + request.Session.Workspace.Id + ":" + request.Session.Id + ":" + request.Id
}

// Preserve child cancellation/deadlines, but inherit values only from the root.
// Passing child values to From would cause GetWorkflowSteps to checkpoint its
// entire result in that child. The root is checked when the worker is created.
func (worker *tadwHandoffWorker) reader(ctx context.Context) (dbos.Context, func()) {
	base, cancel := context.WithCancel(worker.root)
	stop := context.AfterFunc(ctx, cancel)
	if deadline, ok := ctx.Deadline(); ok {
		timed, cancelDeadline := context.WithDeadline(base, deadline)
		return dbos.From(worker.root, timed), func() { stop(); cancelDeadline(); cancel() }
	}
	return dbos.From(worker.root, base), func() { stop(); cancel() }
}

func (worker *tadwHandoffWorker) capture(ctx dbos.Context, name string) (tadwCheckpointRef, error) {
	if err := ctx.Err(); err != nil {
		return tadwCheckpointRef{}, err
	}
	workflowID, err := dbos.GetWorkflowID(ctx)
	if err != nil {
		return tadwCheckpointRef{}, err
	}
	stepID, err := dbos.GetStepID(ctx)
	if err != nil {
		return tadwCheckpointRef{}, err
	}
	if stepID < 0 || stepID == math.MaxInt {
		return tadwCheckpointRef{}, fmt.Errorf("TADW checkpoint has no completed step")
	}
	reader, close := worker.reader(ctx)
	defer close()
	steps, err := dbos.GetWorkflowSteps(reader, workflowID, dbos.WithStepsLoadOutput(false), dbos.WithStepsLimit(stepID+1))
	if err := ctx.Err(); err != nil {
		return tadwCheckpointRef{}, err
	}
	if err != nil {
		return tadwCheckpointRef{}, err
	}
	// OFFSET addresses rows, not function IDs. Reject holes/pending earlier Go
	// steps rather than risk a future insertion moving this checkpoint's ordinal.
	if len(steps) != stepID+1 {
		return tadwCheckpointRef{}, fmt.Errorf("TADW checkpoint requires a dense committed step prefix")
	}
	for index, step := range steps {
		if step.StepID != index {
			return tadwCheckpointRef{}, fmt.Errorf("TADW checkpoint requires a dense committed step prefix")
		}
	}
	step := steps[stepID]
	if step.StepName != name || step.Error != nil {
		return tadwCheckpointRef{}, fmt.Errorf("TADW checkpoint step is unavailable or mismatched")
	}
	if err := ctx.Err(); err != nil {
		return tadwCheckpointRef{}, err
	}
	return tadwCheckpointRef{WorkflowID: workflowID, StepID: stepID}, nil
}

func (worker *tadwHandoffWorker) output(ctx context.Context, ref tadwCheckpointRef, name string) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ref.WorkflowID == "" || ref.StepID < 0 {
		return nil, fmt.Errorf("invalid TADW checkpoint reference")
	}
	reader, close := worker.reader(ctx)
	defer close()
	steps, err := dbos.GetWorkflowSteps(reader, ref.WorkflowID, dbos.WithStepsLoadOutput(true), dbos.WithStepsOffset(ref.StepID), dbos.WithStepsLimit(1))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if len(steps) != 1 || steps[0].StepID != ref.StepID || steps[0].StepName != name || steps[0].Error != nil {
		return nil, fmt.Errorf("TADW checkpoint is missing, failed or mismatched")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return steps[0].Output, nil
}

func cloneTADWInitialHistory(history tadwInitialHistory) tadwInitialHistory {
	history.Window.EventIDs = slices.Clone(history.Window.EventIDs)
	history.History = slices.Clone(history.History)
	return history
}

func (handoff tadwHistoryHandoff) validate() error {
	if handoff.Request.Session != handoff.Anchor.Session || handoff.Request.Session.Id == "" || handoff.Request.Session.Workspace.Id == "" || !typed_id.Valid(typed_id.SessionEvent, handoff.Request.Id) || !typed_id.Valid(typed_id.SessionEvent, handoff.Anchor.Id) || handoff.Request.Id == handoff.Anchor.Id || handoff.Checkpoint.StepID < 0 || handoff.Checkpoint.WorkflowID != tadwHistoryOwnerWorkflowID(handoff.Request) {
		return fmt.Errorf("invalid TADW history handoff")
	}
	return nil
}

// Called exactly once in a request's dedicated owner workflow. The workflow's
// durable result may be this handoff, not the prepared history. It can be a child
// of the reply coordinator, so its GetResult also checkpoints only references.
func (worker *tadwHandoffWorker) Prepare(ctx dbos.Context, request, anchor model.SessionEventRef, selected *database.WorkspaceAgentModel, projectChat bool, limits tadwWindowConfig) (tadwHistoryHandoff, error) {
	workflowID, err := dbos.GetWorkflowID(ctx)
	if err != nil {
		return tadwHistoryHandoff{}, err
	}
	handoff := tadwHistoryHandoff{Request: request, Anchor: anchor, Checkpoint: tadwCheckpointRef{WorkflowID: workflowID}}
	if err := handoff.validate(); err != nil {
		return tadwHistoryHandoff{}, err
	}
	if err := worker.validateRequest(ctx, handoff, selected); err != nil {
		return tadwHistoryHandoff{}, err
	}
	prepared, err := prepareTADWHistory(ctx, worker.store, anchor, selected, projectChat, limits)
	if err != nil {
		return tadwHistoryHandoff{}, err
	}
	handoff.Compatibility, err = tadwHandoffCompatibility(prepared.Agent, prepared.Scope, limits)
	if err != nil {
		return tadwHistoryHandoff{}, err
	}
	handoff.Checkpoint, err = worker.capture(ctx, tadwHistoryCheckpointName)
	if err != nil {
		return tadwHistoryHandoff{}, err
	}
	entry := &tadwHistoryMemoEntry{done: make(chan struct{}), history: cloneTADWInitialHistory(prepared.History)}
	close(entry.done)
	worker.mu.Lock()
	worker.remember(tadwHistoryMemoKey{History: handoff, Limits: limits}, entry)
	worker.mu.Unlock()
	return handoff, nil
}

func (worker *tadwHandoffWorker) validateRequest(ctx context.Context, handoff tadwHistoryHandoff, selected *database.WorkspaceAgentModel) error {
	if selected == nil || selected.Ref.Workspace != handoff.Request.Session.Workspace {
		return fmt.Errorf("invalid TADW handoff agent")
	}
	err, event := worker.store.SessionEventGet(ctx, handoff.Request)
	if err != nil {
		return err
	}
	if event == nil || event.Kind != model.SessionEventKindAgentRequest || event.Parent == nil || *event.Parent != handoff.Anchor || event.Payload["agent"] != selected.Ref.Id {
		return fmt.Errorf("TADW history handoff does not match its owning request")
	}
	return nil
}

func (worker *tadwHandoffWorker) load(ctx context.Context, handoff tadwHistoryHandoff, limits tadwWindowConfig) (tadwInitialHistory, error) {
	if err := handoff.validate(); err != nil {
		return tadwInitialHistory{}, err
	}
	if err := limits.validate(); err != nil {
		return tadwInitialHistory{}, err
	}
	if err := ctx.Err(); err != nil {
		return tadwInitialHistory{}, err
	}
	if len(handoff.Compatibility) != 64 {
		return tadwInitialHistory{}, fmt.Errorf("invalid TADW handoff compatibility")
	}
	key := tadwHistoryMemoKey{History: handoff, Limits: limits}
	var entry *tadwHistoryMemoEntry
	for {
		worker.mu.Lock()
		var exists bool
		entry, exists = worker.histories[key]
		if !exists {
			entry = &tadwHistoryMemoEntry{done: make(chan struct{})}
			worker.remember(key, entry)
		} else {
			worker.order.MoveToFront(entry.position)
		}
		worker.mu.Unlock()
		if !exists {
			break
		}
		select {
		case <-ctx.Done():
			return tadwInitialHistory{}, ctx.Err()
		case <-entry.done:
		}
		if err := ctx.Err(); err != nil {
			return tadwInitialHistory{}, err
		}
		// Cancellation belongs to the caller that owned the cold read, not to
		// another healthy child waiting for the same immutable history.
		if errors.Is(entry.err, context.Canceled) || errors.Is(entry.err, context.DeadlineExceeded) {
			continue
		}
		return cloneTADWInitialHistory(entry.history), entry.err
	}
	history, err := worker.restore(ctx, handoff, limits)
	worker.mu.Lock()
	entry.history, entry.err = cloneTADWInitialHistory(history), err
	if err != nil && worker.histories[key] == entry {
		delete(worker.histories, key)
		worker.order.Remove(entry.position)
	}
	close(entry.done)
	worker.mu.Unlock()
	return history, err
}

func (worker *tadwHandoffWorker) restore(ctx context.Context, handoff tadwHistoryHandoff, limits tadwWindowConfig) (tadwInitialHistory, error) {
	output, err := worker.output(ctx, handoff.Checkpoint, tadwHistoryCheckpointName)
	if err != nil {
		return tadwInitialHistory{}, err
	}
	var ids []string
	if output != nil {
		encoded, ok := output.(string)
		if !ok {
			return tadwInitialHistory{}, fmt.Errorf("TADW checkpoint reader requires DBOS JSON listing output")
		}
		if err := json.Unmarshal([]byte(encoded), &ids); err != nil {
			return tadwInitialHistory{}, fmt.Errorf("decode TADW history checkpoint: %w", err)
		}
	}
	return rehydrateTADWHistory(ctx, worker.store, handoff.Request.Session, ids, limits)
}

// Called once per thinking attempt, outside its provider-retry loop. Admission
// and cancellation checks belong to the surrounding child workflow. Previous is
// its immediate predecessor's reference, never a mutable "latest" watermark.
func (worker *tadwHandoffWorker) Round(ctx dbos.Context, input tadwRoundHandoff, thinking model.SessionEventRef, selected *database.WorkspaceAgentModel, projectChat bool, limits tadwWindowConfig) (*tadwRoundContext, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	if err := input.History.validate(); err != nil {
		return nil, err
	}
	if err := worker.validateRequest(ctx, input.History, selected); err != nil {
		return nil, err
	}
	if thinking.Session != input.History.Request.Session || !typed_id.Valid(typed_id.SessionEvent, thinking.Id) {
		return nil, fmt.Errorf("invalid TADW handoff thinking request")
	}
	err, current := worker.store.SessionEventGet(ctx, thinking)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Kind != model.SessionEventKindThinkingRequest || current.Parent == nil || *current.Parent != input.History.Request || current.AuthorAgent == nil || *current.AuthorAgent != selected.Ref {
		return nil, fmt.Errorf("TADW current thinking request is unavailable or mismatched")
	}
	var payload model.ThinkingRequestPayload
	if err := decodeRoundPayload(current, &payload); err != nil {
		return nil, err
	}
	workflowID, err := dbos.GetWorkflowID(ctx)
	if err != nil {
		return nil, err
	}
	if err := worker.admission(ctx, workflowID, thinking); err != nil {
		return nil, err
	}
	if (payload.Previous == nil) != (input.Previous == nil) {
		return nil, fmt.Errorf("TADW predecessor handoff does not match the thinking chain")
	}
	var freshHistory tadwInitialHistory
	var freshWindow *tadwToolingWindow
	// Unlike the eager standalone wrapper, replay only reads the decision here.
	// XML recovery belongs inside an actually executing provider step callback.
	boundary, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (string, error) {
		if _, err := input.History.compatible(selected, projectChat, limits); err != nil {
			return "", err
		}
		from, err := worker.previousBoundary(stepCtx, input, thinking, payload.Previous, selected)
		if err != nil {
			return "", err
		}
		history, err := worker.load(stepCtx, input.History, limits)
		if err != nil {
			return "", err
		}
		window, err := loadTADWToolingWindow(stepCtx, worker.store, history, input.History.Request, selected.Ref, thinking, limits, from)
		if err != nil {
			return "", err
		}
		freshHistory, freshWindow = history, &window
		return window.FromEventID, nil
	}, dbos.WithStepName(tadwToolingCheckpointName))
	if err != nil {
		return nil, fmt.Errorf("checkpoint TADW handoff tooling: %w", err)
	}
	checkpoint, err := worker.capture(ctx, tadwToolingCheckpointName)
	if err != nil {
		return nil, err
	}
	round := &tadwRoundContext{Handoff: tadwToolingHandoff{Request: input.History.Request, Thinking: thinking, Checkpoint: checkpoint}}
	round.prepare = func(prepareCtx context.Context) error {
		scope, err := input.History.compatible(selected, projectChat, limits)
		if err != nil {
			return err
		}
		history, window := freshHistory, freshWindow
		if window == nil {
			var err error
			history, err = worker.load(prepareCtx, input.History, limits)
			if err != nil {
				return err
			}
			transcript, err := loadTADWContextFrom(prepareCtx, worker.store, history, input.History.Request, selected.Ref, thinking, limits, boundary)
			if err != nil {
				return err
			}
			restored := tadwToolingWindow{FromEventID: boundary, Transcript: transcript}
			window = &restored
		}
		prepared := tadwPreparedHistory{Agent: selected.Ref, Scope: scope, History: history}
		err, provider := agentProviderRequestPrepareTADW(selected, projectChat, prepared, window.Transcript, limits)
		if err != nil {
			return err
		}
		round.Prepared, round.Window, round.Provider = prepared, *window, provider
		return nil
	}
	return round, nil
}

// A small hash preserves the owner's effective selected-agent/configuration
// contract across workers without serializing settings, prompts or credentials.
func tadwHandoffCompatibility(agent model.WorkspaceAgentRef, scope tadwHistoryCacheScope, limits tadwWindowConfig) (string, error) {
	return tadwPreparationDigest(struct {
		Version string
		Agent   model.WorkspaceAgentRef
		Scope   tadwHistoryCacheScope
		Limits  tadwWindowConfig
	}{"tadw-handoff-v1", agent, scope, limits})
}

func (handoff tadwHistoryHandoff) compatible(selected *database.WorkspaceAgentModel, projectChat bool, limits tadwWindowConfig) (tadwHistoryCacheScope, error) {
	scope, err := tadwHistoryCacheScopeForAgent(handoff.Request.Session, selected, projectChat)
	if err != nil {
		return tadwHistoryCacheScope{}, err
	}
	fingerprint, err := tadwHandoffCompatibility(selected.Ref, scope, limits)
	if err != nil {
		return tadwHistoryCacheScope{}, err
	}
	if fingerprint != handoff.Compatibility {
		return tadwHistoryCacheScope{}, fmt.Errorf("TADW handoff selected configuration or limits changed")
	}
	return scope, nil
}

func (worker *tadwHandoffWorker) previousBoundary(ctx context.Context, input tadwRoundHandoff, thinking model.SessionEventRef, previous *model.SessionEventRef, selected *database.WorkspaceAgentModel) (string, error) {
	prior := input.Previous
	if prior == nil {
		return "", nil
	}
	if previous == nil || prior.Request != input.History.Request || prior.Thinking.Session != thinking.Session || prior.Thinking.Id >= thinking.Id || prior.Checkpoint.WorkflowID == "" || prior.Checkpoint.WorkflowID == input.History.Checkpoint.WorkflowID {
		return "", fmt.Errorf("invalid TADW previous-round handoff")
	}
	err, event := worker.store.SessionEventGet(ctx, prior.Thinking)
	if err != nil {
		return "", err
	}
	if event == nil || event.Kind != model.SessionEventKindThinkingRequest || event.Parent == nil || *event.Parent != prior.Request || event.AuthorAgent == nil || *event.AuthorAgent != selected.Ref {
		return "", fmt.Errorf("TADW previous thinking request is unavailable or mismatched")
	}
	err, outcome := worker.store.SessionEventGet(ctx, *previous)
	if err != nil {
		return "", err
	}
	if outcome == nil || outcome.Kind != model.SessionEventKindThinkingSuccess || outcome.Parent == nil || *outcome.Parent != prior.Thinking || outcome.AuthorAgent == nil || *outcome.AuthorAgent != selected.Ref {
		return "", fmt.Errorf("TADW predecessor handoff does not match the thinking outcome")
	}
	if err := worker.admission(ctx, prior.Checkpoint.WorkflowID, prior.Thinking); err != nil {
		return "", err
	}
	output, err := worker.output(ctx, prior.Checkpoint, tadwToolingCheckpointName)
	if err != nil {
		return "", err
	}
	encoded, ok := output.(string)
	if !ok {
		return "", fmt.Errorf("invalid TADW tooling checkpoint output")
	}
	var from string
	if err := json.Unmarshal([]byte(encoded), &from); err != nil {
		return "", fmt.Errorf("decode TADW tooling checkpoint: %w", err)
	}
	return from, nil
}

// Bind a locator to the admitted thinking event, not just to a step name and
// caller-supplied scope. The existing child protocol starts with this checkpoint.
// No admission is rerun here; its reference-valued result is read as recorded.
func (worker *tadwHandoffWorker) admission(ctx context.Context, workflowID string, thinking model.SessionEventRef) error {
	output, err := worker.output(ctx, tadwCheckpointRef{WorkflowID: workflowID, StepID: 0}, "gatehouse.agent-round-prepare")
	if err != nil {
		return err
	}
	encoded, ok := output.(string)
	if !ok {
		return fmt.Errorf("invalid TADW admission checkpoint output")
	}
	var admitted model.SessionEventRef
	if err := json.Unmarshal([]byte(encoded), &admitted); err != nil {
		return fmt.Errorf("decode TADW admission checkpoint: %w", err)
	}
	if admitted != thinking {
		return fmt.Errorf("TADW checkpoint belongs to another admitted thinking request")
	}
	return nil
}

// Caller holds worker.mu. Pending entries can be evicted too: active callers
// retain their entry, and a later miss may duplicate recovery, never selection.
func (worker *tadwHandoffWorker) remember(key tadwHistoryMemoKey, entry *tadwHistoryMemoEntry) {
	if old := worker.histories[key]; old != nil {
		worker.order.Remove(old.position)
	}
	entry.position = worker.order.PushFront(key)
	worker.histories[key] = entry
	if len(worker.histories) > tadwHandoffWorkerHistoryEntries {
		oldest := worker.order.Back()
		delete(worker.histories, oldest.Value.(tadwHistoryMemoKey))
		worker.order.Remove(oldest)
	}
}

// Release only removes worker-owned memo references. In-flight callers can finish
// using their immutable entry; a later cold load restores IDs, never reselects.
func (worker *tadwHandoffWorker) Release(handoff tadwHistoryHandoff) {
	worker.mu.Lock()
	defer worker.mu.Unlock()
	for key := range worker.histories {
		if key.History == handoff {
			worker.order.Remove(worker.histories[key].position)
			delete(worker.histories, key)
		}
	}
}
