package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gatehouse/configschema"
	"gatehouse/kdf"
)

func TestResolveCrypto(t *testing.T) {
	for _, test := range []struct {
		name       string
		configured *configschema.GatehouseConfigCrypto
		want       string
	}{
		{"omitted", nil, kdf.Default},
		{"empty section", &configschema.GatehouseConfigCrypto{}, kdf.Default},
		{"v0", &configschema.GatehouseConfigCrypto{DefaultKdf: cryptoString(kdf.InsecureV0)}, kdf.InsecureV0},
		{"v1", &configschema.GatehouseConfigCrypto{DefaultKdf: cryptoString(kdf.Default)}, kdf.Default},
	} {
		t.Run(test.name, func(t *testing.T) {
			err, got := ResolveCrypto(configschema.GatehouseConfig{ApiVersion: "v1", Crypto: test.configured})
			if err != nil || got.KDFAlgorithm() != test.want {
				t.Fatalf("ResolveCrypto() = %#v, %v; want %q", got, err, test.want)
			}
		})
	}
	if got := (Crypto{}).KDFAlgorithm(); got != kdf.Default {
		t.Fatalf("zero-value crypto default = %q", got)
	}
}

func TestValidateCryptoConfiguration(t *testing.T) {
	for _, test := range []struct {
		name, contents string
		valid          bool
	}{
		{"valid.yaml", "api_version: v1\ncrypto:\n  default_kdf: pbkdf2-hmac-sha256-v0\n", true},
		{"valid.json", `{"api_version":"v1","crypto":{"default_kdf":"pbkdf2-hmac-sha256-v1"}}`, true},
		{"unknown.yaml", "api_version: v1\ncrypto:\n  default_kdf: unknown\n", false},
		{"empty.yaml", "api_version: v1\ncrypto:\n  default_kdf: ''\n", false},
		{"type.yaml", "api_version: v1\ncrypto:\n  default_kdf: 0\n", false},
		{"field.yaml", "api_version: v1\ncrypto:\n  rounds: 1\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name)
			if err := os.WriteFile(path, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			err, _ := ValidateFile(path)
			if (err == nil) != test.valid {
				t.Fatalf("ValidateFile() = %v, want valid=%t", err, test.valid)
			}
		})
	}
	for _, value := range []string{"", "unknown", "pbkdf2-hmac-sha256-v2"} {
		err, _ := ResolveCrypto(configschema.GatehouseConfig{Crypto: &configschema.GatehouseConfigCrypto{DefaultKdf: &value}})
		if err == nil || !strings.Contains(err.Error(), "crypto.default_kdf") {
			t.Fatalf("ResolveCrypto(%q) = %v", value, err)
		}
	}
}

func cryptoString(value string) *string { return &value }
