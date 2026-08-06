package lisp

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type nativeNetHTTPTestDoer func(*http.Request) (*http.Response, error)

func (doer nativeNetHTTPTestDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
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
