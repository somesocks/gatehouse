package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gatehouse/configschema"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

//go:embed schema.json
var schemaJSON []byte

const schemaID = "urn:gatehouse:config-schema:v1"

var compiledSchema struct {
	sync.Once
	schema *jsonschema.Schema
	err    error
}

type State struct {
	Workspaces            []Workspace
	Principals            []Principal
	Keychains             []Keychain
	Groups                []Group
	WorkspaceRoleBindings []WorkspaceRoleBinding
	AgentProviders []AgentProvider
	AgentModels []AgentModel
	WorkspaceAgents []WorkspaceAgent
	StorageProviders []StorageProvider
	WorkspaceStorageProviders []WorkspaceStorageProvider
}

func ValidateFile(path string) (error, configschema.GatehouseConfig) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err, configschema.GatehouseConfig{}
	}

	err, jsonContents := configJSON(path, contents)
	if err != nil {
		return err, configschema.GatehouseConfig{}
	}

	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonContents))
	if err != nil {
		return fmt.Errorf("parse configuration: %w", err), configschema.GatehouseConfig{}
	}

	err, schema := configSchema()
	if err != nil {
		return fmt.Errorf("compile configuration schema: %w", err), configschema.GatehouseConfig{}
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("validate configuration: %w", err), configschema.GatehouseConfig{}
	}

	decodeErr, config := configschema.DecodeGatehouseConfig(instance)
	if decodeErr != nil {
		return fmt.Errorf("decode configuration: %w", decodeErr), configschema.GatehouseConfig{}
	}
	if err, _ := ResolveState(config); err != nil {
		return err, configschema.GatehouseConfig{}
	}
	if err, _ := ResolveServices(config); err != nil {
		return err, configschema.GatehouseConfig{}
	}
	return nil, config
}

func ResolveState(document configschema.GatehouseConfig) (error, State) {
	err, workspaces := ResolveWorkspaces(document)
	if err != nil {
		return err, State{}
	}
	err, principals := ResolvePrincipals(document)
	if err != nil {
		return err, State{}
	}
	err, keychains := ResolveKeychains(document)
	if err != nil {
		return err, State{}
	}
	err, groups := ResolveGroups(document)
	if err != nil {
		return err, State{}
	}
	err, workspaceRoleBindings := ResolveWorkspaceRoleBindings(document, principals, groups)
	if err != nil {
		return err, State{}
	}
	err, providers := ResolveAgentProviders(document)
	if err != nil { return err, State{} }
	err, models := ResolveAgentModels(document, providers)
	if err != nil { return err, State{} }
	err, agents := ResolveWorkspaceAgents(document, models)
	if err != nil { return err, State{} }
	err, storageProviders := ResolveStorageProviders(document)
	if err != nil { return err, State{} }
	err, workspaceStorageProviders := ResolveWorkspaceStorageProviders(document, storageProviders)
	if err != nil { return err, State{} }
	return nil, State{Workspaces: workspaces, Principals: principals, Keychains: keychains, Groups: groups, WorkspaceRoleBindings: workspaceRoleBindings, AgentProviders: providers, AgentModels: models, WorkspaceAgents: agents, StorageProviders: storageProviders, WorkspaceStorageProviders: workspaceStorageProviders}
}

func configJSON(path string, contents []byte) (error, []byte) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return nil, contents
	case ".yaml", ".yml":
		jsonContents, err := yaml.YAMLToJSON(contents)
		if err != nil {
			return fmt.Errorf("parse YAML: %w", err), nil
		}
		return nil, jsonContents
	default:
		return fmt.Errorf("unsupported configuration extension %q", filepath.Ext(path)), nil
	}
}

func configSchema() (error, *jsonschema.Schema) {
	compiledSchema.Do(func() {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
		if err != nil {
			compiledSchema.err = err
			return
		}
		object, ok := document.(map[string]any)
		if !ok {
			compiledSchema.err = fmt.Errorf("configuration schema must be an object")
			return
		}
		object["$id"] = schemaID

		compiler := jsonschema.NewCompiler()
		compiler.AssertFormat()
		if err := compiler.AddResource(schemaID, document); err != nil {
			compiledSchema.err = err
			return
		}

		compiledSchema.schema, compiledSchema.err = compiler.Compile(schemaID)
	})

	return compiledSchema.err, compiledSchema.schema
}
