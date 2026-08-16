package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gatehouse/configschema"
)

type Workspace struct {
	Alias   string
	Name    *string
	Enabled bool
}

var workspaceID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var workspaceAlias = workspaceID

const gatehouseWorkspaceAlias = "gatehouse"
const defaultWorkspaceAlias = "admin"

func ResolveWorkspaces(document configschema.GatehouseConfig) (error, []Workspace) {
	if document.Workspaces == nil {
		return nil, []Workspace{{Alias: defaultWorkspaceAlias, Enabled: true}}
	}

	workspaces := make([]Workspace, 0, len(*document.Workspaces))
	aliases := make(map[string]struct{}, len(*document.Workspaces))
	for index, configured := range *document.Workspaces {
		if !workspaceAlias.MatchString(configured.Alias) {
			return fmt.Errorf("workspaces[%d].alias must match %q", index, workspaceAlias.String()), nil
		}
		if configured.Alias == gatehouseWorkspaceAlias {
			return fmt.Errorf("workspaces[%d].alias %q is reserved", index, configured.Alias), nil
		}
		if _, exists := aliases[configured.Alias]; exists {
			return fmt.Errorf("workspaces[%d].alias %q is duplicated", index, configured.Alias), nil
		}
		if configured.Name != nil && strings.TrimSpace(*configured.Name) == "" {
			return fmt.Errorf("workspaces[%d].name must not be blank", index), nil
		}

		enabled := true
		if configured.Enabled != nil {
			enabled = *configured.Enabled
		}
		aliases[configured.Alias] = struct{}{}
		workspaces = append(workspaces, Workspace{
			Alias:   configured.Alias,
			Name:    configured.Name,
			Enabled: enabled,
		})
	}

	sort.Slice(workspaces, func(left, right int) bool {
		return workspaces[left].Alias < workspaces[right].Alias
	})
	return nil, workspaces
}
