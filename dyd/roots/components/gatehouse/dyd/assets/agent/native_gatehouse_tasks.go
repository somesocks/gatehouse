package agent

import "gatehouse/lisp"

// TaskRead reads an authorized task description and reports whether it is sensitive.
type TaskRead func(id string, offset, length int64) (error, []byte, bool)

// TaskCreate creates an authorized task.
type TaskCreate func(title, description, status string, sensitive bool) (error, Task)

// TaskUpdate replaces an authorized task.
type TaskUpdate func(id, title, description, status string, sensitive bool) (error, Task)

// TaskRemove removes an authorized task.
type TaskRemove func(id string) (error, bool)

// Task describes an authorized flat task.
type Task struct {
	ID        string
	Title     string
	Sensitive bool
	Status    string
	CreatorID string
	UpdaterID string
	CreatedAt string
	UpdatedAt string
}

func taskListFunction(tasks []Task, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("%s requires no arguments", name), nil
		}
		values := make([]lisp.Expr, 0, len(tasks))
		for _, task := range tasks {
			err, value := taskValue(task, name)
			if err != nil {
				return err, nil
			}
			values = append(values, value)
		}
		return nil, lisp.List(values...)
	}
}

func unavailableTaskList(name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func([]lisp.Expr) (error, lisp.Expr) {
		return lisp.Errorf("%s is unavailable", name), nil
	}
}

func taskValue(task Task, name string) (error, lisp.Expr) {
	if task.ID == "" || task.Title == "" || task.Status == "" || task.CreatorID == "" || task.UpdaterID == "" || task.CreatedAt == "" || task.UpdatedAt == "" {
		return lisp.Errorf("%s has invalid task metadata", name), nil
	}
	return nil, lisp.List(
		lisp.Pair("id", lisp.String(task.ID)),
		lisp.Pair("title", lisp.String(task.Title)),
		lisp.Pair("sensitive", lisp.Boolean(task.Sensitive)),
		lisp.Pair("status", lisp.String(task.Status)),
		lisp.Pair("creator_id", lisp.String(task.CreatorID)),
		lisp.Pair("updater_id", lisp.String(task.UpdaterID)),
		lisp.Pair("created_at", lisp.String(task.CreatedAt)),
		lisp.Pair("updated_at", lisp.String(task.UpdatedAt)),
	)
}

func taskReadFunction(read TaskRead, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return noteReadFunction(NoteRead(read), name)
}

func taskCreateFunction(create TaskCreate, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 3 {
			return lisp.Errorf("%s requires title, description, and status", name), nil
		}
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone {
			return lisp.Errorf("%s title must not be sensitive", name), nil
		}
		if lisp.IsSecret(arguments[1]) || lisp.TaintOf(arguments[2]) != lisp.TaintNone {
			return lisp.Errorf("%s description must not be secret and status must not be sensitive", name), nil
		}
		err, title := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, description := lisp.RequireString(arguments[1])
		if err != nil {
			return err, nil
		}
		err, status := lisp.RequireString(arguments[2])
		if err != nil {
			return err, nil
		}
		err, task := create(title, description, status, lisp.IsSensitive(arguments[1]))
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return taskValue(task, name)
	}
}

func taskUpdateFunction(update TaskUpdate, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 4 {
			return lisp.Errorf("%s requires id, title, description, and status", name), nil
		}
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone || lisp.TaintOf(arguments[1]) != lisp.TaintNone {
			return lisp.Errorf("%s id and title must not be sensitive", name), nil
		}
		if lisp.IsSecret(arguments[2]) || lisp.TaintOf(arguments[3]) != lisp.TaintNone {
			return lisp.Errorf("%s description must not be secret and status must not be sensitive", name), nil
		}
		err, id := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, title := lisp.RequireString(arguments[1])
		if err != nil {
			return err, nil
		}
		err, description := lisp.RequireString(arguments[2])
		if err != nil {
			return err, nil
		}
		err, status := lisp.RequireString(arguments[3])
		if err != nil {
			return err, nil
		}
		err, task := update(id, title, description, status, lisp.IsSensitive(arguments[2]))
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return taskValue(task, name)
	}
}

func taskRemoveFunction(remove TaskRemove, name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("%s requires id", name), nil
		}
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone {
			return lisp.Errorf("%s id must not be sensitive", name), nil
		}
		err, id := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		if id == "" {
			return lisp.Errorf("%s requires a non-empty id", name), nil
		}
		err, removed := remove(id)
		if err != nil {
			return lisp.Errorf("%s failed", name), nil
		}
		return nil, lisp.Boolean(removed)
	}
}
