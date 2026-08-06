package lisp

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	nativeNetHTTPID              = "native:net/http/v1"
	nativeNetHTTPTimeout         = 10 * time.Second
	nativeNetHTTPMaximumBodySize = 10 << 20
)

type nativeNetHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

var nativeNetHTTPRequestDocumentation = doc(
	"(http/request method url headers body) -> Response",
	"Performs an HTTP request and returns its status, headers, and bounded body. Completed HTTP responses, including non-2xx responses, are returned normally.",
	"(import (http @native:net/http/v1) (string/contains? (help http/request) \"(http/request\"))",
	"#t",
)

var nativeNetHTTPStatusDocumentation = doc(
	"(http/status response) -> Integer",
	"Returns the status code from an HTTP response value.",
	"(import (http @native:net/http/v1) (http/status (list (pair 'status 200) (pair 'headers null) (pair 'body (bytes/concat)))))",
	"200",
)

var nativeNetHTTPHeadersDocumentation = doc(
	"(http/headers response) -> List",
	"Returns the response headers as (String . String) pairs.",
	"(import (http @native:net/http/v1) (http/headers (list (pair 'status 200) (pair 'headers (list (pair \"Content-Type\" \"text/plain\"))) (pair 'body (bytes/concat)))))",
	"((\"Content-Type\" . \"text/plain\"))",
)

var nativeNetHTTPHeaderDocumentation = doc(
	"(http/header response name) -> List",
	"Returns all values for a case-insensitive response header name.",
	"(import (http @native:net/http/v1) (http/header (list (pair 'status 200) (pair 'headers (list (pair \"Link\" \"first\") (pair \"Link\" \"next\"))) (pair 'body (bytes/concat))) \"link\"))",
	"(\"first\" \"next\")",
)

var nativeNetHTTPBodyDocumentation = doc(
	"(http/body response) -> Bytes",
	"Returns the response body Bytes.",
	"(import (http @native:net/http/v1) (bytes/utf8/decode (http/body (list (pair 'status 200) (pair 'headers null) (pair 'body (bytes/utf8/encode \"ok\"))))))",
	"\"ok\"",
)

func nativeNetHTTPModule() Expr {
	return nativeNetHTTPModuleWithDoer(&http.Client{
		Timeout: nativeNetHTTPTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
}

func nativeNetHTTPModuleWithDoer(doer nativeNetHTTPDoer) Expr {
	request := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeNetHTTPRequest(doer)),
	}, nativeNetHTTPRequestDocumentation.text())
	status := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeNetHTTPStatus),
	}, nativeNetHTTPStatusDocumentation.text())
	headers := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeNetHTTPHeaders),
	}, nativeNetHTTPHeadersDocumentation.text())
	header := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeNetHTTPHeader),
	}, nativeNetHTTPHeaderDocumentation.text())
	body := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeNetHTTPBody),
	}, nativeNetHTTPBodyDocumentation.text())
	return list([]Expr{
		symbol("quote"),
		list([]Expr{
			pairValue(symbol("request"), request),
			pairValue(symbol("status"), status),
			pairValue(symbol("headers"), headers),
			pairValue(symbol("header"), header),
			pairValue(symbol("body"), body),
		}),
	})
}

func nativeNetHTTPRequest(doer nativeNetHTTPDoer) pureBuiltinCall {
	return func(_ *evaluator, arguments []Expr) (error, Expr) {
		if len(arguments) != 4 {
			return expressionError("http/request requires method, URL, headers, and body"), nil
		}
		err, method := requireString(arguments[0])
		if err != nil {
			return err, nil
		}
		err, rawURL := requireString(arguments[1])
		if err != nil {
			return err, nil
		}
		err, headers := nativeNetHTTPParseHeaders(arguments[2])
		if err != nil {
			return err, nil
		}
		err, body := requireBytes(arguments[3])
		if err != nil {
			return err, nil
		}
		if !nativeNetHTTPValidateURL(rawURL) {
			return expressionError("http/request requires an absolute HTTP or HTTPS URL without credentials"), nil
		}

		request, err := http.NewRequest(method, rawURL, bytes.NewReader([]byte(body)))
		if err != nil {
			return expressionError("http/request requires a valid method and URL"), nil
		}
		request.Header = headers
		response, err := doer.Do(request)
		if err != nil {
			return expressionError("http/request failed"), nil
		}
		if response == nil {
			return expressionError("http/request failed"), nil
		}
		var responseBody io.Reader = strings.NewReader("")
		if response.Body != nil {
			defer response.Body.Close()
			responseBody = response.Body
		}
		bodyBytes, err := io.ReadAll(io.LimitReader(responseBody, nativeNetHTTPMaximumBodySize+1))
		if err != nil {
			return expressionError("http/request could not read response body"), nil
		}
		if len(bodyBytes) > nativeNetHTTPMaximumBodySize {
			return expressionError("http/request response body exceeds %d bytes", nativeNetHTTPMaximumBodySize), nil
		}
		return nil, nativeNetHTTPResponse(response.StatusCode, response.Header, string(bodyBytes))
	}
}

func nativeNetHTTPValidateURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return false
	}
	return true
}

func nativeNetHTTPParseHeaders(expression Expr) (error, http.Header) {
	err, values := expressions(expression)
	if err != nil {
		return expressionError("http/request headers must be a proper list of (String . String) pairs"), nil
	}
	headers := make(http.Header, len(values))
	for _, value := range values {
		base, _ := unwrap(value)
		pair, ok := base.(*pair)
		if !ok {
			return expressionError("http/request headers must be a proper list of (String . String) pairs"), nil
		}
		err, name := requireString(pair.first)
		if err != nil {
			return err, nil
		}
		err, headerValue := requireString(pair.rest)
		if err != nil {
			return err, nil
		}
		if name == "" || strings.ContainsAny(name, "\r\n") || strings.ContainsAny(headerValue, "\r\n") {
			return expressionError("http/request headers must contain valid names and values"), nil
		}
		headers.Add(name, headerValue)
	}
	return nil, headers
}

func nativeNetHTTPResponse(status int, headers http.Header, body string) Expr {
	return list([]Expr{
		pairValue(symbol("status"), integer(int64(status))),
		pairValue(symbol("headers"), nativeNetHTTPHeadersValue(headers)),
		pairValue(symbol("body"), bytesValue(body)),
	})
}

func nativeNetHTTPHeadersValue(headers http.Header) Expr {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	values := make([]Expr, 0, len(headers))
	for _, name := range names {
		for _, value := range headers[name] {
			values = append(values, pairValue(stringValue(name), stringValue(value)))
		}
	}
	return list(values)
}

func nativeNetHTTPStatus(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("http/status requires one response value"), nil
	}
	err, response := nativeNetHTTPParseResponse(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, response.status
}

func nativeNetHTTPHeaders(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("http/headers requires one response value"), nil
	}
	err, response := nativeNetHTTPParseResponse(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, response.headers
}

func nativeNetHTTPHeader(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("http/header requires a response value and header name"), nil
	}
	err, response := nativeNetHTTPParseResponse(arguments[0])
	if err != nil {
		return err, nil
	}
	err, name := requireString(arguments[1])
	if err != nil {
		return err, nil
	}
	err, headers := expressions(response.headers)
	if err != nil {
		return expressionError("http/header requires a valid response value"), nil
	}
	values := make([]Expr, 0)
	for _, header := range headers {
		base, _ := unwrap(header)
		pair, ok := base.(*pair)
		if !ok {
			return expressionError("http/header requires a valid response value"), nil
		}
		err, headerName := requireString(pair.first)
		if err != nil {
			return expressionError("http/header requires a valid response value"), nil
		}
		if !strings.EqualFold(headerName, name) {
			continue
		}
		values = append(values, pair.rest)
	}
	return nil, list(values)
}

func nativeNetHTTPBody(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("http/body requires one response value"), nil
	}
	err, response := nativeNetHTTPParseResponse(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, response.body
}

type nativeNetHTTPParsedResponse struct {
	status  Expr
	headers Expr
	body    Expr
}

func nativeNetHTTPParseResponse(expression Expr) (error, nativeNetHTTPParsedResponse) {
	err, fields := expressions(expression)
	if err != nil || len(fields) != 3 {
		return expressionError("http response must contain status, headers, and body fields"), nativeNetHTTPParsedResponse{}
	}
	response := nativeNetHTTPParsedResponse{}
	for _, field := range fields {
		base, _ := unwrap(field)
		fieldPair, ok := base.(*pair)
		if !ok {
			return expressionError("http response must contain status, headers, and body fields"), nativeNetHTTPParsedResponse{}
		}
		name, _ := unwrap(fieldPair.first)
		symbol, ok := name.(*symbolExpr)
		if !ok {
			return expressionError("http response must contain status, headers, and body fields"), nativeNetHTTPParsedResponse{}
		}
		switch symbol.value {
		case "status":
			if response.status != nil {
				return expressionError("http response must contain status, headers, and body fields"), nativeNetHTTPParsedResponse{}
			}
			err, _ := requireInteger(fieldPair.rest)
			if err != nil {
				return expressionError("http response status must be an Integer"), nativeNetHTTPParsedResponse{}
			}
			response.status = fieldPair.rest
		case "headers":
			if response.headers != nil {
				return expressionError("http response must contain status, headers, and body fields"), nativeNetHTTPParsedResponse{}
			}
			err, headers := expressions(fieldPair.rest)
			if err != nil {
				return expressionError("http response headers must be a proper list of (String . String) pairs"), nativeNetHTTPParsedResponse{}
			}
			for _, header := range headers {
				base, _ := unwrap(header)
				headerPair, ok := base.(*pair)
				if !ok {
					return expressionError("http response headers must be a proper list of (String . String) pairs"), nativeNetHTTPParsedResponse{}
				}
				err, _ := requireString(headerPair.first)
				if err != nil {
					return expressionError("http response headers must be a proper list of (String . String) pairs"), nativeNetHTTPParsedResponse{}
				}
				err, _ = requireString(headerPair.rest)
				if err != nil {
					return expressionError("http response headers must be a proper list of (String . String) pairs"), nativeNetHTTPParsedResponse{}
				}
			}
			response.headers = fieldPair.rest
		case "body":
			if response.body != nil {
				return expressionError("http response must contain status, headers, and body fields"), nativeNetHTTPParsedResponse{}
			}
			err, _ := requireBytes(fieldPair.rest)
			if err != nil {
				return expressionError("http response body must be Bytes"), nativeNetHTTPParsedResponse{}
			}
			response.body = fieldPair.rest
		default:
			return expressionError("http response must contain status, headers, and body fields"), nativeNetHTTPParsedResponse{}
		}
	}
	if response.status == nil || response.headers == nil || response.body == nil {
		return expressionError("http response must contain status, headers, and body fields"), nativeNetHTTPParsedResponse{}
	}
	return nil, response
}
