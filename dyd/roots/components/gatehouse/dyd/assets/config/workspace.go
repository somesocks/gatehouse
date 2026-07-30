package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gatehouse/configschema"
)

type Workspace struct {
	Key     string
	Name    string
	Enabled bool
}

var workspaceKey = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func ResolveWorkspaces(document configschema.GatehouseConfig) (error, []Workspace) {
	if document.Workspaces == nil {
		return nil, nil
	}

	workspaces := make([]Workspace, 0, len(*document.Workspaces))
	keys := make(map[string]struct{}, len(*document.Workspaces))
	for index, configured := range *document.Workspaces {
		if !workspaceKey.MatchString(configured.Key) {
			return fmt.Errorf("workspaces[%d].key must match %q", index, workspaceKey.String()), nil
		}
		if _, exists := keys[configured.Key]; exists {
			return fmt.Errorf("workspaces[%d].key %q is duplicated", index, configured.Key), nil
		}
		if strings.TrimSpace(configured.Name) == "" {
			return fmt.Errorf("workspaces[%d].name must not be blank", index), nil
		}

		enabled := true
		if configured.Enabled != nil {
			enabled = *configured.Enabled
		}
		keys[configured.Key] = struct{}{}
		workspaces = append(workspaces, Workspace{
			Key:     configured.Key,
			Name:    configured.Name,
			Enabled: enabled,
		})
	}

	sort.Slice(workspaces, func(left, right int) bool {
		return workspaces[left].Key < workspaces[right].Key
	})
	return nil, workspaces
}
