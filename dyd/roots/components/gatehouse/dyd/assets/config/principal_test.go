package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolvePrincipalsDefaultsEnabledAndSortsByAlias(t *testing.T) {
	disabled := false
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Principals: &[]configschema.Principal{
			{
				Alias: "zebra",
				Identities: &[]configschema.Identity{{
					Alias:     "zebra-gatehouse",
					Key:       "gatehouse:zebra",
					Verifiers: []configschema.Verifier{passwordVerifierRecord("gh-ver:zebra")},
				}},
			},
			{
				Alias:   "alpha-2",
				Name:    stringPointer("Alpha"),
				Enabled: &disabled,
				Identities: &[]configschema.Identity{{
					Alias:     "alpha-gatehouse",
					Key:       "gatehouse:alpha",
					Verifiers: []configschema.Verifier{passwordVerifierRecord("gh-ver:alpha")},
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
			Alias:    "alpha-2",
			Name:     stringPointer("Alpha"),
			Revision: 1,
			Enabled:  false,
			Identities: []Identity{{
				Alias:     "alpha-gatehouse",
				Key:       "gatehouse:alpha",
				Revision:  1,
				Verifiers: []configschema.Verifier{passwordVerifierRecord("gh-ver:alpha")},
				Enabled:   true,
			}},
		},
		{
			Alias:    "zebra",
			Revision: 1,
			Enabled:  true,
			Identities: []Identity{{
				Alias:     "zebra-gatehouse",
				Key:       "gatehouse:zebra",
				Revision:  1,
				Verifiers: []configschema.Verifier{passwordVerifierRecord("gh-ver:zebra")},
				Enabled:   true,
			}},
		},
	}
	if !reflect.DeepEqual(principals, want) {
		t.Fatalf("ResolvePrincipals() = %#v, want %#v", principals, want)
	}
}

func TestResolvePrincipalsResolvesPrecomputedVerifierFromEnvironment(t *testing.T) {
	t.Setenv("GATEHOUSE_ALICE_VERIFIER", "gh-ver:configured-from-env")
	document := principalDocument("alice", []configschema.Identity{{
		Alias:     "alice-gatehouse",
		Key:       "gatehouse:alice",
		Verifiers: []configschema.Verifier{passwordVerifierRecord("env:GATEHOUSE_ALICE_VERIFIER")},
	}})

	err, principals := ResolvePrincipals(document)
	if err != nil {
		t.Fatal(err)
	}
	if got := principals[0].Identities[0].Verifiers; !reflect.DeepEqual(got, []configschema.Verifier{passwordVerifierRecord("gh-ver:configured-from-env")}) {
		t.Fatalf("resolved verifiers = %#v", got)
	}
}

func TestResolvePrincipalsRejectsMissingPrecomputedVerifierEnvironment(t *testing.T) {
	const variable = "GATEHOUSE_TEST_UNSET_PASSWORD_VERIFIER"
	t.Setenv(variable, "set-before-unset")
	if err := os.Unsetenv(variable); err != nil {
		t.Fatal(err)
	}
	document := principalDocument("alice", []configschema.Identity{{
		Alias:     "alice-gatehouse",
		Key:       "gatehouse:alice",
		Verifiers: []configschema.Verifier{passwordVerifierRecord("env:" + variable)},
	}})

	if err, _ := ResolvePrincipals(document); err == nil || !strings.Contains(err.Error(), variable) {
		t.Fatalf("ResolvePrincipals() error = %v, want missing %s", err, variable)
	}
}

func TestValidateFileRejectsBareStringVerifier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
principals:
  - alias: alice
    identities:
      - alias: alice-gatehouse
        key: gatehouse:alice
        verifiers:
          - gh-ver:precomputed
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err, _ := ValidateFile(path); err == nil {
		t.Fatal("ValidateFile() accepted a bare string verifier")
	}
}

func TestValidateFileRejectsUnsupportedVerifierKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
principals:
  - alias: alice
    identities:
      - alias: alice-gatehouse
        key: gatehouse:alice
        verifiers:
          - kind: matrix
            password_verifier: gh-ver:precomputed
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err, _ := ValidateFile(path); err == nil {
		t.Fatal("ValidateFile() accepted an unsupported verifier kind")
	}
}

func TestValidateFileRejectsUnknownPasswordVerifierProperty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
principals:
  - alias: alice
    identities:
      - alias: alice-gatehouse
        key: gatehouse:alice
        verifiers:
          - kind: password
            password_verifier: gh-ver:precomputed
            sources:
              - env:ALICE_PASSWORD
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err, _ := ValidateFile(path); err == nil {
		t.Fatal("ValidateFile() accepted an unknown password verifier property")
	}
}

func TestResolvePrincipalsRejectsInvalidValues(t *testing.T) {
	invalidRevision := 0
	tests := []struct {
		name     string
		document configschema.GatehouseConfig
		contains string
	}{
		{
			name: "invalid principal alias",
			document: configschema.GatehouseConfig{ApiVersion: "v1", Principals: &[]configschema.Principal{{
				Alias: "Alice",
			}}},
			contains: "must match",
		},
		{
			name: "duplicate principal alias",
			document: configschema.GatehouseConfig{ApiVersion: "v1", Principals: &[]configschema.Principal{
				{Alias: "alice"},
				{Alias: "alice"},
			}},
			contains: "duplicated",
		},
		{
			name: "non-positive principal revision",
			document: configschema.GatehouseConfig{ApiVersion: "v1", Principals: &[]configschema.Principal{{
				Alias: "alice", Revision: &invalidRevision,
			}}},
			contains: "revision must be positive",
		},
		{
			name: "identity key without namespace",
			document: principalDocument("alice", []configschema.Identity{{
				Alias: "alice-gatehouse", Key: "alice", Verifiers: []configschema.Verifier{passwordVerifierRecord("gh-ver:configured")},
			}}),
			contains: "namespaced identity",
		},
		{
			name: "empty verifiers",
			document: principalDocument("alice", []configschema.Identity{{
				Alias: "alice-gatehouse", Key: "gatehouse:alice",
			}}),
			contains: "must not be empty",
		},
		{
			name: "unsupported identity namespace",
			document: principalDocument("alice", []configschema.Identity{{
				Alias: "alice-matrix", Key: "matrix:@alice:example.org", Verifiers: []configschema.Verifier{passwordVerifierRecord("gh-ver:configured")},
			}}),
			contains: "supported only for gatehouse identities",
		},
		{
			name: "missing password verifier option",
			document: principalDocument("alice", []configschema.Identity{{
				Alias:     "alice-gatehouse",
				Key:       "gatehouse:alice",
				Verifiers: []configschema.Verifier{{Kind: configschema.VerifierKindPasswordVerifier}},
			}}),
			contains: "configuration is missing",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err, _ := ResolvePrincipals(test.document); err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ResolvePrincipals() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func principalDocument(alias string, identities []configschema.Identity) configschema.GatehouseConfig {
	return configschema.GatehouseConfig{
		ApiVersion: "v1",
		Principals: &[]configschema.Principal{{
			Alias:      alias,
			Identities: &identities,
		}},
	}
}

func passwordVerifierRecord(value string) configschema.Verifier {
	return configschema.Verifier{
		Kind: configschema.VerifierKindPasswordVerifier,
		PasswordVerifier: &configschema.PasswordVerifier{
			Kind:             "password",
			PasswordVerifier: value,
		},
	}
}
