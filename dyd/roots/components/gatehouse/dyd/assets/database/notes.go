package database

import (
	"database/sql"
	"fmt"
	"time"

	"gatehouse/model"
)

type NoteAuthor struct {
	Principal *model.PrincipalRef
	Agent     *model.WorkspaceAgentRef
	Gateway   *model.GatewayRef
}

func noteAuthorValid(author NoteAuthor, workspace model.WorkspaceRef) error {
	count := 0
	if author.Principal != nil {
		count++
	}
	if author.Agent != nil {
		if author.Agent.Workspace != workspace {
			return fmt.Errorf("note author agent belongs to another workspace")
		}
		count++
	}
	if author.Gateway != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("note must have exactly one author")
	}
	return nil
}

func noteAuthorValues(author NoteAuthor) (any, any, any) {
	var principal, agent, gateway any
	if author.Principal != nil {
		principal = author.Principal.Id
	}
	if author.Agent != nil {
		agent = author.Agent.Id
	}
	if author.Gateway != nil {
		gateway = author.Gateway.Id
	}
	return principal, agent, gateway
}

func noteAuthorFromValues(workspace model.WorkspaceRef, principal, agent, gateway sql.NullString) NoteAuthor {
	author := NoteAuthor{}
	if principal.Valid {
		author.Principal = &model.PrincipalRef{Id: principal.String}
	}
	if agent.Valid {
		author.Agent = &model.WorkspaceAgentRef{Workspace: workspace, Id: agent.String}
	}
	if gateway.Valid {
		author.Gateway = &model.GatewayRef{Id: gateway.String}
	}
	return author
}

func noteRevisionCreatedAt() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}
