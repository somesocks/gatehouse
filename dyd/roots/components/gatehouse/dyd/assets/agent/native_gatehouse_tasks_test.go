package agent

import (
	"strings"
	"testing"

	"gatehouse/lisp"
)

func TestGatehouseProjectTasks(t *testing.T) {
	removed := false
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (project/tasks/list)
    (project/tasks/create "Review design" description "ready")
    (project/tasks/update "task" "Review design" description "done")
    (project/tasks/read "task" 0 6)
    (project/tasks/remove "task")))`, lisp.EvalOptions{
		Bindings: []lisp.Binding{
			{Name: "description", Value: lisp.MarkSensitive(lisp.String("Check proposal"))},
		},
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil, &ProjectTasks{
			Tasks: []Task{{ID: "task", Title: "Review design", Sensitive: true, Status: "ready", CreatorID: "agent", UpdaterID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z"}},
			Create: func(title, description, status string, sensitive bool) (error, Task) {
				if title != "Review design" || description != "Check proposal" || status != "ready" || !sensitive {
					t.Fatalf("project task create = (%q, %q, %q, %t)", title, description, status, sensitive)
				}
				return nil, Task{ID: "task", Title: title, Sensitive: sensitive, Status: status, CreatorID: "agent", UpdaterID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z"}
			},
			Update: func(id, title, description, status string, sensitive bool) (error, Task) {
				if id != "task" || title != "Review design" || description != "Check proposal" || status != "done" || !sensitive {
					t.Fatalf("project task update = (%q, %q, %q, %q, %t)", id, title, description, status, sensitive)
				}
				return nil, Task{ID: id, Title: title, Sensitive: sensitive, Status: status, CreatorID: "agent", UpdaterID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z"}
			},
			Read: func(id string, offset, length int64) (error, []byte, bool) {
				if id != "task" || offset != 0 || length != 6 {
					t.Fatalf("project task read = (%q, %d, %d)", id, offset, length)
				}
				return nil, []byte("secret"), true
			},
			Remove: func(id string) (error, bool) {
				removed = id == "task"
				return nil, removed
			},
		})},
	})
	if err != nil || !lisp.IsSensitive(result) || strings.Contains(result.String(), "Check proposal") || !removed {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseSessionTasksUnavailableAndRejectSecrets(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (list
    (error/value (error/catch (session/tasks/list)))
    (error/value (error/catch (session/tasks/read "task" 0 1)))
    (error/value (error/catch (session/tasks/create "Title" "Description" "ready")))
    (error/value (error/catch (session/tasks/update "task" "Title" "Description" "ready")))
    (error/value (error/catch (session/tasks/remove "task")))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewSessionModule(nil, nil, nil, nil)},
	})
	if err != nil || result.String() != `("session/tasks/list is unavailable" "session/tasks/read is unavailable" "session/tasks/create is unavailable" "session/tasks/update is unavailable" "session/tasks/remove is unavailable")` {
		t.Fatalf("Evaluate() unavailable task exports = (%s, %v)", result, err)
	}

	err, _ = lisp.Evaluate(`(import
  (session @native:gatehouse/session/v1)
  (session/tasks/create "Title" description "ready"))`, lisp.EvalOptions{
		Bindings: []lisp.Binding{{Name: "description", Value: lisp.MarkSecret(lisp.String("secret"))}},
		HostModules: []lisp.HostModule{NewSessionModuleWithTasks(nil, nil, nil, nil, &SessionTasks{Create: func(string, string, string, bool) (error, Task) {
			return nil, Task{}
		}})},
	})
	if err == nil || !strings.Contains(err.Error(), "description must not be secret") {
		t.Fatalf("Evaluate() secret task description error = %v", err)
	}
}
