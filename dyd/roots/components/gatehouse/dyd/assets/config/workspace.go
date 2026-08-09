package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gatehouse/configschema"
)

type Workspace struct {
	ID      string
	Name    *string
	Enabled bool
}

var workspaceID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

const gatehouseWorkspaceID = "gatehouse"
const defaultWorkspaceID = "admin"

func ResolveWorkspaces(document configschema.GatehouseConfig) (error, []Workspace) {
	if document.Workspaces == nil {
		return nil, []Workspace{{ID: defaultWorkspaceID, Enabled: true}}
	}

	workspaces := make([]Workspace, 0, len(*document.Workspaces))
	ids := make(map[string]struct{}, len(*document.Workspaces))
	for index, configured := range *document.Workspaces {
		if !workspaceID.MatchString(configured.Id) {
			return fmt.Errorf("workspaces[%d].id must match %q", index, workspaceID.String()), nil
		}
		if configured.Id == gatehouseWorkspaceID {
			return fmt.Errorf("workspaces[%d].id %q is reserved", index, configured.Id), nil
		}
		if _, exists := ids[configured.Id]; exists {
			return fmt.Errorf("workspaces[%d].id %q is duplicated", index, configured.Id), nil
		}
		if configured.Name != nil && strings.TrimSpace(*configured.Name) == "" {
			return fmt.Errorf("workspaces[%d].name must not be blank", index), nil
		}

		enabled := true
		if configured.Enabled != nil {
			enabled = *configured.Enabled
		}
		ids[configured.Id] = struct{}{}
		workspaces = append(workspaces, Workspace{
			ID:      configured.Id,
			Name:    configured.Name,
			Enabled: enabled,
		})
	}

	sort.Slice(workspaces, func(left, right int) bool {
		return workspaces[left].ID < workspaces[right].ID
	})
	return nil, workspaces
}
