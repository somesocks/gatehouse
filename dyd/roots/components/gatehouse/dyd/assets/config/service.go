package config

import (
	"fmt"
	"net"
	"strconv"

	"gatehouse/configschema"
)

const defaultHTTPListen = "127.0.0.1:4283"

type Services struct {
	HTTP *HTTPService
}

type HTTPService struct {
	Enabled bool
	Listen  string
	Web     bool
	API     bool
}

func ResolveServices(document configschema.GatehouseConfig) (error, Services) {
	if document.Services == nil {
		return nil, Services{HTTP: defaultHTTPService()}
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
	if configured.Web != nil && configured.Web.Enabled != nil {
		service.Web = *configured.Web.Enabled
	}
	if configured.Api != nil && configured.Api.Enabled != nil {
		service.API = *configured.Api.Enabled
	}
	if err := validateHTTPListen(service.Listen); err != nil {
		return fmt.Errorf("services.http.listen: %w", err), Services{}
	}

	return nil, Services{HTTP: service}
}

func defaultHTTPService() *HTTPService {
	return &HTTPService{
		Enabled: true,
		Listen:  defaultHTTPListen,
		Web:     true,
		API:     true,
	}
}

func validateHTTPListen(value string) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil || host == "" {
		return fmt.Errorf("must be a host:port address")
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return fmt.Errorf("must have a port from 1 through 65535")
	}
	return nil
}
