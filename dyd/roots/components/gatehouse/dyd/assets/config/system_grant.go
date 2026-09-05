package config

import (
	"fmt"
	"sort"

	"gatehouse/configschema"
)

type SystemGrant struct {
	PrincipalID string
	Revision    int
	Enabled     bool
}

func ResolveSystemGrants(document configschema.GatehouseConfig, principals []Principal) (error, []SystemGrant) {
	if document.SystemGrants == nil {
		if document.Principals == nil {
			return nil, []SystemGrant{{PrincipalID: defaultPrincipalAlias, Revision: 1, Enabled: true}}
		}
		return nil, []SystemGrant{}
	}
	grants := make([]SystemGrant, 0, len(*document.SystemGrants))
	seen := make(map[string]struct{}, len(*document.SystemGrants))
	principalsByAlias := make(map[string]struct{}, len(principals))
	for _, principal := range principals {
		principalsByAlias[principal.Alias] = struct{}{}
	}
	for index, configured := range *document.SystemGrants {
		path := fmt.Sprintf("system_grants[%d]", index)
		if !principalAlias.MatchString(configured.Principal) {
			return fmt.Errorf("%s.principal must match %q", path, principalAlias.String()), nil
		}
		if _, exists := principalsByAlias[configured.Principal]; !exists {
			return fmt.Errorf("%s.principal %q is not configured", path, configured.Principal), nil
		}
		if _, exists := seen[configured.Principal]; exists {
			return fmt.Errorf("%s.principal %q is duplicated", path, configured.Principal), nil
		}
		seen[configured.Principal] = struct{}{}
		revision := 1
		if configured.Revision != nil {
			revision = *configured.Revision
		}
		if revision < 1 {
			return fmt.Errorf("%s.revision must be positive", path), nil
		}
		enabled := true
		if configured.Enabled != nil {
			enabled = *configured.Enabled
		}
		grants = append(grants, SystemGrant{PrincipalID: configured.Principal, Revision: revision, Enabled: enabled})
	}
	sort.Slice(grants, func(left, right int) bool { return grants[left].PrincipalID < grants[right].PrincipalID })
	return nil, grants
}
