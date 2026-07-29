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

func ValidateFile(path string) (configschema.GatehouseConfig, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return configschema.GatehouseConfig{}, err
	}

	jsonContents, err := configJSON(path, contents)
	if err != nil {
		return configschema.GatehouseConfig{}, err
	}

	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonContents))
	if err != nil {
		return configschema.GatehouseConfig{}, fmt.Errorf("parse configuration: %w", err)
	}

	schema, err := configSchema()
	if err != nil {
		return configschema.GatehouseConfig{}, fmt.Errorf("compile configuration schema: %w", err)
	}
	if err := schema.Validate(instance); err != nil {
		return configschema.GatehouseConfig{}, fmt.Errorf("validate configuration: %w", err)
	}

	decodeErr, config := configschema.DecodeGatehouseConfig(instance)
	if decodeErr != nil {
		return configschema.GatehouseConfig{}, fmt.Errorf("decode configuration: %w", decodeErr)
	}
	return config, nil
}

func configJSON(path string, contents []byte) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return contents, nil
	case ".yaml", ".yml":
		jsonContents, err := yaml.YAMLToJSON(contents)
		if err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
		return jsonContents, nil
	default:
		return nil, fmt.Errorf("unsupported configuration extension %q", filepath.Ext(path))
	}
}

func configSchema() (*jsonschema.Schema, error) {
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

	return compiledSchema.schema, compiledSchema.err
}
