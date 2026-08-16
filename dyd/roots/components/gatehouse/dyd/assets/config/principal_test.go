package config

import (
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolvePrincipalsDefaultsEnabledAndSortsByAlias(t *testing.T) {
	disabled := false
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Principals: &[]configschema.GatehouseConfigPrincipalsValues{
			{
				Alias: "zebra",
				Identities: &[]configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
					Alias:     "zebra-matrix",
					Key:       "matrix:@zebra:example.org",
					Verifiers: []interface{}{map[string]any{"kind": "matrix"}},
				}},
			},
			{
				Alias:   "alpha-2",
				Name:    stringPointer("Alpha"),
				Enabled: &disabled,
				Identities: &[]configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
					Alias: "alpha-gatehouse",
					Key:   "gatehouse:alpha",
					Verifiers: []interface{}{map[string]any{
						"value": "gh-ver:AAAA",
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
			Alias:   "alpha-2",
			Name:    stringPointer("Alpha"),
			Enabled: false,
			Identities: []Identity{{
				Alias:    "alpha-gatehouse",
				Key:      "gatehouse:alpha",
				Revision: 1,
				Verifiers: []Verifier{{Value: stringPointer("gh-ver:AAAA"), Stored: "gh-ver:AAAA"}},
				Enabled:  true,
			}},
		},
		{
			Alias:   "zebra",
			Enabled: true,
			Identities: []Identity{{
				Alias:    "zebra-matrix",
				Key:      "matrix:@zebra:example.org",
				Revision: 1,
				Verifiers: []Verifier{{Stored: map[string]any{"kind": "matrix"}}},
				Enabled:  true,
			}},
		},
	}
	if !reflect.DeepEqual(principals, want) {
		t.Fatalf("ResolvePrincipals() = %#v, want %#v", principals, want)
	}
}

func TestResolvePrincipalsRejectsInvalidValues(t *testing.T) {
	validGatehouseVerifier := []interface{}{map[string]any{
		"value": "gh-ver:AAAA",
	}}
	tests := []struct {
		name     string
		document configschema.GatehouseConfig
		contains string
	}{
		{
			name: "invalid principal alias",
			document: configschema.GatehouseConfig{ApiVersion: "v1", Principals: &[]configschema.GatehouseConfigPrincipalsValues{{
				Alias: "Alice",
			}}},
			contains: "must match",
		},
		{
			name: "duplicate principal alias",
			document: configschema.GatehouseConfig{ApiVersion: "v1", Principals: &[]configschema.GatehouseConfigPrincipalsValues{
				{Alias: "alice"},
				{Alias: "alice"},
			}},
			contains: "duplicated",
		},
		{
			name: "identity key without namespace",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Alias:     "alice-gatehouse",
				Key:       "alice",
				Verifiers: validGatehouseVerifier,
			}}),
			contains: "namespaced identity",
		},
		{
			name: "empty verifiers",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Alias: "alice-matrix",
				Key:   "matrix:@alice:example.org",
			}}),
			contains: "must not be empty",
		},
		{
			name: "verifier is not an object",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Alias:     "alice-matrix",
				Key:       "matrix:@alice:example.org",
				Verifiers: []interface{}{"matrix"},
			}}),
			contains: "must be an object",
		},
		{
			name: "verifier without kind",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Alias:     "alice-matrix",
				Key:       "matrix:@alice:example.org",
				Verifiers: []interface{}{map[string]any{}},
			}}),
			contains: "kind must match",
		},
		{
			name: "unsupported gatehouse verifier",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Alias:     "alice-gatehouse",
				Key:       "gatehouse:alice",
				Verifiers: []interface{}{map[string]any{"kind": "matrix"}},
			}}),
			contains: "not supported",
		},
		{
			name: "unsupported identity namespace",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Alias:     "alice-unknown",
				Key:       "unknown:alice",
				Verifiers: []interface{}{map[string]any{"kind": "unknown"}},
			}}),
			contains: "namespace",
		},
		{
			name: "gatehouse verifier without canonical value",
			document: principalDocument("alice", []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues{{
				Alias:     "alice-gatehouse",
				Key:       "gatehouse:alice",
				Verifiers: []interface{}{map[string]any{"kind": "argon2id"}},
			}}),
			contains: "not supported",
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

func principalDocument(alias string, identities []configschema.GatehouseConfigPrincipalsValuesIdentitiesValues) configschema.GatehouseConfig {
	return configschema.GatehouseConfig{
		ApiVersion: "v1",
		Principals: &[]configschema.GatehouseConfigPrincipalsValues{{
			Alias:      alias,
			Identities: &identities,
		}},
	}
}
