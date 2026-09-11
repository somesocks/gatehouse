package authz

type ProjectAction string

const (
	ProjectEdit          ProjectAction = "project.edit"
	ProjectNoteCreate    ProjectAction = "project.note.create"
	ProjectNoteEdit      ProjectAction = "project.note.edit"
	ProjectNoteRemove    ProjectAction = "project.note.remove"
	ProjectTaskCreate    ProjectAction = "project.task.create"
	ProjectTaskEdit      ProjectAction = "project.task.edit"
	ProjectTaskRemove    ProjectAction = "project.task.remove"
	ProjectFileCreate    ProjectAction = "project.file.create"
	ProjectFileFinish    ProjectAction = "project.file.finish"
	ProjectFileRemove    ProjectAction = "project.file.remove"
	ProjectSecretCreate  ProjectAction = "project.secret.create"
	ProjectSecretEdit    ProjectAction = "project.secret.edit"
	ProjectSecretRemove  ProjectAction = "project.secret.remove"
	ProjectRecordCreate  ProjectAction = "project.record.create"
	ProjectRecordEdit    ProjectAction = "project.record.edit"
	ProjectRecordRemove  ProjectAction = "project.record.remove"
	ProjectRecordSchemaCreate ProjectAction = "project.record_schema.create"
	ProjectRecordSchemaEdit   ProjectAction = "project.record_schema.edit"
	ProjectRecordSchemaRemove ProjectAction = "project.record_schema.remove"
	ProjectSessionCreate ProjectAction = "project.session.create"
	ProjectGrantManage   ProjectAction = "project.grant.manage"
)

func ProjectAllows(roles []Role, action ProjectAction) bool {
	if !validProjectAction(action) {
		return false
	}
	for _, role := range roles {
		switch role {
		case Manager:
			return true
		case Contributor:
			switch action {
			case ProjectNoteCreate, ProjectNoteEdit, ProjectNoteRemove,
				ProjectTaskCreate, ProjectTaskEdit, ProjectTaskRemove,
				ProjectRecordCreate, ProjectRecordEdit, ProjectRecordRemove,
				ProjectFileCreate, ProjectFileFinish, ProjectFileRemove,
				ProjectSecretCreate, ProjectSecretEdit, ProjectSecretRemove,
				ProjectSessionCreate:
				return true
			}
		}
	}
	return false
}

func validProjectAction(action ProjectAction) bool {
	switch action {
	case ProjectEdit, ProjectNoteCreate, ProjectNoteEdit, ProjectNoteRemove,
		ProjectTaskCreate, ProjectTaskEdit, ProjectTaskRemove,
		ProjectFileCreate, ProjectFileFinish, ProjectFileRemove,
		ProjectSecretCreate, ProjectSecretEdit, ProjectSecretRemove,
		ProjectRecordCreate, ProjectRecordEdit, ProjectRecordRemove,
		ProjectRecordSchemaCreate, ProjectRecordSchemaEdit, ProjectRecordSchemaRemove,
		ProjectSessionCreate, ProjectGrantManage:
		return true
	default:
		return false
	}
}
