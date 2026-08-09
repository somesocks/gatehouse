package httpservice

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"gatehouse/config"
)

type Service struct {
	listener net.Listener
	server   *http.Server
	done     chan error
}

func Start(configuration config.HTTPService) (error, *Service) {
	listener, err := net.Listen("tcp", configuration.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", configuration.Listen, err), nil
	}

	service := &Service{
		listener: listener,
		server: &http.Server{
			Handler:           Handler(configuration),
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

func Handler(configuration config.HTTPService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("/readyz", health)
	if configuration.Web {
		mux.HandleFunc("/", web)
	}
	return mux
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
