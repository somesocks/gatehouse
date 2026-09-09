package database

import (
	"fmt"
	"strings"
	"time"
)

const (
	taskTitleMaxSize       = 256
	taskDescriptionMaxSize = 4 * 1024
)

// TaskAuthor identifies the creator or last updater of a task.
type TaskAuthor = NoteAuthor

func normalizeTask(title *string, description **string, status *string) error {
	if title == nil || description == nil || status == nil {
		return fmt.Errorf("task fields are required")
	}
	*title = strings.Join(strings.Fields(*title), " ")
	if *title == "" || len(*title) > taskTitleMaxSize {
		return fmt.Errorf("task title is invalid")
	}
	if *description != nil {
		value := strings.TrimSpace(**description)
		if value == "" {
			*description = nil
		} else if len(value) > taskDescriptionMaxSize {
			return fmt.Errorf("task description exceeds the size limit")
		} else {
			*description = &value
		}
	}
	if *status == "" {
		*status = "draft"
	}
	switch *status {
	case "draft", "ready", "in_progress", "done", "cancelled":
		return nil
	default:
		return fmt.Errorf("task status is invalid")
	}
}

func taskAuthorEmpty(author TaskAuthor) bool {
	return author.Principal == nil && author.Agent == nil && author.Gateway == nil
}

func taskUpdatedAt() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}
