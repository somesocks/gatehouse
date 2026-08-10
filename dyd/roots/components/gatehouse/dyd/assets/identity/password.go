package identity

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"crypto/pbkdf2"
	"gatehouse/config"
)

const (
	passwordAlgorithm  = "pbkdf2-hmac-sha256-v1"
	passwordIterations = 600_000
	passwordSaltSize   = 16
	passwordDigestSize = 32
)

type PasswordSourceResolver struct {
	stdin  *bufio.Reader
	getenv func(string) string
}

func NewPasswordSourceResolver() *PasswordSourceResolver {
	return &PasswordSourceResolver{stdin: bufio.NewReader(os.Stdin), getenv: os.Getenv}
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

func ResolveVerifiers(identityID string, configured []config.Verifier, resolver *PasswordSourceResolver) (error, []interface{}) {
	verifiers := make([]interface{}, 0, len(configured))
	for _, verifier := range configured {
		resolved, err := resolveVerifier(identityID, verifier, resolver)
		if err != nil {
			return err, nil
		}
		verifiers = append(verifiers, resolved)
	}
	return nil, verifiers
}

func resolveVerifier(identityID string, configured config.Verifier, resolver *PasswordSourceResolver) (interface{}, error) {
	if configured.Value != nil {
		return *configured.Value, nil
	}
	if configured.Algorithm == nil {
		return configured.Stored, nil
	}
	passwordErr, password := resolver.Resolve(identityID, configured.Sources)
	if passwordErr != nil {
		return nil, passwordErr
	}
	defer clear(password)
	if bytes.IndexByte(password, 0) >= 0 {
		return nil, fmt.Errorf("password for identity %q must not contain NUL", identityID)
	}
	return passwordVerifier(password)
}

func passwordVerifier(password []byte) (interface{}, error) {
	salt := make([]byte, passwordSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate password verifier salt: %w", err)
	}
	digest, err := pbkdf2.Key(sha256.New, string(password), salt, passwordIterations, passwordDigestSize)
	if err != nil {
		return nil, fmt.Errorf("derive password verifier: %w", err)
	}
	return "gh-ver:" + base64.RawURLEncoding.EncodeToString(salt) + "." + base64.RawURLEncoding.EncodeToString(digest) + "?alg=" + passwordAlgorithm, nil
}

func NewDummyPasswordVerifier() (error, string) {
	value, err := passwordVerifier([]byte("gatehouse dummy password verifier"))
	if err != nil {
		return err, ""
	}
	return nil, value.(string)
}

func VerifyPassword(value string, password []byte) (error, bool) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "gh-ver" || parsed.Opaque == "" || parsed.Host != "" || parsed.Path != "" || parsed.Fragment != "" || parsed.RawQuery != "alg="+passwordAlgorithm {
		return fmt.Errorf("invalid gh-ver password verifier"), false
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
	derived, err := pbkdf2.Key(sha256.New, string(password), salt, passwordIterations, passwordDigestSize)
	if err != nil {
		return fmt.Errorf("derive password verifier: %w", err), false
	}
	return nil, subtle.ConstantTimeCompare(digest, derived) == 1
}
