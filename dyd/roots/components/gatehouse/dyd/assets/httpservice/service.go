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
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/sessionsearch"
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
	return start(configuration, store, nil, nil, tokens...)
}

type ReplyDispatcher interface {
	Reconcile() error
}

type ReplyCancellationDispatcher interface {
	CancelReply(context.Context, model.SessionEvent) (error, model.SessionEvent)
}

func StartWithReplyDispatcher(configuration config.HTTPService, store *database.Store, dispatcher ReplyDispatcher, tokens ...*auth.BearerTokens) (error, *Service) {
	return start(configuration, store, dispatcher, nil, tokens...)
}

func StartWithReplyDispatcherAndKeyring(configuration config.HTTPService, store *database.Store, dispatcher ReplyDispatcher, keyring *keychain.Keyring, tokens ...*auth.BearerTokens) (error, *Service) {
	return start(configuration, store, dispatcher, keyring, tokens...)
}

func start(configuration config.HTTPService, store *database.Store, dispatcher ReplyDispatcher, keyring *keychain.Keyring, tokens ...*auth.BearerTokens) (error, *Service) {
	listener, err := net.Listen("tcp", configuration.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", configuration.Listen, err), nil
	}

	service := &Service{
		listener: listener,
		server: &http.Server{
			Handler:           handler(configuration, store, dispatcher, keyring, tokens...),
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
	return handler(configuration, store, nil, nil, tokens...)
}

func HandlerWithReplyDispatcher(configuration config.HTTPService, store *database.Store, dispatcher ReplyDispatcher, tokens ...*auth.BearerTokens) http.Handler {
	return handler(configuration, store, dispatcher, nil, tokens...)
}

func HandlerWithKeyring(configuration config.HTTPService, store *database.Store, keyring *keychain.Keyring, tokens ...*auth.BearerTokens) http.Handler {
	return handler(configuration, store, nil, keyring, tokens...)
}

func handler(configuration config.HTTPService, store *database.Store, dispatcher ReplyDispatcher, keyring *keychain.Keyring, tokens ...*auth.BearerTokens) http.Handler {
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
		mux.HandleFunc("/api/v1/system/grants", systemGrants(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/grants/{grant}", systemGrant(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/principals", systemPrincipals(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/principals/{principal}", systemPrincipal(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/keychains", systemKeychains(keyring, store, tokens[0]))
		mux.HandleFunc("/api/v1/system/agent-providers", systemAgentProviders(store, keyring, tokens[0]))
		mux.HandleFunc("/api/v1/system/agent-providers/{provider}", systemAgentProvider(store, keyring, tokens[0]))
		mux.HandleFunc("/api/v1/system/agent-models", systemAgentModels(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/agent-models/{model}", systemAgentModel(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/storage-providers", systemStorageProviders(store, keyring, tokens[0]))
		mux.HandleFunc("/api/v1/system/storage-providers/{provider}", systemStorageProvider(store, keyring, tokens[0]))
		mux.HandleFunc("/api/v1/system/workspace-agents", systemWorkspaceAgents(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/workspace-agents/{workspace}", systemWorkspaceAgentCreate(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/workspace-agents/{workspace}/{binding}", systemWorkspaceAgent(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/workspace-storage-providers", systemWorkspaceStorageProviders(store, tokens[0]))
		mux.HandleFunc("/api/v1/system/workspace-storage-providers/{workspace}/{provider}", systemWorkspaceStorageProvider(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces", workspaces(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/groups", workspaceGroups(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects", workspaceProjects(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}", workspaceProject(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/notes", workspaceProjectNotes(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/notes/{note}", workspaceProjectNote(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/notes/{note}/revisions", workspaceProjectNoteRevisions(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/notes/{note}/revisions/{revision}", workspaceProjectNoteRevision(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/tasks", workspaceProjectTasks(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/tasks/{task}", workspaceProjectTask(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/secrets", workspaceProjectSecrets(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/secrets/{secret}", workspaceProjectSecret(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/record-schemas", workspaceProjectRecordSchemas(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/record-schemas/{schema}", workspaceProjectRecordSchema(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/record-schemas/{schema}/attributes", workspaceProjectRecordAttributes(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/record-schemas/{schema}/attributes/{attribute}", workspaceProjectRecordAttribute(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/record-schemas/{schema}/records", workspaceProjectRecords(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/record-schemas/{schema}/records/{record}", workspaceProjectRecord(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/record-schemas/{schema}/records/{record}/references", workspaceProjectRecordIncomingReferences(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/record-schemas/{schema}/records/{record}/values", workspaceProjectRecordValues(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/projects/{project}/record-schemas/{schema}/records/{record}/values/mutate", workspaceProjectRecordValuesMutate(store, tokens[0]))
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
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/notes/{note}/revisions", workspaceSessionNoteRevisions(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/notes/{note}/revisions/{revision}", workspaceSessionNoteRevision(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/tasks", workspaceSessionTasks(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/tasks/{task}", workspaceSessionTask(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/secrets", workspaceSessionSecrets(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/secrets/{secret}", workspaceSessionSecret(store, tokens[0]))
		mux.HandleFunc("/api/v1/activity", activity(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/events", workspaceSessionEvents(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/events/search", workspaceSessionEventSearch(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/approvals/{approval}", workspaceSessionApproval(store, tokens[0], dispatcher))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/inputs/{input}/open", workspaceSessionInputOpen(store, tokens[0], configuration.PublicBaseURL))
		mux.HandleFunc("/api/v1/input", sessionInputRead(store, tokens[0]))
		mux.HandleFunc("/api/v1/input/draft", sessionInputPatch(store, tokens[0]))
		mux.HandleFunc("/api/v1/input/files", sessionInputFileCreate(store, tokens[0]))
		mux.HandleFunc("/api/v1/input/files/{file}/finish", sessionInputFileFinish(store, tokens[0]))
		mux.HandleFunc("/api/v1/input/submit", sessionInputTerminal(store, tokens[0], dispatcher, model.SessionEventKindInputSuccess))
		mux.HandleFunc("/api/v1/input/cancel", sessionInputTerminal(store, tokens[0], dispatcher, model.SessionEventKindInputFailure))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/files", workspaceSessionFiles(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/files/{file}/finish", workspaceSessionFileFinish(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/files/{file}/download", workspaceSessionFileDownload(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/files/{file}", workspaceSessionFile(store, tokens[0]))
		mux.HandleFunc("/api/v1/storage", storageProxy(tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/messages/{event}/cancel", workspaceSessionMessageCancel(store, tokens[0], dispatcher))
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
	ID    string  `json:"id"`
	Alias *string `json:"alias,omitempty"`
	Name  *string `json:"name,omitempty"`
}

type groupResponse struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

type workspaceAgentResponse struct {
	ID      string  `json:"id"`
	Alias   string  `json:"alias"`
	Label   *string `json:"label,omitempty"`
	Default bool    `json:"default"`
}

type sessionResponse struct {
	ID        string           `json:"id"`
	Name      *string          `json:"name,omitempty"`
	Project   *projectResponse `json:"project,omitempty"`
	CreatedAt string           `json:"created_at"`
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

type systemGrantCreateRequest struct {
	Principal string `json:"principal"`
}

type systemGrantUpdateRequest struct {
	Enabled *bool `json:"enabled"`
}

type systemPrincipalUpdateRequest struct {
	Enabled *bool `json:"enabled"`
}

type sessionCreateRequest struct {
	Project *string `json:"project"`
}

type sessionUpdateRequest struct {
	Name *string `json:"name"`
}

type sessionProjectRequest struct {
	Project *string `json:"project"`
}

type sessionMessageRequest struct {
	Text        string   `json:"text"`
	Agents      []string `json:"agents"`
	Attachments []string `json:"attachments"`
}

type sessionEventSearchRequest struct {
	Expression string `json:"expression"`
	Cursor     string `json:"cursor,omitempty"`
}

type sessionEventSearchMatchResponse struct {
	Offset int64 `json:"offset"`
	Length int64 `json:"length"`
}

type sessionEventSearchEventResponse struct {
	ID      string                            `json:"id"`
	Kind    string                            `json:"kind"`
	Size    int64                             `json:"size"`
	Preview string                            `json:"preview"`
	Matches []sessionEventSearchMatchResponse `json:"matches"`
}

type sessionEventSearchResponse struct {
	Events     []sessionEventSearchEventResponse `json:"events"`
	NextCursor string                            `json:"next_cursor,omitempty"`
}

type sessionFileCreateRequest struct {
	Name      string  `json:"name"`
	MediaType *string `json:"media_type"`
}

type sessionFileCreateResponse struct {
	File      model.SessionFile `json:"file"`
	UploadURL string            `json:"upload_url"`
}

type fileUpdateRequest struct {
	Name *string `json:"name"`
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

type taskRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Sensitive   *bool   `json:"sensitive"`
	Status      *string `json:"status"`
}

type projectNoteAuthorResponse struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

type noteAgentAuthorResponse struct {
	ID    string  `json:"id"`
	Label *string `json:"label,omitempty"`
}

type noteAuthorResponse struct {
	Principal *projectNoteAuthorResponse `json:"principal,omitempty"`
	Agent     *noteAgentAuthorResponse   `json:"agent,omitempty"`
	Gateway   *string                    `json:"gateway,omitempty"`
}

type projectNoteResponse struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Body        *string            `json:"body,omitempty"`
	Sensitive   bool               `json:"sensitive"`
	Author      noteAuthorResponse `json:"author"`
	CreatedAt   string             `json:"created_at"`
	Revision    int                `json:"revision"`
}

type sessionNoteResponse struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Body        *string            `json:"body,omitempty"`
	Sensitive   bool               `json:"sensitive"`
	Author      noteAuthorResponse `json:"author"`
	CreatedAt   string             `json:"created_at"`
	Revision    int                `json:"revision"`
}

type taskResponse struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description *string            `json:"description,omitempty"`
	Sensitive   bool               `json:"sensitive"`
	Status      string             `json:"status"`
	Creator     noteAuthorResponse `json:"creator"`
	Updater     noteAuthorResponse `json:"updater"`
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
}

type noteRevisionResponse struct {
	Revision    int                `json:"revision"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Body        *string            `json:"body,omitempty"`
	Sensitive   bool               `json:"sensitive"`
	Author      noteAuthorResponse `json:"author"`
	CreatedAt   string             `json:"created_at"`
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
	Event    model.SessionEvent          `json:"event"`
	Children []*sessionEventTreeResponse `json:"children"`
}

type projectRecordSchemaRequest struct {
	Name        *string `json:"name"`
	Label       *string `json:"label"`
	Description *string `json:"description"`
}

type projectRecordSchemaResponse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Label       string             `json:"label"`
	Description string             `json:"description"`
	Author      noteAuthorResponse `json:"author"`
	CreatedAt   string             `json:"created_at"`
}

type projectRecordAttributeRequest struct {
	Name         *string `json:"name"`
	Label        *string `json:"label"`
	Description  *string `json:"description"`
	Type         *string `json:"type"`
	TargetSchema *string `json:"target_schema"`
	Cardinality  *string `json:"cardinality"`
	Uniqueness   *string `json:"uniqueness"`
	Display      *string `json:"display"`
	DisplayOrder *int    `json:"display_order"`
}

type projectRecordAttributeResponse struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Label        string             `json:"label"`
	Description  string             `json:"description"`
	Type         string             `json:"type"`
	TargetSchema *string            `json:"target_schema,omitempty"`
	Cardinality  string             `json:"cardinality"`
	Uniqueness   string             `json:"uniqueness"`
	Display      string             `json:"display"`
	DisplayOrder int                `json:"display_order"`
	Author       noteAuthorResponse `json:"author"`
	CreatedAt    string             `json:"created_at"`
}

type projectRecordValueCreateRequest struct {
	Attribute string `json:"attribute"`
	Value     any    `json:"value"`
	Sensitive bool   `json:"sensitive"`
}

type projectRecordCreateRequest struct {
	Values []projectRecordValueCreateRequest `json:"values"`
}

type projectRecordValueUpdateRequest struct {
	ID        string `json:"id"`
	Value     any    `json:"value"`
	Sensitive *bool  `json:"sensitive"`
}

type projectRecordValueDeleteRequest struct {
	ID string `json:"id"`
}

type projectRecordValuesMutationRequest struct {
	Create []projectRecordValueCreateRequest `json:"create"`
	Update []projectRecordValueUpdateRequest `json:"update"`
	Delete []projectRecordValueDeleteRequest `json:"delete"`
}

type projectRecordResponse struct {
	ID        string                           `json:"id"`
	Author    noteAuthorResponse               `json:"author"`
	CreatedAt string                           `json:"created_at"`
	Values    []projectRecordCardValueResponse `json:"values,omitempty"`
}

type projectRecordReferenceDisplayValueResponse struct {
	Value     any  `json:"value"`
	Sensitive bool `json:"sensitive"`
}

type projectRecordReferenceDisplayResponse struct {
	SchemaLabel   string                                       `json:"schema_label"`
	PrimaryValues []projectRecordReferenceDisplayValueResponse `json:"primary_values"`
}

type projectRecordFileReferenceDisplayResponse struct {
	Name string `json:"name"`
}

type projectRecordCardValueResponse struct {
	Attribute string                                     `json:"attribute"`
	Value     any                                        `json:"value"`
	Sensitive bool                                       `json:"sensitive"`
	Reference *projectRecordReferenceDisplayResponse     `json:"reference,omitempty"`
	File      *projectRecordFileReferenceDisplayResponse `json:"file,omitempty"`
}

type projectRecordValueResponse struct {
	ID        string                                     `json:"id"`
	Attribute string                                     `json:"attribute"`
	Value     any                                        `json:"value"`
	Sensitive bool                                       `json:"sensitive"`
	Author    noteAuthorResponse                         `json:"author"`
	CreatedAt string                                     `json:"created_at"`
	Reference *projectRecordReferenceDisplayResponse     `json:"reference,omitempty"`
	File      *projectRecordFileReferenceDisplayResponse `json:"file,omitempty"`
}

type projectRecordSearchResponse struct {
	Records    []projectRecordResponse `json:"records"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type projectRecordValuesResponse struct {
	Values     []projectRecordValueResponse `json:"values"`
	NextCursor string                       `json:"next_cursor,omitempty"`
}

type projectRecordIncomingReferenceResponse struct {
	ID            string                                       `json:"id"`
	PrimaryValues []projectRecordReferenceDisplayValueResponse `json:"primary_values"`
}

type projectRecordIncomingReferenceGroupResponse struct {
	SourceSchema    projectRecordSchemaReferenceResponse     `json:"source_schema"`
	SourceAttribute projectRecordAttributeReferenceResponse  `json:"source_attribute"`
	References      []projectRecordIncomingReferenceResponse `json:"references"`
}

type projectRecordSchemaReferenceResponse struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type projectRecordAttributeReferenceResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
}

type projectRecordIncomingReferencesResponse struct {
	Groups     []projectRecordIncomingReferenceGroupResponse `json:"groups"`
	NextCursor string                                        `json:"next_cursor,omitempty"`
}

type projectRecordValuesMutationResponse struct {
	Created []projectRecordValueResponse `json:"created"`
	Removed []string                     `json:"removed"`
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
			result[index] = workspaceAgentResponse{ID: agent.ID, Alias: agent.Alias, Label: agent.Label, Default: agent.Default}
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
		switch request.Method {
		case http.MethodGet:
			workspaceID := request.PathValue("workspace")
			if workspaceID == "" {
				http.NotFound(response, request)
				return
			}
			name, cursor, limit, ok := catalogSearchParameters(response, request)
			if !ok {
				return
			}
			if cursor != "" && !typed_id.Valid(typed_id.Project, cursor) {
				http.Error(response, "invalid project cursor", http.StatusBadRequest)
				return
			}
			err, projects, nextCursor := store.ProjectsSearch(request.Context(), model.WorkspaceRef{Id: workspaceID}, claims.Principal.Ref, database.ProjectSearch{Name: name, Cursor: cursor, Limit: limit})
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
			workspace, roles, ok := authorizedWorkspaceRoles(response, request, store, claims)
			if !ok {
				return
			}
			if !workspaceActionAllowed(response, roles, authz.WorkspaceProjectCreate) {
				return
			}
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
				groups = append(groups, model.GroupRef{Id: groupID})
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
		projectRef, ok := authorizedProject(response, request, store, claims)
		if !ok {
			return
		}
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
		if !projectActionAllowed(response, request, store, claims, projectRef, authz.ProjectEdit) {
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

func workspaceProjectRecordSchemas(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
			err, schemas := store.ProjectRecordSchemasGet(request.Context(), project, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]projectRecordSchemaResponse, 0, len(schemas))
			for _, schema := range schemas {
				result = append(result, projectRecordSchemaResponseFromModel(schema))
			}
			writeJSON(response, result)
		case http.MethodPost:
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectRecordSchemaCreate) {
				return
			}
			var input projectRecordSchemaRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || input.Name == nil || input.Label == nil {
				http.Error(response, "invalid project record schema", http.StatusBadRequest)
				return
			}
			description := ""
			if input.Description != nil {
				description = *input.Description
			}
			id, err := typed_id.New(typed_id.ProjectRecordSchema)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			err, schema := store.ProjectRecordSchemaCreate(request.Context(), model.ProjectRecordSchema{Ref: model.ProjectRecordSchemaRef{Project: project, Id: id}, Name: *input.Name, Label: *input.Label, Description: description}, claims.Principal.Ref, database.ProjectRecordAuthor{})
			if err != nil {
				http.Error(response, "project record schema could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, projectRecordSchemaResponseFromModel(schema))
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceProjectRecordSchema(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch && request.Method != http.MethodDelete {
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
		schema, ok := authorizedProjectRecordSchema(response, request, store, claims, project)
		if !ok {
			return
		}
		switch request.Method {
		case http.MethodGet:
			writeJSON(response, projectRecordSchemaResponseFromModel(*schema))
		case http.MethodPatch:
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectRecordSchemaEdit) {
				return
			}
			var input projectRecordSchemaRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || (input.Label == nil && input.Description == nil) {
				http.Error(response, "invalid project record schema", http.StatusBadRequest)
				return
			}
			label, description := schema.Label, schema.Description
			if input.Label != nil {
				label = *input.Label
			}
			if input.Description != nil {
				description = *input.Description
			}
			err, updated := store.ProjectRecordSchemaDetailsSetAs(request.Context(), schema.Ref, claims.Principal.Ref, database.ProjectRecordAuthor{}, label, description)
			if err != nil {
				http.Error(response, "project record schema could not be updated", http.StatusBadRequest)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, projectRecordSchemaResponseFromModel(*updated))
		case http.MethodDelete:
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectRecordSchemaRemove) {
				return
			}
			err, removed := store.ProjectRecordSchemaRemove(request.Context(), schema.Ref, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "project record schema could not be removed", http.StatusBadRequest)
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

func workspaceProjectRecordAttributes(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		project, ok := authorizedProject(response, request, store, claims)
		if !ok {
			return
		}
		schema, ok := authorizedProjectRecordSchema(response, request, store, claims, project)
		if !ok {
			return
		}
		switch request.Method {
		case http.MethodGet:
			err, attributes := store.ProjectRecordAttributesGet(request.Context(), schema.Ref, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]projectRecordAttributeResponse, 0, len(attributes))
			for _, attribute := range attributes {
				result = append(result, projectRecordAttributeResponseFromModel(attribute))
			}
			writeJSON(response, result)
		case http.MethodPost:
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectRecordSchemaEdit) {
				return
			}
			attribute, ok := projectRecordAttributeFromRequest(response, request, schema.Ref, model.ProjectRecordAttribute{}, true)
			if !ok {
				return
			}
			id, err := typed_id.New(typed_id.ProjectRecordAttribute)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			attribute.Ref.Id = id
			err, stored := store.ProjectRecordAttributeCreate(request.Context(), attribute, claims.Principal.Ref, database.ProjectRecordAuthor{})
			if err != nil {
				http.Error(response, "project record attribute could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, projectRecordAttributeResponseFromModel(stored))
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceProjectRecordAttribute(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch && request.Method != http.MethodDelete {
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
		schema, ok := authorizedProjectRecordSchema(response, request, store, claims, project)
		if !ok {
			return
		}
		attribute, ok := authorizedProjectRecordAttribute(response, request, store, claims, schema.Ref)
		if !ok {
			return
		}
		switch request.Method {
		case http.MethodGet:
			writeJSON(response, projectRecordAttributeResponseFromModel(*attribute))
		case http.MethodPatch:
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectRecordSchemaEdit) {
				return
			}
			updated, ok := projectRecordAttributeFromRequest(response, request, schema.Ref, *attribute, false)
			if !ok {
				return
			}
			err, stored := store.ProjectRecordAttributeSetAs(request.Context(), updated, claims.Principal.Ref, database.ProjectRecordAuthor{})
			if err != nil {
				http.Error(response, "project record attribute could not be updated", http.StatusBadRequest)
				return
			}
			if stored == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, projectRecordAttributeResponseFromModel(*stored))
		case http.MethodDelete:
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectRecordSchemaRemove) {
				return
			}
			err, removed := store.ProjectRecordAttributeRemove(request.Context(), attribute.Ref, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "project record attribute could not be removed", http.StatusBadRequest)
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

func workspaceProjectRecords(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		project, ok := authorizedProject(response, request, store, claims)
		if !ok {
			return
		}
		schema, ok := authorizedProjectRecordSchema(response, request, store, claims, project)
		if !ok {
			return
		}
		switch request.Method {
		case http.MethodGet:
			limit, cursor, ok := projectRecordPagination(response, request, typed_id.ProjectRecord)
			if !ok {
				return
			}
			err, cards := store.ProjectRecordCardsGet(request.Context(), schema.Ref, claims.Principal.Ref, limit, cursor)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]projectRecordResponse, 0, len(cards))
			records := make([]model.ProjectRecord, 0, len(cards))
			for _, card := range cards {
				result = append(result, projectRecordResponseFromCard(card))
				records = append(records, card.Record)
			}
			nextCursor, err := projectRecordNextCursor(request.Context(), store, schema.Ref, claims.Principal.Ref, records, limit)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			writeJSON(response, projectRecordSearchResponse{Records: result, NextCursor: nextCursor})
		case http.MethodPost:
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectRecordCreate) {
				return
			}
			var input projectRecordCreateRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				http.Error(response, "invalid project record", http.StatusBadRequest)
				return
			}
			values, ok := projectRecordValueCreates(response, request, store, schema.Ref, claims.Principal.Ref, input.Values)
			if !ok {
				return
			}
			id, err := typed_id.New(typed_id.ProjectRecord)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			err, record, created := store.ProjectRecordCreate(request.Context(), model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: schema.Ref, Id: id}}, claims.Principal.Ref, database.ProjectRecordAuthor{}, values)
			if err != nil {
				http.Error(response, "project record could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, struct {
				Record projectRecordResponse        `json:"record"`
				Values []projectRecordValueResponse `json:"values"`
			}{Record: projectRecordResponseFromModel(record), Values: projectRecordValueResponses(created)})
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceProjectRecord(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodDelete {
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
		schema, ok := authorizedProjectRecordSchema(response, request, store, claims, project)
		if !ok {
			return
		}
		record, ok := authorizedProjectRecord(response, request, store, claims, schema.Ref)
		if !ok {
			return
		}
		if request.Method == http.MethodGet {
			writeJSON(response, projectRecordResponseFromModel(*record))
			return
		}
		if !projectActionAllowed(response, request, store, claims, project, authz.ProjectRecordRemove) {
			return
		}
		err, removed := store.ProjectRecordRemove(request.Context(), record.Ref, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "project record could not be removed", http.StatusBadRequest)
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

func workspaceProjectRecordValues(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
		schema, ok := authorizedProjectRecordSchema(response, request, store, claims, project)
		if !ok {
			return
		}
		record, ok := authorizedProjectRecord(response, request, store, claims, schema.Ref)
		if !ok {
			return
		}
		limit, cursor, ok := projectRecordPagination(response, request, typed_id.ProjectRecordValue)
		if !ok {
			return
		}
		err, values := store.ProjectRecordValuesGet(request.Context(), record.Ref, claims.Principal.Ref, limit, cursor)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		err, references := store.ProjectRecordValueReferenceDisplaysGet(request.Context(), record.Ref, claims.Principal.Ref, values)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		err, files := store.ProjectRecordValueFileReferenceDisplaysGet(request.Context(), record.Ref, claims.Principal.Ref, values)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		nextCursor, err := projectRecordValuesNextCursor(request.Context(), store, record.Ref, claims.Principal.Ref, values, limit)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(response, projectRecordValuesResponse{Values: projectRecordValueResponsesWithReferences(values, references, files), NextCursor: nextCursor})
	}
}

func workspaceProjectRecordIncomingReferences(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
		schema, ok := authorizedProjectRecordSchema(response, request, store, claims, project)
		if !ok {
			return
		}
		record, ok := authorizedProjectRecord(response, request, store, claims, schema.Ref)
		if !ok {
			return
		}
		limit, cursor, ok := projectRecordPagination(response, request, typed_id.ProjectRecordValue)
		if !ok {
			return
		}
		err, groups := store.ProjectRecordIncomingReferencesGet(request.Context(), record.Ref, claims.Principal.Ref, limit, cursor)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		nextCursor, err := projectRecordIncomingReferencesNextCursor(request.Context(), store, record.Ref, claims.Principal.Ref, groups, limit)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(response, projectRecordIncomingReferencesResponse{Groups: projectRecordIncomingReferenceGroupResponses(groups), NextCursor: nextCursor})
	}
}

func workspaceProjectRecordValuesMutate(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
		schema, ok := authorizedProjectRecordSchema(response, request, store, claims, project)
		if !ok {
			return
		}
		record, ok := authorizedProjectRecord(response, request, store, claims, schema.Ref)
		if !ok {
			return
		}
		if !projectActionAllowed(response, request, store, claims, project, authz.ProjectRecordEdit) {
			return
		}
		var input projectRecordValuesMutationRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(response, "invalid project record value mutation", http.StatusBadRequest)
			return
		}
		creates, ok := projectRecordValueCreates(response, request, store, schema.Ref, claims.Principal.Ref, input.Create)
		if !ok {
			return
		}
		mutation := database.ProjectRecordValuesMutation{Create: creates}
		for _, update := range input.Update {
			mutation.Update = append(mutation.Update, database.ProjectRecordValueUpdate{ID: update.ID, Value: update.Value, Sensitive: update.Sensitive})
		}
		for _, deletion := range input.Delete {
			mutation.Delete = append(mutation.Delete, deletion.ID)
		}
		err, result := store.ProjectRecordValuesMutate(request.Context(), record.Ref, claims.Principal.Ref, database.ProjectRecordAuthor{}, mutation)
		if err != nil {
			http.Error(response, "project record values could not be mutated", http.StatusBadRequest)
			return
		}
		if len(result.Created) == 0 && len(result.Removed) == 0 {
			http.NotFound(response, request)
			return
		}
		writeJSON(response, projectRecordValuesMutationResponse{Created: projectRecordValueResponses(result.Created), Removed: result.Removed})
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
		if request.Method != http.MethodGet && request.Method != http.MethodPatch {
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
		sessionRef := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
		err, session := store.SessionGet(request.Context(), sessionRef, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if session == nil {
			http.NotFound(response, request)
			return
		}
		if request.Method == http.MethodPatch {
			var input sessionUpdateRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || input.Name == nil {
				http.Error(response, "invalid session", http.StatusBadRequest)
				return
			}
			if !sessionActionAllowed(response, request, store, claims, sessionRef, authz.SessionEdit) {
				return
			}
			err, session = store.SessionNameUpdate(request.Context(), sessionRef, claims.Principal.Ref, *input.Name)
			if err != nil {
				http.Error(response, "session could not be updated", http.StatusBadRequest)
				return
			}
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
	workspaceID := request.PathValue("workspace")
	if workspaceID == "" {
		http.NotFound(response, request)
		return
	}
	workspace := model.WorkspaceRef{Id: workspaceID}
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
	if project == nil {
		var roles []authz.Role
		workspace, roles, ok = authorizedWorkspaceRoles(response, request, store, claims)
		if !ok {
			return
		}
		if !workspaceActionAllowed(response, roles, authz.WorkspaceSessionCreate) {
			return
		}
	} else {
		err, available := store.ProjectGet(request.Context(), *project, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if available == nil {
			http.NotFound(response, request)
			return
		}
		if !projectActionAllowed(response, request, store, claims, *project, authz.ProjectSessionCreate) {
			return
		}
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
		session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
		err, available := store.SessionGet(request.Context(), session, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if available == nil {
			http.NotFound(response, request)
			return
		}
		if !sessionActionAllowed(response, request, store, claims, session, authz.SessionProjectSet) {
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
			err, available := store.ProjectGet(request.Context(), *project, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if available == nil {
				http.NotFound(response, request)
				return
			}
			if !projectActionAllowed(response, request, store, claims, *project, authz.ProjectSessionCreate) {
				return
			}
		} else {
			_, roles, ok := authorizedWorkspaceRoles(response, request, store, claims)
			if !ok {
				return
			}
			if !workspaceActionAllowed(response, roles, authz.WorkspaceSessionCreate) {
				return
			}
		}
		err, stored := store.SessionProjectSet(request.Context(), session, project, claims.Principal.Ref)
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

func activity(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
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
		seenCheckpoints := make(map[string]struct{}, len(input.Topics))
		checkpoints := make([]database.ActivityTopicCheckpoint, 0, len(input.Topics))
		for _, topic := range input.Topics {
			if strings.TrimSpace(topic.Topic) == "" {
				http.Error(response, "invalid activity topic", http.StatusBadRequest)
				return
			}
			events, err := database.ActivityEventSelectorsNormalize(topic.Events)
			if err != nil {
				http.Error(response, "invalid activity event selectors", http.StatusBadRequest)
				return
			}
			identity := topic.Topic + "\x00" + strings.Join(events, "\x00")
			if _, exists := seenCheckpoints[identity]; exists {
				http.Error(response, "duplicate activity checkpoint", http.StatusBadRequest)
				return
			}
			seenCheckpoints[identity] = struct{}{}
			checkpoint := database.ActivityTopicCheckpoint{Name: topic.Name, Topic: topic.Topic, Events: events}
			if topic.Cursor != nil {
				if !typed_id.Valid(typed_id.ActivityEvent, topic.Cursor.Id) {
					http.Error(response, "invalid activity cursor", http.StatusBadRequest)
					return
				}
				checkpoint.ID = topic.Cursor.Id
			}
			checkpoints = append(checkpoints, checkpoint)
		}
		err, advanced := store.ActivityTopicCheckpointsGet(request.Context(), claims.Principal.Ref, checkpoints)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := make([]model.ActivityTopicCheckpoint, 0, len(advanced))
		for _, checkpoint := range advanced {
			entry := model.ActivityTopicCheckpoint{Name: checkpoint.Name, Topic: checkpoint.Topic, Events: checkpoint.Events}
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
		if !sessionActionAllowed(response, request, store, claims, session, authz.SessionMessageCreate) {
			return
		}

		var message sessionMessageRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&message); err != nil || (strings.TrimSpace(message.Text) == "" && len(message.Attachments) == 0) {
			http.Error(response, "invalid message", http.StatusBadRequest)
			return
		}
		seenAgents := make(map[string]struct{}, len(message.Agents))
		for _, agent := range message.Agents {
			if _, exists := seenAgents[agent]; exists {
				continue
			}
			seenAgents[agent] = struct{}{}
			err, selected := store.WorkspaceAgentModelGet(request.Context(), session.Workspace, agent)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if selected == nil || selected.Ref.Id != agent {
				http.Error(response, "invalid agent", http.StatusBadRequest)
				return
			}
		}
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		err, payload := messagePayload(message)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		event := model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Kind:            model.SessionEventKindMessageText,
			AuthorPrincipal: &claims.Principal,
			Payload:         payload,
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
		if request.Method != http.MethodGet && request.Method != http.MethodPost {
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
		if request.Method == http.MethodGet {
			err, files := store.SessionFilesGet(request.Context(), session)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			writeJSON(response, files)
			return
		}
		if !sessionActionAllowed(response, request, store, claims, session, authz.SessionFileCreate) {
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
		created := model.SessionFile{Ref: model.SessionFileRef{Session: session, Id: fileID}, Name: input.Name, MediaType: input.MediaType, Enabled: true}
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
		if !sessionActionAllowed(response, request, store, claims, file.Ref.Session, authz.SessionFileFinish) {
			return
		}
		if err := tokens.StorageClient().Finish(request.Context(), object.ID); err != nil {
			http.Error(response, "storage object is not ready", http.StatusConflict)
			return
		}
		err, file, object := store.SessionFileFinish(request.Context(), file.Ref, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if file == nil || object == nil {
			http.NotFound(response, request)
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

func workspaceSessionFile(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPatch && request.Method != http.MethodDelete {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		file, _, ok := authorizedSessionFile(response, request, store, claims, request.PathValue("file"))
		if !ok || file == nil {
			return
		}
		switch request.Method {
		case http.MethodPatch:
			if !sessionActionAllowed(response, request, store, claims, file.Ref.Session, authz.SessionFileUpdate) {
				return
			}
			var input fileUpdateRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || input.Name == nil || strings.TrimSpace(*input.Name) == "" {
				http.Error(response, "invalid session file update", http.StatusBadRequest)
				return
			}
			err, updated, _ := store.SessionFileUpdate(request.Context(), file.Ref, database.FileUpdate{Name: *input.Name}, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, updated)
		case http.MethodDelete:
			if !sessionActionAllowed(response, request, store, claims, file.Ref.Session, authz.SessionFileRemove) {
				return
			}
			err, removed := store.SessionFileRemove(request.Context(), file.Ref, claims.Principal.Ref)
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
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectNoteCreate) {
				return
			}
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
			writeJSONStatus(response, http.StatusCreated, projectNoteResponse{ID: stored.Ref.Id, Title: stored.Title, Description: stored.Description, Body: &stored.Body, Sensitive: stored.Sensitive, Author: noteAuthorResponse{Principal: &projectNoteAuthorResponse{ID: claims.Principal.Ref.Id, Name: claims.Principal.Name}}, CreatedAt: stored.CreatedAt, Revision: stored.Revision})
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
			if !projectActionAllowed(response, request, store, claims, note.Project, authz.ProjectNoteEdit) {
				return
			}
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
			sensitive := current.Note.Sensitive
			if input.Sensitive != nil {
				sensitive = *input.Sensitive
			}
			err, updated := store.ProjectNoteDetailsSetAs(request.Context(), note, claims.Principal.Ref, database.NoteAuthor{Principal: &claims.Principal.Ref}, sensitive, &title, &description, &body)
			if err != nil {
				http.Error(response, "project note could not be updated", http.StatusBadRequest)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			updated.AuthorName = claims.Principal.Name
			writeJSON(response, projectNoteResponseFromDetail(*updated))
		case http.MethodDelete:
			if !projectActionAllowed(response, request, store, claims, note.Project, authz.ProjectNoteRemove) {
				return
			}
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

func workspaceProjectTasks(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
			err, tasks := store.ProjectTasksGet(request.Context(), project, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]taskResponse, 0, len(tasks))
			for _, task := range tasks {
				result = append(result, projectTaskResponseFromSummary(task))
			}
			writeJSON(response, result)
		case http.MethodPost:
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectTaskCreate) {
				return
			}
			var input taskRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validTaskRequest(input, true) {
				http.Error(response, "invalid project task", http.StatusBadRequest)
				return
			}
			id, err := typed_id.New(typed_id.ProjectTask)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			status := ""
			if input.Status != nil {
				status = *input.Status
			}
			task := model.ProjectTask{Ref: model.ProjectTaskRef{Project: project, Id: id}, Title: *input.Title, Description: input.Description, Sensitive: input.Sensitive != nil && *input.Sensitive, Status: status}
			err, stored := store.ProjectTaskCreate(request.Context(), task, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "project task could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, projectTaskResponseFromDetail(stored))
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceProjectTask(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch && request.Method != http.MethodDelete {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		task, ok := projectTaskRef(response, request)
		if !ok {
			return
		}
		err, current := store.ProjectTaskGet(request.Context(), task, claims.Principal.Ref)
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
			writeJSON(response, projectTaskResponseFromDetail(*current))
		case http.MethodPatch:
			if !projectActionAllowed(response, request, store, claims, task.Project, authz.ProjectTaskEdit) {
				return
			}
			var input taskRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validTaskRequest(input, false) {
				http.Error(response, "invalid project task", http.StatusBadRequest)
				return
			}
			title, description, sensitive, status := current.Title, current.Description, current.Sensitive, current.Status
			if input.Title != nil {
				title = *input.Title
			}
			if input.Description != nil {
				description = input.Description
			}
			if input.Sensitive != nil {
				sensitive = *input.Sensitive
			}
			if input.Status != nil {
				status = *input.Status
			}
			err, updated := store.ProjectTaskDetailsSetAs(request.Context(), task, claims.Principal.Ref, database.TaskAuthor{Principal: &claims.Principal.Ref}, sensitive, status, title, description)
			if err != nil {
				http.Error(response, "project task could not be updated", http.StatusBadRequest)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, projectTaskResponseFromDetail(*updated))
		case http.MethodDelete:
			if !projectActionAllowed(response, request, store, claims, task.Project, authz.ProjectTaskRemove) {
				return
			}
			err, removed := store.ProjectTaskRemove(request.Context(), task, claims.Principal.Ref)
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

func workspaceProjectNoteRevisions(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
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
		err, revisions := store.ProjectNoteRevisionsGet(request.Context(), note, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if len(revisions) == 0 {
			http.NotFound(response, request)
			return
		}
		result := make([]noteRevisionResponse, 0, len(revisions))
		for _, revision := range revisions {
			result = append(result, projectNoteRevisionResponseFromSummary(revision))
		}
		writeJSON(response, result)
	}
}

func workspaceProjectNoteRevision(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
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
		revision, err := strconv.Atoi(request.PathValue("revision"))
		if err != nil || revision < 1 {
			http.NotFound(response, request)
			return
		}
		err, current := store.ProjectNoteRevisionGet(request.Context(), model.ProjectNoteRevisionRef{Note: note, Revision: revision}, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if current == nil {
			http.NotFound(response, request)
			return
		}
		writeJSON(response, projectNoteRevisionResponseFromDetail(*current))
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
			if !projectActionAllowed(response, request, store, claims, project, authz.ProjectSecretCreate) {
				return
			}
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
			if !projectActionAllowed(response, request, store, claims, secret.Project, authz.ProjectSecretEdit) {
				return
			}
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
			if !projectActionAllowed(response, request, store, claims, secret.Project, authz.ProjectSecretRemove) {
				return
			}
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
			if !sessionActionAllowed(response, request, store, claims, session, authz.SessionNoteCreate) {
				return
			}
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
			writeJSONStatus(response, http.StatusCreated, sessionNoteResponse{ID: stored.Ref.Id, Title: stored.Title, Description: stored.Description, Body: &stored.Body, Sensitive: stored.Sensitive, Author: noteAuthorResponse{Principal: &projectNoteAuthorResponse{ID: claims.Principal.Ref.Id, Name: claims.Principal.Name}}, CreatedAt: stored.CreatedAt, Revision: stored.Revision})
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
			if !sessionActionAllowed(response, request, store, claims, note.Session, authz.SessionNoteEdit) {
				return
			}
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
			sensitive := current.Note.Sensitive
			if input.Sensitive != nil {
				sensitive = *input.Sensitive
			}
			err, updated := store.SessionNoteDetailsSetAs(request.Context(), note, claims.Principal.Ref, database.NoteAuthor{Principal: &claims.Principal.Ref}, sensitive, &title, &description, &body)
			if err != nil {
				http.Error(response, "session note could not be updated", http.StatusBadRequest)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			updated.AuthorName = claims.Principal.Name
			writeJSON(response, sessionNoteResponseFromDetail(*updated))
		case http.MethodDelete:
			if !sessionActionAllowed(response, request, store, claims, note.Session, authz.SessionNoteRemove) {
				return
			}
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

func workspaceSessionTasks(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
			err, tasks := store.SessionTasksGet(request.Context(), session, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			result := make([]taskResponse, 0, len(tasks))
			for _, task := range tasks {
				result = append(result, sessionTaskResponseFromSummary(task))
			}
			writeJSON(response, result)
		case http.MethodPost:
			if !sessionActionAllowed(response, request, store, claims, session, authz.SessionTaskCreate) {
				return
			}
			var input taskRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validTaskRequest(input, true) {
				http.Error(response, "invalid session task", http.StatusBadRequest)
				return
			}
			id, err := typed_id.New(typed_id.SessionTask)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			status := ""
			if input.Status != nil {
				status = *input.Status
			}
			task := model.SessionTask{Ref: model.SessionTaskRef{Session: session, Id: id}, Title: *input.Title, Description: input.Description, Sensitive: input.Sensitive != nil && *input.Sensitive, Status: status}
			err, stored := store.SessionTaskCreate(request.Context(), task, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "session task could not be created", http.StatusBadRequest)
				return
			}
			writeJSONStatus(response, http.StatusCreated, sessionTaskResponseFromDetail(stored))
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func workspaceSessionTask(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch && request.Method != http.MethodDelete {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		task, ok := sessionTaskRef(response, request)
		if !ok {
			return
		}
		err, current := store.SessionTaskGet(request.Context(), task, claims.Principal.Ref)
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
			writeJSON(response, sessionTaskResponseFromDetail(*current))
		case http.MethodPatch:
			if !sessionActionAllowed(response, request, store, claims, task.Session, authz.SessionTaskEdit) {
				return
			}
			var input taskRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || !validTaskRequest(input, false) {
				http.Error(response, "invalid session task", http.StatusBadRequest)
				return
			}
			title, description, sensitive, status := current.Title, current.Description, current.Sensitive, current.Status
			if input.Title != nil {
				title = *input.Title
			}
			if input.Description != nil {
				description = input.Description
			}
			if input.Sensitive != nil {
				sensitive = *input.Sensitive
			}
			if input.Status != nil {
				status = *input.Status
			}
			err, updated := store.SessionTaskDetailsSetAs(request.Context(), task, claims.Principal.Ref, database.TaskAuthor{Principal: &claims.Principal.Ref}, sensitive, status, title, description)
			if err != nil {
				http.Error(response, "session task could not be updated", http.StatusBadRequest)
				return
			}
			if updated == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, sessionTaskResponseFromDetail(*updated))
		case http.MethodDelete:
			if !sessionActionAllowed(response, request, store, claims, task.Session, authz.SessionTaskRemove) {
				return
			}
			err, removed := store.SessionTaskRemove(request.Context(), task, claims.Principal.Ref)
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

func workspaceSessionNoteRevisions(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
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
		err, revisions := store.SessionNoteRevisionsGet(request.Context(), note, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if len(revisions) == 0 {
			http.NotFound(response, request)
			return
		}
		result := make([]noteRevisionResponse, 0, len(revisions))
		for _, revision := range revisions {
			result = append(result, sessionNoteRevisionResponseFromSummary(revision))
		}
		writeJSON(response, result)
	}
}

func workspaceSessionNoteRevision(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
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
		revision, err := strconv.Atoi(request.PathValue("revision"))
		if err != nil || revision < 1 {
			http.NotFound(response, request)
			return
		}
		err, current := store.SessionNoteRevisionGet(request.Context(), model.SessionNoteRevisionRef{Note: note, Revision: revision}, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if current == nil {
			http.NotFound(response, request)
			return
		}
		writeJSON(response, sessionNoteRevisionResponseFromDetail(*current))
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
			if !sessionActionAllowed(response, request, store, claims, session, authz.SessionSecretCreate) {
				return
			}
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
			if !sessionActionAllowed(response, request, store, claims, secret.Session, authz.SessionSecretEdit) {
				return
			}
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
			if !sessionActionAllowed(response, request, store, claims, secret.Session, authz.SessionSecretRemove) {
				return
			}
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
		if !projectActionAllowed(response, request, store, claims, project, authz.ProjectFileCreate) {
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
		if !projectActionAllowed(response, request, store, claims, file.Ref.Project, authz.ProjectFileFinish) {
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
		err, content := tokens.StorageClient().Get(request.Context(), object.ID, 0)
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
		if request.Method != http.MethodPatch && request.Method != http.MethodDelete {
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
		switch request.Method {
		case http.MethodPatch:
			if !projectActionAllowed(response, request, store, claims, file.Ref.Project, authz.ProjectFileUpdate) {
				return
			}
			var input fileUpdateRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || input.Name == nil || strings.TrimSpace(*input.Name) == "" {
				http.Error(response, "invalid project file update", http.StatusBadRequest)
				return
			}
			err, updated, object := store.ProjectFileUpdate(request.Context(), file.Ref, database.FileUpdate{Name: *input.Name}, claims.Principal.Ref)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if updated == nil || object == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, projectFileResponseFromModel(*updated, object))
		case http.MethodDelete:
			if !projectActionAllowed(response, request, store, claims, file.Ref.Project, authz.ProjectFileRemove) {
				return
			}
			err, removed := store.ProjectFileRemove(request.Context(), file.Ref, claims.Principal.Ref)
			if err != nil {
				if strings.Contains(err.Error(), "project file is referenced by a record value") {
					http.Error(response, "project file is referenced by a record value", http.StatusConflict)
					return
				}
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
			err, content := objects.Get(request.Context(), token.ID, 0)
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

func messagePayload(message sessionMessageRequest) (error, map[string]interface{}) {
	payload := model.MessageTextPayload{}
	if strings.TrimSpace(message.Text) != "" {
		payload.Text = &message.Text
	}
	if len(message.Agents) > 0 {
		payload.Agents = &message.Agents
	}
	if len(message.Attachments) > 0 {
		payload.Attachments = &message.Attachments
	}
	converted, err := database.SessionEventPayloadFrom(payload)
	return err, converted
}

func workspaceSessionMessageCancel(store *database.Store, tokens *auth.BearerTokens, dispatcher ReplyDispatcher) http.HandlerFunc {
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
		if !sessionActionAllowed(response, request, store, claims, session, authz.SessionMessageCancel) {
			return
		}
		parent := model.SessionEventRef{Session: session, Id: messageID}
		err, message := store.SessionEventGet(request.Context(), parent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if message == nil || message.Kind != model.SessionEventKindAgentRequest || message.Parent == nil || message.AuthorPrincipal == nil {
			http.NotFound(response, request)
			return
		}
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		payload, err := database.SessionEventPayloadFrom(model.CancelRequestPayload{})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		event := model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Parent:          &parent,
			Kind:            model.SessionEventKindCancelRequest,
			AuthorPrincipal: &claims.Principal,
			Payload:         payload,
		}
		canceller, ok := dispatcher.(ReplyCancellationDispatcher)
		if !ok {
			http.Error(response, "reply cancellation is unavailable", http.StatusServiceUnavailable)
			return
		}
		err, stored := canceller.CancelReply(request.Context(), event)
		if errors.Is(err, database.ErrSessionReplyAlreadyCompleted) {
			http.Error(response, "reply is already completed", http.StatusConflict)
			return
		}
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
		if !sessionActionAllowed(response, request, store, claims, session, authz.SessionApprovalRespond) {
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
		if approval == nil || approval.Kind != model.SessionEventKindApprovalRequest || approval.AuthorAgent == nil || approval.Parent == nil {
			http.NotFound(response, request)
			return
		}
		err, tool := store.SessionEventGet(request.Context(), *approval.Parent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if tool == nil || tool.Kind != model.SessionEventKindToolRequest || tool.AuthorAgent == nil {
			http.NotFound(response, request)
			return
		}
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		kind := model.SessionEventKindApprovalSuccess
		payload, err := database.SessionEventPayloadFrom(model.ApprovalSuccessPayload{})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if approvalInput.Decision == "rejected" {
			kind = model.SessionEventKindApprovalFailure
			payload, err = database.SessionEventPayloadFrom(model.ApprovalFailurePayload{Code: "rejected"})
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
		}
		event := model.SessionEvent{
			Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &approvalRef, Kind: kind, AuthorPrincipal: &claims.Principal, Payload: payload,
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
		view := request.URL.Query().Get("view")
		if view != "" && view != "transcript" {
			http.Error(response, "invalid event view", http.StatusBadRequest)
			return
		}
		err, entries := store.SessionEventsTreePageGet(request.Context(), session, afterID, limit)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if view == "transcript" {
			sessionEventTranscriptProject(entries)
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
			if view != "transcript" {
				sessionInputEventProject(&entries[index].Event)
			}
		}
		writeJSON(response, sessionEventTrees(entries))
	}
}

func sessionEventTranscriptProject(entries []database.SessionEventTreeEntry) {
	for index := range entries {
		event := &entries[index].Event
		if sessionInputEventProject(event) {
			continue
		}
		payload := map[string]interface{}{}
		var keys []string
		switch event.Kind {
		case model.SessionEventKindMessageText:
			keys = []string{"text", "attachments", "agents"}
		case model.SessionEventKindAgentRequest:
			keys = []string{"agent"}
		case model.SessionEventKindAgentSuccess:
			keys = []string{"text", "attachments"}
		case model.SessionEventKindAgentFailure:
			keys = []string{"code", "message"}
		case model.SessionEventKindToolRequest:
			keys = []string{"name", "reason"}
		case model.SessionEventKindToolFailure:
			keys = []string{"name", "call_id", "code", "message", "output"}
		case model.SessionEventKindApprovalRequest:
			keys = []string{"description"}
		case model.SessionEventKindApprovalFailure:
			keys = []string{"code", "message"}
		case model.SessionEventKindThinkingUpdate:
			keys = []string{"reason", "until"}
		case model.SessionEventKindThinkingFailure, model.SessionEventKindCancelFailure:
			keys = []string{"code", "message"}
		}
		for _, key := range keys {
			if value, exists := event.Payload[key]; exists {
				payload[key] = value
			}
		}
		event.Payload = payload
	}
}

// Input form schemas and completed results remain private even when callers
// request the otherwise unprojected session-event tree. The scoped input API
// is the only HTTP entry point for form content while the request is pending.
func sessionInputEventProject(event *model.SessionEvent) bool {
	var allowed string
	switch event.Kind {
	case model.SessionEventKindInputRequest:
		allowed = "description"
	case model.SessionEventKindInputFailure:
		payload := map[string]interface{}{}
		for _, key := range []string{"code", "message"} {
			if value, exists := event.Payload[key]; exists {
				payload[key] = value
			}
		}
		event.Payload = payload
		return true
	case model.SessionEventKindInputSuccess:
	default:
		return false
	}
	payload := map[string]interface{}{}
	if allowed != "" {
		if value, exists := event.Payload[allowed]; exists {
			payload[allowed] = value
		}
	}
	event.Payload = payload
	return true
}

func workspaceSessionEventSearch(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 8<<10))
		decoder.DisallowUnknownFields()
		input := sessionEventSearchRequest{}
		if err := decoder.Decode(&input); err != nil {
			http.Error(response, "invalid session event search request", http.StatusBadRequest)
			return
		}
		err, expression := sessionsearch.Parse(input.Expression)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		err, beforeID := sessionsearch.Cursor(input.Expression, input.Cursor)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		err, events, more := store.SessionEventsSearch(request.Context(), session, expression, beforeID, 8)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := sessionEventSearchResponse{Events: make([]sessionEventSearchEventResponse, 0, len(events))}
		for _, event := range events {
			entry, matches := sessionsearch.Result(event, expression)
			if !matches {
				continue
			}
			locations := make([]sessionEventSearchMatchResponse, len(entry.Matches))
			for index, location := range entry.Matches {
				locations[index] = sessionEventSearchMatchResponse{Offset: location.Offset, Length: location.Length}
			}
			result.Events = append(result.Events, sessionEventSearchEventResponse{ID: entry.ID, Kind: entry.Kind, Size: entry.Size, Preview: entry.Preview, Matches: locations})
		}
		if more && len(events) > 0 {
			result.NextCursor = sessionsearch.NextCursor(input.Expression, events[len(events)-1].Ref.Id)
		}
		writeJSON(response, result)
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

func authorizedProjectRecordSchema(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims, project model.ProjectRef) (*model.ProjectRecordSchema, bool) {
	id := request.PathValue("schema")
	if !typed_id.Valid(typed_id.ProjectRecordSchema, id) {
		http.NotFound(response, request)
		return nil, false
	}
	err, schema := store.ProjectRecordSchemaGet(request.Context(), model.ProjectRecordSchemaRef{Project: project, Id: id}, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if schema == nil {
		http.NotFound(response, request)
		return nil, false
	}
	return schema, true
}

func authorizedProjectRecordAttribute(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims, schema model.ProjectRecordSchemaRef) (*model.ProjectRecordAttribute, bool) {
	id := request.PathValue("attribute")
	if !typed_id.Valid(typed_id.ProjectRecordAttribute, id) {
		http.NotFound(response, request)
		return nil, false
	}
	err, attribute := store.ProjectRecordAttributeGet(request.Context(), model.ProjectRecordAttributeRef{Schema: schema, Id: id}, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if attribute == nil {
		http.NotFound(response, request)
		return nil, false
	}
	return attribute, true
}

func authorizedProjectRecord(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims, schema model.ProjectRecordSchemaRef) (*model.ProjectRecord, bool) {
	id := request.PathValue("record")
	if !typed_id.Valid(typed_id.ProjectRecord, id) {
		http.NotFound(response, request)
		return nil, false
	}
	err, record := store.ProjectRecordGet(request.Context(), model.ProjectRecordRef{Schema: schema, Id: id}, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if record == nil {
		http.NotFound(response, request)
		return nil, false
	}
	return record, true
}

func projectRecordAttributeFromRequest(response http.ResponseWriter, request *http.Request, schema model.ProjectRecordSchemaRef, current model.ProjectRecordAttribute, create bool) (model.ProjectRecordAttribute, bool) {
	var input projectRecordAttributeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(response, "invalid project record attribute", http.StatusBadRequest)
		return model.ProjectRecordAttribute{}, false
	}
	if create {
		if input.Name == nil || input.Label == nil || input.Type == nil || input.Cardinality == nil || input.Uniqueness == nil || input.Display == nil {
			http.Error(response, "invalid project record attribute", http.StatusBadRequest)
			return model.ProjectRecordAttribute{}, false
		}
		description := ""
		if input.Description != nil {
			description = *input.Description
		}
		displayOrder := 0
		if input.DisplayOrder != nil {
			displayOrder = *input.DisplayOrder
		}
		current = model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema}, Name: *input.Name, Label: *input.Label, Description: description, Type: *input.Type, Cardinality: *input.Cardinality, Uniqueness: *input.Uniqueness, Display: *input.Display, DisplayOrder: displayOrder}
	} else {
		if input.Name != nil || (input.Label == nil && input.Description == nil && input.Type == nil && input.TargetSchema == nil && input.Cardinality == nil && input.Uniqueness == nil && input.Display == nil && input.DisplayOrder == nil) {
			http.Error(response, "invalid project record attribute", http.StatusBadRequest)
			return model.ProjectRecordAttribute{}, false
		}
		if input.Label != nil {
			current.Label = *input.Label
		}
		if input.Description != nil {
			current.Description = *input.Description
		}
		if input.Type != nil {
			current.Type = *input.Type
			if current.Type != "record" {
				current.TargetSchema = nil
			}
		}
		if input.Cardinality != nil {
			current.Cardinality = *input.Cardinality
		}
		if input.Uniqueness != nil {
			current.Uniqueness = *input.Uniqueness
		}
		if input.Display != nil {
			current.Display = *input.Display
		}
		if input.DisplayOrder != nil {
			current.DisplayOrder = *input.DisplayOrder
		}
	}
	if input.DisplayOrder != nil && *input.DisplayOrder < 0 {
		http.Error(response, "invalid project record attribute", http.StatusBadRequest)
		return model.ProjectRecordAttribute{}, false
	}
	if input.TargetSchema != nil {
		if !typed_id.Valid(typed_id.ProjectRecordSchema, *input.TargetSchema) {
			http.Error(response, "invalid project record attribute", http.StatusBadRequest)
			return model.ProjectRecordAttribute{}, false
		}
		current.TargetSchema = &model.ProjectRecordSchemaRef{Project: schema.Project, Id: *input.TargetSchema}
	}
	return current, true
}

func projectRecordPagination(response http.ResponseWriter, request *http.Request, kind string) (int, string, bool) {
	limit := 50
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			http.Error(response, "limit must be between 1 and 100", http.StatusBadRequest)
			return 0, "", false
		}
		limit = parsed
	}
	cursor := request.URL.Query().Get("cursor")
	if cursor != "" && !typed_id.Valid(kind, cursor) {
		http.Error(response, "invalid cursor", http.StatusBadRequest)
		return 0, "", false
	}
	return limit, cursor, true
}

func projectRecordNextCursor(ctx context.Context, store *database.Store, schema model.ProjectRecordSchemaRef, principal model.PrincipalRef, records []model.ProjectRecord, limit int) (string, error) {
	if len(records) < limit {
		return "", nil
	}
	err, more := store.ProjectRecordsGet(ctx, schema, principal, 1, records[len(records)-1].Ref.Id)
	if err != nil {
		return "", err
	}
	if len(more) == 0 {
		return "", nil
	}
	return records[len(records)-1].Ref.Id, nil
}

func projectRecordValuesNextCursor(ctx context.Context, store *database.Store, record model.ProjectRecordRef, principal model.PrincipalRef, values []model.ProjectRecordValue, limit int) (string, error) {
	if len(values) < limit {
		return "", nil
	}
	err, more := store.ProjectRecordValuesGet(ctx, record, principal, 1, values[len(values)-1].Ref.Id)
	if err != nil {
		return "", err
	}
	if len(more) == 0 {
		return "", nil
	}
	return values[len(values)-1].Ref.Id, nil
}

func projectRecordIncomingReferencesNextCursor(ctx context.Context, store *database.Store, record model.ProjectRecordRef, principal model.PrincipalRef, groups []database.ProjectRecordIncomingReferenceGroup, limit int) (string, error) {
	count, cursor := 0, ""
	for _, group := range groups {
		for _, reference := range group.References {
			count++
			if cursor == "" || reference.ValueID < cursor {
				cursor = reference.ValueID
			}
		}
	}
	if count < limit {
		return "", nil
	}
	err, more := store.ProjectRecordIncomingReferencesGet(ctx, record, principal, 1, cursor)
	if err != nil {
		return "", err
	}
	if len(more) == 0 {
		return "", nil
	}
	return cursor, nil
}

func projectRecordValueCreates(response http.ResponseWriter, request *http.Request, store *database.Store, schema model.ProjectRecordSchemaRef, principal model.PrincipalRef, input []projectRecordValueCreateRequest) ([]database.ProjectRecordValueCreate, bool) {
	err, attributes := store.ProjectRecordAttributesGet(request.Context(), schema, principal)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return nil, false
	}
	byName := make(map[string]model.ProjectRecordAttributeRef, len(attributes))
	for _, attribute := range attributes {
		byName[attribute.Name] = attribute.Ref
	}
	result := make([]database.ProjectRecordValueCreate, 0, len(input))
	for _, value := range input {
		attribute, ok := byName[value.Attribute]
		if !ok {
			http.Error(response, "invalid project record value attribute", http.StatusBadRequest)
			return nil, false
		}
		result = append(result, database.ProjectRecordValueCreate{Attribute: attribute, Value: value.Value, Sensitive: value.Sensitive})
	}
	return result, true
}

func projectRecordSchemaResponseFromModel(schema model.ProjectRecordSchema) projectRecordSchemaResponse {
	return projectRecordSchemaResponse{ID: schema.Ref.Id, Name: schema.Name, Label: schema.Label, Description: schema.Description, Author: projectRecordAuthorResponse(schema.AuthorPrincipal, schema.AuthorAgent), CreatedAt: schema.CreatedAt}
}

func projectRecordAttributeResponseFromModel(attribute model.ProjectRecordAttribute) projectRecordAttributeResponse {
	result := projectRecordAttributeResponse{ID: attribute.Ref.Id, Name: attribute.Name, Label: attribute.Label, Description: attribute.Description, Type: attribute.Type, Cardinality: attribute.Cardinality, Uniqueness: attribute.Uniqueness, Display: attribute.Display, DisplayOrder: attribute.DisplayOrder, Author: projectRecordAuthorResponse(attribute.AuthorPrincipal, attribute.AuthorAgent), CreatedAt: attribute.CreatedAt}
	if attribute.TargetSchema != nil {
		result.TargetSchema = &attribute.TargetSchema.Id
	}
	return result
}

func projectRecordResponseFromModel(record model.ProjectRecord) projectRecordResponse {
	return projectRecordResponse{ID: record.Ref.Id, Author: projectRecordAuthorResponse(record.AuthorPrincipal, record.AuthorAgent), CreatedAt: record.CreatedAt}
}

func projectRecordResponseFromCard(card database.ProjectRecordCard) projectRecordResponse {
	result := projectRecordResponseFromModel(card.Record)
	result.Values = make([]projectRecordCardValueResponse, 0, len(card.Values))
	for _, value := range card.Values {
		item := projectRecordCardValueResponse{Attribute: value.Attribute, Value: value.Value, Sensitive: value.Sensitive}
		item.Reference = projectRecordReferenceDisplayResponseFromModel(value.Reference)
		item.File = projectRecordFileReferenceDisplayResponseFromModel(value.File)
		result.Values = append(result.Values, item)
	}
	return result
}

func projectRecordFileReferenceDisplayResponseFromModel(file *database.ProjectRecordFileReferenceDisplay) *projectRecordFileReferenceDisplayResponse {
	if file == nil {
		return nil
	}
	return &projectRecordFileReferenceDisplayResponse{Name: file.Name}
}

func projectRecordReferenceDisplayResponseFromModel(reference *database.ProjectRecordReferenceDisplay) *projectRecordReferenceDisplayResponse {
	if reference == nil {
		return nil
	}
	result := &projectRecordReferenceDisplayResponse{SchemaLabel: reference.SchemaLabel, PrimaryValues: make([]projectRecordReferenceDisplayValueResponse, 0, len(reference.PrimaryValues))}
	for _, primary := range reference.PrimaryValues {
		result.PrimaryValues = append(result.PrimaryValues, projectRecordReferenceDisplayValueResponse{Value: primary.Value, Sensitive: primary.Sensitive})
	}
	return result
}

func projectRecordValueResponses(values []model.ProjectRecordValue) []projectRecordValueResponse {
	return projectRecordValueResponsesWithReferences(values, nil, nil)
}

func projectRecordValueResponsesWithReferences(values []model.ProjectRecordValue, references map[string]*database.ProjectRecordReferenceDisplay, files map[string]*database.ProjectRecordFileReferenceDisplay) []projectRecordValueResponse {
	result := make([]projectRecordValueResponse, 0, len(values))
	for _, value := range values {
		item := projectRecordValueResponse{ID: value.Ref.Id, Attribute: value.Attribute.Id, Value: value.Value, Sensitive: value.Sensitive, Author: projectRecordAuthorResponse(value.AuthorPrincipal, value.AuthorAgent), CreatedAt: value.CreatedAt}
		item.Reference = projectRecordReferenceDisplayResponseFromModel(references[value.Ref.Id])
		item.File = projectRecordFileReferenceDisplayResponseFromModel(files[value.Ref.Id])
		result = append(result, item)
	}
	return result
}

func projectRecordIncomingReferenceGroupResponses(groups []database.ProjectRecordIncomingReferenceGroup) []projectRecordIncomingReferenceGroupResponse {
	result := make([]projectRecordIncomingReferenceGroupResponse, 0, len(groups))
	for _, group := range groups {
		item := projectRecordIncomingReferenceGroupResponse{SourceSchema: projectRecordSchemaReferenceResponse{ID: group.Schema.Id, Label: group.SchemaLabel}, SourceAttribute: projectRecordAttributeReferenceResponse{ID: group.Attribute.Id, Name: group.AttributeName, Label: group.AttributeLabel}, References: make([]projectRecordIncomingReferenceResponse, 0, len(group.References))}
		for _, reference := range group.References {
			value := projectRecordIncomingReferenceResponse{ID: reference.Record.Id, PrimaryValues: make([]projectRecordReferenceDisplayValueResponse, 0, len(reference.PrimaryValues))}
			for _, primary := range reference.PrimaryValues {
				value.PrimaryValues = append(value.PrimaryValues, projectRecordReferenceDisplayValueResponse{Value: primary.Value, Sensitive: primary.Sensitive})
			}
			item.References = append(item.References, value)
		}
		result = append(result, item)
	}
	return result
}

func projectRecordAuthorResponse(principal *model.PrincipalRef, agent *model.WorkspaceAgentRef) noteAuthorResponse {
	result := noteAuthorResponse{}
	if principal != nil {
		result.Principal = &projectNoteAuthorResponse{ID: principal.Id}
	}
	if agent != nil {
		result.Agent = &noteAgentAuthorResponse{ID: agent.Id}
	}
	return result
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
	workspace, _, ok := authorizedWorkspaceRoles(response, request, store, claims)
	return workspace, ok
}

func workspaceActionAllowed(response http.ResponseWriter, roles []authz.Role, action authz.WorkspaceAction) bool {
	if !authz.WorkspaceAllows(roles, action) {
		http.Error(response, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func projectActionAllowed(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims, project model.ProjectRef, action authz.ProjectAction) bool {
	err, roles := store.ProjectRolesGet(request.Context(), project, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return false
	}
	if !authz.ProjectAllows(roles, action) {
		http.Error(response, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func sessionActionAllowed(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims, session model.SessionRef, action authz.SessionAction) bool {
	err, roles := store.SessionRolesGet(request.Context(), session, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return false
	}
	if !authz.SessionAllows(roles, action) {
		http.Error(response, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func authorizedWorkspaceRoles(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims) (model.WorkspaceRef, []authz.Role, bool) {
	workspaceID := request.PathValue("workspace")
	if workspaceID == "" {
		http.NotFound(response, request)
		return model.WorkspaceRef{}, nil, false
	}
	workspace := model.WorkspaceRef{Id: workspaceID}
	err, roles := store.WorkspaceRolesGet(request.Context(), workspace, claims.Principal.Ref)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return model.WorkspaceRef{}, nil, false
	}
	if len(roles) == 0 {
		http.NotFound(response, request)
		return model.WorkspaceRef{}, nil, false
	}
	return workspace, roles, true
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

func projectTaskRef(response http.ResponseWriter, request *http.Request) (model.ProjectTaskRef, bool) {
	workspaceID := request.PathValue("workspace")
	projectID := request.PathValue("project")
	taskID := request.PathValue("task")
	if workspaceID == "" || !typed_id.Valid(typed_id.Project, projectID) || !typed_id.Valid(typed_id.ProjectTask, taskID) {
		http.NotFound(response, request)
		return model.ProjectTaskRef{}, false
	}
	return model.ProjectTaskRef{Project: model.ProjectRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: projectID}, Id: taskID}, true
}

func sessionTaskRef(response http.ResponseWriter, request *http.Request) (model.SessionTaskRef, bool) {
	workspaceID := request.PathValue("workspace")
	sessionID := request.PathValue("session")
	taskID := request.PathValue("task")
	if workspaceID == "" || !typed_id.Valid(typed_id.Session, sessionID) || !typed_id.Valid(typed_id.SessionTask, taskID) {
		http.NotFound(response, request)
		return model.SessionTaskRef{}, false
	}
	return model.SessionTaskRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}, Id: taskID}, true
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
	if !required && input.Title == nil && input.Description == nil && input.Body == nil && input.Sensitive == nil {
		return false
	}
	return (input.Title == nil || len(*input.Title) <= 256) && (input.Description == nil || len(*input.Description) <= 4*1024) && (input.Body == nil || len(*input.Body) <= 1024*1024)
}

func validSessionNoteRequest(input sessionNoteRequest, required bool) bool {
	return validProjectNoteRequest(projectNoteRequest{Title: input.Title, Description: input.Description, Body: input.Body, Sensitive: input.Sensitive}, required)
}

func validTaskRequest(input taskRequest, required bool) bool {
	if required && input.Title == nil {
		return false
	}
	if !required && input.Title == nil && input.Description == nil && input.Sensitive == nil && input.Status == nil {
		return false
	}
	if (input.Title != nil && len(*input.Title) > 256) || (input.Description != nil && len(*input.Description) > 4*1024) {
		return false
	}
	if input.Status == nil {
		return true
	}
	switch *input.Status {
	case "", "draft", "ready", "in_progress", "done", "cancelled":
		return true
	default:
		return false
	}
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
	return projectNoteResponse{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, Author: noteAuthorResponseFromValues(note.AuthorPrincipal, note.AuthorName, note.AuthorAgent, note.AuthorAgentLabel, note.AuthorGateway), CreatedAt: note.CreatedAt, Revision: note.Revision}
}

func projectNoteResponseFromDetail(detail database.ProjectNoteDetail) projectNoteResponse {
	note := detail.Note
	return projectNoteResponse{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Body: &note.Body, Sensitive: note.Sensitive, Author: noteAuthorResponseFromValues(note.AuthorPrincipal, detail.AuthorName, note.AuthorAgent, detail.AuthorAgentLabel, note.AuthorGateway), CreatedAt: note.CreatedAt, Revision: note.Revision}
}

func projectSecretResponseFromSummary(secret database.ProjectSecretSummary) projectSecretResponse {
	return projectSecretResponse{ID: secret.Ref.Id, Description: secret.Description, Author: projectNoteAuthorResponse{ID: secret.AuthorPrincipal.Id, Name: secret.AuthorName}, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt}
}

func projectSecretResponseFromDetail(detail database.ProjectSecretDetail) projectSecretResponse {
	secret := detail.Secret
	return projectSecretResponse{ID: secret.Ref.Id, Description: secret.Description, Author: projectNoteAuthorResponse{ID: secret.AuthorPrincipal.Id, Name: detail.AuthorName}, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt}
}

func sessionNoteResponseFromSummary(note database.SessionNoteSummary) sessionNoteResponse {
	return sessionNoteResponse{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, Author: noteAuthorResponseFromValues(note.AuthorPrincipal, note.AuthorName, note.AuthorAgent, note.AuthorAgentLabel, note.AuthorGateway), CreatedAt: note.CreatedAt, Revision: note.Revision}
}

func sessionNoteResponseFromDetail(detail database.SessionNoteDetail) sessionNoteResponse {
	note := detail.Note
	return sessionNoteResponse{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Body: &note.Body, Sensitive: note.Sensitive, Author: noteAuthorResponseFromValues(note.AuthorPrincipal, detail.AuthorName, note.AuthorAgent, detail.AuthorAgentLabel, note.AuthorGateway), CreatedAt: note.CreatedAt, Revision: note.Revision}
}

func projectTaskResponseFromSummary(task model.ProjectTask) taskResponse {
	return taskResponse{ID: task.Ref.Id, Title: task.Title, Sensitive: task.Sensitive, Status: task.Status, Creator: taskAuthorResponse(task.CreatorPrincipal, task.CreatorAgent, task.CreatorGateway), Updater: taskAuthorResponse(task.UpdaterPrincipal, task.UpdaterAgent, task.UpdaterGateway), CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt}
}

func projectTaskResponseFromDetail(task model.ProjectTask) taskResponse {
	result := projectTaskResponseFromSummary(task)
	result.Description = task.Description
	return result
}

func sessionTaskResponseFromSummary(task model.SessionTask) taskResponse {
	return taskResponse{ID: task.Ref.Id, Title: task.Title, Sensitive: task.Sensitive, Status: task.Status, Creator: taskAuthorResponse(task.CreatorPrincipal, task.CreatorAgent, task.CreatorGateway), Updater: taskAuthorResponse(task.UpdaterPrincipal, task.UpdaterAgent, task.UpdaterGateway), CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt}
}

func sessionTaskResponseFromDetail(task model.SessionTask) taskResponse {
	result := sessionTaskResponseFromSummary(task)
	result.Description = task.Description
	return result
}

func taskAuthorResponse(principal *model.PrincipalRef, agent *model.WorkspaceAgentRef, gateway *model.GatewayRef) noteAuthorResponse {
	return noteAuthorResponseFromValues(principal, nil, agent, nil, gateway)
}

func noteAuthorResponseFromValues(principal *model.PrincipalRef, principalName *string, agent *model.WorkspaceAgentRef, agentLabel *string, gateway *model.GatewayRef) noteAuthorResponse {
	author := noteAuthorResponse{}
	if principal != nil {
		author.Principal = &projectNoteAuthorResponse{ID: principal.Id, Name: principalName}
	}
	if agent != nil {
		author.Agent = &noteAgentAuthorResponse{ID: agent.Id, Label: agentLabel}
	}
	if gateway != nil {
		author.Gateway = &gateway.Id
	}
	return author
}

func projectNoteRevisionResponseFromSummary(revision database.ProjectNoteRevisionSummary) noteRevisionResponse {
	return noteRevisionResponse{Revision: revision.Ref.Revision, Title: revision.Title, Description: revision.Description, Sensitive: revision.Sensitive, Author: noteAuthorResponseFromValues(revision.AuthorPrincipal, revision.AuthorName, revision.AuthorAgent, revision.AuthorAgentLabel, revision.AuthorGateway), CreatedAt: revision.CreatedAt}
}

func projectNoteRevisionResponseFromDetail(detail database.ProjectNoteRevisionDetail) noteRevisionResponse {
	revision := detail.Revision
	return noteRevisionResponse{Revision: revision.Ref.Revision, Title: revision.Title, Description: revision.Description, Body: &revision.Body, Sensitive: revision.Sensitive, Author: noteAuthorResponseFromValues(revision.AuthorPrincipal, detail.AuthorName, revision.AuthorAgent, detail.AuthorAgentLabel, revision.AuthorGateway), CreatedAt: revision.CreatedAt}
}

func sessionNoteRevisionResponseFromSummary(revision database.SessionNoteRevisionSummary) noteRevisionResponse {
	return noteRevisionResponse{Revision: revision.Ref.Revision, Title: revision.Title, Description: revision.Description, Sensitive: revision.Sensitive, Author: noteAuthorResponseFromValues(revision.AuthorPrincipal, revision.AuthorName, revision.AuthorAgent, revision.AuthorAgentLabel, revision.AuthorGateway), CreatedAt: revision.CreatedAt}
}

func sessionNoteRevisionResponseFromDetail(detail database.SessionNoteRevisionDetail) noteRevisionResponse {
	revision := detail.Revision
	return noteRevisionResponse{Revision: revision.Ref.Revision, Title: revision.Title, Description: revision.Description, Body: &revision.Body, Sensitive: revision.Sensitive, Author: noteAuthorResponseFromValues(revision.AuthorPrincipal, detail.AuthorName, revision.AuthorAgent, detail.AuthorAgentLabel, revision.AuthorGateway), CreatedAt: revision.CreatedAt}
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
