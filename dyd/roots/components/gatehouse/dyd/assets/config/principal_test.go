package config

import (
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolvePrincipalsDefaultsEnabledAndSortsByID(t *testing.T) {
	disabled := false
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Principals: &[]configschema.GatehouseConfigPrincipalsValues{
			{
				Id: "zebra",
				Identities: &[]configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
					Id:        "matrix:@zebra:example.org",
					Verifiers: []interface{}{map[string]any{"kind": "matrix"}},
				}},
			},
			{
				Id:      "alpha-2",
				Name:    stringPointer("Alpha"),
				Enabled: &disabled,
				Identities: &[]configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
					Id: "gatehouse:alpha",
					Verifiers: []interface{}{map[string]any{
						"kind":              "argon2id",
						"password_verifier": "env:GATEHOUSE_ALPHA_PASSWORD_VERIFIER",
					}},
				}},
			},
		},
	}

	err, principals := ResolvePrincipals(document)
	if err != nil {
		t.Fatal(err)
	}
	want := []Principal{
		{
			ID:      "alpha-2",
			Name:    stringPointer("Alpha"),
			Enabled: false,
			Identities: []Identity{{
				ID:        "gatehouse:alpha",
				Verifiers: `[{"kind":"argon2id","password_verifier":"env:GATEHOUSE_ALPHA_PASSWORD_VERIFIER"}]`,
				Enabled:   true,
			}},
		},
		{
			ID:      "zebra",
			Enabled: true,
			Identities: []Identity{{
				ID:        "matrix:@zebra:example.org",
				Verifiers: `[{"kind":"matrix"}]`,
				Enabled:   true,
			}},
		},
	}
	if !reflect.DeepEqual(principals, want) {
		t.Fatalf("ResolvePrincipals() = %#v, want %#v", principals, want)
	}
}

func TestResolvePrincipalsRejectsInvalidValues(t *testing.T) {
	validGatehouseVerifier := []interface{}{map[string]any{
		"kind":              "argon2id",
		"password_verifier": "env:GATEHOUSE_PASSWORD_VERIFIER",
	}}
	tests := []struct {
		name     string
		document configschema.GatehouseConfig
		contains string
	}{
		{
			name: "invalid principal ID",
			document: configschema.GatehouseConfig{ApiVersion: "v1", Principals: &[]configschema.GatehouseConfigPrincipalsValues{{
				Id: "Alice",
			}}},
			contains: "must match",
		},
		{
			name: "duplicate principal ID",
			document: configschema.GatehouseConfig{ApiVersion: "v1", Principals: &[]configschema.GatehouseConfigPrincipalsValues{
				{Id: "alice"},
				{Id: "alice"},
			}},
			contains: "duplicated",
		},
		{
			name: "identity without namespace",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Id:        "alice",
				Verifiers: validGatehouseVerifier,
			}}),
			contains: "namespaced identity",
		},
		{
			name: "empty verifiers",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Id: "matrix:@alice:example.org",
			}}),
			contains: "must not be empty",
		},
		{
			name: "verifier is not an object",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Id:        "matrix:@alice:example.org",
				Verifiers: []interface{}{"matrix"},
			}}),
			contains: "must be an object",
		},
		{
			name: "verifier without kind",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Id:        "matrix:@alice:example.org",
				Verifiers: []interface{}{map[string]any{}},
			}}),
			contains: "kind must match",
		},
		{
			name: "unsupported gatehouse verifier",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Id:        "gatehouse:alice",
				Verifiers: []interface{}{map[string]any{"kind": "matrix"}},
			}}),
			contains: "not supported",
		},
		{
			name: "unsupported identity namespace",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Id:        "unknown:alice",
				Verifiers: []interface{}{map[string]any{"kind": "unknown"}},
			}}),
			contains: "namespace",
		},
		{
			name: "gatehouse verifier without password verifier",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Id:        "gatehouse:alice",
				Verifiers: []interface{}{map[string]any{"kind": "argon2id"}},
			}}),
			contains: "password_verifier",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err, _ := ResolvePrincipals(test.document)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ResolvePrincipals() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func principalDocument(id string, identities []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues) configschema.GatehouseConfig {
	return configschema.GatehouseConfig{
		ApiVersion: "v1",
		Principals: &[]configschema.GatehouseConfigPrincipalsValues{{
			Id:         id,
			Identities: &identities,
		}},
	}
}
