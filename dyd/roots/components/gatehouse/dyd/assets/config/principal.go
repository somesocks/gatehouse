package config

import (
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
	Revision  int
	Verifiers []Verifier
	Enabled   bool
}

type Verifier struct {
	Value     *string
	Algorithm *string
	Sources   []PasswordSource
	Stored    any
}

type PasswordSource string

var principalID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var identityID = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:.+$`)
var verifierKind = regexp.MustCompile(`^[a-z][a-z0-9+.-]*$`)

const (
	defaultPrincipalID      = "root"
	defaultIdentityRevision = 1
	passwordAlgorithm       = "pbkdf2-hmac-sha256-v1"
)

func ResolvePrincipals(document configschema.GatehouseConfig) (error, []Principal) {
	if document.Principals == nil {
		return nil, []Principal{{
			ID:      defaultPrincipalID,
			Enabled: true,
			Identities: []Identity{{
				ID:       "gatehouse:root",
				Revision: defaultIdentityRevision,
				Verifiers: []Verifier{{
					Algorithm: stringValue(passwordAlgorithm),
					Sources:   []PasswordSource{"env:GATEHOUSE_ROOT_PASSWORD", "stdin:"},
				}},
				Enabled: true,
			}},
		}}
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
				verifiers := make([]Verifier, 0, len(configuredIdentity.Verifiers))
				for verifierIndex, value := range configuredIdentity.Verifiers {
					verifier, err := resolveVerifier(namespace, value)
					if err != nil {
						return fmt.Errorf("principals[%d].identities[%d].verifiers[%d]: %w", principalIndex, identityIndex, verifierIndex, err), nil
					}
					verifiers = append(verifiers, verifier)
				}

				identityEnabled := true
				if configuredIdentity.Enabled != nil {
					identityEnabled = *configuredIdentity.Enabled
				}
				revision := defaultIdentityRevision
				if configuredIdentity.Revision != nil {
					revision = *configuredIdentity.Revision
				}
				if revision <= 0 {
					return fmt.Errorf("principals[%d].identities[%d].revision must be positive", principalIndex, identityIndex), nil
				}
				identityIDs[configuredIdentity.Id] = struct{}{}
				identities = append(identities, Identity{
					ID:        configuredIdentity.Id,
					Revision:  revision,
					Verifiers: verifiers,
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

func stringValue(value string) *string {
	return &value
}

func resolveVerifier(namespace string, value any) (Verifier, error) {
	verifier, ok := value.(map[string]any)
	if !ok {
		return Verifier{}, fmt.Errorf("must be an object")
	}
	if direct, exists := verifier["value"]; exists {
		if len(verifier) != 1 {
			return Verifier{}, fmt.Errorf("value verifier must not have additional fields")
		}
		text, ok := direct.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return Verifier{}, fmt.Errorf("value must be a non-blank string")
		}
		if namespace != "gatehouse" {
			return Verifier{}, fmt.Errorf("value verifiers are supported only for gatehouse identities")
		}
		if !strings.HasPrefix(text, "gh-ver:") {
			return Verifier{}, fmt.Errorf("gatehouse verifier value must use the gh-ver scheme")
		}
		return Verifier{Value: &text, Stored: text}, nil
	}
	if algorithm, exists := verifier["algorithm"]; exists {
		if len(verifier) != 2 {
			return Verifier{}, fmt.Errorf("source verifier must have algorithm and sources only")
		}
		if namespace != "gatehouse" {
			return Verifier{}, fmt.Errorf("source verifiers are supported only for gatehouse identities")
		}
		algorithmText, ok := algorithm.(string)
		if !ok || algorithmText != passwordAlgorithm {
			return Verifier{}, fmt.Errorf("algorithm must be %q", passwordAlgorithm)
		}
		sourceValues, ok := verifier["sources"].([]any)
		if !ok || len(sourceValues) == 0 {
			return Verifier{}, fmt.Errorf("sources must be a non-empty list")
		}
		sources := make([]PasswordSource, 0, len(sourceValues))
		seen := make(map[string]struct{}, len(sourceValues))
		for sourceIndex, sourceValue := range sourceValues {
			source, ok := sourceValue.(string)
			if !ok {
				return Verifier{}, fmt.Errorf("sources[%d] must be a string", sourceIndex)
			}
			if _, exists := seen[source]; exists {
				return Verifier{}, fmt.Errorf("sources[%d] %q is duplicated", sourceIndex, source)
			}
			switch {
			case environmentReference.MatchString(source):
			case source == "stdin:":
			default:
				return Verifier{}, fmt.Errorf("sources[%d] must be an env:VARIABLE_NAME or stdin: source", sourceIndex)
			}
			seen[source] = struct{}{}
			sources = append(sources, PasswordSource(source))
		}
		return Verifier{Algorithm: &algorithmText, Sources: sources}, nil
	}
	if err := validateVerifier(namespace, verifier); err != nil {
		return Verifier{}, err
	}
	return Verifier{Stored: verifier}, nil
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
		if kind != "password" {
			return fmt.Errorf("kind %q is not supported for gatehouse identities", kind)
		}
		passwordVerifier, ok := verifier["password_verifier"].(string)
		if !ok || strings.TrimSpace(passwordVerifier) == "" {
			return fmt.Errorf("password verifier requires a non-blank password_verifier")
		}
		return nil
	default:
		return fmt.Errorf("identity namespace %q is not supported", namespace)
	}
}
