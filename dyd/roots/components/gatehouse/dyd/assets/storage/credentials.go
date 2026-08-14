package storage

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"gatehouse/config"
)

type SecretKeySourceResolver struct {
	stdin  *bufio.Reader
	getenv func(string) string
}

func NewSecretKeySourceResolver() *SecretKeySourceResolver {
	return &SecretKeySourceResolver{stdin: bufio.NewReader(os.Stdin), getenv: os.Getenv}
}

func (resolver *SecretKeySourceResolver) Resolve(providerID string, sources []config.StorageProviderSecretKeySource) (error, []byte) {
	for _, source := range sources {
		var key []byte
		switch {
		case strings.HasPrefix(string(source), "env:"):
			key = []byte(resolver.getenv(strings.TrimPrefix(string(source), "env:")))
		case source == "stdin:":
			value, err := resolver.stdin.ReadBytes('\n')
			if err != nil && err != io.EOF {
				return fmt.Errorf("read secret access key for storage provider %q: %w", providerID, err), nil
			}
			key = bytes.TrimSuffix(bytes.TrimSuffix(value, []byte("\n")), []byte("\r"))
		default:
			return fmt.Errorf("resolve secret access key for storage provider %q: unsupported source %q", providerID, source), nil
		}
		if len(key) > 0 {
			return nil, key
		}
	}
	return fmt.Errorf("resolve secret access key for storage provider %q: no source provided a key", providerID), nil
}
