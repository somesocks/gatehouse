package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"gatehouse/configschema"
)

const defaultHTTPListen = "127.0.0.1:4283"

type Services struct {
	HTTP *HTTPService
}

type HTTPService struct {
	Enabled       bool
	Listen        string
	PublicBaseURL string
	Keychain      string
	Web           bool
	API           bool
}

func ResolveServices(document configschema.GatehouseConfig) (error, Services) {
	if document.Services == nil {
		service := defaultHTTPService()
		service.PublicBaseURL = loopbackHTTPPublicBaseURL(service.Listen)
		return nil, Services{HTTP: service}
	}
	if document.Services.Http == nil {
		return nil, Services{}
	}

	configured := document.Services.Http
	service := defaultHTTPService()
	if configured.Enabled != nil {
		service.Enabled = *configured.Enabled
	}
	if configured.Listen != nil {
		service.Listen = *configured.Listen
	}
	if configured.PublicBaseUrl != nil {
		origin, err := CanonicalHTTPPublicBaseURL(*configured.PublicBaseUrl)
		if err != nil {
			return fmt.Errorf("services.http.public_base_url: %w", err), Services{}
		}
		service.PublicBaseURL = origin
	}
	if configured.Keychain != nil {
		service.Keychain = *configured.Keychain
	}
	if configured.Web != nil && configured.Web.Enabled != nil {
		service.Web = *configured.Web.Enabled
	}
	if configured.Api != nil && configured.Api.Enabled != nil {
		service.API = *configured.Api.Enabled
	}
	if err := validateHTTPListen(service.Listen); err != nil {
		return fmt.Errorf("services.http.listen: %w", err), Services{}
	}
	if configured.PublicBaseUrl == nil {
		service.PublicBaseURL = loopbackHTTPPublicBaseURL(service.Listen)
	}
	return nil, Services{HTTP: service}
}

func defaultHTTPService() *HTTPService {
	return &HTTPService{
		Enabled:  true,
		Listen:   defaultHTTPListen,
		Keychain: defaultKeychainID,
		Web:      true,
		API:      true,
	}
}

func validateHTTPListen(value string) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil || host == "" {
		return fmt.Errorf("must be a host:port address")
	}
	_, err = strconv.ParseUint(portText, 10, 16)
	if err != nil {
		return fmt.Errorf("must have a port from 0 through 65535")
	}
	return nil
}

// A fixed loopback listener has an unambiguous local origin without trusting
// the incoming Host or proxy headers. Wildcard, remote, and ephemeral listeners
// must have an explicitly configured public base URL to launch an input.
func loopbackHTTPPublicBaseURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil || port == "0" {
		return ""
	}
	address := net.ParseIP(host)
	if !strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") && (address == nil || !address.IsLoopback()) {
		return ""
	}
	return "http://" + net.JoinHostPort(host, port)
}

// CanonicalHTTPPublicBaseURL accepts only an origin. Gatehouse serves /app and
// /api at the root of this origin, so paths and other URL components would
// produce incorrect (and potentially credential-leaking) launch links.
func CanonicalHTTPPublicBaseURL(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value || strings.Contains(value, "#") {
		return "", fmt.Errorf("must be an absolute HTTP(S) origin")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.Opaque != "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return "", fmt.Errorf("must be an absolute HTTP(S) origin without a path, credentials, query, or fragment")
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return "", fmt.Errorf("must include a port number after ':'")
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return "", fmt.Errorf("port must be from 1 through 65535")
		}
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		address := net.ParseIP(host)
		if !strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") && (address == nil || !address.IsLoopback()) {
			return "", fmt.Errorf("HTTP is only allowed for localhost or loopback addresses")
		}
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}
