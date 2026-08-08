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
	ID   string
	Refs []string
}

func ResolveKeychains(document configschema.GatehouseConfig) (error, []Keychain) {
	if document.Keychains == nil {
		return nil, []Keychain{{
			ID:   defaultKeychainID,
			Refs: []string{"env:GATEHOUSE_KEYCHAIN", "stdin:"},
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
		if len(configured.Refs) == 0 {
			return fmt.Errorf("keychains[%d].refs must not be empty", keychainIndex), nil
		}

		refs := make([]string, 0, len(configured.Refs))
		seenRefs := make(map[string]struct{}, len(configured.Refs))
		for refIndex, ref := range configured.Refs {
			if _, exists := seenRefs[ref]; exists {
				return fmt.Errorf("keychains[%d].refs[%d] %q is duplicated", keychainIndex, refIndex, ref), nil
			}
			switch {
			case environmentReference.MatchString(ref):
			case ref == "stdin:":
				if refIndex != len(configured.Refs)-1 {
					return fmt.Errorf("keychains[%d].refs[%d] stdin: must be the final fallback", keychainIndex, refIndex), nil
				}
			default:
				return fmt.Errorf("keychains[%d].refs[%d] must be an env:VARIABLE_NAME or stdin: reference", keychainIndex, refIndex), nil
			}
			seenRefs[ref] = struct{}{}
			refs = append(refs, ref)
		}

		ids[configured.Id] = struct{}{}
		keychains = append(keychains, Keychain{ID: configured.Id, Refs: refs})
	}
	sort.Slice(keychains, func(left, right int) bool {
		return keychains[left].ID < keychains[right].ID
	})
	return nil, keychains
}
