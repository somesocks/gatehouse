package agent

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"gatehouse/config"
)

type APIKeySourceResolver struct {
	stdin  *bufio.Reader
	getenv func(string) string
}

func NewAPIKeySourceResolver() *APIKeySourceResolver {
	return &APIKeySourceResolver{stdin: bufio.NewReader(os.Stdin), getenv: os.Getenv}
}

func (resolver *APIKeySourceResolver) Resolve(providerID string, sources []config.AgentProviderAPIKeySource) (error, []byte) {
	for _, source := range sources {
		var key []byte
		switch {
		case strings.HasPrefix(string(source), "env:"):
			key = []byte(resolver.getenv(strings.TrimPrefix(string(source), "env:")))
		case source == "stdin:":
			value, err := resolver.stdin.ReadBytes('\n')
			if err != nil && err != io.EOF {
				return fmt.Errorf("read API key for provider %q: %w", providerID, err), nil
			}
			key = bytes.TrimSuffix(bytes.TrimSuffix(value, []byte("\n")), []byte("\r"))
		default:
			return fmt.Errorf("resolve API key for provider %q: unsupported source %q", providerID, source), nil
		}
		if len(key) > 0 {
			return nil, key
		}
	}
	return fmt.Errorf("resolve API key for provider %q: no source provided a key", providerID), nil
}
