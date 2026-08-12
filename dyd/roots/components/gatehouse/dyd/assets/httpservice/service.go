package httpservice

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
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
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions", workspaceSessions(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/events", workspaceSessionEvents(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/sessions/{session}/messages", workspaceSessionMessages(store, tokens[0], dispatcher))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/tools", workspaceTools(store, tokens[0]))
		mux.HandleFunc("/api/v1/workspaces/{workspace}/resources", workspaceResources(store, tokens[0]))
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
		identityID := strings.TrimSpace(credentials.Identity)
		if !strings.Contains(identityID, ":") {
			identityID = "gatehouse:" + identityID
		}
		err, token := tokens.Login(request.Context(), identityID, []byte(credentials.Password))
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
	Name *string `json:"name,omitempty"`
}

type toolResponse struct {
	ID string `json:"id"`
}

type groupResponse struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

type resourceResponse struct {
	ID     string `json:"id"`
	Secret bool   `json:"secret"`
}

type sessionResponse struct {
	ID string `json:"id"`
}

type sessionMessageRequest struct {
	Text string `json:"text"`
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
		err, configured := store.WorkspacesGet(request.Context(), model.PrincipalRef{Id: claims.Principal})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := make([]workspaceResponse, 0, len(configured))
		for _, workspace := range configured {
			result = append(result, workspaceResponse{ID: workspace.Ref.Id, Name: workspace.Name})
		}
		writeJSON(response, result)
	}
}

func workspaceTools(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
		err, ids := store.WorkspaceToolIDsGet(request.Context(), workspace, model.PrincipalRef{Id: claims.Principal})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := make([]toolResponse, 0, len(ids))
		for _, id := range ids {
			result = append(result, toolResponse{ID: id})
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
		err, groups := store.WorkspaceGroupsGet(request.Context(), workspace, model.PrincipalRef{Id: claims.Principal})
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
	err, sessions := store.SessionsGet(
		request.Context(),
		model.WorkspaceRef{Id: workspaceID},
		model.PrincipalRef{Id: claims.Principal},
	)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return
	}
	result := make([]sessionResponse, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, sessionResponse{ID: session.Ref.Id})
	}
	writeJSON(response, result)
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
	id, err := randomUUID()
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return
	}
	principal := model.PrincipalRef{Id: claims.Principal}
	session := model.Session{Ref: model.SessionRef{Workspace: workspace, Id: id}, AuthorPrincipal: &principal, Enabled: true}
	err, stored := store.SessionsCreate(request.Context(), session, principal)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(response, http.StatusCreated, sessionResponse{ID: stored.Ref.Id})
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
		if workspaceID == "" || sessionID == "" {
			http.NotFound(response, request)
			return
		}
		session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
		err, authorized := store.SessionGet(request.Context(), session, model.PrincipalRef{Id: claims.Principal})
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
		if err := decoder.Decode(&message); err != nil || strings.TrimSpace(message.Text) == "" {
			http.Error(response, "invalid message", http.StatusBadRequest)
			return
		}
		id, err := randomUUID()
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		event := model.SessionEvent{
			Ref:             model.SessionEventRef{Session: session, Id: id},
			Kind:            "message.text",
			AuthorPrincipal: &model.PrincipalRef{Id: claims.Principal},
			Payload:         map[string]interface{}{"text": message.Text},
		}
		err, stored := store.SessionMessagesCreate(request.Context(), event)
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
		if workspaceID == "" || sessionID == "" {
			http.NotFound(response, request)
			return
		}
		session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: sessionID}
		err, authorized := store.SessionGet(request.Context(), session, model.PrincipalRef{Id: claims.Principal})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if authorized == nil {
			http.NotFound(response, request)
			return
		}
		afterCreatedAt := request.URL.Query().Get("after_created_at")
		afterID := request.URL.Query().Get("after_id")
		if (afterCreatedAt == "") != (afterID == "") {
			http.Error(response, "invalid event cursor", http.StatusBadRequest)
			return
		}
		if afterCreatedAt != "" {
			parsed, err := time.Parse("2006-01-02T15:04:05.000Z", afterCreatedAt)
			if err != nil || parsed.Format("2006-01-02T15:04:05.000Z") != afterCreatedAt || !validUUID(afterID) {
				http.Error(response, "invalid event cursor", http.StatusBadRequest)
				return
			}
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
		err, entries := store.SessionEventsTreePageGet(request.Context(), session, afterCreatedAt, afterID, limit)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
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

func workspaceResources(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
		err, resources := store.WorkspaceResourceSummariesGet(request.Context(), workspace, model.PrincipalRef{Id: claims.Principal})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		result := make([]resourceResponse, 0, len(resources))
		for _, resource := range resources {
			result = append(result, resourceResponse{ID: resource.ID, Secret: resource.Secret})
		}
		writeJSON(response, result)
	}
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
	err, configured := store.WorkspaceGet(request.Context(), workspace, model.PrincipalRef{Id: claims.Principal})
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

func writeJSON(response http.ResponseWriter, value any) {
	writeJSONStatus(response, http.StatusOK, value)
}

func writeJSONStatus(response http.ResponseWriter, status int, value any) {
	noStore(response)
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func randomUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
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
