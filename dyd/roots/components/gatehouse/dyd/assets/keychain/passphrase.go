package keychain

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gatehouse/config"
	"golang.org/x/term"
)

type PassphraseSourceResolver struct {
	stdin        *bufio.Reader
	stderr       io.Writer
	getenv       func(string) string
	readPassword func() (error, []byte)
}

func NewPassphraseSourceResolver() *PassphraseSourceResolver {
	resolver := newPassphraseSourceResolver(os.Stdin, os.Stderr, os.Getenv, nil)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		resolver.readPassword = func() (error, []byte) {
			passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return err, nil
			}
			return nil, passphrase
		}
	}
	return resolver
}

func newPassphraseSourceResolver(stdin io.Reader, stderr io.Writer, getenv func(string) string, readPassword func() (error, []byte)) *PassphraseSourceResolver {
	return &PassphraseSourceResolver{
		stdin:        bufio.NewReader(stdin),
		stderr:       stderr,
		getenv:       getenv,
		readPassword: readPassword,
	}
}

func (resolver *PassphraseSourceResolver) Resolve(keychain config.Keychain) (error, []byte) {
	for _, source := range keychain.Sources {
		var err error
		var passphrase []byte
		switch {
		case strings.HasPrefix(string(source), "env:"):
			passphrase = []byte(resolver.getenv(strings.TrimPrefix(string(source), "env:")))
		case source == "stdin:":
			err, passphrase = resolver.readStdin(keychain.ID)
		default:
			return fmt.Errorf("resolve passphrase for keychain %q: unsupported source %q", keychain.ID, source), nil
		}
		if err != nil {
			return fmt.Errorf("resolve passphrase for keychain %q from %q: %w", keychain.ID, source, err), nil
		}
		if len(passphrase) > 0 {
			return nil, passphrase
		}
	}
	return fmt.Errorf("resolve passphrase for keychain %q: no source provided a passphrase", keychain.ID), nil
}

func (resolver *PassphraseSourceResolver) readStdin(keychainID string) (error, []byte) {
	if _, err := fmt.Fprintf(resolver.stderr, "Keychain %q passphrase: ", keychainID); err != nil {
		return err, nil
	}
	if resolver.readPassword != nil {
		err, passphrase := resolver.readPassword()
		if _, newlineErr := fmt.Fprintln(resolver.stderr); newlineErr != nil && err == nil {
			err = newlineErr
		}
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return err, passphrase
	}

	passphrase, err := resolver.stdin.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err, nil
	}
	passphrase = bytes.TrimSuffix(passphrase, []byte("\n"))
	passphrase = bytes.TrimSuffix(passphrase, []byte("\r"))
	return nil, passphrase
}
