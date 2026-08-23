package lisp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

type nativeNetHTTPTestDoer func(*http.Request) (*http.Response, error)

func (doer nativeNetHTTPTestDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type nativeNetHTTPTestResolver func(context.Context, string, string) ([]netip.Addr, error)

func (resolver nativeNetHTTPTestResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return resolver(ctx, network, host)
}

func TestNativeNetHTTPRequest(t *testing.T) {
	cache := newModuleCache()
	cache.modules[nativeNetHTTPID] = nativeNetHTTPModuleWithDoer(nativeNetHTTPTestDoer(func(request *http.Request) (*http.Response, error) {
		if got, want := request.Method, "POST"; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := request.URL.String(), "https://api.example.test/items"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		if got, want := request.Header.Values("Accept"), []string{"application/json"}; !equalStrings(got, want) {
			t.Fatalf("Accept = %q, want %q", got, want)
		}
		if got, want := request.Header.Values("X-Label"), []string{"first", "second"}; !equalStrings(got, want) {
			t.Fatalf("X-Label = %q, want %q", got, want)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), "{\"name\":\"gatehouse\"}"; got != want {
			t.Fatalf("body = %q, want %q", got, want)
		}
		return &http.Response{
			StatusCode: 201,
			Header: http.Header{
				"Content-Type": {"application/json"},
				"Link":         {"first", "next"},
			},
			Body: io.NopCloser(strings.NewReader("created")),
		}, nil
	}))

	err, result := runWithModuleCache(`(import
  (http @native:net/http/v1)
  (let
    ((response
      (http/request
        "POST"
        "https://api.example.test/items"
        (list
          (pair "Accept" "application/json")
          (pair "X-Label" "first")
          (pair "X-Label" "second"))
        (bytes/utf8/encode "{\"name\":\"gatehouse\"}"))))
    (list
      (http/status response)
      (http/headers response)
      (http/header response "link")
      (bytes/utf8/decode (http/body response)))))`, cache)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), `(201 (("Content-Type" . "application/json") ("Link" . "first") ("Link" . "next")) ("first" "next") "created")`; got != want {
		t.Fatalf("result = %s, want %s", got, want)
	}
}

func TestNativeNetHTTPRequestPreservesHTTPFailures(t *testing.T) {
	cache := newModuleCache()
	cache.modules[nativeNetHTTPID] = nativeNetHTTPModuleWithDoer(nativeNetHTTPTestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(strings.NewReader("missing")),
		}, nil
	}))

	err, result := runWithModuleCache(`(import
  (http @native:net/http/v1)
  (let ((response (http/request "GET" "https://api.example.test/missing" null (bytes/concat))))
    (list (http/status response) (bytes/utf8/decode (http/body response)))))`, cache)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), `(404 "missing")`; got != want {
		t.Fatalf("result = %s, want %s", got, want)
	}
}

func TestNativeNetHTTPDoesNotFollowRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirect" {
			http.Redirect(response, request, "/target", http.StatusFound)
			return
		}
		t.Fatal("redirect target was requested")
	}))
	defer server.Close()

	err, result := runWithModuleCache(fmt.Sprintf(`(import
  (http @native:net/http/v1)
  (http/status (http/request "GET" %q null (bytes/concat))))`, server.URL+"/redirect"), newModuleCache())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), "302"; got != want {
		t.Fatalf("result = %s, want %s", got, want)
	}
}

func TestNativeNetHTTPValidationAndTaint(t *testing.T) {
	cache := newModuleCache()
	cache.modules[nativeNetHTTPID] = nativeNetHTTPModuleWithDoer(nativeNetHTTPTestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}))

	for _, test := range []struct {
		source   string
		contains string
	}{
		{`(import (http @native:net/http/v1) (error/value (error/catch (http/request "GET" "https://api.example.test" null))))`, "requires method, URL, headers, and body"},
		{`(import (http @native:net/http/v1) (error/value (error/catch (http/request "GET" "file:///etc/passwd" null (bytes/concat)))))`, "absolute HTTP or HTTPS URL"},
		{`(import (http @native:net/http/v1) (error/value (error/catch (http/request "GET" "https://user:pass@api.example.test" null (bytes/concat)))))`, "without credentials"},
		{`(import (http @native:net/http/v1) (error/value (error/catch (http/request "GET" "https://api.example.test" (list "bad") (bytes/concat)))))`, "proper list"},
		{`(import (http @native:net/http/v1) (secret? (http/request "GET" "https://api.example.test" (secret/mark (list (pair "Authorization" "Bearer secret"))) (bytes/concat))))`, "#t"},
	} {
		t.Run(test.contains, func(t *testing.T) {
			err, result := runWithModuleCache(test.source, cache)
			if err != nil {
				t.Fatal(err)
			}
			if got := result.String(); !strings.Contains(got, test.contains) {
				t.Fatalf("result = %s, want %q", got, test.contains)
			}
		})
	}
}

func TestNativeNetHTTPRejectsOversizedBodies(t *testing.T) {
	cache := newModuleCache()
	cache.modules[nativeNetHTTPID] = nativeNetHTTPModuleWithDoer(nativeNetHTTPTestDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", nativeNetHTTPMaximumBodySize+1))),
		}, nil
	}))

	err, result := runWithModuleCache(`(import
  (http @native:net/http/v1)
  (error/value (error/catch (http/request "GET" "https://api.example.test" null (bytes/concat)))))`, cache)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), `"http/request response body exceeds 10485760 bytes"`; got != want {
		t.Fatalf("result = %s, want %s", got, want)
	}
}

func TestPublicHTTPSURLValidation(t *testing.T) {
	resolver := nativeNetHTTPTestResolver(func(_ context.Context, network, host string) ([]netip.Addr, error) {
		if network != "ip" || host == "unresolvable.example.test" {
			return nil, errors.New("unresolvable")
		}
		if host == "mixed.example.test" {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	})
	for _, test := range []struct {
		url  string
		want bool
	}{
		{"https://example.test", true},
		{"https://8.8.8.8", true},
		{"http://example.test", false},
		{"https://user:pass@example.test", false},
		{"https://127.0.0.1", false},
		{"https://mixed.example.test", false},
		{"https://unresolvable.example.test", false},
	} {
		t.Run(test.url, func(t *testing.T) {
			err := validatePublicHTTPSURL(context.Background(), test.url, resolver)
			if (err == nil) != test.want {
				t.Fatalf("validatePublicHTTPSURL() = %v, want success %t", err, test.want)
			}
		})
	}
}

func TestPublicHTTPSClientRevalidatesConnectionsAndRedirects(t *testing.T) {
	resolver := nativeNetHTTPTestResolver(func(_ context.Context, _ string, host string) ([]netip.Addr, error) {
		if host == "redirect.example.test" {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	})
	dialed := ""
	client := newPublicHTTPSClient(time.Second, resolver, func(_ context.Context, _ string, address string) (net.Conn, error) {
		dialed = address
		return nil, errors.New("unavailable")
	})
	_, err := client.Transport.(*http.Transport).DialContext(context.Background(), "tcp", "example.test:443")
	if err == nil || dialed != "8.8.8.8:443" {
		t.Fatalf("DialContext() = %v, dialed %q", err, dialed)
	}
	redirectURL, err := url.Parse("https://redirect.example.test/")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(&http.Request{URL: redirectURL}, []*http.Request{{}}); err == nil {
		t.Fatal("CheckRedirect() accepted a private destination")
	}
	if err := client.CheckRedirect(&http.Request{URL: redirectURL}, make([]*http.Request, publicHTTPMaximumRedirects)); err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("CheckRedirect() = %v", err)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
