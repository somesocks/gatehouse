package httpservice

import (
	"context"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

//go:embed web
var webAssets embed.FS

var webFiles = func() fs.FS {
	files, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(err)
	}
	return files
}()

var webFileServer = http.FileServer(http.FS(webFiles))

type Service struct {
	listener net.Listener
	server   *http.Server
	done     chan error
}

func Start(configuration config.HTTPService, store *database.Store, tokens ...*auth.BearerTokens) (error, *Service) {
	return start(configuration, store, nil, tokens...)
}

type ReplyDispatcher interface {
	Reconcile() error
}

func StartWithReplyDispatcher(configuration config.HTTPService, store *database.Store, dispatcher ReplyDispatcher, tokens ...*auth.BearerTokens) (error, *Service) {
	return start(configuration, store, dispatcher, tokens...)
}

func start(configuration config.HTTPService, store *database.Store, dispatcher ReplyDispatcher, tokens ...*auth.BearerTokens) (error, *Service) {
	listener, err := net.Listen("tcp", configuration.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", configuration.Listen, err), nil
	}

	service := &Service{
		listener: listener,
		server: &http.Server{
			Handler:           handler(configuration, store, dispatcher, tokens...),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       time.Minute,
			MaxHeaderBytes:    1 << 20,
		},
		done: make(chan error, 1),
	}
	go func() {
		err := service.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		service.done <- err
		close(service.done)
	}()
	return nil, service
}

func (service *Service) Address() string {
	return service.listener.Addr().String()
}

func (service *Service) Done() <-chan error {
	return service.done
}

func (service *Service) Shutdown(ctx context.Context) error {
	return service.server.Shutdown(ctx)
}

func Handler(configuration config.HTTPService, store *database.Store, tokens ...*auth.BearerTokens) http.Handler {
	return handler(configuration, store, nil, tokens...)
}

func HandlerWithReplyDispatcher(configuration config.HTTPService, store *database.Store, dispatcher ReplyDispatcher, tokens ...*auth.BearerTokens) http.Handler {
	return handler(configuration, store, dispatcher, tokens...)
}

func handler(configuration config.HTTPService, store *database.Store, dispatcher ReplyDispatcher, tokens ...*auth.BearerTokens) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("/readyz", health)
	if configuration.Web {
		mux.HandleFunc("/", webRoot)
		mux.HandleFunc("/app", web)
		mux.HandleFunc("/app/", web)
	}
	if configuration.API && len(tokens) > 0 && tokens[0] != nil {
		mux.HandleFunc("/api/v1/auth/login", login(tokens[0]))
		mux.HandleFunc("/api/v1/auth/me", me(tokens[0]))
		mux.HandleFunc("/api/v1/auth/logout", logout)
		mux.HandleFunc("/api/v1/workspaces", workspaces(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/groups", workspaceGroups(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects", workspaceProjects(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}", workspaceProject(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/notes", workspaceProjectNotes(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/notes/{note}", workspaceProjectNote(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/secrets", workspaceProjectSecrets(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/secrets/{secret}", workspaceProjectSecret(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/files", workspaceProjectFiles(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/files/start", workspaceProjectFileStart(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/files/{file}/finish", workspaceProjectFileFinish(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/files/{file}/download", workspaceProjectFileDownload(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/files/{file}", workspaceProjectFile(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/agents", workspaceAgents(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions", workspaceSessions(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}", workspaceSession(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/project", workspaceSessionProject(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/notes", workspaceSessionNotes(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/notes/{note}", workspaceSessionNote(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/secrets", workspaceSessionSecrets(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/secrets/{secret}", workspaceSessionSecret(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/activity", workspaceActivity(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/events", workspaceSessionEvents(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/approvals/{approval}", workspaceSessionApproval(store, tokens[0], dispatcher))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/files", workspaceSessionFiles(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/files/{file}/finish", workspaceSessionFileFinish(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/files/{file}/download", workspaceSessionFileDownload(store, tokens[0]))
		mux.HandleFunc("/api/v1/storage", storageProxy(tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/messages/{event}/cancel", workspaceSessionMessageCancel(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/messages", workspaceSessionMessages(store, tokens[0], dispatcher))
	}
	return mux
}

type loginRequest struct {
	Identity string `json:"identity"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

func login(tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var credentials loginRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&credentials); err != nil || strings.TrimSpace(credentials.Identity) == "" || credentials.Password == "" {
			invalidCredentials(response)
			return
		}
		identityKey := strings.TrimSpace(credentials.Identity)
		if !strings.Contains(identityKey, ":") {
			identityKey = "gatehouse:" + identityKey
		}
		err, token := tokens.Login(request.Context(), identityKey, []byte(credentials.Password))
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				invalidCredentials(response)
				return
			}
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		noStore(response)
		http.SetCookie(response, authenticationCookie(token))
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(loginResponse{AccessToken: token, TokenType: "Bearer"})
	}
}

func me(tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		err, claims := tokens.Authenticate(request.Context(), requestAuthorization(request))
		if err != nil {
			if errors.Is(err, auth.ErrUnauthenticated) {
				response.Header().Set("WWW-Authenticate", "Bearer")
				response.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		noStore(response)
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(claims)
	}
}

type workspaceResponse struct {
	ID   string  `json:"id"`
	Alias *string `json:"alias,omitempty"`
	Name *string `json:"name,omitempty"`
}

type groupResponse struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

type workspaceAgentResponse struct {
	ID    string  `json:"id"`
	Label *string `json:"label,omitempty"`
}

type sessionResponse struct {
	ID        string  `json:"id"`
	Name      *string `json:"name,omitempty"`
	Project   *projectResponse `json:"project,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type projectResponse struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

func projectResponseFromModel(project model.Project) projectResponse {
	return projectResponse{ID: project.Ref.Id, Name: project.Name, Description: project.Description, CreatedAt: project.CreatedAt}
}

func sessionResponses(ctx context.Context, store *database.Store, workspace model.WorkspaceRef, principal model.PrincipalRef, sessions []model.Session) (error, []sessionResponse) {
	refs := make([]model.ProjectRef, 0, len(sessions))
	for _, session := range sessions {
		if session.Project != nil {
			refs = append(refs, *session.Project)
		}
	}
	err, projects := store.ProjectsGetByRefs(ctx, workspace, principal, refs)
	if err != nil {
		return err, nil
	}
	projectByID := make(map[string]projectResponse, len(projects))
	for _, project := range projects {
		projectByID[project.Ref.Id] = projectResponseFromModel(project)
	}
	result := make([]sessionResponse, 0, len(sessions))
	for _, session := range sessions {
		entry := sessionResponse{ID: session.Ref.Id, Name: session.Name, CreatedAt: session.CreatedAt}
		if session.Project != nil {
			if project, ok := projectByID[session.Project.Id]; ok {
				entry.Project = &project
			}
		}
		result = append(result, entry)
	}
	return nil, result
}

type sessionSearchResponse struct {
	Sessions   []sessionResponse `json:"sessions"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

type projectSearchResponse struct {
	Projects   []projectResponse `json:"projects"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

type projectCreateRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Groups      []string `json:"groups"`
}

type projectUpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type sessionCreateRequest struct {
	Project *string `json:"project"`
}

type sessionProjectRequest struct {
	Project *string `json:"project"`
}

type sessionMessageRequest struct {
	Text        string   `json:"text"`
	Agent       string   `json:"agent"`
	Attachments []string `json:"attachments"`
}

type sessionFileCreateRequest struct {
	Name      string  `json:"name"`
	MediaType *string `json:"media_type"`
}

type sessionFileCreateResponse struct {
	File      model.SessionFile `json:"file"`
	UploadURL string            `json:"upload_url"`
}

type projectFileResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	MediaType   *string `json:"media_type,omitempty"`
	Size        *int64  `json:"size,omitempty"`
	Fingerprint *string `json:"fingerprint,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

type projectFileCreateResponse struct {
	File      projectFileResponse `json:"file"`
	UploadURL string              `json:"upload_url"`
}

type projectNoteRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Body        *string `json:"body"`
	Sensitive   *bool   `json:"sensitive"`
}

type projectSecretRequest struct {
	Description *string `json:"description"`
	Value       *string `json:"value"`
}

type projectSecretResponse struct {
	ID          string                    `json:"id"`
	Description string                    `json:"description"`
	Author      projectNoteAuthorResponse `json:"author"`
	CreatedAt   string                    `json:"created_at"`
	UpdatedAt   string                    `json:"updated_at"`
}

type sessionNoteRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Body        *string `json:"body"`
	Sensitive   *bool   `json:"sensitive"`
}

type projectNoteAuthorResponse struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

type projectNoteResponse struct {
	ID          string                    `json:"id"`
	Title       string                    `json:"title"`
	Description string                    `json:"description"`
	Body        *string                   `json:"body,omitempty"`
	Sensitive   bool                      `json:"sensitive"`
	Author      projectNoteAuthorResponse `json:"author"`
	CreatedAt   string                    `json:"created_at"`
}

type sessionNoteResponse struct {
	ID          string                    `json:"id"`
	Title       string                    `json:"title"`
	Description string                    `json:"description"`
	Body        *string                   `json:"body,omitempty"`
	Sensitive   bool                      `json:"sensitive"`
	Author      projectNoteAuthorResponse `json:"author"`
	CreatedAt   string                    `json:"created_at"`
}

type sessionSecretRequest struct {
	Description *string `json:"description"`
	Value       *string `json:"value"`
}

type sessionSecretResponse struct {
	ID          string                    `json:"id"`
	Description string                    `json:"description"`
	Author      projectNoteAuthorResponse `json:"author"`
	CreatedAt   string                    `json:"created_at"`
	UpdatedAt   string                    `json:"updated_at"`
}

type sessionEventTreeResponse struct {
	Event    model.SessionEvent           `json:"event"`
	Children []*sessionEventTreeResponse `json:"children"`
}

func workspaces(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		err, configured := store.WorkspacesGet(request.Context(), claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := make([]workspaceResponse, 0, len(configured))
		for _, workspace := range configured {
			result = append(result, workspaceResponse{ID: workspace.Ref.Id, Alias: workspace.Alias, Name: workspace.Name})
		}
		writeJSON(response, result)
	}
}

func workspaceGroups(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspace, ok := authorizedWorkspace(response, request, store, claims)
		if !ok {
			return
		}
		err, groups := store.WorkspaceGroupsGet(request.Context(), workspace, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := make([]groupResponse, 0, len(groups))
		for _, group := range groups {
			result = append(result, groupResponse{ID: group.Ref.Id, Name: group.Name})
		}
		writeJSON(response, result)
	}
}

func workspaceAgents(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspace, ok := authorizedWorkspace(response, request, store, claims)
		if !ok {
			return
		}
		err, agents := store.WorkspaceAgentsGet(request.Context(), workspace)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := make([]workspaceAgentResponse, len(agents))
		for index, agent := range agents {
			result[index] = workspaceAgentResponse{ID: agent.ID, Label: agent.Label}
		}
		writeJSON(response, result)
	}
}

func workspaceProjects(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspace, ok := authorizedWorkspace(response, request, store, claims)
		if !ok {
			return
		}
		switch request.Method {
		case http.MethodGet:
			name, cursor, limit, ok := catalogSearchParameters(response, request)
			if !ok {
				return
			}
			if cursor != "" && !typed_id.Valid(typed_id.Project, cursor) {
				http.Error(response, "invalid project cursor", http.StatusBadRequest)
				return
			}
			err, projects, nextCursor := store.ProjectsSearch(request.Context(), workspace, claims.Principal.Ref, database.ProjectSearch{Name: name, Cursor: cursor, Limit: limit})
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]projectResponse, 0, len(projects))
			for _, project := range projects {
				result = append(result, projectResponse{ID: project.Ref.Id, Name: project.Name, Description: project.Description, CreatedAt: project.CreatedAt})
			}
			writeJSON(response, projectSearchResponse{Projects: result, NextCursor: nextCursor})
		case http.MethodPost:
			var input projectCreateRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				http.Error(response, "invalid project", http.StatusBadRequest)
				return
			}
			id, err := typed_id.New(typed_id.Project)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			groups := make([]model.GroupRef, 0, len(input.Groups))
			for _, groupID := range input.Groups {
				if !typed_id.Valid(typed_id.Group, groupID) {
					http.Error(response, "invalid project group", http.StatusBadRequest)
					return
				}
				groups = append(groups, model.GroupRef{Workspace: workspace, Id: groupID})
			}
			err, project := store.ProjectsCreate(request.Context(), model.Project{Ref: model.ProjectRef{Workspace: workspace, Id: id}, Name: input.Name, Description: input.Description, Enabled: true}, claims.Principal.Ref, groups)
			if err != nil {
				http.Error(response, "project could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, projectResponse{ID: project.Ref.Id, Name: project.Name, Description: project.Description, CreatedAt: project.CreatedAt})
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceProject(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspace, ok := authorizedWorkspace(response, request, store, claims)
		if !ok {
			return
		}
		projectID := request.PathValue("project")
		if !typed_id.Valid(typed_id.Project, projectID) {
			http.NotFound(response, request)
			return
		}
		projectRef := model.ProjectRef{Workspace: workspace, Id: projectID}
		if request.Method == http.MethodGet {
			err, project := store.ProjectGet(request.Context(), projectRef, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if project == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, projectResponse{ID: project.Ref.Id, Name: project.Name, Description: project.Description, CreatedAt: project.CreatedAt})
			return
		}
		var input projectUpdateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(response, "invalid project", http.StatusBadRequest)
			return
		}
		err, current := store.ProjectGet(request.Context(), projectRef, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if current == nil {
			http.NotFound(response, request)
			return
		}
		name, description := input.Name, input.Description
		if name == nil {
			name = current.Name
		}
		if description == nil {
			description = current.Description
		}
		err, project := store.ProjectDetailsSet(request.Context(), projectRef, claims.Principal.Ref, name, description)
		if err != nil {
			http.Error(response, "project could not be updated", http.StatusBadRequest)
			return
		}
		if project == nil {
			http.NotFound(response, request)
			return
		}
		writeJSON(response, projectResponse{ID: project.Ref.Id, Name: project.Name, Description: project.Description, CreatedAt: project.CreatedAt})
	}
}

func workspaceSessions(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			workspaceSessionsGet(store, tokens, response, request)
		case http.MethodPost:
			workspaceSessionsCreate(store, tokens, response, request)
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceSession(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspaceID := request.PathValue("workspace")
		sessionID := request.PathValue("session")
		if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) {
			http.NotFound(response, request)
			return
		}
		err, session := store.SessionGet(request.Context(), model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if session == nil {
			http.NotFound(response, request)
			return
		}
		err, entries := sessionResponses(request.Context(), store, model.WorkspaceRef{Id: workspaceID}, claims.Principal.Ref, []model.Session{*session})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(response, entries[0])
	}
}

func workspaceSessionsGet(store *database.Store, tokens *auth.BearerTokens, response http.ResponseWriter, request *http.Request) {
	claims, ok := authenticate(response, request, tokens)
	if !ok {
		return
	}
	workspaceID := request.PathValue("workspace")
	if workspaceID == "" {
		http.NotFound(response, request)
		return
	}
	name, cursor, limit, ok := catalogSearchParameters(response, request)
	if !ok {
		return
	}
	if cursor != "" && !typed_id.Valid(typed_id.Session, cursor) {
		http.Error(response, "invalid session cursor", http.StatusBadRequest)
		return
	}
	projectID := request.URL.Query().Get("project")
	if projectID != "" && !typed_id.Valid(typed_id.Project, projectID) {
		http.Error(response, "invalid project", http.StatusBadRequest)
		return
	}
	err, sessions, nextCursor := store.SessionsSearch(
		request.Context(),
		model.WorkspaceRef{Id: workspaceID},
		claims.Principal.Ref,
		database.SessionSearch{Name: name, Project: projectID, Cursor: cursor, Limit: limit},
	)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return
	}
	err, result := sessionResponses(request.Context(), store, model.WorkspaceRef{Id: workspaceID}, claims.Principal.Ref, sessions)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(response, sessionSearchResponse{Sessions: result, NextCursor: nextCursor})
}

func catalogSearchParameters(response http.ResponseWriter, request *http.Request) (string, string, int, bool) {
	name := strings.TrimSpace(request.URL.Query().Get("name"))
	if len(name) > 256 {
		http.Error(response, "search name is too long", http.StatusBadRequest)
		return "", "", 0, false
	}
	limit := 50
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			http.Error(response, "limit must be between 1 and 100", http.StatusBadRequest)
			return "", "", 0, false
		}
		limit = parsed
	}
	return name, request.URL.Query().Get("cursor"), limit, true
}

func workspaceSessionsCreate(store *database.Store, tokens *auth.BearerTokens, response http.ResponseWriter, request *http.Request) {
	claims, ok := authenticate(response, request, tokens)
	if !ok {
		return
	}
	workspace, ok := authorizedWorkspace(response, request, store, claims)
	if !ok {
		return
	}
	var input sessionCreateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil && err != io.EOF {
		http.Error(response, "invalid session", http.StatusBadRequest)
		return
	}
	var project *model.ProjectRef
	if input.Project != nil {
		if !typed_id.Valid(typed_id.Project, *input.Project) {
			http.Error(response, "invalid project", http.StatusBadRequest)
			return
		}
		project = &model.ProjectRef{Workspace: workspace, Id: *input.Project}
	}
	id, err := typed_id.New(typed_id.Session)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return
	}
	principal := claims.Principal.Ref
	session := model.Session{Ref: model.SessionRef{Workspace: workspace, Id: id}, Project: project, AuthorPrincipal: &principal, Enabled: true}
	err, stored := store.SessionsCreate(request.Context(), session, principal)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return
	}
	err, entries := sessionResponses(request.Context(), store, workspace, principal, []model.Session{stored})
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(response, http.StatusCreated, entries[0])
}

func workspaceSessionProject(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPatch {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspaceID, sessionID := request.PathValue("workspace"), request.PathValue("session")
		if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) {
			http.NotFound(response, request)
			return
		}
		var input sessionProjectRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(response, "invalid session project", http.StatusBadRequest)
			return
		}
		var project *model.ProjectRef
		if input.Project != nil {
			if !typed_id.Valid(typed_id.Project, *input.Project) {
				http.Error(response, "invalid project", http.StatusBadRequest)
				return
			}
			project = &model.ProjectRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: *input.Project}
		}
		err, stored := store.SessionProjectSet(request.Context(), model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}, project, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "session project could not be updated", http.StatusBadRequest)
			return
		}
		if stored == nil {
			http.NotFound(response, request)
			return
		}
		err, entries := sessionResponses(request.Context(), store, model.WorkspaceRef{Id: workspaceID}, claims.Principal.Ref, []model.Session{*stored})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(response, entries[0])
	}
}

func workspaceActivity(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspace, ok := authorizedWorkspace(response, request, store, claims)
		if !ok {
			return
		}
		var input model.ActivityTopicCheckpoints
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || len(input.Topics) == 0 || len(input.Topics) > 32 {
			http.Error(response, "invalid activity topics", http.StatusBadRequest)
			return
		}
		seenTopics := make(map[string]struct{}, len(input.Topics))
		checkpoints := make([]database.ActivityTopicCheckpoint, 0, len(input.Topics))
		for _, topic := range input.Topics {
			if strings.TrimSpace(topic.Topic) == "" {
				http.Error(response, "invalid activity topic", http.StatusBadRequest)
				return
			}
			if _, exists := seenTopics[topic.Topic]; exists {
				http.Error(response, "duplicate activity topic", http.StatusBadRequest)
				return
			}
			seenTopics[topic.Topic] = struct{}{}
			checkpoint := database.ActivityTopicCheckpoint{Topic: topic.Topic}
			if topic.Cursor != nil {
				if !typed_id.Valid(typed_id.ActivityEvent, topic.Cursor.Id) {
					http.Error(response, "invalid activity cursor", http.StatusBadRequest)
					return
				}
				checkpoint.ID = topic.Cursor.Id
			}
			checkpoints = append(checkpoints, checkpoint)
		}
		err, advanced := store.ActivityTopicCheckpointsGet(request.Context(), workspace, claims.Principal.Ref, checkpoints)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := make([]model.ActivityTopicCheckpoint, 0, len(advanced))
		for _, checkpoint := range advanced {
			entry := model.ActivityTopicCheckpoint{Topic: checkpoint.Topic}
			if checkpoint.ID != "" {
				entry.Cursor = &model.ActivityCursor{Id: checkpoint.ID}
			}
			result = append(result, entry)
		}
		writeJSON(response, model.ActivityTopicCheckpoints{Topics: result})
	}
}

func workspaceSessionMessages(store *database.Store, tokens *auth.BearerTokens, dispatcher ReplyDispatcher) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspaceID := request.PathValue("workspace")
		sessionID := request.PathValue("session")
		if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) {
			http.NotFound(response, request)
			return
		}
		session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
		err, authorized := store.SessionGet(request.Context(), session, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if authorized == nil {
			http.NotFound(response, request)
			return
		}

		var message sessionMessageRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&message); err != nil || (strings.TrimSpace(message.Text) == "" && len(message.Attachments) == 0) {
			http.Error(response, "invalid message", http.StatusBadRequest)
			return
		}
		if message.Agent != "" {
			err, selected := store.WorkspaceAgentModelSelect(request.Context(), session.Workspace, message.Agent)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if selected == nil || selected.Ref.Model.Id != message.Agent {
				http.Error(response, "invalid agent", http.StatusBadRequest)
				return
			}
		}
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		event := model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Kind:            "message.text",
			AuthorPrincipal: &claims.Principal,
			Payload:         messagePayload(message),
		}
		err, stored := store.SessionMessagesCreate(request.Context(), event)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		err, hydrated := store.SessionEventAttachmentsHydrate(request.Context(), session, []model.SessionEvent{stored})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if dispatcher != nil {
			_ = dispatcher.Reconcile()
		}
		writeJSONStatus(response, http.StatusAccepted, hydrated[0])
	}
}

func workspaceSessionFiles(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspaceID := request.PathValue("workspace")
		sessionID := request.PathValue("session")
		if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) {
			http.NotFound(response, request)
			return
		}
		session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
		err, authorized := store.SessionGet(request.Context(), session, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if authorized == nil {
			http.NotFound(response, request)
			return
		}
		var input sessionFileCreateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" || (input.MediaType != nil && strings.TrimSpace(*input.MediaType) == "") {
			http.Error(response, "invalid session file", http.StatusBadRequest)
			return
		}
		fileID, err := typed_id.New(typed_id.SessionFile)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		storageObjectID, err := typed_id.New(typed_id.StorageObject)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		created := model.SessionFile{Ref: model.SessionFileRef{Session: session, Id: fileID}, Name: input.Name, MediaType: input.MediaType}
		err, stored, objectID := store.SessionFileCreate(request.Context(), created, storageObjectID, claims.Principal.Ref)
		if err != nil {
			if strings.Contains(err.Error(), "no available storage provider") {
				http.Error(response, "no storage provider available", http.StatusServiceUnavailable)
				return
			}
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		err, token := storageToken(request.Context(), tokens, objectID, "put", 15*time.Minute)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSONStatus(response, http.StatusCreated, sessionFileCreateResponse{File: stored, UploadURL: storageURL(request, token)})
	}
}

func workspaceSessionFileFinish(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		file, object, ok := authorizedSessionFile(response, request, store, claims, request.PathValue("file"))
		if !ok || file == nil || object == nil {
			return
		}
		if err := tokens.StorageClient().Finish(request.Context(), object.ID); err != nil {
			http.Error(response, "storage object is not ready", http.StatusConflict)
			return
		}
		writeJSON(response, file)
	}
}

func workspaceSessionFileDownload(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		file, object, ok := authorizedSessionFile(response, request, store, claims, request.PathValue("file"))
		if !ok || file == nil || object == nil {
			return
		}
		if object.State != "success" {
			http.Error(response, "storage object is not ready", http.StatusConflict)
			return
		}
		err, token := storageToken(request.Context(), tokens, object.ID, "get", 5*time.Minute)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		noStore(response)
		http.Redirect(response, request, storageURL(request, token), http.StatusTemporaryRedirect)
	}
}

func workspaceProjectFiles(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		project, ok := authorizedProject(response, request, store, claims)
		if !ok {
			return
		}
		err, files := store.ProjectFilesGet(request.Context(), project, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := make([]projectFileResponse, 0, len(files))
		for _, file := range files {
			createdAt, err := typed_id.Timestamp(typed_id.ProjectFile, file.ID)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			size, fingerprint := file.Size, file.Fingerprint
			result = append(result, projectFileResponse{
				ID: file.ID, Name: file.Name, MediaType: file.MediaType, Size: &size, Fingerprint: &fingerprint,
				CreatedAt: createdAt.Format("2006-01-02T15:04:05.000Z"),
			})
		}
		writeJSON(response, result)
	}
}

func workspaceProjectNotes(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		project, ok := authorizedProject(response, request, store, claims)
		if !ok {
			return
		}
		switch request.Method {
		case http.MethodGet:
			err, notes := store.ProjectNotesGet(request.Context(), project, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]projectNoteResponse, 0, len(notes))
			for _, note := range notes {
				result = append(result, projectNoteResponseFromSummary(note))
			}
			writeJSON(response, result)
		case http.MethodPost:
			var input projectNoteRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validProjectNoteRequest(input, true) {
				http.Error(response, "invalid project note", http.StatusBadRequest)
				return
			}
			id, err := typed_id.New(typed_id.ProjectNote)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			description, body := "", ""
			if input.Description != nil {
				description = *input.Description
			}
			if input.Body != nil {
				body = *input.Body
			}
			sensitive := input.Sensitive != nil && *input.Sensitive
			note := model.ProjectNote{Ref: model.ProjectNoteRef{Project: project, Id: id}, Title: *input.Title, Description: description, Body: body, Sensitive: sensitive}
			err, stored := store.ProjectNoteCreate(request.Context(), note, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "project note could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, projectNoteResponse{ID: stored.Ref.Id, Title: stored.Title, Description: stored.Description, Body: &stored.Body, Sensitive: stored.Sensitive, Author: projectNoteAuthorResponse{ID: stored.AuthorPrincipal.Id, Name: claims.Principal.Name}, CreatedAt: stored.CreatedAt})
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceProjectNote(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch && request.Method != http.MethodDelete {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		note, ok := projectNoteRef(response, request)
		if !ok {
			return
		}
		err, current := store.ProjectNoteGet(request.Context(), note, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if current == nil {
			http.NotFound(response, request)
			return
		}
		switch request.Method {
		case http.MethodGet:
			writeJSON(response, projectNoteResponseFromDetail(*current))
		case http.MethodPatch:
			var input projectNoteRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validProjectNoteRequest(input, false) {
				http.Error(response, "invalid project note", http.StatusBadRequest)
				return
			}
			title, description, body := current.Note.Title, current.Note.Description, current.Note.Body
			if input.Title != nil {
				title = *input.Title
			}
			if input.Description != nil {
				description = *input.Description
			}
			if input.Body != nil {
				body = *input.Body
			}
			err, updated := store.ProjectNoteDetailsSet(request.Context(), note, claims.Principal.Ref, &title, &description, &body)
			if err != nil {
				http.Error(response, "project note could not be updated", http.StatusBadRequest)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, projectNoteResponseFromDetail(*updated))
		case http.MethodDelete:
			err, removed := store.ProjectNoteRemove(request.Context(), note, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if !removed {
				http.NotFound(response, request)
				return
			}
			noStore(response)
			response.WriteHeader(http.StatusNoContent)
		}
	}
}

func workspaceProjectSecrets(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		project, ok := authorizedProject(response, request, store, claims)
		if !ok {
			return
		}
		noStore(response)
		switch request.Method {
		case http.MethodGet:
			err, secrets := store.ProjectSecretsGet(request.Context(), project, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]projectSecretResponse, 0, len(secrets))
			for _, secret := range secrets {
				result = append(result, projectSecretResponseFromSummary(secret))
			}
			writeJSON(response, result)
		case http.MethodPost:
			var input projectSecretRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validProjectSecretRequest(input, true) {
				http.Error(response, "invalid project secret", http.StatusBadRequest)
				return
			}
			id, err := typed_id.New(typed_id.ProjectSecret)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			ref := model.ProjectSecretRef{Project: project, Id: id}
			value := []byte(*input.Value)
			err, ciphertext := tokens.Encrypt(request.Context(), database.ProjectSecretAssociatedData(ref), value)
			clear(value)
			if err != nil {
				http.Error(response, "project secret could not be encrypted", http.StatusInternalServerError)
				return
			}
			secret := model.ProjectSecret{Ref: ref, Description: *input.Description, Ciphertext: ciphertext}
			err, stored := store.ProjectSecretCreate(request.Context(), secret, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "project secret could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, projectSecretResponse{ID: stored.Ref.Id, Description: stored.Description, Author: projectNoteAuthorResponse{ID: stored.AuthorPrincipal.Id, Name: claims.Principal.Name}, CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt})
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceProjectSecret(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch && request.Method != http.MethodDelete {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		secret, ok := projectSecretRef(response, request)
		if !ok {
			return
		}
		err, current := store.ProjectSecretGet(request.Context(), secret, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if current == nil {
			http.NotFound(response, request)
			return
		}
		noStore(response)
		switch request.Method {
		case http.MethodGet:
			writeJSON(response, projectSecretResponseFromDetail(*current))
		case http.MethodPatch:
			var input projectSecretRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validProjectSecretRequest(input, false) {
				http.Error(response, "invalid project secret", http.StatusBadRequest)
				return
			}
			description, ciphertext := current.Secret.Description, current.Secret.Ciphertext
			if input.Description != nil {
				description = *input.Description
			}
			if input.Value != nil {
				value := []byte(*input.Value)
				err, ciphertext = tokens.Encrypt(request.Context(), database.ProjectSecretAssociatedData(secret), value)
				clear(value)
				if err != nil {
					http.Error(response, "project secret could not be encrypted", http.StatusInternalServerError)
					return
				}
			}
			err, updated := store.ProjectSecretDetailsSet(request.Context(), secret, claims.Principal.Ref, &description, &ciphertext)
			if err != nil {
				http.Error(response, "project secret could not be updated", http.StatusBadRequest)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, projectSecretResponseFromDetail(*updated))
		case http.MethodDelete:
			err, removed := store.ProjectSecretRemove(request.Context(), secret, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if !removed {
				http.NotFound(response, request)
				return
			}
			response.WriteHeader(http.StatusNoContent)
		}
	}
}

func workspaceSessionNotes(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		session, ok := authorizedSession(response, request, store, claims)
		if !ok {
			return
		}
		switch request.Method {
		case http.MethodGet:
			err, notes := store.SessionNotesGet(request.Context(), session, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]sessionNoteResponse, 0, len(notes))
			for _, note := range notes {
				result = append(result, sessionNoteResponseFromSummary(note))
			}
			writeJSON(response, result)
		case http.MethodPost:
			var input sessionNoteRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validSessionNoteRequest(input, true) {
				http.Error(response, "invalid session note", http.StatusBadRequest)
				return
			}
			id, err := typed_id.New(typed_id.SessionNote)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			description, body := "", ""
			if input.Description != nil {
				description = *input.Description
			}
			if input.Body != nil {
				body = *input.Body
			}
			sensitive := input.Sensitive != nil && *input.Sensitive
			note := model.SessionNote{Ref: model.SessionNoteRef{Session: session, Id: id}, Title: *input.Title, Description: description, Body: body, Sensitive: sensitive}
			err, stored := store.SessionNoteCreate(request.Context(), note, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "session note could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, sessionNoteResponse{ID: stored.Ref.Id, Title: stored.Title, Description: stored.Description, Body: &stored.Body, Sensitive: stored.Sensitive, Author: projectNoteAuthorResponse{ID: stored.AuthorPrincipal.Id, Name: claims.Principal.Name}, CreatedAt: stored.CreatedAt})
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceSessionNote(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch && request.Method != http.MethodDelete {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		note, ok := sessionNoteRef(response, request)
		if !ok {
			return
		}
		err, current := store.SessionNoteGet(request.Context(), note, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if current == nil {
			http.NotFound(response, request)
			return
		}
		switch request.Method {
		case http.MethodGet:
			writeJSON(response, sessionNoteResponseFromDetail(*current))
		case http.MethodPatch:
			var input sessionNoteRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validSessionNoteRequest(input, false) {
				http.Error(response, "invalid session note", http.StatusBadRequest)
				return
			}
			title, description, body := current.Note.Title, current.Note.Description, current.Note.Body
			if input.Title != nil {
				title = *input.Title
			}
			if input.Description != nil {
				description = *input.Description
			}
			if input.Body != nil {
				body = *input.Body
			}
			err, updated := store.SessionNoteDetailsSet(request.Context(), note, claims.Principal.Ref, &title, &description, &body)
			if err != nil {
				http.Error(response, "session note could not be updated", http.StatusBadRequest)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, sessionNoteResponseFromDetail(*updated))
		case http.MethodDelete:
			err, removed := store.SessionNoteRemove(request.Context(), note, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if !removed {
				http.NotFound(response, request)
				return
			}
			noStore(response)
			response.WriteHeader(http.StatusNoContent)
		}
	}
}

func workspaceSessionSecrets(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		session, ok := authorizedSession(response, request, store, claims)
		if !ok {
			return
		}
		noStore(response)
		switch request.Method {
		case http.MethodGet:
			err, secrets := store.SessionSecretsGet(request.Context(), session, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]sessionSecretResponse, 0, len(secrets))
			for _, secret := range secrets {
				result = append(result, sessionSecretResponseFromSummary(secret))
			}
			writeJSON(response, result)
		case http.MethodPost:
			var input sessionSecretRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validSessionSecretRequest(input, true) {
				http.Error(response, "invalid session secret", http.StatusBadRequest)
				return
			}
			id, err := typed_id.New(typed_id.SessionSecret)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			ref := model.SessionSecretRef{Session: session, Id: id}
			value := []byte(*input.Value)
			err, ciphertext := tokens.Encrypt(request.Context(), database.SessionSecretAssociatedData(ref), value)
			clear(value)
			if err != nil {
				http.Error(response, "session secret could not be encrypted", http.StatusInternalServerError)
				return
			}
			secret := model.SessionSecret{Ref: ref, Description: *input.Description, Ciphertext: ciphertext}
			err, stored := store.SessionSecretCreate(request.Context(), secret, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "session secret could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, sessionSecretResponse{ID: stored.Ref.Id, Description: stored.Description, Author: projectNoteAuthorResponse{ID: stored.AuthorPrincipal.Id, Name: claims.Principal.Name}, CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt})
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceSessionSecret(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch && request.Method != http.MethodDelete {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		secret, ok := sessionSecretRef(response, request)
		if !ok {
			return
		}
		err, current := store.SessionSecretGet(request.Context(), secret, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if current == nil {
			http.NotFound(response, request)
			return
		}
		noStore(response)
		switch request.Method {
		case http.MethodGet:
			writeJSON(response, sessionSecretResponseFromDetail(*current))
		case http.MethodPatch:
			var input sessionSecretRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validSessionSecretRequest(input, false) {
				http.Error(response, "invalid session secret", http.StatusBadRequest)
				return
			}
			description, ciphertext := current.Secret.Description, current.Secret.Ciphertext
			if input.Description != nil {
				description = *input.Description
			}
			if input.Value != nil {
				value := []byte(*input.Value)
				err, ciphertext = tokens.Encrypt(request.Context(), database.SessionSecretAssociatedData(secret), value)
				clear(value)
				if err != nil {
					http.Error(response, "session secret could not be encrypted", http.StatusInternalServerError)
					return
				}
			}
			err, updated := store.SessionSecretDetailsSet(request.Context(), secret, claims.Principal.Ref, &description, &ciphertext)
			if err != nil {
				http.Error(response, "session secret could not be updated", http.StatusBadRequest)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, sessionSecretResponseFromDetail(*updated))
		case http.MethodDelete:
			err, removed := store.SessionSecretRemove(request.Context(), secret, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if !removed {
				http.NotFound(response, request)
				return
			}
			response.WriteHeader(http.StatusNoContent)
		}
	}
}

func workspaceProjectFileStart(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		project, ok := authorizedProject(response, request, store, claims)
		if !ok {
			return
		}
		var input sessionFileCreateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" || (input.MediaType != nil && strings.TrimSpace(*input.MediaType) == "") {
			http.Error(response, "invalid project file", http.StatusBadRequest)
			return
		}
		fileID, err := typed_id.New(typed_id.ProjectFile)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		storageObjectID, err := typed_id.New(typed_id.StorageObject)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		created := model.ProjectFile{Ref: model.ProjectFileRef{Project: project, Id: fileID}, Name: input.Name, MediaType: input.MediaType, Enabled: true}
		err, stored, objectID := store.ProjectFileCreate(request.Context(), created, storageObjectID, claims.Principal.Ref)
		if err != nil {
			if strings.Contains(err.Error(), "no available storage provider") {
				http.Error(response, "no storage provider available", http.StatusServiceUnavailable)
				return
			}
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		err, token := storageToken(request.Context(), tokens, objectID, "put", 15*time.Minute)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSONStatus(response, http.StatusCreated, projectFileCreateResponse{File: projectFileResponseFromModel(stored, nil), UploadURL: storageURL(request, token)})
	}
}

func workspaceProjectFileFinish(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		file, object, ok := authorizedProjectFile(response, request, store, claims, request.PathValue("file"))
		if !ok || file == nil || object == nil {
			return
		}
		if err := tokens.StorageClient().Finish(request.Context(), object.ID); err != nil {
			http.Error(response, "storage object is not ready", http.StatusConflict)
			return
		}
		err, file, object := store.ProjectFileFinish(request.Context(), file.Ref, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if file == nil || object == nil {
			http.NotFound(response, request)
			return
		}
		writeJSON(response, projectFileResponseFromModel(*file, object))
	}
}

func workspaceProjectFileDownload(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		file, object, ok := authorizedProjectFile(response, request, store, claims, request.PathValue("file"))
		if !ok || file == nil || object == nil {
			return
		}
		if object.State != "success" {
			http.Error(response, "storage object is not ready", http.StatusConflict)
			return
		}
		err, content := tokens.StorageClient().Get(request.Context(), object.ID)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if content == nil {
			http.NotFound(response, request)
			return
		}
		defer content.Close()
		noStore(response)
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Name})
		if disposition == "" {
			disposition = "attachment"
		}
		response.Header().Set("Content-Disposition", disposition)
		response.Header().Set("Content-Type", "application/octet-stream")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.Copy(response, content)
	}
}

func workspaceProjectFile(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		project, fileID, ok := projectFileRef(response, request)
		if !ok {
			return
		}
		err, removed := store.ProjectFileRemove(request.Context(), model.ProjectFileRef{Project: project, Id: fileID}, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if !removed {
			http.NotFound(response, request)
			return
		}
		noStore(response)
		response.WriteHeader(http.StatusNoContent)
	}
}

func storageProxy(tokens *auth.BearerTokens) http.HandlerFunc {
	objects := tokens.StorageClient()
	return func(response http.ResponseWriter, request *http.Request) {
		encoded := request.URL.Query().Get("token")
		if encoded == "" {
			http.NotFound(response, request)
			return
		}
		err, token := tokens.AuthenticateStorageToken(request.Context(), encoded)
		if err != nil || !validStorageToken(token) {
			http.NotFound(response, request)
			return
		}
		switch {
		case request.Method == http.MethodPut && token.Action == "put":
			if err := objects.Put(request.Context(), token.ID, request.Body, request.ContentLength); err != nil {
				http.Error(response, "storage upload failed", http.StatusConflict)
				return
			}
			noStore(response)
			response.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && token.Action == "get":
			err, content := objects.Get(request.Context(), token.ID)
			if err != nil || content == nil {
				http.NotFound(response, request)
				return
			}
			defer content.Close()
			noStore(response)
			_, _ = io.Copy(response, content)
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func messagePayload(message sessionMessageRequest) map[string]interface{} {
	payload := map[string]interface{}{}
	if strings.TrimSpace(message.Text) != "" {
		payload["text"] = message.Text
	}
	if message.Agent != "" {
		payload["agent"] = message.Agent
	}
	if len(message.Attachments) > 0 {
		payload["attachments"] = message.Attachments
	}
	return payload
}

func workspaceSessionMessageCancel(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspaceID := request.PathValue("workspace")
		sessionID := request.PathValue("session")
		messageID := request.PathValue("event")
		if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) || !typed_id.Valid(typed_id.SessionEvent, messageID) {
			http.NotFound(response, request)
			return
		}
		session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
		err, authorized := store.SessionGet(request.Context(), session, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if authorized == nil {
			http.NotFound(response, request)
			return
		}
		parent := model.SessionEventRef{Session: session, Id: messageID}
		err, message := store.SessionEventGet(request.Context(), parent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if message == nil || message.Parent != nil || message.Kind != "message.text" || message.AuthorPrincipal == nil {
			http.NotFound(response, request)
			return
		}
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		event := model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Parent:          &parent,
			Kind:            "cancel.request",
			AuthorPrincipal: &claims.Principal,
			Payload:         map[string]interface{}{},
		}
		err, stored := store.SessionEventsCreate(request.Context(), event)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSONStatus(response, http.StatusAccepted, stored)
	}
}

type sessionApprovalRequest struct {
	Decision string `json:"decision"`
}

func workspaceSessionApproval(store *database.Store, tokens *auth.BearerTokens, dispatcher ReplyDispatcher) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspaceID := request.PathValue("workspace")
		sessionID := request.PathValue("session")
		approvalID := request.PathValue("approval")
		if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) || !typed_id.Valid(typed_id.SessionEvent, approvalID) {
			http.NotFound(response, request)
			return
		}
		session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
		err, authorized := store.SessionGet(request.Context(), session, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if authorized == nil {
			http.NotFound(response, request)
			return
		}
		var approvalInput sessionApprovalRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&approvalInput); err != nil || (approvalInput.Decision != "approved" && approvalInput.Decision != "rejected") {
			http.Error(response, "invalid approval decision", http.StatusBadRequest)
			return
		}
		approvalRef := model.SessionEventRef{Session: session, Id: approvalID}
		err, approval := store.SessionEventGet(request.Context(), approvalRef)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if approval == nil || approval.Kind != "approval.request" || approval.AuthorAgent == nil || approval.Parent == nil {
			http.NotFound(response, request)
			return
		}
		err, tool := store.SessionEventGet(request.Context(), *approval.Parent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if tool == nil || tool.Kind != "tool.request" || tool.AuthorAgent == nil {
			http.NotFound(response, request)
			return
		}
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		kind := "approval.approved"
		if approvalInput.Decision == "rejected" {
			kind = "approval.rejected"
		}
		event := model.SessionEvent{
			Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &approvalRef, Kind: kind, AuthorPrincipal: &claims.Principal, Payload: map[string]interface{}{},
		}
		err, stored := store.SessionApprovalResponseCreate(request.Context(), event)
		if errors.Is(err, database.ErrSessionApprovalResolved) {
			http.Error(response, "approval is already resolved", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if dispatcher != nil {
			_ = dispatcher.Reconcile()
		}
		writeJSONStatus(response, http.StatusAccepted, stored)
	}
}

func workspaceSessionEvents(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		workspaceID := request.PathValue("workspace")
		sessionID := request.PathValue("session")
		if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) {
			http.NotFound(response, request)
			return
		}
		session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
		err, authorized := store.SessionGet(request.Context(), session, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if authorized == nil {
			http.NotFound(response, request)
			return
		}
		afterID := request.URL.Query().Get("after_id")
		if afterID != "" && !typed_id.Valid(typed_id.SessionEvent, afterID) {
			http.Error(response, "invalid event cursor", http.StatusBadRequest)
			return
		}
		limit := 100
		if encodedLimit := request.URL.Query().Get("limit"); encodedLimit != "" {
			parsed, err := strconv.Atoi(encodedLimit)
			if err != nil || parsed < 1 || parsed > 100 {
				http.Error(response, "invalid event limit", http.StatusBadRequest)
				return
			}
			limit = parsed
		}
		err, entries := store.SessionEventsTreePageGet(request.Context(), session, afterID, limit)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		events := make([]model.SessionEvent, len(entries))
		for index, entry := range entries {
			events[index] = entry.Event
		}
		err, hydrated := store.SessionEventAttachmentsHydrate(request.Context(), session, events)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		for index := range entries {
			entries[index].Event = hydrated[index]
		}
		writeJSON(response, sessionEventTrees(entries))
	}
}

func sessionEventTrees(entries []database.SessionEventTreeEntry) []*sessionEventTreeResponse {
	trees := make([]*sessionEventTreeResponse, 0)
	stack := make([]*sessionEventTreeResponse, 0)
	for _, entry := range entries {
		node := &sessionEventTreeResponse{Event: entry.Event, Children: []*sessionEventTreeResponse{}}
		if entry.Depth == 0 {
			trees = append(trees, node)
			stack = []*sessionEventTreeResponse{node}
			continue
		}
		parent := stack[entry.Depth-1]
		parent.Children = append(parent.Children, node)
		stack = append(stack[:entry.Depth], node)
	}
	return trees
}

func authenticate(response http.ResponseWriter, request *http.Request, tokens *auth.BearerTokens) (auth.Claims, bool) {
	err, claims := tokens.Authenticate(request.Context(), requestAuthorization(request))
	if err == nil {
		return claims, true
	}
	if errors.Is(err, auth.ErrUnauthenticated) {
		response.Header().Set("WWW-Authenticate", "Bearer")
		response.WriteHeader(http.StatusUnauthorized)
	} else {
		http.Error(response, "internal server error", http.StatusInternalServerError)
	}
	return auth.Claims{}, false
}

func authorizedWorkspace(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims) (model.WorkspaceRef, bool) {
	workspaceID := request.PathValue("workspace")
	if workspaceID == "" {
		http.NotFound(response, request)
		return model.WorkspaceRef{}, false
	}
	workspace := model.WorkspaceRef{Id: workspaceID}
	err, configured := store.WorkspaceGet(request.Context(), workspace, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return model.WorkspaceRef{}, false
	}
	if configured == nil {
		http.NotFound(response, request)
		return model.WorkspaceRef{}, false
	}
	return workspace, true
}

func authorizedProject(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims) (model.ProjectRef, bool) {
	workspaceID := request.PathValue("workspace")
	projectID := request.PathValue("project")
	if workspaceID == "" || !typed_id.Valid(typed_id.Project, projectID) {
		http.NotFound(response, request)
		return model.ProjectRef{}, false
	}
	project := model.ProjectRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: projectID}
	err, available := store.ProjectGet(request.Context(), project, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return model.ProjectRef{}, false
	}
	if available == nil {
		http.NotFound(response, request)
		return model.ProjectRef{}, false
	}
	return project, true
}

func authorizedSession(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims) (model.SessionRef, bool) {
	workspaceID := request.PathValue("workspace")
	sessionID := request.PathValue("session")
	if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) {
		http.NotFound(response, request)
		return model.SessionRef{}, false
	}
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
	err, available := store.SessionGet(request.Context(), session, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return model.SessionRef{}, false
	}
	if available == nil {
		http.NotFound(response, request)
		return model.SessionRef{}, false
	}
	return session, true
}

func projectFileRef(response http.ResponseWriter, request *http.Request) (model.ProjectRef, string, bool) {
	workspaceID := request.PathValue("workspace")
	projectID := request.PathValue("project")
	fileID := request.PathValue("file")
	if workspaceID == "" || !typed_id.Valid(typed_id.Project, projectID) || !typed_id.Valid(typed_id.ProjectFile, fileID) {
		http.NotFound(response, request)
		return model.ProjectRef{}, "", false
	}
	return model.ProjectRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: projectID}, fileID, true
}

func projectNoteRef(response http.ResponseWriter, request *http.Request) (model.ProjectNoteRef, bool) {
	workspaceID := request.PathValue("workspace")
	projectID := request.PathValue("project")
	noteID := request.PathValue("note")
	if workspaceID == "" || !typed_id.Valid(typed_id.Project, projectID) || !typed_id.Valid(typed_id.ProjectNote, noteID) {
		http.NotFound(response, request)
		return model.ProjectNoteRef{}, false
	}
	return model.ProjectNoteRef{Project: model.ProjectRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: projectID}, Id: noteID}, true
}

func projectSecretRef(response http.ResponseWriter, request *http.Request) (model.ProjectSecretRef, bool) {
	workspaceID := request.PathValue("workspace")
	projectID := request.PathValue("project")
	secretID := request.PathValue("secret")
	if workspaceID == "" || !typed_id.Valid(typed_id.Project, projectID) || !typed_id.Valid(typed_id.ProjectSecret, secretID) {
		http.NotFound(response, request)
		return model.ProjectSecretRef{}, false
	}
	return model.ProjectSecretRef{Project: model.ProjectRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: projectID}, Id: secretID}, true
}

func sessionNoteRef(response http.ResponseWriter, request *http.Request) (model.SessionNoteRef, bool) {
	workspaceID := request.PathValue("workspace")
	sessionID := request.PathValue("session")
	noteID := request.PathValue("note")
	if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) || !typed_id.Valid(typed_id.SessionNote, noteID) {
		http.NotFound(response, request)
		return model.SessionNoteRef{}, false
	}
	return model.SessionNoteRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}, Id: noteID}, true
}

func sessionSecretRef(response http.ResponseWriter, request *http.Request) (model.SessionSecretRef, bool) {
	workspaceID := request.PathValue("workspace")
	sessionID := request.PathValue("session")
	secretID := request.PathValue("secret")
	if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) || !typed_id.Valid(typed_id.SessionSecret, secretID) {
		http.NotFound(response, request)
		return model.SessionSecretRef{}, false
	}
	return model.SessionSecretRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}, Id: secretID}, true
}

func validProjectNoteRequest(input projectNoteRequest, required bool) bool {
	if required && input.Title == nil {
		return false
	}
	if !required && input.Sensitive != nil {
		return false
	}
	if !required && input.Title == nil && input.Description == nil && input.Body == nil {
		return false
	}
	return (input.Title == nil || len(*input.Title) <= 256) && (input.Description == nil || len(*input.Description) <= 4*1024) && (input.Body == nil || len(*input.Body) <= 1024*1024)
}

func validSessionNoteRequest(input sessionNoteRequest, required bool) bool {
	if !required && input.Sensitive != nil {
		return false
	}
	return validProjectNoteRequest(projectNoteRequest{Title: input.Title, Description: input.Description, Body: input.Body}, required)
}

func validSessionSecretRequest(input sessionSecretRequest, required bool) bool {
	if required && (input.Description == nil || input.Value == nil) {
		return false
	}
	if !required && input.Description == nil && input.Value == nil {
		return false
	}
	return (input.Description == nil || len(*input.Description) <= 4*1024) && (input.Value == nil || len(*input.Value) <= 1024*1024)
}

func validProjectSecretRequest(input projectSecretRequest, required bool) bool {
	if required && (input.Description == nil || input.Value == nil) {
		return false
	}
	if !required && input.Description == nil && input.Value == nil {
		return false
	}
	return (input.Description == nil || len(*input.Description) <= 4*1024) && (input.Value == nil || len(*input.Value) <= 1024*1024)
}

func projectNoteResponseFromSummary(note database.ProjectNoteSummary) projectNoteResponse {
	return projectNoteResponse{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, Author: projectNoteAuthorResponse{ID: note.AuthorPrincipal.Id, Name: note.AuthorName}, CreatedAt: note.CreatedAt}
}

func projectNoteResponseFromDetail(detail database.ProjectNoteDetail) projectNoteResponse {
	note := detail.Note
	return projectNoteResponse{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Body: &note.Body, Sensitive: note.Sensitive, Author: projectNoteAuthorResponse{ID: note.AuthorPrincipal.Id, Name: detail.AuthorName}, CreatedAt: note.CreatedAt}
}

func projectSecretResponseFromSummary(secret database.ProjectSecretSummary) projectSecretResponse {
	return projectSecretResponse{ID: secret.Ref.Id, Description: secret.Description, Author: projectNoteAuthorResponse{ID: secret.AuthorPrincipal.Id, Name: secret.AuthorName}, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt}
}

func projectSecretResponseFromDetail(detail database.ProjectSecretDetail) projectSecretResponse {
	secret := detail.Secret
	return projectSecretResponse{ID: secret.Ref.Id, Description: secret.Description, Author: projectNoteAuthorResponse{ID: secret.AuthorPrincipal.Id, Name: detail.AuthorName}, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt}
}

func sessionNoteResponseFromSummary(note database.SessionNoteSummary) sessionNoteResponse {
	return sessionNoteResponse{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, Author: projectNoteAuthorResponse{ID: note.AuthorPrincipal.Id, Name: note.AuthorName}, CreatedAt: note.CreatedAt}
}

func sessionNoteResponseFromDetail(detail database.SessionNoteDetail) sessionNoteResponse {
	note := detail.Note
	return sessionNoteResponse{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Body: &note.Body, Sensitive: note.Sensitive, Author: projectNoteAuthorResponse{ID: note.AuthorPrincipal.Id, Name: detail.AuthorName}, CreatedAt: note.CreatedAt}
}

func sessionSecretResponseFromSummary(secret database.SessionSecretSummary) sessionSecretResponse {
	return sessionSecretResponse{ID: secret.Ref.Id, Description: secret.Description, Author: projectNoteAuthorResponse{ID: secret.AuthorPrincipal.Id, Name: secret.AuthorName}, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt}
}

func sessionSecretResponseFromDetail(detail database.SessionSecretDetail) sessionSecretResponse {
	secret := detail.Secret
	return sessionSecretResponse{ID: secret.Ref.Id, Description: secret.Description, Author: projectNoteAuthorResponse{ID: secret.AuthorPrincipal.Id, Name: detail.AuthorName}, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt}
}

func authorizedProjectFile(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims, fileID string) (*model.ProjectFile, *database.StorageObject, bool) {
	project, expectedFileID, ok := projectFileRef(response, request)
	if !ok || fileID != expectedFileID {
		return nil, nil, false
	}
	file := model.ProjectFileRef{Project: project, Id: fileID}
	err, stored, object := store.ProjectFileGet(request.Context(), file, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return nil, nil, false
	}
	if stored == nil || object == nil {
		http.NotFound(response, request)
		return nil, nil, false
	}
	return stored, object, true
}

func projectFileResponseFromModel(file model.ProjectFile, object *database.StorageObject) projectFileResponse {
	response := projectFileResponse{ID: file.Ref.Id, Name: file.Name, MediaType: file.MediaType, CreatedAt: file.CreatedAt}
	if object != nil && object.State == "success" {
		size := object.Size
		fingerprint := "sha256:" + hex.EncodeToString(object.SHA256)
		response.Size = &size
		response.Fingerprint = &fingerprint
	}
	return response
}

func authorizedSessionFile(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims, fileID string) (*model.SessionFile, *database.StorageObject, bool) {
	workspaceID := request.PathValue("workspace")
	sessionID := request.PathValue("session")
	if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) || !typed_id.Valid(typed_id.SessionFile, fileID) {
		http.NotFound(response, request)
		return nil, nil, false
	}
	file := model.SessionFileRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}, Id: fileID}
	err, stored, object := store.SessionFileGet(request.Context(), file, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return nil, nil, false
	}
	if stored == nil {
		http.NotFound(response, request)
		return nil, nil, false
	}
	return stored, object, true
}

func storageToken(ctx context.Context, tokens *auth.BearerTokens, id, action string, lifetime time.Duration) (error, string) {
	expiresAt := time.Now().UTC().Add(lifetime).Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	return tokens.MintStorageToken(ctx, auth.StorageToken{ID: id, Action: action, ExpiresAt: expiresAt})
}

func validStorageToken(token auth.StorageToken) bool {
	expiresAt, err := time.Parse("2006-01-02T15:04:05.000Z", token.ExpiresAt)
	return err == nil && expiresAt.After(time.Now().UTC())
}

func storageURL(_ *http.Request, token string) string {
	return "/api/v1/storage?token=" + url.QueryEscape(token)
}

func writeJSON(response http.ResponseWriter, value any) {
	writeJSONStatus(response, http.StatusOK, value)
}

func writeJSONStatus(response http.ResponseWriter, status int, value any) {
	noStore(response)
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func logout(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	noStore(response)
	http.SetCookie(response, expiredAuthenticationCookie())
	response.WriteHeader(http.StatusNoContent)
}

func requestAuthorization(request *http.Request) string {
	if authorization := request.Header.Get("Authorization"); authorization != "" {
		return authorization
	}
	cookie, err := request.Cookie("gatehouse_auth")
	if err != nil || cookie.Value == "" {
		return ""
	}
	return "Bearer " + cookie.Value
}

func authenticationCookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     "gatehouse_auth",
		Value:    token,
		Path:     "/api",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

func expiredAuthenticationCookie() *http.Cookie {
	cookie := authenticationCookie("")
	cookie.Expires = time.Unix(1, 0)
	cookie.MaxAge = -1
	return cookie
}

func invalidCredentials(response http.ResponseWriter) {
	noStore(response)
	response.Header().Set("WWW-Authenticate", "Bearer")
	response.WriteHeader(http.StatusUnauthorized)
}

func noStore(response http.ResponseWriter) {
	response.Header().Set("Cache-Control", "no-store")
}

func health(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = response.Write([]byte("ok\n"))
}

func web(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if request.URL.Path == "/app" {
		http.Redirect(response, request, "/app/", http.StatusTemporaryRedirect)
		return
	}
	name := strings.TrimPrefix(request.URL.Path, "/app/")
	if name == "" {
		name = "index.html"
	}
	info, err := fs.Stat(webFiles, name)
	if err != nil || info.IsDir() {
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(response, request)
			return
		}
		name = "index.html"
	}
	served := request.Clone(request.Context())
	if name == "index.html" {
		served.URL.Path = "/"
	} else {
		served.URL.Path = "/" + name
	}
	served.URL.RawPath = ""
	webFileServer.ServeHTTP(response, served)
}

func webRoot(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(response, request)
		return
	}
	if request.Method != http.MethodGet {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(response, request, "/app/", http.StatusTemporaryRedirect)
}
