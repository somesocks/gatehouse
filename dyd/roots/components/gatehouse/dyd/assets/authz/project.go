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
		ProjectSessionCreate, ProjectGrantManage:
		return true
	default:
		return false
	}
}
