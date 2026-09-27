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
	want := &HTTPService{Enabled: true, Listen: "127.0.0.1:4283", PublicBaseURL: "http://127.0.0.1:4283", Keychain: "default", Web: true, API: true}
	if *services.HTTP != *want {
		t.Fatalf("ResolveServices() = %#v, want %#v", services.HTTP, want)
	}
}

func TestResolveServicesUsesExplicitConfiguration(t *testing.T) {
	disabled := false
	listen := "localhost:7183"
	publicBaseURL := "https://gatehouse.example.test/"
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Services: &configschema.GatehouseConfigServices{
			Http: &configschema.GatehouseConfigServicesHttp{
				Listen:        &listen,
				PublicBaseUrl: &publicBaseURL,
				Web:           &configschema.GatehouseConfigServicesHttpWeb{Enabled: &disabled},
			},
		},
	}

	err, services := ResolveServices(document)
	if err != nil {
		t.Fatal(err)
	}
	want := &HTTPService{Enabled: true, Listen: "localhost:7183", PublicBaseURL: "https://gatehouse.example.test", Keychain: "default", Web: false, API: true}
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
	for _, listen := range []string{"", "4283", ":4283", "localhost:http", "localhost:65536"} {
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

func TestResolveServicesDerivesOnlyFixedLoopbackOrigins(t *testing.T) {
	for _, test := range []struct {
		listen string
		origin string
	}{
		{"localhost:7183", "http://localhost:7183"},
		{"127.0.0.2:4283", "http://127.0.0.2:4283"},
		{"[::1]:4283", "http://[::1]:4283"},
		{"localhost:0", ""},
		{"0.0.0.0:4283", ""},
		{"[::]:4283", ""},
		{"192.0.2.1:4283", ""},
	} {
		t.Run(test.listen, func(t *testing.T) {
			err, services := ResolveServices(configschema.GatehouseConfig{
				ApiVersion: "v1",
				Services: &configschema.GatehouseConfigServices{Http: &configschema.GatehouseConfigServicesHttp{
					Listen: &test.listen,
				}},
			})
			if err != nil || services.HTTP == nil || services.HTTP.PublicBaseURL != test.origin {
				t.Fatalf("ResolveServices(%q) = (%#v, %v), want origin %q", test.listen, services.HTTP, err, test.origin)
			}
		})
	}
}

func TestResolveServicesValidatesPublicBaseURL(t *testing.T) {
	for _, test := range []struct {
		url  string
		want string
	}{
		{"https://gatehouse.example.test", "https://gatehouse.example.test"},
		{"https://gatehouse.example.test/", "https://gatehouse.example.test"},
		{"http://localhost:4283/", "http://localhost:4283"},
		{"http://127.0.0.1:4283", "http://127.0.0.1:4283"},
		{"http://[::1]:4283", "http://[::1]:4283"},
		{"", ""},
		{"gatehouse.example.test", ""},
		{"ftp://gatehouse.example.test", ""},
		{"http://gatehouse.example.test", ""},
		{"https://user:secret@gatehouse.example.test", ""},
		{"https://gatehouse.example.test/input", ""},
		{"https://gatehouse.example.test/?token=secret", ""},
		{"https://gatehouse.example.test/#token", ""},
		{"https://gatehouse.example.test/#", ""},
		{"https://gatehouse.example.test:", ""},
		{"https://gatehouse.example.test:65536", ""},
	} {
		t.Run(test.url, func(t *testing.T) {
			err, services := ResolveServices(configschema.GatehouseConfig{
				ApiVersion: "v1",
				Services: &configschema.GatehouseConfigServices{Http: &configschema.GatehouseConfigServicesHttp{
					PublicBaseUrl: &test.url,
				}},
			})
			if test.want == "" {
				if err == nil || !strings.Contains(err.Error(), "services.http.public_base_url") {
					t.Fatalf("ResolveServices(%q) error = %v", test.url, err)
				}
			} else if err != nil || services.HTTP == nil || services.HTTP.PublicBaseURL != test.want {
				t.Fatalf("ResolveServices(%q) = (%#v, %v), want %q", test.url, services.HTTP, err, test.want)
			}
		})
	}
}
