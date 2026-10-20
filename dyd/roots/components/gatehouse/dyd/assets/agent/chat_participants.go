package agent

import (
	"context"
	"errors"
	"fmt"

	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func (runtime *SessionEventReplyRuntime) requireAgentParticipant(ctx context.Context, session model.SessionRef, agent model.WorkspaceAgentRef, requests ...model.SessionEventRef) error {
	err, active := runtime.store.SessionAgentParticipantGet(ctx, session, agent)
	if err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("bot is not a participant in this chat")
	}
	for _, ref := range requests {
		if err, authorized := runtime.store.SessionRequestAuthorizedGet(ctx, ref); err != nil {
			return err
		} else if !authorized {
			return fmt.Errorf("requesting principal is no longer authorized in this workspace and chat")
		}
	}
	return nil
}

func (runtime *SessionEventReplyRuntime) cancelDetachedAgentRequests() error {
	err, refs := runtime.store.SessionDetachedAgentRequestsGet(runtime.dbos, 100)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		err, request := runtime.store.SessionEventGet(runtime.dbos, ref)
		if err != nil {
			return err
		}
		if request == nil || request.AuthorPrincipal == nil {
			continue
		}
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			return err
		}
		err, _ = runtime.CancelReply(runtime.dbos, model.SessionEvent{
			Ref: model.SessionEventRef{Session: ref.Session, Id: id}, Parent: &ref,
			Kind: model.SessionEventKindCancelRequest, AuthorPrincipal: request.AuthorPrincipal, Payload: map[string]interface{}{},
		})
		if err != nil && !errors.Is(err, database.ErrSessionReplyAlreadyCompleted) {
			return err
		}
	}
	return nil
}
