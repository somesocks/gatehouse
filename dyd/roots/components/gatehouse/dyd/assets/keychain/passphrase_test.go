package keychain

import (
	"bytes"
	"strings"
	"testing"

	"gatehouse/config"
)

func TestPassphraseSourceResolverChecksSourcesInOrder(t *testing.T) {
	var prompted bytes.Buffer
	var lookedUp []string
	resolver := newPassphraseSourceResolver(strings.NewReader("stdin-passphrase\n"), &prompted, func(name string) string {
		lookedUp = append(lookedUp, name)
		if name == "UNREACHED" {
			t.Fatal("resolved a source after finding a passphrase")
		}
		return ""
	}, nil)
	err, passphrase := resolver.Resolve(config.Keychain{
		ID: "default",
		Sources: []config.KeychainPassphraseSource{
			"env:EMPTY",
			"stdin:",
			"env:UNREACHED",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(passphrase), "stdin-passphrase"; got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
	if got, want := strings.Join(lookedUp, ","), "EMPTY"; got != want {
		t.Fatalf("environment lookups = %q, want %q", got, want)
	}
	if got, want := prompted.String(), "Keychain \"default\" passphrase: "; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestPassphraseSourceResolverSkipsEmptySources(t *testing.T) {
	var prompted bytes.Buffer
	resolver := newPassphraseSourceResolver(strings.NewReader("\n"), &prompted, func(name string) string {
		if name != "FALLBACK" {
			t.Fatalf("environment lookup = %q, want FALLBACK", name)
		}
		return "environment-passphrase"
	}, nil)
	err, passphrase := resolver.Resolve(config.Keychain{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"stdin:", "env:FALLBACK"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(passphrase), "environment-passphrase"; got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
	if got, want := prompted.String(), "Keychain \"default\" passphrase: "; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestPassphraseSourceResolverPromptsForEachKeychain(t *testing.T) {
	var prompted bytes.Buffer
	resolver := newPassphraseSourceResolver(strings.NewReader("alpha-passphrase\nbeta-passphrase\n"), &prompted, func(string) string {
		return ""
	}, nil)
	for _, keychain := range []config.Keychain{
		{ID: "alpha", Sources: []config.KeychainPassphraseSource{"stdin:"}},
		{ID: "beta", Sources: []config.KeychainPassphraseSource{"stdin:"}},
	} {
		err, passphrase := resolver.Resolve(keychain)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(passphrase), keychain.ID+"-passphrase"; got != want {
			t.Fatalf("Resolve(%q) = %q, want %q", keychain.ID, got, want)
		}
	}
	if got, want := prompted.String(), "Keychain \"alpha\" passphrase: Keychain \"beta\" passphrase: "; got != want {
		t.Fatalf("prompts = %q, want %q", got, want)
	}
}

func TestPassphraseSourceResolverRejectsExhaustedSources(t *testing.T) {
	resolver := newPassphraseSourceResolver(strings.NewReader(""), &bytes.Buffer{}, func(string) string {
		return ""
	}, nil)
	err, _ := resolver.Resolve(config.Keychain{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:EMPTY", "stdin:"},
	})
	if err == nil || !strings.Contains(err.Error(), "no source provided a passphrase") {
		t.Fatalf("Resolve() error = %v, want exhausted source error", err)
	}
}

func TestPassphraseSourceResolverUsesHiddenTerminalInput(t *testing.T) {
	var prompted bytes.Buffer
	resolver := newPassphraseSourceResolver(strings.NewReader("unused\n"), &prompted, func(string) string {
		return ""
	}, func() (error, []byte) {
		return nil, []byte("terminal-passphrase")
	})
	err, passphrase := resolver.Resolve(config.Keychain{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"stdin:"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(passphrase), "terminal-passphrase"; got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
	if got, want := prompted.String(), "Keychain \"default\" passphrase: \n"; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}
