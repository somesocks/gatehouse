package agent

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
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

type transcriptToolCall struct {
	Event      model.SessionEvent
	CallID     string
	Arguments  string
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
	transcriptMCMTRTreeAttributes(&result, event)
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

func transcriptToolCallArgumentsEvent(content transcriptToolCall) string {
	if content.Omitted {
		return transcriptOmittedEvent(content.Event, "tool-call", "", content.CallID, content.SizeBytes)
	}
	var body strings.Builder
	body.WriteString("<arguments")
	transcriptTruncationAttributes(&body, content.Truncated, content.SizeBytes, content.ShownBytes)
	body.WriteString(">")
	body.WriteString(transcriptEscape(content.Arguments))
	body.WriteString("</arguments>")
	return transcriptEvent(content.Event, "tool-call", "", content.CallID, body.String())
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
	transcriptMCMTRTreeAttributes(&result, event)
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

func transcriptMCMTRTreeAttributes(result *strings.Builder, event model.SessionEvent) {
	root, enabled := event.Payload["_mcmtr_root"].(string)
	if !enabled {
		return
	}
	transcriptAttribute(result, "root-id", root)
	if event.AuthorPrincipal != nil {
		transcriptAttribute(result, "author", "principal:"+event.AuthorPrincipal.Ref.Id)
	} else if event.AuthorAgent != nil {
		transcriptAttribute(result, "author", "agent:"+event.AuthorAgent.Id)
	} else if event.AuthorGateway != nil {
		transcriptAttribute(result, "author", "gateway:"+event.AuthorGateway.Id)
	}
}

func transcriptPayloadEvent(event model.SessionEvent, payload string, truncated bool, sizeBytes, shownBytes int) string {
	var body strings.Builder
	body.WriteString("<payload")
	transcriptTruncationAttributes(&body, truncated, sizeBytes, shownBytes)
	body.WriteString(">")
	body.WriteString(transcriptEscape(payload))
	body.WriteString("</payload>")
	return transcriptEvent(event, event.Kind, "", "", body.String())
}

type mcmtrText struct {
	value      string
	truncated  bool
	omitted    bool
	sizeBytes  int
	shownBytes int
}

const (
	mcmtrToolResultContentMaximumBytes = 4 * 1024
	mcmtrHighTierRetentionDivisor      = 4
)

var mcmtrStreams = []string{"user", "agent", "tool"}

type mcmtrRecord struct {
	event    model.SessionEvent
	kind     string
	status   string
	callID   string
	channel  contextSchedule
	stream   string
	contents mcmtrText
}

// compileMCMTRContext projects durable session events into individual provider
// messages. MCMTR controls which records and previews fit; it does not combine
// historical records into a mutable transcript message.
func compileMCMTRContext(events []model.SessionEvent, active model.SessionEventRef, profile mcmtrProfile, state mcmtrContextState) ([]openAICompatibleMessage, mcmtrContextState, error) {
	messages, next, _, err := compileMCMTRContextWithShared(events, active, profile, state)
	return messages, next, err
}

func compileMCMTRContextWithShared(events []model.SessionEvent, active model.SessionEventRef, profile mcmtrProfile, state mcmtrContextState) ([]openAICompatibleMessage, mcmtrContextState, int, error) {
	events = mcmtrAnnotateRoots(events)
	byID := make(map[string]model.SessionEvent, len(events))
	for _, event := range events {
		byID[event.Ref.Id] = event
	}
	activeEvent, ok := byID[active.Id]
	if !ok || activeEvent.Ref.Session != active.Session || activeEvent.Kind != "message.text" || activeEvent.AuthorPrincipal == nil {
		return nil, state, 0, fmt.Errorf("reply to session event %q: active user message is unavailable", active.Id)
	}

	all := make([]mcmtrRecord, 0, len(events))
	for _, event := range events {
		record, include, err := mcmtrRecordFor(event)
		if err != nil {
			return nil, state, 0, err
		}
		if !include {
			continue
		}
		record.stream = mcmtrChannel(event)
		all = append(all, record)
	}
	activeText, _ := activeEvent.Payload["text"].(string)
	activeContent := mcmtrLimitText(activeText, max(0, profile.BufferBytes/len(mcmtrStreams)-512))
	activeMessage, _, _ := transcriptMessageContent(activeEvent)
	activeMessage.Truncated = activeContent.truncated
	activeMessage.Omitted = activeContent.omitted
	activeMessage.SizeBytes = activeContent.sizeBytes
	activeMessage.ShownBytes = activeContent.shownBytes
	activeRendered := transcriptMessageEvent(*activeMessage, activeContent.value)
	if activeContent.omitted {
		activeRendered = transcriptOmittedEvent(activeEvent, "message", "", "", activeContent.sizeBytes)
	}
	highInput := make([]mcmtrRecord, 0, len(all)-1)
	for _, record := range all {
		if record.event.Ref.Id != active.Id {
			highInput = append(highInput, record)
		}
	}
	highBudget := max(0, profile.BufferBytes-len(activeRendered))
	high, state := mcmtrHighTier(highInput, state, highBudget, mcmtrHighTierRetainedPerStream(profile.BufferBytes))

	highRecords := make([]mcmtrRecord, 0, len(all))
	lowerRecords := make([]mcmtrRecord, 0, len(all))
	remainingHigh := highBudget
	for _, record := range all {
		if record.event.Ref.Id == active.Id {
			continue
		}
		if high[record.event.Ref.Id] {
			record.contents = mcmtrLimitExistingText(record.contents, max(0, remainingHigh-512))
			remainingHigh = max(0, remainingHigh-mcmtrRecordCost(record))
			highRecords = append(highRecords, record)
			continue
		}
		lowerRecords = append(lowerRecords, record)
	}

	shared := max(0, profile.HistoryBytes-profile.BufferBytes)
	selected := append([]mcmtrRecord(nil), highRecords...)
	for index := len(lowerRecords) - 1; index >= 0; index-- {
		record := lowerRecords[index]
		record.contents = mcmtrLimitExistingText(record.contents, max(0, min(1024, shared-512)))
		if mcmtrRecordCost(record) > shared {
			continue
		}
		shared -= mcmtrRecordCost(record)
		selected = append(selected, record)
	}
	sort.Slice(selected, func(left, right int) bool { return selected[left].event.Ref.Id < selected[right].event.Ref.Id })
	before, after := mcmtrSplitActiveRecords(selected, active.Id)
	messages := mcmtrRecordMessages(before)
	messages = append(messages, openAICompatibleMessage{Role: "user", Content: activeRendered})
	messages = append(messages, mcmtrRecordMessages(after)...)
	return messages, state, shared, nil
}

func mcmtrSplitActiveRecords(records []mcmtrRecord, activeID string) ([]mcmtrRecord, []mcmtrRecord) {
	before := make([]mcmtrRecord, 0, len(records))
	after := make([]mcmtrRecord, 0, len(records))
	for _, record := range records {
		root, _ := record.event.Payload["_mcmtr_root"].(string)
		if root == activeID {
			after = append(after, record)
			continue
		}
		before = append(before, record)
	}
	return before, after
}

type mcmtrNativeBatch struct {
	callIDs       map[string]bool
	resultCallIDs map[string]string
	leaderID      string
	ordered       []openAICompatibleToolCall
}

func mcmtrRecordMessages(records []mcmtrRecord) []openAICompatibleMessage {
	batches := mcmtrNativeBatches(records)
	callBatches := map[string]mcmtrNativeBatch{}
	resultBatches := map[string]mcmtrNativeBatch{}
	for _, batch := range batches {
		for callID := range batch.callIDs {
			callBatches[callID] = batch
		}
		for resultID := range batch.resultCallIDs {
			resultBatches[resultID] = batch
		}
	}
	messages := make([]openAICompatibleMessage, 0, len(records))
	for _, record := range records {
		if batch, ok := callBatches[record.event.Ref.Id]; ok {
			if record.event.Ref.Id == batch.leaderID {
				messages = append(messages, openAICompatibleMessage{Role: "assistant", ToolCalls: batch.ordered})
			}
			continue
		}
		if batch, ok := resultBatches[record.event.Ref.Id]; ok {
			messages = append(messages, openAICompatibleMessage{Role: "tool", ToolCallID: batch.resultCallIDs[record.event.Ref.Id], Content: transcriptToolResultEvent(transcriptToolOutput{Event: record.event, Status: record.status, Truncated: record.contents.truncated, Omitted: record.contents.omitted, SizeBytes: record.contents.sizeBytes, ShownBytes: record.contents.shownBytes}, record.contents.value)})
			continue
		}
		messages = append(messages, mcmtrHistoryMessage(record))
	}
	return messages
}

func mcmtrNativeBatches(records []mcmtrRecord) []mcmtrNativeBatch {
	type candidateCall struct {
		record   mcmtrRecord
		position int
	}
	type candidate struct {
		calls []candidateCall
	}
	results := map[string]mcmtrRecord{}
	candidates := map[string]*candidate{}
	for _, record := range records {
		switch record.kind {
		case "tool-call":
			if record.contents.truncated || record.contents.omitted || record.event.Parent == nil {
				continue
			}
			batch, batchOK := record.event.Payload["batch"].(float64)
			position, positionOK := record.event.Payload["position"].(float64)
			if !batchOK || !positionOK {
				continue
			}
			key := record.event.Parent.Id + "\x00" + fmt.Sprintf("%.0f", batch)
			if candidates[key] == nil {
				candidates[key] = &candidate{}
			}
			candidates[key].calls = append(candidates[key].calls, candidateCall{record: record, position: int(position)})
		case "tool-result":
			if !record.contents.omitted && record.event.Parent != nil {
				results[record.event.Parent.Id] = record
			}
		}
	}
	batches := make([]mcmtrNativeBatch, 0, len(candidates))
	for _, candidate := range candidates {
		batch := mcmtrNativeBatch{callIDs: map[string]bool{}, resultCallIDs: map[string]string{}}
		for _, call := range candidate.calls {
			if _, ok := results[call.record.event.Ref.Id]; !ok {
				batch = mcmtrNativeBatch{}
				break
			}
		}
		if batch.callIDs == nil {
			continue
		}
		sort.Slice(candidate.calls, func(left, right int) bool { return candidate.calls[left].position < candidate.calls[right].position })
		for _, call := range candidate.calls {
			stored, err := openAICompatibleStoredToolCall(call.record.event)
			if err != nil {
				batch = mcmtrNativeBatch{}
				break
			}
			if batch.leaderID == "" || call.record.event.Ref.Id < batch.leaderID {
				batch.leaderID = call.record.event.Ref.Id
			}
			result := results[call.record.event.Ref.Id]
			batch.callIDs[call.record.event.Ref.Id] = true
			batch.resultCallIDs[result.event.Ref.Id] = stored.ID
			batch.ordered = append(batch.ordered, stored)
		}
		if batch.callIDs != nil {
			batches = append(batches, batch)
		}
	}
	return batches
}

func mcmtrHistoryMessage(record mcmtrRecord) openAICompatibleMessage {
	role := "assistant"
	if record.kind == "message" && record.event.AuthorPrincipal != nil {
		role = "user"
	}
	return openAICompatibleMessage{Role: role, Content: record.render()}
}

func mcmtrRecordCost(record mcmtrRecord) int {
	return len(record.render())
}

func mcmtrAnnotateRoots(events []model.SessionEvent) []model.SessionEvent {
	byID := make(map[string]model.SessionEvent, len(events))
	for _, event := range events {
		byID[event.Ref.Id] = event
	}
	roots := make(map[string]string, len(events))
	var rootFor func(model.SessionEvent) string
	rootFor = func(event model.SessionEvent) string {
		if root, ok := roots[event.Ref.Id]; ok {
			return root
		}
		root := event.Ref.Id
		if event.Parent != nil {
			if parent, ok := byID[event.Parent.Id]; ok {
				root = rootFor(parent)
			} else {
				// The bounded suffix may omit ancestors; retain its tree anchor.
				root = event.Parent.Id
			}
		}
		roots[event.Ref.Id] = root
		return root
	}
	result := make([]model.SessionEvent, len(events))
	for index, event := range events {
		payload := make(map[string]interface{}, len(event.Payload)+1)
		for key, value := range event.Payload {
			payload[key] = value
		}
		payload["_mcmtr_root"] = rootFor(event)
		event.Payload = payload
		result[index] = event
	}
	return result
}

func mcmtrChannel(event model.SessionEvent) string {
	if event.Kind == "tool.request" || event.Kind == "tool.success" || event.Kind == "tool.failure" {
		return "tool"
	}
	if event.Kind == "message.text" && event.AuthorPrincipal != nil {
		return "user"
	}
	return "agent"
}

func mcmtrHighTier(records []mcmtrRecord, state mcmtrContextState, buffer, retainedPerStream int) (map[string]bool, mcmtrContextState) {
	candidates := make(map[string][]mcmtrRecord, len(mcmtrStreams))
	used := 0
	for _, record := range records {
		checkpoint := mcmtrHighCheckpoint(state, record.stream)
		if checkpoint == "" || record.event.Ref.Id >= checkpoint {
			candidates[record.stream] = append(candidates[record.stream], record)
			used += mcmtrRecordCost(record)
		}
	}

	high := map[string]bool{}
	if used <= buffer {
		for _, stream := range mcmtrStreams {
			streamCandidates := candidates[stream]
			if len(streamCandidates) == 0 {
				continue
			}
			if mcmtrHighCheckpoint(state, stream) == "" {
				mcmtrSetHighCheckpoint(&state, stream, streamCandidates[0].event.Ref.Id)
			}
			for _, record := range streamCandidates {
				high[record.event.Ref.Id] = true
			}
		}
		return high, state
	}

	for _, stream := range mcmtrStreams {
		streamCandidates := candidates[stream]
		if len(streamCandidates) == 0 {
			continue
		}
		used := 0
		start := len(streamCandidates)
		for index := len(streamCandidates) - 1; index >= 0; index-- {
			cost := mcmtrRecordCost(streamCandidates[index])
			if used > 0 && used+cost > retainedPerStream {
				break
			}
			used += cost
			start = index
		}
		if start == len(streamCandidates) {
			start = len(streamCandidates) - 1
		}
		mcmtrSetHighCheckpoint(&state, stream, streamCandidates[start].event.Ref.Id)
		for _, record := range streamCandidates[start:] {
			high[record.event.Ref.Id] = true
		}
	}
	return high, state
}

func mcmtrHighTierRetainedPerStream(buffer int) int {
	return buffer / (mcmtrHighTierRetentionDivisor * len(mcmtrStreams))
}

func mcmtrLimitText(value string, limit int) mcmtrText {
	return mcmtrLimitExistingText(mcmtrText{value: value, sizeBytes: len(value), shownBytes: len(value)}, limit)
}

// mcmtrLimitExistingText retains the raw-size metadata from an earlier cap.
func mcmtrLimitExistingText(content mcmtrText, limit int) mcmtrText {
	if !content.truncated && !content.omitted && content.sizeBytes == 0 && content.shownBytes == 0 {
		content.sizeBytes, content.shownBytes = len(content.value), len(content.value)
	}
	if limit <= 0 && len(content.value) > 0 {
		content.value, content.omitted, content.shownBytes = "", true, 0
		return content
	}
	if limit < len(content.value) {
		content.value, content.shownBytes = transcriptPreview(content.value, limit)
		content.truncated = true
	}
	return content
}

// mcmtrNativeCallsFit rejects a tool batch before it can create durable events
// when even its smallest recoverable native representation cannot fit.
func mcmtrNativeCallsFit(calls []openAICompatibleToolCall, buffer int) bool {
	used := 0
	for _, call := range calls {
		preview := transcriptToolCall{CallID: call.ID, Arguments: call.Function.Arguments, Omitted: true, SizeBytes: len(call.Function.Arguments)}
		used += len(preview.call().Function.Arguments) + 256
	}
	return used <= buffer
}

func mcmtrSelectText(value string, schedule contextSchedule, used *int) mcmtrText {
	content := mcmtrText{value: value, sizeBytes: len(value), shownBytes: len(value)}
	limit := contextContentLimit(len(value), schedule, used)
	if limit == 0 && len(value) > 0 {
		content.value = ""
		content.omitted = true
		content.shownBytes = 0
		return content
	}
	if limit < len(value) {
		content.value, content.shownBytes = transcriptPreview(value, limit)
		content.truncated = true
	}
	return content
}

func mcmtrRecordFor(event model.SessionEvent) (mcmtrRecord, bool, error) {
	if event.Kind == "agent.request" || strings.HasPrefix(event.Kind, "thinking.") || strings.HasPrefix(event.Kind, "approval.") {
		return mcmtrRecord{}, false, nil
	}
	record := mcmtrRecord{event: event, kind: event.Kind, channel: agentContextSchedule}
	switch event.Kind {
	case "message.text", "agent.reply":
		record.kind = "message"
		record.contents.value, _ = event.Payload["text"].(string)
		if event.AuthorPrincipal != nil {
			record.channel = userContextSchedule
		}
	case "tool.request":
		if event.AuthorAgent == nil {
			return mcmtrRecord{}, false, nil
		}
		call, err := openAICompatibleStoredToolCall(event)
		if err != nil {
			return mcmtrRecord{}, false, err
		}
		record.kind, record.callID, record.contents.value, record.channel = "tool-call", call.ID, call.Function.Arguments, toolResultSchedule
	case "tool.success", "tool.failure":
		output, ok := event.Payload["output"].(string)
		if !ok {
			return mcmtrRecord{}, false, fmt.Errorf("session tool output %q has no text output", event.Ref.Id)
		}
		record.kind, record.contents, record.channel = "tool-result", mcmtrLimitText(output, mcmtrToolResultContentMaximumBytes), toolResultSchedule
		if event.Kind == "tool.success" {
			record.status = "success"
		} else {
			record.status = "failure"
		}
	default:
		payloadValue := make(map[string]interface{}, len(event.Payload))
		for key, value := range event.Payload {
			if key != "_mcmtr_root" {
				payloadValue[key] = value
			}
		}
		payload, err := json.Marshal(payloadValue)
		if err != nil {
			return mcmtrRecord{}, false, fmt.Errorf("encode session event %q payload: %w", event.Ref.Id, err)
		}
		record.contents.value = string(payload)
	}
	return record, true, nil
}

func (record mcmtrRecord) render() string {
	if record.contents.omitted {
		return transcriptOmittedEvent(record.event, record.kind, record.status, record.callID, record.contents.sizeBytes)
	}
	switch record.kind {
	case "message":
		content, _, _ := transcriptMessageContent(record.event)
		content.Truncated, content.SizeBytes, content.ShownBytes = record.contents.truncated, record.contents.sizeBytes, record.contents.shownBytes
		return transcriptMessageEvent(*content, record.contents.value)
	case "tool-call":
		return transcriptToolCallArgumentsEvent(transcriptToolCall{Event: record.event, CallID: record.callID, Arguments: record.contents.value, Truncated: record.contents.truncated, SizeBytes: record.contents.sizeBytes, ShownBytes: record.contents.shownBytes})
	case "tool-result":
		return transcriptToolResultEvent(transcriptToolOutput{Event: record.event, Status: record.status, Truncated: record.contents.truncated, SizeBytes: record.contents.sizeBytes, ShownBytes: record.contents.shownBytes}, record.contents.value)
	default:
		return transcriptPayloadEvent(record.event, record.contents.value, record.contents.truncated, record.contents.sizeBytes, record.contents.shownBytes)
	}
}

func mcmtrTranscript(records []mcmtrRecord, budget, fixed int, calls []transcriptToolCall, results []mcmtrRecord) string {
	used := fixed + len("<session-transcript></session-transcript>")
	for _, call := range calls {
		used += len(call.call().Function.Arguments)
	}
	for _, result := range results {
		used += len(transcriptToolResultEvent(transcriptToolOutput{Event: result.event, Status: result.status, Truncated: result.contents.truncated, Omitted: result.contents.omitted, SizeBytes: result.contents.sizeBytes, ShownBytes: result.contents.shownBytes}, result.contents.value))
	}
	selected := make([]string, 0, len(records))
	for index := len(records) - 1; index >= 0; index-- {
		rendered := records[index].render()
		if used+len(rendered) > budget {
			omitted := transcriptOmittedEvent(records[index].event, records[index].kind, records[index].status, records[index].callID, records[index].contents.sizeBytes)
			if used+len(omitted) > budget {
				continue
			}
			rendered = omitted
		}
		used += len(rendered)
		selected = append(selected, rendered)
	}
	if len(selected) == 0 {
		return ""
	}
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return "<session-transcript>" + strings.Join(selected, "") + "</session-transcript>"
}

func (content transcriptToolCall) call() openAICompatibleToolCall {
	call := openAICompatibleToolCall{ID: content.CallID, Type: "function"}
	call.Function.Name = "lisp"
	if !content.Truncated && !content.Omitted {
		call.Function.Arguments = content.Arguments
		return call
	}
	preview := content.Arguments
	if content.Omitted {
		preview = ""
	}
	arguments, _ := json.Marshal(struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}{Code: preview, Reason: fmt.Sprintf("MCMTR preview of session event %q arguments; truncated=%t size-bytes=%d shown-bytes=%d. Use session/events/read to recover the full arguments.", content.Event.Ref.Id, true, content.SizeBytes, content.ShownBytes)})
	call.Function.Arguments = string(arguments)
	return call
}

type contextSchedule struct {
	fullLimit, fullUntil       int
	previewLimit, previewUntil int
	shortLimit, shortUntil     int
}

var (
	userContextSchedule  = contextSchedule{fullLimit: 4 * 1024, fullUntil: 12 * 1024, previewLimit: 1024, previewUntil: 18 * 1024, shortLimit: 250, shortUntil: 21 * 1024}
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
