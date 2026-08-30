package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gatehouse/lisp"
)

func TestGatehouseWebModuleSearchesWithExaMCP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Accept") != "application/json, text/event-stream" || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("User-Agent") != "gatehouse" {
			t.Fatalf("request = %s %v", request.Method, request.Header)
		}
		var payload webSearchMCPRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		want := webSearchMCPRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call"}
		want.Params.Name = "web_search_exa"
		want.Params.Arguments = webSearchMCPArguments{
			Query: "Gatehouse web search", Type: "auto", NumResults: 8, Livecrawl: "fallback", ContextMaxCharacters: 10000,
		}
		if payload != want {
			t.Fatalf("payload = %#v", payload)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"- Gatehouse: https://example.test/gatehouse"}]}}`))
	}))
	defer server.Close()

	err, result := lisp.Evaluate(`(import (web @native:gatehouse/web/v1) (web/search "Gatehouse web search"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{newWebModule(server.URL, server.Client())},
	})
	if err != nil || result.String() != `"- Gatehouse: https://example.test/gatehouse"` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseWebModuleAcceptsServerSentEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = response.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"result\"}]}}\n\n"))
	}))
	defer server.Close()

	err, result := lisp.Evaluate(`(import (web @native:gatehouse/web/v1) (web/search "query"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{newWebModule(server.URL, server.Client())},
	})
	if err != nil || result.String() != `"result"` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestAgentPreludeDoesNotRequireApprovalForWebCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"result":{"content":[{"type":"text","text":"result"}]}}`))
	}))
	defer server.Close()
	err, result := lisp.Evaluate(`(list (web/search "query") (web/fetch "https://example.test/page"))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, nil),
			NewSessionModule(nil, nil, nil, nil),
			NewPolicyModule(nil),
			newWebModuleWithFetch(server.URL, server.Client(), func(string) (string, error) { return "<h1>Raw HTML</h1>", nil }),
		},
	})
	if err != nil || result.String() != `("result" "<h1>Raw HTML</h1>")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehouseWebModuleFetchesRawHTML(t *testing.T) {
	fetched := ""
	err, result := lisp.Evaluate(`(import (web @native:gatehouse/web/v1) (web/fetch "https://example.test/page"))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{newWebModuleWithFetch("", nil, func(rawURL string) (string, error) {
			fetched = rawURL
			return "<h1>Raw HTML</h1>", nil
		})},
	})
	if err != nil || result.String() != `"<h1>Raw HTML</h1>"` || fetched != "https://example.test/page" {
		t.Fatalf("Evaluate() = (%s, %v), fetched = %q", result, err, fetched)
	}
}

func TestGatehouseWebModuleRejectsInvalidQueriesAndResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	err, result := lisp.Evaluate(`(import
  (web @native:gatehouse/web/v1)
  (list
    (error/value (error/catch (web/search)))
    (error/value (error/catch (web/search " ")))
	    (error/value (error/catch (web/search (taint/secret/mark "query"))))
    (error/value (error/catch (web/search "query")))
    (error/value (error/catch (web/fetch)))
    (error/value (error/catch (web/fetch " ")))
	    (error/value (error/catch (web/fetch (taint/secret/mark "https://example.test"))))
    (error/value (error/catch (web/fetch "https://example.test")))))`, lisp.EvalOptions{HostModules: []lisp.HostModule{newWebModuleWithFetch(server.URL, server.Client(), func(string) (string, error) { return "", errors.New("unavailable") })}})
	if err != nil || result.String() != `("web/search requires a query" "web/search query must not be blank" "web/search query must not be sensitive" "web/search failed" "web/fetch requires a URL" "web/fetch URL must not be blank" "web/fetch URL must not be sensitive" "web/fetch failed")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}

	if _, _, err := webSearchMCPPayload([]byte(`{"result":`)); err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("webSearchMCPPayload() error = %v", err)
	}
}
