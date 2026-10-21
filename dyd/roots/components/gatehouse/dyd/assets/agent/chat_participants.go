package agent

import (
	"context"
	"fmt"

	"gatehouse/model"
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
