package lisp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type httpTestContextKey struct{}

type httpTestBody struct {
	io.Reader
	closed atomic.Bool
}

func (body *httpTestBody) Close() error {
	body.closed.Store(true)
	return nil
}

type httpTestReader func([]byte) (int, error)

func (read httpTestReader) Read(buffer []byte) (int, error) { return read(buffer) }

func evaluateHTTPWithContext(ctx context.Context, source string, doer nativeNetHTTPDoer) (error, Expr) {
	cache := newModuleCache()
	cache.modules[nativeNetHTTPID] = nativeNetHTTPModuleWithDoer(doer)
	env := bootstrap()
	bindImports(env, cache)
	err, expression := Read(source)
	if err != nil {
		return err, nil
	}
	return newEvaluator(ctx).eval(expression, env)
}

func TestNativeNetHTTPTimeoutSelectionAndCleanup(t *testing.T) {
	for _, test := range []struct {
		name, option string
		timeout      time.Duration
		parentLimit  bool
	}{
		{"default", "", 10 * time.Second, false},
		{"shorter", "(http/timeout 5000)", 5 * time.Second, false},
		{"longer", "(http/timeout 30000)", 30 * time.Second, false},
		{"parent deadline", "(http/timeout 30000)", 5 * time.Second, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), httpTestContextKey{}, "evaluation")
			var parentDeadline time.Time
			if test.parentLimit {
				parentDeadline = time.Now().Add(test.timeout)
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, parentDeadline)
				defer cancel()
			}
			body := &httpTestBody{Reader: strings.NewReader("complete")}
			doer := nativeNetHTTPTestDoer(func(request *http.Request) (*http.Response, error) {
				if request.Context().Value(httpTestContextKey{}) != "evaluation" {
					t.Fatal("request lost its evaluation context")
				}
				deadline, ok := request.Context().Deadline()
				if !ok {
					t.Fatal("request has no deadline")
				}
				if test.parentLimit {
					if !deadline.Equal(parentDeadline) {
						t.Fatalf("request deadline = %v, want parent deadline %v", deadline, parentDeadline)
					}
				} else if remaining := time.Until(deadline); remaining > test.timeout || remaining < test.timeout-time.Second {
					t.Fatalf("request budget = %v, want approximately %v", remaining, test.timeout)
				}
				// An HTTP 504 is a completed response, not a local timeout.
				return &http.Response{StatusCode: 504, Body: body}, nil
			})
			source := fmt.Sprintf(`(import (http @native:net/http/v1)
                (http/status (http/request "GET" "https://example.test" null (bytes/concat) %s)))`, test.option)
			err, result := evaluateHTTPWithContext(ctx, source, doer)
			if err != nil || result.String() != "504" {
				t.Fatalf("request = %v, %v", result, err)
			}
			if !body.closed.Load() || ctx.Err() != nil {
				t.Fatalf("body closed=%t, parent error=%v", body.closed.Load(), ctx.Err())
			}
		})
	}
}

func TestNativeNetHTTPRejectsInvalidTimeouts(t *testing.T) {
	doer := nativeNetHTTPTestDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid timeout dispatched an HTTP request")
		return nil, errors.New("unexpected request")
	})
	for _, source := range []string{
		`(http/timeout)`,
		`(http/timeout 1 2)`,
		`(http/timeout 0)`,
		`(http/timeout -1)`,
		`(http/timeout "100")`,
		`(http/timeout 9223372036854775807)`,
		`(http/request "GET" "https://example.test" null (bytes/concat) 5000)`,
		`(http/request "GET" "https://example.test" null (bytes/concat) '(other 5000))`,
		`(http/request "GET" "https://example.test" null (bytes/concat) '(http/timeout 0))`,
		`(http/request "GET" "https://example.test" null (bytes/concat) '(http/timeout 9223372036854775807))`,
		`(http/request "GET" "https://example.test" null (bytes/concat) (http/timeout 1) (http/timeout 2))`,
	} {
		t.Run(source, func(t *testing.T) {
			err, _ := evaluateHTTPWithContext(context.Background(), `(import (http @native:net/http/v1) `+source+`)`, doer)
			if err == nil {
				t.Fatal("invalid timeout accepted")
			}
		})
	}
}

func TestNativeNetHTTPTimeoutPreservesTaint(t *testing.T) {
	doer := nativeNetHTTPTestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	err, result := evaluateHTTPWithContext(context.Background(), `(import (http @native:net/http/v1)
        (taint/secret? (http/request "GET" "https://example.test" null (bytes/concat)
            (http/timeout (taint/secret/mark 1000)))))`, doer)
	if err != nil || result.String() != "#t" {
		t.Fatalf("timeout option taint = %v, %v", result, err)
	}
}

func TestNativeNetHTTPCancellationInterruptsNetworkIO(t *testing.T) {
	for _, bodyStarted := range []bool{false, true} {
		t.Run(fmt.Sprintf("reading-body=%t", bodyStarted), func(t *testing.T) {
			started, disconnected := make(chan struct{}), make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if bodyStarted {
					response.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(response, "partial")
					response.(http.Flusher).Flush()
				}
				close(started)
				select {
				case <-request.Context().Done():
					close(disconnected)
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			completed := make(chan error, 1)
			go func() {
				err, _ := evaluateHTTPWithContext(ctx, fmt.Sprintf(`(import (http @native:net/http/v1)
                    (error/catch (http/request "GET" %q null (bytes/concat))))`, server.URL), server.Client())
				completed <- err
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach the server")
			}
			cancel()
			select {
			case err := <-completed:
				if !errors.Is(err, ErrInterrupted) || !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled evaluation = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("HTTP operation did not stop after cancellation")
			}
			select {
			case <-disconnected:
			case <-time.After(5 * time.Second):
				t.Fatal("server did not observe request cancellation")
			}
		})
	}
}

func TestNativeNetHTTPExpiredEvaluationDoesNotDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	doer := nativeNetHTTPTestDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("cancelled evaluation dispatched a request")
		return nil, errors.New("unexpected request")
	})
	err, _ := evaluateHTTPWithContext(ctx, `(import (http @native:net/http/v1)
        (http/request "GET" "https://example.test" null (bytes/concat)))`, doer)
	if !errors.Is(err, ErrInterrupted) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled evaluation = %v", err)
	}
}

func TestNativeNetHTTPTimeoutAndDeadlineErrors(t *testing.T) {
	for _, duringBody := range []bool{false, true} {
		t.Run(fmt.Sprintf("reading-body=%t", duringBody), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var body *httpTestBody
			doer := nativeNetHTTPTestDoer(func(request *http.Request) (*http.Response, error) {
				if !duringBody {
					<-request.Context().Done()
					return nil, request.Context().Err()
				}
				body = &httpTestBody{Reader: httpTestReader(func([]byte) (int, error) {
					<-request.Context().Done()
					return 0, request.Context().Err()
				})}
				return &http.Response{StatusCode: 200, Body: body}, nil
			})
			request := `(http/request "GET" "https://example.test" null (bytes/concat) (http/timeout 50))`
			err, _ := evaluateHTTPWithContext(ctx, `(import (http @native:net/http/v1) `+request+`)`, doer)
			if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrInterrupted) || !strings.Contains(err.Error(), "http/request timed out") {
				t.Fatalf("request timeout = %v", err)
			}
			if duringBody && !body.closed.Load() {
				t.Fatal("timed-out response body was not closed")
			}
			err, result := evaluateHTTPWithContext(ctx, `(import (http @native:net/http/v1)
                (begin (error/catch `+request+`) (int/add 40 2)))`, doer)
			if err != nil || result.String() != "42" {
				t.Fatalf("evaluation after request timeout = %v, %v", result, err)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	doer := nativeNetHTTPTestDoer(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	err, _ := evaluateHTTPWithContext(ctx, `(import (http @native:net/http/v1)
        (error/catch (http/request "GET" "https://example.test" null (bytes/concat) (http/timeout 30000))))`, doer)
	if !errors.Is(err, ErrInterrupted) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("evaluation deadline = %v", err)
	}
}

func TestNativeNetHTTPClosesBodyOnReadFailure(t *testing.T) {
	body := &httpTestBody{Reader: httpTestReader(func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF })}
	doer := nativeNetHTTPTestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: body}, nil
	})
	err, _ := evaluateHTTPWithContext(context.Background(), `(import (http @native:net/http/v1)
        (http/request "GET" "https://example.test" null (bytes/concat)))`, doer)
	if err == nil || !strings.Contains(err.Error(), "could not read response body") || !body.closed.Load() {
		t.Fatalf("read failure = %v, body closed=%t", err, body.closed.Load())
	}
}

func TestPublicHTTPSResolutionPreservesContextErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := nativeNetHTTPTestResolver(func(got context.Context, _, _ string) ([]netip.Addr, error) {
		if got != ctx {
			t.Fatal("DNS lookup lost the request context")
		}
		return nil, got.Err()
	})
	if err := validatePublicHTTPSURL(ctx, "https://example.test/", resolver); !errors.Is(err, context.Canceled) {
		t.Fatalf("DNS cancellation = %v", err)
	}
}
