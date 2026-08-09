package config

import (
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolveServicesUsesImplicitHTTPService(t *testing.T) {
	err, services := ResolveServices(configschema.GatehouseConfig{ApiVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if services.HTTP == nil {
		t.Fatal("ResolveServices() returned no implicit HTTP service")
	}
	want := &HTTPService{Enabled: true, Listen: "127.0.0.1:4283", Keychain: "default", Web: true, API: true}
	if *services.HTTP != *want {
		t.Fatalf("ResolveServices() = %#v, want %#v", services.HTTP, want)
	}
}

func TestResolveServicesUsesExplicitConfiguration(t *testing.T) {
	disabled := false
	listen := "localhost:7183"
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Services: &configschema.GatehouseConfigServices{
			Http: &configschema.GatehouseConfigServicesHttp{
				Listen: &listen,
				Web:    &configschema.GatehouseConfigServicesHttpWeb{Enabled: &disabled},
			},
		},
	}

	err, services := ResolveServices(document)
	if err != nil {
		t.Fatal(err)
	}
	want := &HTTPService{Enabled: true, Listen: "localhost:7183", Keychain: "default", Web: false, API: true}
	if services.HTTP == nil || *services.HTTP != *want {
		t.Fatalf("ResolveServices() = %#v, want %#v", services.HTTP, want)
	}

	document.Services.Http.Enabled = &disabled
	err, services = ResolveServices(document)
	if err != nil {
		t.Fatal(err)
	}
	if services.HTTP == nil || services.HTTP.Enabled {
		t.Fatalf("ResolveServices() disabled service = %#v, want disabled HTTP service", services.HTTP)
	}

	document.Services.Http = nil
	err, services = ResolveServices(document)
	if err != nil {
		t.Fatal(err)
	}
	if services.HTTP != nil {
		t.Fatalf("ResolveServices() explicit empty services = %#v, want no services", services)
	}
}

func TestResolveServicesRejectsInvalidListener(t *testing.T) {
	for _, listen := range []string{"", "4283", ":4283", "localhost:http", "localhost:0", "localhost:65536"} {
		t.Run(listen, func(t *testing.T) {
			err, _ := ResolveServices(configschema.GatehouseConfig{
				ApiVersion: "v1",
				Services: &configschema.GatehouseConfigServices{
					Http: &configschema.GatehouseConfigServicesHttp{Listen: &listen},
				},
			})
			if err == nil || !strings.Contains(err.Error(), "services.http.listen") {
				t.Fatalf("ResolveServices() error = %v, want listener validation error", err)
			}
		})
	}
}
