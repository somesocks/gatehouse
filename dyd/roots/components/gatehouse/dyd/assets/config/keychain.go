package config

import (
	"fmt"
	"regexp"
	"sort"

	"gatehouse/configschema"
)

const defaultKeychainID = "default"

var keychainID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

type Keychain struct {
	ID      string
	Sources []KeychainPassphraseSource
}

type KeychainPassphraseSource string

func ResolveKeychains(document configschema.GatehouseConfig) (error, []Keychain) {
	if document.Keychains == nil {
		return nil, []Keychain{{
			ID:      defaultKeychainID,
			Sources: []KeychainPassphraseSource{"env:GATEHOUSE_KEYCHAIN", "stdin:"},
		}}
	}

	keychains := make([]Keychain, 0, len(*document.Keychains))
	ids := make(map[string]struct{}, len(*document.Keychains))
	for keychainIndex, configured := range *document.Keychains {
		if !keychainID.MatchString(configured.Id) {
			return fmt.Errorf("keychains[%d].id must match %q", keychainIndex, keychainID.String()), nil
		}
		if _, exists := ids[configured.Id]; exists {
			return fmt.Errorf("keychains[%d].id %q is duplicated", keychainIndex, configured.Id), nil
		}
		if len(configured.Sources) == 0 {
			return fmt.Errorf("keychains[%d].sources must not be empty", keychainIndex), nil
		}

		sources := make([]KeychainPassphraseSource, 0, len(configured.Sources))
		seenSources := make(map[string]struct{}, len(configured.Sources))
		for sourceIndex, source := range configured.Sources {
			if _, exists := seenSources[source]; exists {
				return fmt.Errorf("keychains[%d].sources[%d] %q is duplicated", keychainIndex, sourceIndex, source), nil
			}
			switch {
			case environmentReference.MatchString(source):
			case source == "stdin:":
			default:
				return fmt.Errorf("keychains[%d].sources[%d] must be an env:VARIABLE_NAME or stdin: source", keychainIndex, sourceIndex), nil
			}
			seenSources[source] = struct{}{}
			sources = append(sources, KeychainPassphraseSource(source))
		}

		ids[configured.Id] = struct{}{}
		keychains = append(keychains, Keychain{ID: configured.Id, Sources: sources})
	}
	sort.Slice(keychains, func(left, right int) bool {
		return keychains[left].ID < keychains[right].ID
	})
	return nil, keychains
}
