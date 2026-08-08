package config

import (
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestResolveKeychainsUsesImplicitDefault(t *testing.T) {
	err, keychains := ResolveKeychains(configschema.GatehouseConfig{ApiVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Keychain{{
		ID:   "default",
		Refs: []string{"env:GATEHOUSE_KEYCHAIN", "stdin:"},
	}}
	if !reflect.DeepEqual(keychains, want) {
		t.Fatalf("ResolveKeychains() = %#v, want %#v", keychains, want)
	}
}

func TestResolveKeychainsUsesExplicitConfiguration(t *testing.T) {
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Keychains: &[]configschema.GatehouseConfigKeychainsValues{
			{Id: "zebra", Refs: []string{"env:ZEBRA_KEY"}},
			{Id: "alpha", Refs: []string{"env:ALPHA_KEY", "stdin:"}},
		},
	}
	err, keychains := ResolveKeychains(document)
	if err != nil {
		t.Fatal(err)
	}
	want := []Keychain{
		{ID: "alpha", Refs: []string{"env:ALPHA_KEY", "stdin:"}},
		{ID: "zebra", Refs: []string{"env:ZEBRA_KEY"}},
	}
	if !reflect.DeepEqual(keychains, want) {
		t.Fatalf("ResolveKeychains() = %#v, want %#v", keychains, want)
	}

	document.Keychains = &[]configschema.GatehouseConfigKeychainsValues{}
	err, keychains = ResolveKeychains(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(keychains) != 0 {
		t.Fatalf("ResolveKeychains() explicit empty configuration = %#v, want no keychains", keychains)
	}
}

func TestResolveKeychainsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		keychains []configschema.GatehouseConfigKeychainsValues
		contains  string
	}{
		{
			name: "invalid ID",
			keychains: []configschema.GatehouseConfigKeychainsValues{{
				Id: "Default", Refs: []string{"env:KEY"},
			}},
			contains: "must match",
		},
		{
			name: "duplicate ID",
			keychains: []configschema.GatehouseConfigKeychainsValues{
				{Id: "default", Refs: []string{"env:KEY"}},
				{Id: "default", Refs: []string{"env:OTHER_KEY"}},
			},
			contains: "duplicated",
		},
		{
			name: "empty refs",
			keychains: []configschema.GatehouseConfigKeychainsValues{{
				Id: "default", Refs: []string{},
			}},
			contains: "must not be empty",
		},
		{
			name: "invalid source",
			keychains: []configschema.GatehouseConfigKeychainsValues{{
				Id: "default", Refs: []string{"file:./key"},
			}},
			contains: "must be an env",
		},
		{
			name: "stdin is not final",
			keychains: []configschema.GatehouseConfigKeychainsValues{{
				Id: "default", Refs: []string{"stdin:", "env:KEY"},
			}},
			contains: "must be the final fallback",
		},
		{
			name: "duplicate source",
			keychains: []configschema.GatehouseConfigKeychainsValues{{
				Id: "default", Refs: []string{"env:KEY", "env:KEY"},
			}},
			contains: "duplicated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := configschema.GatehouseConfig{ApiVersion: "v1", Keychains: &test.keychains}
			err, _ := ResolveKeychains(document)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ResolveKeychains() error = %v, want %q", err, test.contains)
			}
		})
	}
}
