package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gatehouse/model"
	"gatehouse/typed_id"
)

const sessionResultFileMaximumNameLength = 128

// SessionResultFileCreate creates a fresh file/object pair for one publication
// attempt. A crash before the event commit may leave this file unreferenced.
func (store *Store) SessionResultFileCreate(ctx context.Context, source model.SessionEventRef, principal model.PrincipalRef) (error, *model.SessionFile, *StorageObject) {
	if !typed_id.Valid(typed_id.SessionEvent, source.Id) {
		return fmt.Errorf("create result file: source event is invalid"), nil, nil
	}
	err, event := store.SessionEventGet(ctx, source)
	if err != nil {
		return err, nil, nil
	}
	if event == nil || event.AuthorAgent == nil || (event.Kind != model.SessionEventKindToolRequest && event.Kind != model.SessionEventKindThinkingRequest) {
		return fmt.Errorf("create result file: source request is unavailable"), nil, nil
	}
	fileID, err := typed_id.New(typed_id.SessionFile)
	if err != nil {
		return fmt.Errorf("generate result file ID: %w", err), nil, nil
	}
	objectID, err := typed_id.New(typed_id.StorageObject)
	if err != nil {
		return fmt.Errorf("generate result storage object ID: %w", err), nil, nil
	}
	ref := model.SessionFileRef{Session: source.Session, Id: fileID}
	mediaType := "text/plain; charset=utf-8"
	fileName := sessionResultFileName(*event)
	err, _, _ = store.sessionFileCreate(ctx, model.SessionFile{Ref: ref, Name: fileName, MediaType: &mediaType, AuthorAgent: event.AuthorAgent}, objectID, principal, store.sessionFileProviderOrEmbedded)
	if err != nil {
		return err, nil, nil
	}
	err, file, object := store.SessionFileGet(ctx, ref, principal)
	return err, file, object
}

func sessionResultFileName(event model.SessionEvent) string {
	label, _ := event.Payload["reason"].(string)
	if strings.TrimSpace(label) == "" {
		label = "provider-diagnostic"
	}
	const extension = ".txt"
	maxReasonLength := sessionResultFileMaximumNameLength - len(event.Ref.Id) - 1 - len(extension)
	if maxReasonLength < 0 {
		maxReasonLength = 0
	}
	var slug strings.Builder
	separator := false
	for _, char := range strings.ToLower(label) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			extra := 1
			if separator && slug.Len() > 0 {
				extra++
			}
			if slug.Len()+extra > maxReasonLength {
				break
			}
			if separator && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			separator = false
			slug.WriteRune(char)
		} else {
			separator = true
		}
	}
	reason := strings.Trim(slug.String(), "-")
	if reason == "" {
		reason = "provider-diagnostic"
		if len(reason) > maxReasonLength {
			reason = reason[:maxReasonLength]
		}
	}
	return event.Ref.Id + "-" + reason + extension
}

// SessionEventOutputFile resolves the canonical reference, including its owning
// session. Hydrated public summaries are deliberately not valid stored references.
func SessionEventOutputFile(event model.SessionEvent) (model.SessionFileRef, bool, error) {
	value, present := event.Payload["output_file"]
	if !present {
		return model.SessionFileRef{}, false, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return model.SessionFileRef{}, false, err
	}
	var ref model.SessionFileRef
	if err := json.Unmarshal(encoded, &ref); err != nil {
		return ref, false, err
	}
	if !typed_id.Valid(typed_id.SessionFile, ref.Id) || ref.Session != event.Ref.Session {
		return ref, false, fmt.Errorf("event output file must belong to the event's session")
	}
	return ref, true, nil
}

func (store *Store) sessionEventOutputValidate(event model.SessionEvent, queryRow sessionRowQuery) error {
	_, inline := event.Payload["output"]
	_, file := event.Payload["output_file"]
	if event.Kind == model.SessionEventKindToolSuccess || event.Kind == model.SessionEventKindToolFailure {
		if inline == file {
			return fmt.Errorf("tool result requires exactly one of output and output_file")
		}
		if inline {
			if _, ok := event.Payload["output"].(string); !ok {
				return fmt.Errorf("tool output must be text")
			}
		}
	}
	if !file {
		return nil
	}
	if event.Kind != model.SessionEventKindToolSuccess && event.Kind != model.SessionEventKindToolFailure && event.Kind != model.SessionEventKindThinkingFailure {
		return fmt.Errorf("output_file is only valid for tool results and thinking failures")
	}
	ref, _, err := SessionEventOutputFile(event)
	if err != nil {
		return err
	}
	p := keychainPlaceholder(store.kind)
	var ready bool
	if err := queryRow(`SELECT EXISTS (SELECT 1 FROM gatehouse_session_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object WHERE files.workspace = `+p(1)+` AND files.session = `+p(2)+` AND files.id = `+p(3)+` AND files.enabled = TRUE AND objects.state = 'success')`, ref.Session.Workspace.Id, ref.Session.Id, ref.Id).Scan(&ready); err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("event output file is not a published result artifact")
	}
	return nil
}

// SessionEventOutputFilesHydrate adds metadata only to transient projections.
func (store *Store) SessionEventOutputFilesHydrate(ctx context.Context, events []model.SessionEvent) (error, []model.SessionEvent) {
	for index, event := range events {
		ref, present, err := SessionEventOutputFile(event)
		if err != nil {
			return err, nil
		}
		if !present {
			continue
		}
		// Rendering needs metadata, not permission to read the body. Removed
		// files retain their metadata so their event history remains renderable.
		p := keychainPlaceholder(store.kind)
		file, err := scanSessionFileSummary(store.QueryRowContext(ctx, `SELECT files.id, files.name, files.media_type, objects.size, objects.sha256, files.author_principal, files.author_agent
			FROM gatehouse_session_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object
			WHERE files.workspace = `+p(1)+` AND files.session = `+p(2)+` AND files.id = `+p(3), ref.Session.Workspace.Id, ref.Session.Id, ref.Id), ref.Session)
		if err != nil {
			return err, nil
		}
		value := map[string]interface{}{"id": file.ID, "name": file.Name, "size": float64(file.Size), "fingerprint": file.Fingerprint}
		if file.AuthorPrincipal != nil {
			value["author_principal"] = file.AuthorPrincipal
		}
		if file.AuthorAgent != nil {
			value["author_agent"] = file.AuthorAgent
		}
		if file.MediaType != nil {
			value["media_type"] = *file.MediaType
		}
		payload := make(map[string]interface{}, len(event.Payload))
		for key, item := range event.Payload {
			payload[key] = item
		}
		payload["output_file"] = value
		events[index].Payload = payload
	}
	return nil, events
}
