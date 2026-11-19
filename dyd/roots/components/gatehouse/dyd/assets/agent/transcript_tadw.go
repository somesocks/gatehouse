package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gatehouse/database"
	"gatehouse/model"
)

// Bump when XML serialization/ordering semantics change incompatibly. This is
// an internal compatibility salt, not a workspace-agent identity or resource.
const tadwTranscriptFormatVersion = "tadw-flat-xml-v2"

// tadwRenderedEvent is an independent XML fragment. Its trailing newline belongs
// to the event, so appending another event never changes an existing fragment or
// its byte weight. Cache breakpoints can fall immediately after any fragment.
type tadwRenderedEvent struct {
	ID  string
	XML string
}

func (event tadwRenderedEvent) Bytes() int64 {
	return int64(len(event.XML))
}

// tadwTranscript retains window and event boundaries without adding an envelope.
// Selection, budgeting, compaction and provider cache controls belong to callers.
type tadwTranscript struct {
	History []tadwRenderedEvent
	Tooling []tadwRenderedEvent
}

func (transcript tadwTranscript) HistoryBytes() int64 {
	return tadwRenderedBytes(transcript.History)
}

func (transcript tadwTranscript) ToolingBytes() int64 {
	return tadwRenderedBytes(transcript.Tooling)
}

func (transcript tadwTranscript) Bytes() int64 {
	return transcript.HistoryBytes() + transcript.ToolingBytes()
}

func (transcript tadwTranscript) String() string {
	var result strings.Builder
	for _, window := range [][]tadwRenderedEvent{transcript.History, transcript.Tooling} {
		for _, event := range window {
			result.WriteString(event.XML)
		}
	}
	return result.String()
}

func tadwRenderedBytes(events []tadwRenderedEvent) int64 {
	var size int64
	for _, event := range events {
		size += event.Bytes()
	}
	return size
}

// renderTADWTranscript preserves the supplied history/tooling order. Events must
// already have hydrated attachment/output-file metadata; rendering performs no
// database or file reads and does not mutate its inputs.
func renderTADWTranscript(history, tooling []model.SessionEvent) (tadwTranscript, error) {
	var result tadwTranscript
	for index, window := range [][]model.SessionEvent{history, tooling} {
		for _, event := range window {
			rendered, include, err := renderTADWEvent(event)
			if err != nil {
				return tadwTranscript{}, err
			}
			if !include {
				continue
			}
			if index == 0 {
				result.History = append(result.History, rendered)
			} else {
				result.Tooling = append(result.Tooling, rendered)
			}
		}
	}
	return result, nil
}

// tadwEventIncluded lets selection omit bookkeeping before file hydration, using
// the exact same visibility rules as the renderer.
func tadwEventIncluded(event model.SessionEvent) bool {
	switch event.Kind {
	case model.SessionEventKindAgentFailure:
		return event.Payload["code"] != "cancelled"
	case model.SessionEventKindMessageText, model.SessionEventKindAgentRequest,
		model.SessionEventKindAgentSuccess, model.SessionEventKindToolRequest,
		model.SessionEventKindToolSuccess, model.SessionEventKindToolFailure:
		return true
	default:
		return false
	}
}

// renderTADWEvent renders only the explicit transcript allowlist. Other event
// kinds (including provider bookkeeping) have no fragment and cost zero bytes;
// there is deliberately no fallback that serializes arbitrary event payloads.
func renderTADWEvent(event model.SessionEvent) (tadwRenderedEvent, bool, error) {
	if !tadwEventIncluded(event) {
		return tadwRenderedEvent{}, false, nil
	}
	tag := event.Kind
	if event.Kind == model.SessionEventKindMessageText {
		tag = "message"
	}
	if event.Ref.Id == "" || event.Parent != nil && event.Parent.Id == "" {
		return tadwRenderedEvent{}, false, fmt.Errorf("render TADW event %q: event/parent ID is empty", event.Ref.Id)
	}
	author, err := tadwAuthorID(event)
	if err != nil {
		return tadwRenderedEvent{}, false, fmt.Errorf("render TADW event %q: %w", event.Ref.Id, err)
	}
	attributes, body, err := tadwEventBody(event)
	if err != nil {
		return tadwRenderedEvent{}, false, fmt.Errorf("render TADW event %q: %w", event.Ref.Id, err)
	}

	var result strings.Builder
	result.WriteString("<" + tag)
	transcriptAttribute(&result, "id", event.Ref.Id)
	if event.Parent != nil {
		transcriptAttribute(&result, "parent", event.Parent.Id)
	}
	group, err := database.SessionMessageGroupAuthor(event)
	if err != nil {
		return tadwRenderedEvent{}, false, fmt.Errorf("render TADW event %q: %w", event.Ref.Id, err)
	}
	if group != nil {
		transcriptAttribute(&result, "author", group.Id)
		transcriptGroupAuthorLabels(&result, group)
	} else {
		transcriptAttribute(&result, "author", author)
	}
	for _, attribute := range attributes {
		transcriptAttribute(&result, attribute.name, attribute.value)
	}
	if body == "" {
		result.WriteString(" />\n")
	} else {
		result.WriteString(">")
		result.WriteString(body)
		result.WriteString("</" + tag + ">\n")
	}
	return tadwRenderedEvent{ID: event.Ref.Id, XML: result.String()}, true, nil
}

func tadwAuthorID(event model.SessionEvent) (string, error) {
	var id string
	count := 0
	if event.AuthorPrincipal != nil {
		id = event.AuthorPrincipal.Ref.Id
		count++
	}
	if event.AuthorAgent != nil {
		id = event.AuthorAgent.Id
		count++
	}
	if event.AuthorGateway != nil {
		id = event.AuthorGateway.Id
		count++
	}
	if count != 1 || id == "" {
		return "", fmt.Errorf("expected exactly one nonempty author ID")
	}
	return id, nil
}

type tadwAttribute struct {
	name, value string
}

func tadwEventBody(event model.SessionEvent) ([]tadwAttribute, string, error) {
	switch event.Kind {
	case model.SessionEventKindMessageText, model.SessionEventKindAgentSuccess:
		body, err := tadwMessageBody(event)
		return nil, body, err
	case model.SessionEventKindAgentRequest:
		agent, err := tadwPayloadString(event.Payload, "agent", true)
		return []tadwAttribute{{"agent", agent}}, "", err
	case model.SessionEventKindAgentFailure:
		code, err := tadwPayloadString(event.Payload, "code", true)
		if err != nil {
			return nil, "", err
		}
		message, err := tadwPayloadString(event.Payload, "message", false)
		return []tadwAttribute{{"code", code}}, transcriptEscape(message), err
	case model.SessionEventKindToolRequest:
		name, err := tadwPayloadString(event.Payload, "name", true)
		if err != nil {
			return nil, "", err
		}
		code, err := tadwPayloadString(event.Payload, "code", true)
		if err != nil {
			return nil, "", err
		}
		reason, err := tadwPayloadString(event.Payload, "reason", true)
		if err != nil {
			return nil, "", err
		}
		body := "<code>" + transcriptEscape(code) + "</code><reason>" + transcriptEscape(reason) + "</reason>"
		return []tadwAttribute{{"name", name}}, body, nil
	case model.SessionEventKindToolSuccess, model.SessionEventKindToolFailure:
		return tadwToolResultBody(event)
	default:
		return nil, "", fmt.Errorf("unsupported transcript kind %q", event.Kind)
	}
}

func tadwMessageBody(event model.SessionEvent) (string, error) {
	var body strings.Builder
	if event.Kind == model.SessionEventKindMessageText {
		targets, err := database.SessionMessageTargets(event)
		if err != nil {
			return "", err
		}
		for _, target := range targets {
			body.WriteString("<recipient")
			transcriptAttribute(&body, "id", target.Ref.Id)
			body.WriteString(" />")
		}
	}
	text, err := tadwPayloadString(event.Payload, "text", false)
	if err != nil {
		return "", err
	}
	body.WriteString(transcriptEscape(text))
	if value, exists := event.Payload["attachments"]; exists {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode attachments: %w", err)
		}
		var files []json.RawMessage
		if err := json.Unmarshal(encoded, &files); err != nil {
			return "", fmt.Errorf("attachments require hydrated file metadata: %w", err)
		}
		for _, file := range files {
			fragment, err := tadwFileReference("attachment", file)
			if err != nil {
				return "", fmt.Errorf("render attachment: %w", err)
			}
			body.WriteString(fragment)
		}
	}
	return body.String(), nil
}

func tadwToolResultBody(event model.SessionEvent) ([]tadwAttribute, string, error) {
	var attributes []tadwAttribute
	var body strings.Builder
	if event.Kind == model.SessionEventKindToolFailure {
		code, err := tadwPayloadString(event.Payload, "code", true)
		if err != nil {
			return nil, "", err
		}
		attributes = append(attributes, tadwAttribute{"code", code})
		message, err := tadwPayloadString(event.Payload, "message", false)
		if err != nil {
			return nil, "", err
		}
		body.WriteString(transcriptEscape(message))
	}
	_, inline := event.Payload["output"]
	file, fileBacked := event.Payload["output_file"]
	if inline == fileBacked {
		return nil, "", fmt.Errorf("tool result requires exactly one of output and output_file")
	}
	if fileBacked {
		fragment, err := tadwFileReference("output-file", file)
		if err != nil {
			return nil, "", fmt.Errorf("render output_file: %w", err)
		}
		body.WriteString(fragment)
	} else {
		output, err := tadwPayloadString(event.Payload, "output", false)
		if err != nil {
			return nil, "", err
		}
		if body.Len() > 0 && output != "" {
			body.WriteByte('\n')
		}
		body.WriteString(transcriptEscape(output))
	}
	return attributes, body.String(), nil
}

func tadwPayloadString(payload map[string]interface{}, key string, required bool) (string, error) {
	value, exists := payload[key]
	if !exists && !required {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("payload %q must be text", key)
	}
	if required && text == "" {
		return "", fmt.Errorf("payload %q must be nonempty text", key)
	}
	return text, nil
}

// tadwFileReference consumes transient metadata projections, not canonical
// SessionFileRef values or attachment IDs. The caller must hydrate those first.
func tadwFileReference(tag string, value interface{}) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var file struct {
		ID        string      `json:"id"`
		Name      string      `json:"name"`
		MediaType string      `json:"media_type"`
		Size      json.Number `json:"size"`
	}
	if err := json.Unmarshal(encoded, &file); err != nil {
		return "", fmt.Errorf("file reference requires hydrated metadata: %w", err)
	}
	size, err := file.Size.Int64()
	if file.ID == "" || file.Name == "" || err != nil || size < 0 {
		return "", fmt.Errorf("file reference requires hydrated id, name and nonnegative integer size")
	}
	var result strings.Builder
	result.WriteString("<" + tag)
	transcriptAttribute(&result, "id", file.ID)
	transcriptAttribute(&result, "name", file.Name)
	// An absent optional media type is represented honestly as an empty value.
	transcriptAttribute(&result, "media-type", file.MediaType)
	transcriptAttribute(&result, "size-bytes", strconv.FormatInt(size, 10))
	result.WriteString(" />")
	return result.String(), nil
}
