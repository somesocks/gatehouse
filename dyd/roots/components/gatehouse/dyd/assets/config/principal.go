package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gatehouse/configschema"
)

type Principal struct {
	Alias      string
	Name       *string
	Revision   int
	Enabled    bool
	Identities []Identity
}

type Identity struct {
	Alias                  string
	Key                    string
	Revision               int
	Verifiers              []configschema.Verifier
	PasswordSources        []PasswordSource
	Enabled                bool
}

type PasswordSource string

var principalID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var principalAlias = principalID
var identityAlias = principalID
var identityKey = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:.+$`)

const (
	defaultPrincipalAlias    = "root"
	DefaultPrincipalRevision = 1
	defaultIdentityRevision  = 1
)

func ResolvePrincipals(document configschema.GatehouseConfig) (error, []Principal) {
	if document.Principals == nil {
		return nil, []Principal{{
			Alias:    defaultPrincipalAlias,
			Revision: DefaultPrincipalRevision,
			Enabled:  true,
			Identities: []Identity{{
				Alias:                  "root",
				Key:                    "gatehouse:root",
				Revision:               defaultIdentityRevision,
				PasswordSources: []PasswordSource{"env:GATEHOUSE_ROOT_PASSWORD", "stdin:"},
				Enabled:         true,
			}},
		}}
	}

	principals := make([]Principal, 0, len(*document.Principals))
	principalAliases := make(map[string]struct{}, len(*document.Principals))
	identityAliases := make(map[string]struct{})
	for principalIndex, configured := range *document.Principals {
		if !principalAlias.MatchString(configured.Alias) {
			return fmt.Errorf("principals[%d].alias must match %q", principalIndex, principalAlias.String()), nil
		}
		if _, exists := principalAliases[configured.Alias]; exists {
			return fmt.Errorf("principals[%d].alias %q is duplicated", principalIndex, configured.Alias), nil
		}
		if configured.Name != nil && strings.TrimSpace(*configured.Name) == "" {
			return fmt.Errorf("principals[%d].name must not be blank", principalIndex), nil
		}

		enabled := true
		if configured.Enabled != nil {
			enabled = *configured.Enabled
		}
		revision := DefaultPrincipalRevision
		if configured.Revision != nil {
			revision = *configured.Revision
		}
		if revision <= 0 {
			return fmt.Errorf("principals[%d].revision must be positive", principalIndex), nil
		}

		identities := make([]Identity, 0)
		if configured.Identities != nil {
			identities = make([]Identity, 0, len(*configured.Identities))
			for identityIndex, configuredIdentity := range *configured.Identities {
				if !identityAlias.MatchString(configuredIdentity.Alias) {
					return fmt.Errorf("principals[%d].identities[%d].alias must match %q", principalIndex, identityIndex, identityAlias.String()), nil
				}
				if _, exists := identityAliases[configuredIdentity.Alias]; exists {
					return fmt.Errorf("principals[%d].identities[%d].alias %q is duplicated", principalIndex, identityIndex, configuredIdentity.Alias), nil
				}
				if !identityKey.MatchString(configuredIdentity.Key) {
					return fmt.Errorf("principals[%d].identities[%d].key must be a namespaced identity", principalIndex, identityIndex), nil
				}
				if len(configuredIdentity.Verifiers) == 0 {
					return fmt.Errorf("principals[%d].identities[%d].verifiers must not be empty", principalIndex, identityIndex), nil
				}

				namespace := configuredIdentity.Key[:strings.IndexByte(configuredIdentity.Key, ':')]
				verifiers := make([]configschema.Verifier, 0, len(configuredIdentity.Verifiers))
				for verifierIndex, verifier := range configuredIdentity.Verifiers {
					resolved, err := resolveVerifier(namespace, verifier)
					if err != nil {
						return fmt.Errorf("principals[%d].identities[%d].verifiers[%d]: %w", principalIndex, identityIndex, verifierIndex, err), nil
					}
					verifiers = append(verifiers, resolved)
				}

				identityEnabled := true
				if configuredIdentity.Enabled != nil {
					identityEnabled = *configuredIdentity.Enabled
				}
				identityRevision := defaultIdentityRevision
				if configuredIdentity.Revision != nil {
					identityRevision = *configuredIdentity.Revision
				}
				if identityRevision <= 0 {
					return fmt.Errorf("principals[%d].identities[%d].revision must be positive", principalIndex, identityIndex), nil
				}
				identityAliases[configuredIdentity.Alias] = struct{}{}
				identities = append(identities, Identity{
					Alias:     configuredIdentity.Alias,
					Key:       configuredIdentity.Key,
					Revision:  identityRevision,
					Verifiers: verifiers,
					Enabled:   identityEnabled,
				})
			}
		}
		sort.Slice(identities, func(left, right int) bool {
			return identities[left].Alias < identities[right].Alias
		})

		principalAliases[configured.Alias] = struct{}{}
		principals = append(principals, Principal{
			Alias:      configured.Alias,
			Name:       configured.Name,
			Revision:   revision,
			Enabled:    enabled,
			Identities: identities,
		})
	}
	sort.Slice(principals, func(left, right int) bool {
		return principals[left].Alias < principals[right].Alias
	})
	return nil, principals
}

func resolveVerifier(namespace string, configured configschema.Verifier) (configschema.Verifier, error) {
	if namespace != "gatehouse" {
		return configschema.Verifier{}, fmt.Errorf("password verifiers are supported only for gatehouse identities")
	}
	if configured.Kind != configschema.VerifierKindPasswordVerifier || configured.PasswordVerifier == nil {
		return configschema.Verifier{}, fmt.Errorf("password verifier configuration is missing")
	}

	passwordVerifier := *configured.PasswordVerifier
	if passwordVerifier.Kind != "password" {
		return configschema.Verifier{}, fmt.Errorf("verifier kind must be %q", "password")
	}
	resolved, err := resolveEnvironmentReference(passwordVerifier.PasswordVerifier, "password_verifier")
	if err != nil {
		return configschema.Verifier{}, err
	}
	if !strings.HasPrefix(resolved, "gh-ver:") {
		return configschema.Verifier{}, fmt.Errorf("password_verifier must use the gh-ver scheme")
	}
	passwordVerifier.PasswordVerifier = resolved
	return configschema.Verifier{
		Kind:             configschema.VerifierKindPasswordVerifier,
		PasswordVerifier: &passwordVerifier,
	}, nil
}
