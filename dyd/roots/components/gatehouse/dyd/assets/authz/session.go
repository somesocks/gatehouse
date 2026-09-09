package authz

type SessionAction string

const (
	SessionEdit            SessionAction = "session.edit"
	SessionMessageCreate   SessionAction = "session.message.create"
	SessionMessageCancel   SessionAction = "session.message.cancel"
	SessionNoteCreate      SessionAction = "session.note.create"
	SessionNoteEdit        SessionAction = "session.note.edit"
	SessionNoteRemove      SessionAction = "session.note.remove"
	SessionTaskCreate      SessionAction = "session.task.create"
	SessionTaskEdit        SessionAction = "session.task.edit"
	SessionTaskRemove      SessionAction = "session.task.remove"
	SessionFileCreate      SessionAction = "session.file.create"
	SessionFileFinish      SessionAction = "session.file.finish"
	SessionFileRemove      SessionAction = "session.file.remove"
	SessionSecretCreate    SessionAction = "session.secret.create"
	SessionSecretEdit      SessionAction = "session.secret.edit"
	SessionSecretRemove    SessionAction = "session.secret.remove"
	SessionApprovalRespond SessionAction = "session.approval.respond"
	SessionProjectSet      SessionAction = "session.project.set"
	SessionGrantManage     SessionAction = "session.grant.manage"
)

func SessionAllows(roles []Role, action SessionAction) bool {
	if !validSessionAction(action) {
		return false
	}
	for _, role := range roles {
		switch role {
		case Manager:
			return true
		case Contributor:
			switch action {
			case SessionMessageCreate, SessionMessageCancel,
				SessionNoteCreate, SessionNoteEdit, SessionNoteRemove,
				SessionTaskCreate, SessionTaskEdit, SessionTaskRemove,
				SessionFileCreate, SessionFileFinish, SessionFileRemove,
				SessionSecretCreate, SessionSecretEdit, SessionSecretRemove,
				SessionApprovalRespond:
				return true
			}
		}
	}
	return false
}

func validSessionAction(action SessionAction) bool {
	switch action {
	case SessionEdit, SessionMessageCreate, SessionMessageCancel,
		SessionNoteCreate, SessionNoteEdit, SessionNoteRemove,
		SessionTaskCreate, SessionTaskEdit, SessionTaskRemove,
		SessionFileCreate, SessionFileFinish, SessionFileRemove,
		SessionSecretCreate, SessionSecretEdit, SessionSecretRemove,
		SessionApprovalRespond, SessionProjectSet, SessionGrantManage:
		return true
	default:
		return false
	}
}
