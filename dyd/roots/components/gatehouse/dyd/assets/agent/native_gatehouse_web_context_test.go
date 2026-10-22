package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/lisp"
)

func TestGatehouseWebSearchCancellation(t *testing.T) {
	for _, bodyStarted := range []bool{false, true} {
		t.Run(fmt.Sprintf("reading-body=%t", bodyStarted), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if bodyStarted {
					response.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(response, "data: partial")
					response.(http.Flusher).Flush()
				}
				close(started)
				select {
				case <-request.Context().Done():
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			completed := make(chan error, 1)
			go func() {
				err, _ := lisp.Evaluate(`(import (web @native:gatehouse/web/v1) (error/catch (web/search "query")))`, lisp.EvalOptions{
					Context: ctx, HostModules: []lisp.HostModule{newWebModule(server.URL, server.Client())},
				})
				completed <- err
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("search did not reach server")
			}
			cancel()
			select {
			case err := <-completed:
				if !errors.Is(err, lisp.ErrInterrupted) || !errors.Is(err, context.Canceled) {
					t.Fatalf("search cancellation = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("search did not stop after evaluation cancellation")
			}
		})
	}
}

type webContextTestTransport func(*http.Request) (*http.Response, error)

func (transport webContextTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type webContextTestBody struct {
	read   func([]byte) (int, error)
	closed atomic.Bool
}

func (body *webContextTestBody) Read(buffer []byte) (int, error) { return body.read(buffer) }
func (body *webContextTestBody) Close() error {
	body.closed.Store(true)
	return nil
}

type webContextTestKey struct{}

func TestGatehouseWebFetchCancellationClosesBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), webContextTestKey{}, "evaluation"))
	defer cancel()
	reading, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	body := &webContextTestBody{}
	original := webFetchHTTPClient
	t.Cleanup(func() { webFetchHTTPClient = original })
	webFetchHTTPClient = &http.Client{Transport: webContextTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.Context().Value(webContextTestKey{}) != "evaluation" {
			return nil, errors.New("fetch lost evaluation context")
		}
		body.read = func([]byte) (int, error) {
			close(reading)
			select {
			case <-request.Context().Done():
				return 0, request.Context().Err()
			case <-release:
				return 0, io.ErrUnexpectedEOF
			}
		}
		return &http.Response{StatusCode: 200, Body: body}, nil
	})}
	completed := make(chan error, 1)
	go func() {
		// Literal public IP avoids DNS; the injected transport performs no network I/O.
		err, _ := lisp.Evaluate(`(import (web @native:gatehouse/web/v1) (error/catch (web/fetch "https://1.1.1.1/page")))`, lisp.EvalOptions{
			Context: ctx, HostModules: []lisp.HostModule{NewWebModule()},
		})
		completed <- err
	}()
	select {
	case <-reading:
	case <-time.After(5 * time.Second):
		t.Fatal("fetch did not read the response body")
	}
	cancel()
	select {
	case err := <-completed:
		if !errors.Is(err, lisp.ErrInterrupted) || !errors.Is(err, context.Canceled) || !body.closed.Load() {
			t.Fatalf("fetch cancellation = %v, body closed=%t", err, body.closed.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fetch body read did not stop after cancellation")
	}
}

func TestGatehouseWebRequestTimeoutIsCatchable(t *testing.T) {
	err, result := lisp.Evaluate(`(import (web @native:gatehouse/web/v1)
        (error/value (error/catch (web/fetch "https://example.test"))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{newWebModuleWithFetch("", nil, func(context.Context, string) (string, error) {
			return "", fmt.Errorf("fetch request: %w", context.DeadlineExceeded)
		})},
	})
	if err != nil || !strings.Contains(result.String(), "web/fetch timed out") {
		t.Fatalf("fetch timeout = %v, %v", result, err)
	}
}
