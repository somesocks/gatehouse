package agent

import (
	"encoding/xml"
	"strconv"
	"strings"
	"unicode/utf8"

	"gatehouse/model"
)

type transcriptMessage struct {
	Event       model.SessionEvent
	Attachments []transcriptAttachment
	Truncated   bool
	Omitted     bool
	SizeBytes   int
	ShownBytes  int
}

type transcriptAttachment struct {
	ID        string
	Name      string
	MediaType string
	SizeBytes int64
}

type transcriptToolOutput struct {
	Event      model.SessionEvent
	Status     string
	Truncated  bool
	Omitted    bool
	SizeBytes  int
	ShownBytes int
}

func transcriptMessageContent(event model.SessionEvent) (*transcriptMessage, string, bool) {
	text, _ := event.Payload["text"].(string)
	attachments := transcriptAttachments(event.Payload["attachments"])
	if strings.TrimSpace(text) == "" {
		text = ""
	}
	return &transcriptMessage{Event: event, Attachments: attachments}, text, true
}

func transcriptAttachments(value any) []transcriptAttachment {
	values, ok := value.([]interface{})
	if !ok {
		return nil
	}
	attachments := make([]transcriptAttachment, 0, len(values))
	for _, value := range values {
		file, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		id, idOK := file["id"].(string)
		name, nameOK := file["name"].(string)
		size, sizeOK := file["size"].(float64)
		if !idOK || !nameOK || !sizeOK || size < 0 {
			continue
		}
		attachment := transcriptAttachment{ID: id, Name: name, SizeBytes: int64(size)}
		attachment.MediaType, _ = file["media_type"].(string)
		attachments = append(attachments, attachment)
	}
	return attachments
}

func transcriptEvents(events ...string) string {
	var result strings.Builder
	result.WriteString("<events>")
	for _, event := range events {
		result.WriteString(event)
	}
	result.WriteString("</events>")
	return result.String()
}

func transcriptEvent(event model.SessionEvent, kind, status, callID, contents string) string {
	var result strings.Builder
	result.WriteString("<event")
	transcriptAttribute(&result, "id", event.Ref.Id)
	if event.Parent != nil {
		transcriptAttribute(&result, "parent-id", event.Parent.Id)
	}
	transcriptAttribute(&result, "kind", kind)
	if status != "" {
		transcriptAttribute(&result, "status", status)
	}
	if callID != "" {
		transcriptAttribute(&result, "call-id", callID)
	}
	if contents == "" {
		result.WriteString(" />")
		return result.String()
	}
	result.WriteString(">")
	result.WriteString(contents)
	result.WriteString("</event>")
	return result.String()
}

func transcriptMessageEvent(content transcriptMessage, text string) string {
	var body strings.Builder
	if text != "" {
		body.WriteString("<text")
		transcriptTruncationAttributes(&body, content.Truncated, content.SizeBytes, content.ShownBytes)
		body.WriteString(">")
		body.WriteString(transcriptEscape(text))
		body.WriteString("</text>")
	}
	for _, attachment := range content.Attachments {
		body.WriteString("<attachment")
		transcriptAttribute(&body, "id", attachment.ID)
		transcriptAttribute(&body, "name", attachment.Name)
		if attachment.MediaType != "" {
			transcriptAttribute(&body, "media-type", attachment.MediaType)
		}
		transcriptAttribute(&body, "size-bytes", strconv.FormatInt(attachment.SizeBytes, 10))
		body.WriteString(" />")
	}
	if text == "" && len(content.Attachments) == 0 {
		body.WriteString("<no-reply />")
	}
	return transcriptEvent(content.Event, "message", "", "", body.String())
}

func transcriptToolCallEvent(event model.SessionEvent, callID string) string {
	return transcriptEvent(event, "tool-call", "", callID, "")
}

func transcriptToolResultEvent(content transcriptToolOutput, output string) string {
	if content.Omitted {
		return transcriptOmittedEvent(content.Event, "tool-result", content.Status, "", content.SizeBytes)
	}
	var body strings.Builder
	body.WriteString("<output")
	transcriptTruncationAttributes(&body, content.Truncated, content.SizeBytes, content.ShownBytes)
	body.WriteString(">")
	body.WriteString(transcriptEscape(output))
	body.WriteString("</output>")
	return transcriptEvent(content.Event, "tool-result", content.Status, "", body.String())
}

func transcriptOmittedEvent(event model.SessionEvent, kind, status, callID string, sizeBytes int) string {
	var result strings.Builder
	result.WriteString("<event")
	transcriptAttribute(&result, "id", event.Ref.Id)
	if event.Parent != nil {
		transcriptAttribute(&result, "parent-id", event.Parent.Id)
	}
	transcriptAttribute(&result, "kind", kind)
	if status != "" {
		transcriptAttribute(&result, "status", status)
	}
	if callID != "" {
		transcriptAttribute(&result, "call-id", callID)
	}
	transcriptAttribute(&result, "truncated", "true")
	transcriptAttribute(&result, "size-bytes", strconv.Itoa(sizeBytes))
	transcriptAttribute(&result, "shown-bytes", "0")
	result.WriteString(" />")
	return result.String()
}

type contextSchedule struct {
	fullLimit, fullUntil       int
	previewLimit, previewUntil int
	shortLimit, shortUntil     int
}

var (
	userContextSchedule = contextSchedule{fullLimit: 4 * 1024, fullUntil: 12 * 1024, previewLimit: 1024, previewUntil: 18 * 1024, shortLimit: 250, shortUntil: 21 * 1024}
	agentContextSchedule = contextSchedule{fullLimit: 2 * 1024, fullUntil: 6 * 1024, previewLimit: 768, previewUntil: 10500, shortLimit: 256, shortUntil: 13500}
	toolResultSchedule   = contextSchedule{fullLimit: 4 * 1024, fullUntil: 12 * 1024, previewLimit: 1024, previewUntil: 18 * 1024, shortLimit: 256, shortUntil: 21 * 1024}
)

func selectContextMessages(messages []openAICompatibleMessage, budget int) []openAICompatibleMessage {
	result := append([]openAICompatibleMessage(nil), messages...)
	selectedCalls := map[string]bool{}
	callBytes := 0
	for index := len(result) - 1; index >= 0; index-- {
		if len(result[index].ToolCalls) == 0 {
			continue
		}
		calls := make([]openAICompatibleToolCall, 0, len(result[index].ToolCalls))
		for _, call := range result[index].ToolCalls {
			if len(call.Function.Arguments) > 2*1024 || callBytes+len(call.Function.Arguments) > 6*1024 {
				continue
			}
			calls = append(calls, call)
			selectedCalls[call.ID] = true
			callBytes += len(call.Function.Arguments)
		}
		result[index].ToolCalls = calls
	}
	userBytes, agentBytes, resultBytes := 0, 0, 0
	for index := len(result) - 1; index >= 0; index-- {
		if result[index].Message != nil {
			used := &agentBytes
			schedule := agentContextSchedule
			if result[index].Role == "user" {
				used = &userBytes
				schedule = userContextSchedule
			}
			result[index] = selectMessageContent(result[index], schedule, used)
		}
		if result[index].ToolOutput != nil {
			if !selectedCalls[result[index].ToolCallID] {
				result[index] = omitToolOutput(result[index])
				continue
			}
			result[index] = selectToolOutput(result[index], toolResultSchedule, &resultBytes)
		}
	}
	included := make([]bool, len(result))
	used := 0
	for index := len(result) - 1; index >= 0; index-- {
		message := result[index]
		if len(message.ToolCalls) == 0 && message.Role == "assistant" && message.Content == "" && message.Message == nil && message.ToolOutput == nil {
			continue
		}
		preview := renderTranscriptMessages([]openAICompatibleMessage{message})[0]
		cost := len(preview.Content)
		for _, call := range preview.ToolCalls {
			cost += len(call.Function.Arguments)
		}
		if used+cost > budget {
			continue
		}
		used += cost
		included[index] = true
	}
	calls := map[string]int{}
	outputs := map[string]int{}
	for index := range result {
		for _, call := range result[index].ToolCalls {
			calls[call.ID] = index
		}
		if result[index].Role == "tool" && result[index].ToolCallID != "" {
			outputs[result[index].ToolCallID] = index
		}
	}
	for callID, index := range calls {
		output, exists := outputs[callID]
		if exists && included[index] && !included[output] {
			included[index] = false
		}
		if exists && included[output] && !included[index] {
			result[output] = omitToolOutput(result[output])
		}
	}
	filtered := make([]openAICompatibleMessage, 0, len(result))
	for index, message := range result {
		if included[index] {
			filtered = append(filtered, message)
		}
	}
	return filtered
}

func selectMessageContent(message openAICompatibleMessage, schedule contextSchedule, used *int) openAICompatibleMessage {
	content := *message.Message
	limit := contextContentLimit(len(message.Content), schedule, used)
	if limit == 0 {
		content.Omitted = true
		content.SizeBytes = len(message.Content)
		content.ShownBytes = 0
		message.Content = ""
		message.Message = &content
		return message
	}
	if limit < len(message.Content) {
		content.SizeBytes = len(message.Content)
		message.Content, content.ShownBytes = transcriptPreview(message.Content, limit)
		content.Truncated = true
	}
	message.Message = &content
	return message
}

func selectToolOutput(message openAICompatibleMessage, schedule contextSchedule, used *int) openAICompatibleMessage {
	content := *message.ToolOutput
	limit := contextContentLimit(len(message.Content), schedule, used)
	if limit == 0 {
		return omitToolOutput(message)
	}
	if limit < len(message.Content) {
		content.SizeBytes = len(message.Content)
		message.Content, content.ShownBytes = transcriptPreview(message.Content, limit)
		content.Truncated = true
	}
	message.ToolOutput = &content
	return message
}

func omitToolOutput(message openAICompatibleMessage) openAICompatibleMessage {
	content := *message.ToolOutput
	content.Omitted = true
	content.SizeBytes = len(message.Content)
	content.ShownBytes = 0
	message.Role = "assistant"
	message.ToolCallID = ""
	message.Content = ""
	message.ToolOutput = &content
	return message
}

func contextContentLimit(size int, schedule contextSchedule, used *int) int {
	if size <= schedule.fullLimit && *used+size <= schedule.fullUntil {
		*used += size
		return size
	}
	if *used < schedule.previewUntil {
		limit := min(size, schedule.previewLimit)
		if *used+limit <= schedule.previewUntil {
			*used += limit
			return limit
		}
	}
	if *used < schedule.shortUntil {
		limit := min(size, schedule.shortLimit)
		if *used+limit <= schedule.shortUntil {
			*used += limit
			return limit
		}
	}
	return 0
}

func renderTranscriptMessages(messages []openAICompatibleMessage) []openAICompatibleMessage {
	result := append([]openAICompatibleMessage(nil), messages...)
	for index := range result {
		if result[index].Message != nil {
			if result[index].Message.Omitted {
				result[index].Content = transcriptEvents(transcriptOmittedEvent(result[index].Message.Event, "message", "", "", result[index].Message.SizeBytes))
			} else {
				result[index].Content = transcriptEvents(transcriptMessageEvent(*result[index].Message, result[index].Content))
			}
		}
		if result[index].ToolOutput != nil {
			result[index].Content = transcriptEvents(transcriptToolResultEvent(*result[index].ToolOutput, result[index].Content))
		}
	}
	return result
}

func transcriptPreview(value string, maximum int) (string, int) {
	if maximum >= len(value) {
		return value, len(value)
	}
	for maximum > 0 && !utf8.ValidString(value[:maximum]) {
		maximum--
	}
	return value[:maximum], maximum
}

func transcriptTruncationAttributes(result *strings.Builder, truncated bool, sizeBytes, shownBytes int) {
	if !truncated {
		return
	}
	transcriptAttribute(result, "truncated", "true")
	transcriptAttribute(result, "size-bytes", strconv.Itoa(sizeBytes))
	transcriptAttribute(result, "shown-bytes", strconv.Itoa(shownBytes))
}

func transcriptAttribute(result *strings.Builder, name, value string) {
	result.WriteString(" ")
	result.WriteString(name)
	result.WriteString("=\"")
	result.WriteString(transcriptEscape(value))
	result.WriteString("\"")
}

func transcriptEscape(value string) string {
	var result strings.Builder
	_ = xml.EscapeText(&result, []byte(value))
	return result.String()
}
