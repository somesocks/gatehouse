package httpservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"gatehouse/auth"
	"gatehouse/config"
)

type Service struct {
	listener net.Listener
	server   *http.Server
	done     chan error
}

func Start(configuration config.HTTPService, tokens ...*auth.BearerTokens) (error, *Service) {
	listener, err := net.Listen("tcp", configuration.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", configuration.Listen, err), nil
	}

	service := &Service{
		listener: listener,
		server: &http.Server{
			Handler:           Handler(configuration, tokens...),
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

func Handler(configuration config.HTTPService, tokens ...*auth.BearerTokens) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("/readyz", health)
	if configuration.Web {
		mux.HandleFunc("/", web)
	}
	if configuration.API && len(tokens) > 0 && tokens[0] != nil {
		mux.HandleFunc("/api/v1/auth/login", login(tokens[0]))
		mux.HandleFunc("/api/v1/auth/me", me(tokens[0]))
		mux.HandleFunc("/api/v1/auth/logout", logout)
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
		err, token := tokens.Login(request.Context(), credentials.Identity, []byte(credentials.Password))
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
	if request.URL.Path != "/" {
		http.NotFound(response, request)
		return
	}
	if request.Method != http.MethodGet {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = response.Write([]byte("<!doctype html><title>Gatehouse</title><h1>Gatehouse</h1>\n"))
}
