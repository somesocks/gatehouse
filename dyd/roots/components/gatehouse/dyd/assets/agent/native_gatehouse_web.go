package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gatehouse/lisp"
)

const (
	gatehouseWebModuleID         = "native:gatehouse/web/v1"
	webSearchEndpoint            = "https://mcp.exa.ai/mcp"
	webSearchTimeout             = 25 * time.Second
	webSearchMaximumResponseSize = 10 << 20
	webFetchTimeout              = 30 * time.Second
	webFetchMaximumResponseSize  = 5 << 20
)

type webSearchMCPArguments struct {
	Query                string `json:"query"`
	Type                 string `json:"type"`
	NumResults           int    `json:"numResults"`
	Livecrawl            string `json:"livecrawl"`
	ContextMaxCharacters int    `json:"contextMaxCharacters"`
}

type webSearchMCPRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		Name      string                `json:"name"`
		Arguments webSearchMCPArguments `json:"arguments"`
	} `json:"params"`
}

var webSearchDocumentation = capabilityDocumentation{
	"(web/search query) -> String",
	"Searches the web and returns bounded provider results with source URLs. Query must be a non-blank, non-sensitive string.",
	"(web/search \"Gatehouse agent capabilities\")",
	"\"Search results with source URLs\"",
}

var webFetchDocumentation = capabilityDocumentation{
	"(web/fetch url) -> String",
	"Fetches a public HTTPS URL and returns its response body without conversion. The URL must be non-blank and non-sensitive.",
	"(web/fetch \"https://example.com/\")",
	"\"<!doctype html>...\"",
}

var webSearchHTTPClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

var webFetchHTTPClient = lisp.PublicHTTPSClient(0)

// NewWebModule constructs the web capability module for one agent evaluation.
func NewWebModule() lisp.HostModule {
	return newWebModule(webSearchEndpoint, webSearchHTTPClient)
}

func newWebModule(endpoint string, client *http.Client) lisp.HostModule {
	return newWebModuleWithFetch(endpoint, client, webFetch)
}

func newWebModuleWithFetch(endpoint string, client *http.Client, fetch webFetchRequest) lisp.HostModule {
	return lisp.HostModule{ID: gatehouseWebModuleID, Exports: []lisp.HostExport{
		{Name: "search", Value: document(lisp.FunctionWithContextNonLeaky(webSearchFunction(endpoint, client)), webSearchDocumentation)},
		{Name: "fetch", Value: document(lisp.FunctionWithContextNonLeaky(webFetchFunction(fetch)), webFetchDocumentation)},
	}}
}

func webSearchFunction(endpoint string, client *http.Client) func(*lisp.FunctionContext, []lisp.Expr) (error, lisp.Expr) {
	return func(call *lisp.FunctionContext, arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("web/search requires a query"), nil
		}
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone {
			return lisp.Errorf("web/search query must not be sensitive"), nil
		}
		err, query := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		query = strings.TrimSpace(query)
		if query == "" {
			return lisp.Errorf("web/search query must not be blank"), nil
		}
		result, err := webSearch(call.Context(), endpoint, client, query)
		if err != nil {
			return webRequestFailure("web/search", err), nil
		}
		return nil, lisp.String(result)
	}
}

type webFetchRequest func(context.Context, string) (string, error)

func webFetchFunction(fetch webFetchRequest) func(*lisp.FunctionContext, []lisp.Expr) (error, lisp.Expr) {
	return func(call *lisp.FunctionContext, arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("web/fetch requires a URL"), nil
		}
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone {
			return lisp.Errorf("web/fetch URL must not be sensitive"), nil
		}
		err, rawURL := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return lisp.Errorf("web/fetch URL must not be blank"), nil
		}
		body, err := fetch(call.Context(), rawURL)
		if err != nil {
			return webRequestFailure("web/fetch", err), nil
		}
		return nil, lisp.String(body)
	}
}

func webFetch(ctx context.Context, rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()
	if err := lisp.ValidatePublicHTTPSURL(ctx, rawURL); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "text/html, text/plain;q=0.9, application/xhtml+xml;q=0.8, */*;q=0.1")
	request.Header.Set("User-Agent", "gatehouse")
	response, err := webFetchHTTPClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("response status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, webFetchMaximumResponseSize+1))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(body) > webFetchMaximumResponseSize {
		return "", fmt.Errorf("response exceeds %d bytes", webFetchMaximumResponseSize)
	}
	return string(body), nil
}

func webSearch(ctx context.Context, endpoint string, client *http.Client, query string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, webSearchTimeout)
	defer cancel()
	requestPayload := webSearchMCPRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call"}
	requestPayload.Params.Name = "web_search_exa"
	requestPayload.Params.Arguments = webSearchMCPArguments{
		Query: query, Type: "auto", NumResults: 8, Livecrawl: "fallback", ContextMaxCharacters: 10000,
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "gatehouse")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("response status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, webSearchMaximumResponseSize+1))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(body) > webSearchMaximumResponseSize {
		return "", fmt.Errorf("response exceeds %d bytes", webSearchMaximumResponseSize)
	}
	result, ok, err := webSearchMCPPayload(body)
	if err != nil {
		return "", err
	}
	if ok {
		return result, nil
	}
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		result, ok, err := webSearchMCPPayload([]byte(line[len("data: "):]))
		if err != nil {
			return "", err
		}
		if ok {
			return result, nil
		}
	}
	return "", fmt.Errorf("response has no text content")
}

func webRequestFailure(operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s timed out: %w", operation, context.DeadlineExceeded)
	}
	return lisp.Errorf("%s failed", operation)
}

func webSearchMCPPayload(body []byte) (string, bool, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		return "", false, nil
	}
	var response struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", false, fmt.Errorf("decode response: %w", err)
	}
	for _, content := range response.Result.Content {
		if content.Type == "text" {
			return content.Text, true, nil
		}
	}
	return "", false, nil
}
