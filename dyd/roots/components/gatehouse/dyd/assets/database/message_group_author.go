package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"gatehouse/model"
	"gatehouse/typed_id"
)

var ErrMessageSenderGroup = errors.New("You can no longer send as the selected group. Choose another group or clear Send as.")

// SessionMessageGroupAuthor reads the message's sender identity, not the acting
// principal. A projected agent request carries the same sender as its message.
func SessionMessageGroupAuthor(event model.SessionEvent) (*model.MessageGroupAuthor, error) {
	value, exists := event.Payload["author_group"]
	if !exists || (event.Kind != model.SessionEventKindMessageText && event.Kind != model.SessionEventKindAgentRequest) {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, ErrMessageSenderGroup
	}
	var group model.MessageGroupAuthor
	if err := json.Unmarshal(encoded, &group); err != nil || !typed_id.Valid(typed_id.Group, group.Id) || event.AuthorPrincipal == nil {
		return nil, ErrMessageSenderGroup
	}
	return &group, nil
}

func (store *Store) sessionMessageGroupAuthorSnapshot(ctx context.Context, tx *sql.Tx, event model.SessionEvent) error {
	group, err := SessionMessageGroupAuthor(event)
	if err != nil || group == nil {
		return err
	}
	p := keychainPlaceholder(store.kind)
	row := tx.QueryRowContext(ctx, `SELECT groups.alias, groups.name`+store.workspaceMemberGroupSQL()+` AND groups.id = `+p(3),
		event.Ref.Session.Workspace.Id, event.AuthorPrincipal.Ref.Id, group.Id)
	group.Alias, group.Name = nil, nil
	if err := row.Scan(&group.Alias, &group.Name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMessageSenderGroup
		}
		return err
	}
	event.Payload["author_group"] = group
	return nil
}
