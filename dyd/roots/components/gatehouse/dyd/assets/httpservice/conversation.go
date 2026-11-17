package httpservice

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"gatehouse/apischema"
	"gatehouse/auth"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func workspaceSessionConversation(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspace, sessionID := request.PathValue("workspace"), request.PathValue("session")
		if workspace == "" || !typed_id.Valid(typed_id.Session, sessionID) {
			http.NotFound(response, request)
			return
		}
		session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspace}, Id: sessionID}
		err, authorized := store.SessionGet(request.Context(), session, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if authorized == nil {
			http.NotFound(response, request)
			return
		}
		input := database.ConversationBranchQuery{Parent: request.URL.Query().Get("parent"), Cursor: request.URL.Query().Get("cursor"), Limit: 20}
		if value := request.URL.Query().Get("limit"); value != "" {
			limit, err := strconv.Atoi(value)
			if err != nil || limit < 1 || limit > 100 {
				http.Error(response, "invalid branch limit", http.StatusBadRequest)
				return
			}
			input.Limit = limit
		}
		err, branch := store.SessionConversationBranchGet(request.Context(), session, input)
		if err != nil {
			switch {
			case errors.Is(err, database.ErrConversationQuery):
				http.Error(response, err.Error(), http.StatusBadRequest)
			case errors.Is(err, database.ErrConversationParent):
				http.NotFound(response, request)
			default:
				http.Error(response, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		wire, err := conversationBranchWire(request, store, session, input.Parent, branch)
		if err != nil {
			if errors.Is(err, database.ErrConversationParent) {
				http.NotFound(response, request)
			} else {
				http.Error(response, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		writeJSON(response, wire)
	}
}

func conversationBranchWire(request *http.Request, store *database.Store, session model.SessionRef, parent string, branch *database.ConversationBranch) (any, error) {
	// Deduplicate before existing batch attribution/file hydration. Projection
	// removes form descriptors, drafts/results and execution-only payload fields.
	entries := []database.SessionEventTreeEntry{}
	seen := map[string]bool{}
	add := func(event model.SessionEvent) {
		if !seen[event.Ref.Id] {
			seen[event.Ref.Id] = true
			entries = append(entries, database.SessionEventTreeEntry{Event: event})
		}
	}
	if parent != "" {
		err, event := store.SessionEventGet(request.Context(), model.SessionEventRef{Session: session, Id: parent})
		if err != nil {
			return nil, err
		}
		if event == nil {
			return nil, database.ErrConversationParent
		}
		add(*event)
	}
	for _, entry := range branch.Entries {
		add(entry.Event)
		for _, preview := range entry.Previews {
			add(preview)
		}
	}
	for _, agent := range branch.AgentRequests {
		if agent.Outcome != nil {
			add(*agent.Outcome)
		}
	}
	for _, interaction := range branch.Interactions {
		add(interaction.Request)
		if interaction.Response != nil {
			add(*interaction.Response)
		}
	}
	sessionEventTranscriptProject(entries)
	events := make([]model.SessionEvent, len(entries))
	for i, entry := range entries {
		events[i] = entry.Event
	}
	err, events := store.SessionEventAttachmentsHydrate(request.Context(), session, events)
	if err != nil {
		return nil, err
	}
	err, events = store.SessionEventOutputFilesHydrate(request.Context(), events)
	if err != nil {
		return nil, err
	}
	err, agents := store.SessionEventAgentsGet(request.Context(), session, events)
	if err != nil {
		return nil, err
	}
	encoded := map[string]any{}
	for _, event := range events {
		err, wire := transcriptEventWire(event, agents)
		if err != nil {
			return nil, err
		}
		encoded[event.Ref.Id] = wire
	}
	resultEntries := make([]any, 0, len(branch.Entries))
	for _, entry := range branch.Entries {
		previews := make([]any, 0, len(entry.Previews))
		for _, event := range entry.Previews {
			previews = append(previews, encoded[event.Ref.Id])
		}
		resultEntries = append(resultEntries, map[string]any{"event": encoded[entry.Event.Ref.Id], "previews": previews, "has_more_replies": entry.HasMoreReplies})
	}
	resultAgents := make([]any, 0, len(branch.AgentRequests))
	for _, agent := range branch.AgentRequests {
		interactions := make([]any, 0, len(agent.Interactions))
		for _, ref := range agent.Interactions {
			interactions = append(interactions, conversationEventRefWire(ref))
		}
		value := map[string]any{"request": conversationEventRefWire(agent.Request), "state": agent.State, "interactions": interactions}
		if agent.Outcome != nil {
			value["outcome"] = encoded[agent.Outcome.Ref.Id]
		}
		resultAgents = append(resultAgents, value)
	}
	resultInteractions := make([]any, 0, len(branch.Interactions))
	for _, interaction := range branch.Interactions {
		value := map[string]any{"request": encoded[interaction.Request.Ref.Id]}
		if interaction.Response != nil {
			value["response"] = encoded[interaction.Response.Ref.Id]
		}
		resultInteractions = append(resultInteractions, value)
	}
	value := map[string]any{"entries": resultEntries, "agent_requests": resultAgents, "interactions": resultInteractions}
	if parent != "" {
		value["parent"] = encoded[parent]
	}
	if branch.NextCursor != "" {
		value["next_cursor"] = branch.NextCursor
	}
	// Validate the envelope with the same generated contract used by clients.
	err, typed := apischema.DecodeConversationBranch(value)
	if err != nil {
		return nil, fmt.Errorf("type conversation branch: %w", err)
	}
	err, wire := apischema.EncodeConversationBranch(typed)
	return wire, err
}

func conversationEventRefWire(ref model.SessionEventRef) map[string]any {
	return map[string]any{"id": ref.Id, "session": map[string]any{"id": ref.Session.Id, "workspace": map[string]any{"id": ref.Session.Workspace.Id}}}
}
