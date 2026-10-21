package identity

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"crypto/pbkdf2"
	"gatehouse/config"
	"gatehouse/configschema"
	"gatehouse/kdf"
)

const (
	passwordSaltSize   = 16
	passwordDigestSize = 32
)

type PasswordSourceResolver struct {
	stdin  *bufio.Reader
	getenv func(string) string
	crypto config.Crypto
}

func NewPasswordSourceResolver(crypto config.Crypto) *PasswordSourceResolver {
	return &PasswordSourceResolver{stdin: bufio.NewReader(os.Stdin), getenv: os.Getenv, crypto: crypto}
}

func (resolver *PasswordSourceResolver) Resolve(identityID string, sources []config.PasswordSource) (error, []byte) {
	for _, source := range sources {
		var password []byte
		switch {
		case strings.HasPrefix(string(source), "env:"):
			password = []byte(resolver.getenv(strings.TrimPrefix(string(source), "env:")))
		case source == "stdin:":
			value, err := resolver.stdin.ReadBytes('\n')
			if err != nil && err != io.EOF {
				return fmt.Errorf("read password for identity %q: %w", identityID, err), nil
			}
			password = bytes.TrimSuffix(bytes.TrimSuffix(value, []byte("\n")), []byte("\r"))
		default:
			return fmt.Errorf("resolve password for identity %q: unsupported source %q", identityID, source), nil
		}
		if len(password) > 0 {
			return nil, password
		}
	}
	return fmt.Errorf("resolve password for identity %q: no source provided a password", identityID), nil
}

func ResolveVerifiers(identityID string, configured []configschema.Verifier, passwordSources []config.PasswordSource, resolver *PasswordSourceResolver) (error, []interface{}) {
	verifiers := make([]interface{}, 0, len(configured)+1)
	for index, verifier := range configured {
		record, err := configuredPasswordVerifier(verifier)
		if err != nil {
			return fmt.Errorf("resolve verifier %d for identity %q: %w", index, identityID, err), nil
		}
		verifiers = append(verifiers, record)
	}
	if len(passwordSources) > 0 {
		resolveErr, passwordVerifier := resolveDefaultPasswordVerifier(identityID, passwordSources, resolver)
		if resolveErr != nil {
			return resolveErr, nil
		}
		verifiers = append(verifiers, passwordVerifier)
	}
	return nil, verifiers
}

func configuredPasswordVerifier(configured configschema.Verifier) (configschema.PasswordVerifier, error) {
	if configured.Kind != configschema.VerifierKindPasswordVerifier || configured.PasswordVerifier == nil {
		return configschema.PasswordVerifier{}, fmt.Errorf("unsupported verifier kind")
	}
	record := *configured.PasswordVerifier
	if record.Kind != "password" || !strings.HasPrefix(record.PasswordVerifier, "gh-ver:") {
		return configschema.PasswordVerifier{}, fmt.Errorf("invalid password verifier record")
	}
	return record, nil
}

func resolveDefaultPasswordVerifier(identityID string, sources []config.PasswordSource, resolver *PasswordSourceResolver) (error, configschema.PasswordVerifier) {
	passwordErr, password := resolver.Resolve(identityID, sources)
	if passwordErr != nil {
		return passwordErr, configschema.PasswordVerifier{}
	}
	defer clear(password)
	if bytes.IndexByte(password, 0) >= 0 {
		return fmt.Errorf("password for identity %q must not contain NUL", identityID), configschema.PasswordVerifier{}
	}
	verifier, err := passwordVerifier(password, resolver.crypto)
	if err != nil {
		return err, configschema.PasswordVerifier{}
	}
	return nil, configschema.PasswordVerifier{Kind: "password", PasswordVerifier: verifier}
}

type verifierSourceDocument struct {
	Verifiers       []configschema.PasswordVerifier `json:"verifiers"`
	PasswordSources []config.PasswordSource         `json:"password_sources,omitempty"`
	DefaultKDF      string                          `json:"default_kdf,omitempty"`
}

func EncodeVerifierSources(configured []configschema.Verifier, passwordSources []config.PasswordSource, crypto config.Crypto) (error, string) {
	algorithm := crypto.KDFAlgorithm()
	if _, err := kdf.Iterations(algorithm); err != nil {
		return err, ""
	}
	records := make([]configschema.PasswordVerifier, 0, len(configured))
	for index, verifier := range configured {
		record, err := configuredPasswordVerifier(verifier)
		if err != nil {
			return fmt.Errorf("encode verifier %d: %w", index, err), ""
		}
		records = append(records, record)
	}
	// Preserve the legacy source document for the default policy.
	if algorithm == kdf.Default {
		algorithm = ""
	}
	encoded, err := json.Marshal(verifierSourceDocument{Verifiers: records, PasswordSources: passwordSources, DefaultKDF: algorithm})
	if err != nil {
		return fmt.Errorf("encode verifier sources: %w", err), ""
	}
	return nil, string(encoded)
}

// ResolveVerifiersJSON resolves default password sources while preserving typed verifier records.
func ResolveVerifiersJSON(identityID, source string, resolver *PasswordSourceResolver) (error, string) {
	var configured verifierSourceDocument
	if err := json.Unmarshal([]byte(source), &configured); err != nil {
		return fmt.Errorf("decode verifier sources for identity %q: %w", identityID, err), ""
	}
	verifiers := make([]interface{}, 0, len(configured.Verifiers)+1)
	for index, verifier := range configured.Verifiers {
		if verifier.Kind != "password" || !strings.HasPrefix(verifier.PasswordVerifier, "gh-ver:") {
			return fmt.Errorf("identity %q verifier %d has an invalid password verifier", identityID, index), ""
		}
		verifiers = append(verifiers, verifier)
	}
	if len(configured.PasswordSources) > 0 {
		local := *resolver
		if configured.DefaultKDF != "" {
			local.crypto = config.Crypto{DefaultKDF: configured.DefaultKDF}
		}
		resolveErr, verifier := resolveDefaultPasswordVerifier(identityID, configured.PasswordSources, &local)
		if resolveErr != nil {
			return resolveErr, ""
		}
		verifiers = append(verifiers, verifier)
	}
	encoded, err := json.Marshal(verifiers)
	if err != nil {
		return fmt.Errorf("encode verifier sources for identity %q: %w", identityID, err), ""
	}
	return nil, string(encoded)
}

func passwordVerifier(password []byte, crypto config.Crypto) (string, error) {
	algorithm := crypto.KDFAlgorithm()
	iterations, err := kdf.Iterations(algorithm)
	if err != nil {
		return "", err
	}
	salt := make([]byte, passwordSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password verifier salt: %w", err)
	}
	digest, err := pbkdf2.Key(sha256.New, string(password), salt, iterations, passwordDigestSize)
	if err != nil {
		return "", fmt.Errorf("derive password verifier: %w", err)
	}
	return "gh-ver:" + base64.RawURLEncoding.EncodeToString(salt) + "." + base64.RawURLEncoding.EncodeToString(digest) + "?alg=" + algorithm, nil
}

// NewPasswordVerifier hashes a password supplied through administration APIs.
func NewPasswordVerifier(password []byte, crypto config.Crypto) (string, error) {
	if len(password) == 0 || bytes.IndexByte(password, 0) >= 0 {
		return "", fmt.Errorf("password must be nonempty and must not contain NUL")
	}
	return passwordVerifier(password, crypto)
}

func NewDummyPasswordVerifier(crypto config.Crypto) (error, string) {
	value, err := passwordVerifier([]byte("gatehouse dummy password verifier"), crypto)
	if err != nil {
		return err, ""
	}
	return nil, value
}

func VerifyPassword(value string, password []byte) (error, bool) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "gh-ver" || parsed.Opaque == "" || parsed.Host != "" || parsed.Path != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid gh-ver password verifier"), false
	}
	algorithm := parsed.Query().Get("alg")
	iterations, err := kdf.Iterations(algorithm)
	if err != nil || parsed.RawQuery != "alg="+algorithm {
		return fmt.Errorf("invalid gh-ver password verifier algorithm"), false
	}
	parts := strings.Split(parsed.Opaque, ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid gh-ver password verifier payload"), false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(salt) != passwordSaltSize {
		return fmt.Errorf("invalid gh-ver password verifier salt"), false
	}
	digest, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(digest) != passwordDigestSize {
		return fmt.Errorf("invalid gh-ver password verifier digest"), false
	}
	derived, err := pbkdf2.Key(sha256.New, string(password), salt, iterations, passwordDigestSize)
	if err != nil {
		return fmt.Errorf("derive password verifier: %w", err), false
	}
	return nil, subtle.ConstantTimeCompare(digest, derived) == 1
}
