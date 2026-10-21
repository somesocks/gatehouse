package config

import (
	"fmt"

	"gatehouse/configschema"
	"gatehouse/kdf"
)

// Crypto selects the KDF for newly generated passwords and wrapped keychain keys.
// Existing records always use their stored algorithm. The zero value uses v1.
type Crypto struct {
	DefaultKDF string
}

func (configuration Crypto) KDFAlgorithm() string {
	if configuration.DefaultKDF == "" {
		return kdf.Default
	}
	return configuration.DefaultKDF
}

func ResolveCrypto(document configschema.GatehouseConfig) (error, Crypto) {
	algorithm := kdf.Default
	if document.Crypto != nil && document.Crypto.DefaultKdf != nil {
		algorithm = *document.Crypto.DefaultKdf
	}
	if _, err := kdf.Iterations(algorithm); err != nil {
		return fmt.Errorf("crypto.default_kdf: %w", err), Crypto{}
	}
	return nil, Crypto{DefaultKDF: algorithm}
}
