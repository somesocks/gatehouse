package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gatehouse/configschema"
)

type Principal struct {
	ID         string
	Name       *string
	Enabled    bool
	Identities []Identity
}

type Identity struct {
	ID        string
	Verifiers string
	Enabled   bool
}

var principalID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var identityID = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:.+$`)
var verifierKind = regexp.MustCompile(`^[a-z][a-z0-9+.-]*$`)

func ResolvePrincipals(document configschema.GatehouseConfig) (error, []Principal) {
	if document.Principals == nil {
		return nil, nil
	}

	principals := make([]Principal, 0, len(*document.Principals))
	principalIDs := make(map[string]struct{}, len(*document.Principals))
	identityIDs := make(map[string]struct{})
	for principalIndex, configured := range *document.Principals {
		if !principalID.MatchString(configured.Id) {
			return fmt.Errorf("principals[%d].id must match %q", principalIndex, principalID.String()), nil
		}
		if _, exists := principalIDs[configured.Id]; exists {
			return fmt.Errorf("principals[%d].id %q is duplicated", principalIndex, configured.Id), nil
		}
		if configured.Name != nil && strings.TrimSpace(*configured.Name) == "" {
			return fmt.Errorf("principals[%d].name must not be blank", principalIndex), nil
		}

		enabled := true
		if configured.Enabled != nil {
			enabled = *configured.Enabled
		}
		identities := make([]Identity, 0)
		if configured.Identities != nil {
			identities = make([]Identity, 0, len(*configured.Identities))
			for identityIndex, configuredIdentity := range *configured.Identities {
				if !identityID.MatchString(configuredIdentity.Id) {
					return fmt.Errorf("principals[%d].identities[%d].id must be a namespaced identity", principalIndex, identityIndex), nil
				}
				if _, exists := identityIDs[configuredIdentity.Id]; exists {
					return fmt.Errorf("principals[%d].identities[%d].id %q is duplicated", principalIndex, identityIndex, configuredIdentity.Id), nil
				}
				if len(configuredIdentity.Verifiers) == 0 {
					return fmt.Errorf("principals[%d].identities[%d].verifiers must not be empty", principalIndex, identityIndex), nil
				}

				namespace := configuredIdentity.Id[:strings.IndexByte(configuredIdentity.Id, ':')]
				for verifierIndex, value := range configuredIdentity.Verifiers {
					verifier, ok := value.(map[string]any)
					if !ok {
						return fmt.Errorf("principals[%d].identities[%d].verifiers[%d] must be an object", principalIndex, identityIndex, verifierIndex), nil
					}
					if err := validateVerifier(namespace, verifier); err != nil {
						return fmt.Errorf("principals[%d].identities[%d].verifiers[%d]: %w", principalIndex, identityIndex, verifierIndex, err), nil
					}
				}
				verifiers, err := json.Marshal(configuredIdentity.Verifiers)
				if err != nil {
					return fmt.Errorf("encode principals[%d].identities[%d].verifiers: %w", principalIndex, identityIndex, err), nil
				}

				identityEnabled := true
				if configuredIdentity.Enabled != nil {
					identityEnabled = *configuredIdentity.Enabled
				}
				identityIDs[configuredIdentity.Id] = struct{}{}
				identities = append(identities, Identity{
					ID:        configuredIdentity.Id,
					Verifiers: string(verifiers),
					Enabled:   identityEnabled,
				})
			}
		}
		sort.Slice(identities, func(left, right int) bool {
			return identities[left].ID < identities[right].ID
		})

		principalIDs[configured.Id] = struct{}{}
		principals = append(principals, Principal{
			ID:         configured.Id,
			Name:       configured.Name,
			Enabled:    enabled,
			Identities: identities,
		})
	}
	sort.Slice(principals, func(left, right int) bool {
		return principals[left].ID < principals[right].ID
	})
	return nil, principals
}

func validateVerifier(namespace string, verifier map[string]any) error {
	kind, ok := verifier["kind"].(string)
	if !ok || !verifierKind.MatchString(kind) {
		return fmt.Errorf("kind must match %q", verifierKind.String())
	}

	switch namespace {
	case "matrix":
		if kind != "matrix" {
			return fmt.Errorf("kind %q is not supported for matrix identities", kind)
		}
		return nil
	case "gatehouse":
		if kind != "argon2id" {
			return fmt.Errorf("kind %q is not supported for gatehouse identities", kind)
		}
		passwordVerifier, ok := verifier["password_verifier"].(string)
		if !ok || strings.TrimSpace(passwordVerifier) == "" {
			return fmt.Errorf("argon2id verifier requires a non-blank password_verifier")
		}
		return nil
	default:
		return fmt.Errorf("identity namespace %q is not supported", namespace)
	}
}
