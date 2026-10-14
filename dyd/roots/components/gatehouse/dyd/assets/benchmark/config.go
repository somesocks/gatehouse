package benchmark

import (
	"bytes"
	_ "embed"
	"fmt"
	"iter"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gatehouse/benchmarkschema"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

//go:embed source.json
var sourceJSON []byte

//go:embed loaded.json
var loadedJSON []byte

type inputCollection struct {
	field     string
	textField string
}

var inputCollections = []inputCollection{
	{"agent_providers", ""},
	{"agent_models", ""},
	{"system_prompts", "text"},
	{"questions", "text"},
	{"lisp_environments", "prelude"},
}

type recordSchema struct {
	name   string
	schema *jsonschema.Schema
}

var compiledSchemas struct {
	sync.Once
	source  *jsonschema.Schema
	loaded  *jsonschema.Schema
	records map[string]recordSchema
	err     error
}

// Source preserves the declaring location and original reference for later exports.
type Source struct {
	File      string
	Reference string
}

type Config struct {
	Document benchmarkschema.GatehouseBenchmarkConfig
	Sources  map[string]Source
	RunCount uint64
}

// Run describes an input combination, without creating any runtime resources.
type Run struct {
	TestAlias         string
	ModelAlias        string
	SystemPromptAlias string
	QuestionAlias     string
	EnvironmentAlias  string
	Repetition        int
}

// Runs yields the Cartesian product in declaration/selection order. Validation
// counts runs without allocating a potentially large matrix in memory.
func (configuration Config) Runs() iter.Seq[Run] {
	return func(yield func(Run) bool) {
		for _, test := range configuration.Document.Tests {
			for _, model := range test.Matrix.Models {
				for _, prompt := range test.Matrix.SystemPrompts {
					for _, question := range test.Matrix.Questions {
						for _, environment := range test.Matrix.LispEnvironments {
							for repetition := 0; repetition < repetitions(test); repetition++ {
								if !yield(Run{test.Alias, model, prompt, question, environment, repetition + 1}) {
									return
								}
							}
						}
					}
				}
			}
		}
	}
}

// ValidateFile loads individual definition files and text files and resolves
// provider endpoint environment references, then validates the configuration.
// Credential sources remain deferred descriptors; this path never reads
// credentials or initializes Gatehouse state.
func ValidateFile(path string) (error, Config) {
	path, err := filepath.Abs(path)
	if err != nil {
		return err, Config{}
	}
	instance, err := readYAML(path)
	if err != nil {
		return err, Config{}
	}
	if err := prepareSchemas(); err != nil {
		return fmt.Errorf("compile benchmark schemas: %w", err), Config{}
	}
	if err := compiledSchemas.source.Validate(instance); err != nil {
		return fmt.Errorf("%s: validate benchmark source: %w", path, err), Config{}
	}
	object := instance.(map[string]any)
	sources := map[string]Source{}
	for _, collection := range inputCollections {
		entries := object[collection.field].([]any)
		if len(entries) == 0 {
			return fmt.Errorf("%s: %s must not be empty", path, collection.field), Config{}
		}
		for index, entry := range entries {
			location := fmt.Sprintf("%s[%d]", collection.field, index)
			origin := path
			if reference, ok := entry.(string); ok {
				origin, err = referencePath(path, reference)
				if err != nil {
					return fmt.Errorf("%s: %s: %w", path, location, err), Config{}
				}
				entry, err = readYAML(origin)
				if err != nil {
					return fmt.Errorf("%s: %s: %w", path, location, err), Config{}
				}
				sources[location] = Source{origin, reference}
			} else {
				sources[location] = Source{File: origin}
			}
			// Definition files must contain one record, not another reference or
			// an array. Validate before inspecting fields or loading their text.
			definition := compiledSchemas.records[collection.field]
			if err := definition.schema.Validate(entry); err != nil {
				return fmt.Errorf("%s: %s: expected a valid %s record: %w", origin, location, definition.name, err), Config{}
			}
			record := entry.(map[string]any)
			if endpoint, ok := record["endpoint"].(string); collection.field == "agent_providers" && ok && strings.HasPrefix(endpoint, "env:") {
				if !environmentReferencePattern.MatchString(endpoint) {
					return fmt.Errorf("%s: %s.endpoint: expected env:VARIABLE_NAME reference", origin, location), Config{}
				}
				name := strings.TrimPrefix(endpoint, "env:")
				value := os.Getenv(name)
				if value == "" {
					return fmt.Errorf("%s: %s.endpoint: environment variable %q is unset or empty", origin, location, name), Config{}
				}
				record["endpoint"] = value
			}
			if text, ok := record[collection.textField].(string); collection.textField != "" && ok && strings.HasPrefix(text, "file:") {
				textPath, err := referencePath(origin, text)
				if err != nil {
					return fmt.Errorf("%s: %s.%s: %w", origin, location, collection.textField, err), Config{}
				}
				contents, err := os.ReadFile(textPath)
				if err != nil {
					return fmt.Errorf("%s: %s.%s: read text file %s: %w", origin, location, collection.textField, textPath, err), Config{}
				}
				record[collection.textField] = string(contents)
				sources[location+"."+collection.textField] = Source{textPath, text}
			}
			entries[index] = record
		}
	}
	if err := compiledSchemas.loaded.Validate(object); err != nil {
		return fmt.Errorf("%s: validate loaded benchmark: %w", path, err), Config{}
	}
	err, document := benchmarkschema.DecodeGatehouseBenchmarkConfig(object)
	if err != nil {
		return fmt.Errorf("decode benchmark configuration: %w", err), Config{}
	}
	result := Config{Document: document, Sources: sources}
	if err := result.validate(object); err != nil {
		return withOrigin(path, sources, err), Config{}
	}
	return nil, result
}

func readYAML(path string) (any, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read YAML definition %s: %w", path, err)
	}
	jsonContents, err := yaml.YAMLToJSONStrict(contents)
	if err != nil {
		return nil, fmt.Errorf("parse YAML %s: %w", path, err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonContents))
	if err != nil {
		return nil, fmt.Errorf("parse configuration %s: %w", path, err)
	}
	return instance, nil
}

func referencePath(declaringFile, reference string) (string, error) {
	path, ok := strings.CutPrefix(reference, "file:")
	if !ok || strings.TrimSpace(path) == "" || strings.ContainsAny(path, "\r\n") {
		return "", fmt.Errorf("expected file:<nonempty path> reference, got %q", reference)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(declaringFile), path)
	}
	return filepath.Clean(path), nil
}

func prepareSchemas() error {
	compiledSchemas.Do(func() {
		compiler := jsonschema.NewCompiler()
		compiler.AssertFormat()
		var loadedProperties map[string]any
		for _, resource := range []struct {
			id   string
			data []byte
		}{{"urn:gatehouse:benchmark:source:v1", sourceJSON}, {"urn:gatehouse:benchmark:loaded:v1", loadedJSON}} {
			document, err := jsonschema.UnmarshalJSON(bytes.NewReader(resource.data))
			if err != nil {
				compiledSchemas.err = err
				return
			}
			object := document.(map[string]any)
			object["$id"] = resource.id
			if err := compiler.AddResource(resource.id, document); err != nil {
				compiledSchemas.err = err
				return
			}
			if resource.id == "urn:gatehouse:benchmark:loaded:v1" {
				definitions := object["$defs"].(map[string]any)
				rootName := strings.TrimPrefix(object["$ref"].(string), "#/$defs/")
				root := definitions[rootName].(map[string]any)
				loadedProperties = root["properties"].(map[string]any)
			}
		}
		compiledSchemas.source, compiledSchemas.err = compiler.Compile("urn:gatehouse:benchmark:source:v1")
		if compiledSchemas.err != nil {
			return
		}
		compiledSchemas.loaded, compiledSchemas.err = compiler.Compile("urn:gatehouse:benchmark:loaded:v1")
		if compiledSchemas.err != nil {
			return
		}
		compiledSchemas.records = map[string]recordSchema{}
		// Derive definition names and targets from the generated collection refs.
		for _, collection := range inputCollections {
			property := loadedProperties[collection.field].(map[string]any)
			items := property["items"].(map[string]any)
			reference := items["$ref"].(string)
			schema, err := compiler.Compile("urn:gatehouse:benchmark:loaded:v1" + reference)
			if err != nil {
				compiledSchemas.err = err
				return
			}
			compiledSchemas.records[collection.field] = recordSchema{
				name: strings.TrimPrefix(reference, "#/$defs/"), schema: schema,
			}
		}
	})
	return compiledSchemas.err
}

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func (configuration *Config) validate(object map[string]any) error {
	aliases := map[string]map[string]bool{}
	for _, field := range []string{"agent_providers", "agent_models", "system_prompts", "questions", "lisp_environments", "tests"} {
		entries := object[field].([]any)
		if len(entries) == 0 {
			return fmt.Errorf("%s must not be empty", field)
		}
		aliases[field] = map[string]bool{}
		for index, entry := range entries {
			alias := entry.(map[string]any)["alias"].(string)
			if !aliasPattern.MatchString(alias) {
				return fmt.Errorf("%s[%d].alias %q must match %s", field, index, alias, aliasPattern)
			}
			if aliases[field][alias] {
				return fmt.Errorf("%s[%d].alias %q is duplicated", field, index, alias)
			}
			aliases[field][alias] = true
		}
	}
	providers := map[string]benchmarkschema.BenchmarkProvider{}
	for index, provider := range configuration.Document.AgentProviders {
		if err := validateProvider(index, provider); err != nil {
			return err
		}
		providers[provider.Alias] = provider
	}
	models := map[string]benchmarkschema.BenchmarkModel{}
	for index, model := range configuration.Document.AgentModels {
		if err := validateModel(index, model, providers); err != nil {
			return err
		}
		models[model.Alias] = model
	}
	for index, prompt := range configuration.Document.SystemPrompts {
		switch prompt.Mode {
		case "builtin":
			if prompt.Text != nil {
				return fmt.Errorf("system_prompts[%d] %q: builtin mode must not specify text", index, prompt.Alias)
			}
		case "append", "replace":
			if prompt.Text == nil {
				return fmt.Errorf("system_prompts[%d] %q: %s mode requires text", index, prompt.Alias, prompt.Mode)
			}
		default:
			return fmt.Errorf("system_prompts[%d] %q: unknown mode %q; expected builtin, append, or replace", index, prompt.Alias, prompt.Mode)
		}
	}
	for index, question := range configuration.Document.Questions {
		if strings.TrimSpace(question.Text) == "" {
			return fmt.Errorf("questions[%d] %q: text must not be empty", index, question.Alias)
		}
	}
	for index, test := range configuration.Document.Tests {
		location := fmt.Sprintf("tests[%d] %q", index, test.Alias)
		if repetitions(test) <= 0 {
			return fmt.Errorf("%s: repetitions must be positive", location)
		}
		count := uint64(repetitions(test))
		for _, dimension := range []struct {
			name       string
			collection string
			selection  []string
		}{
			{"models", "agent_models", test.Matrix.Models},
			{"system_prompts", "system_prompts", test.Matrix.SystemPrompts},
			{"questions", "questions", test.Matrix.Questions},
			{"lisp_environments", "lisp_environments", test.Matrix.LispEnvironments},
		} {
			if len(dimension.selection) == 0 {
				return fmt.Errorf("%s.matrix.%s must not be empty", location, dimension.name)
			}
			selected := map[string]bool{}
			for _, alias := range dimension.selection {
				if !aliases[dimension.collection][alias] {
					return fmt.Errorf("%s.matrix.%s: unknown alias %q", location, dimension.name, alias)
				}
				if selected[alias] {
					return fmt.Errorf("%s.matrix.%s: duplicate alias %q", location, dimension.name, alias)
				}
				if dimension.name == "models" {
					model := models[alias]
					if !enabled(model.Enabled) || !enabled(providers[model.Provider].Enabled) {
						return fmt.Errorf("%s.matrix.models: model %q or its provider is disabled", location, alias)
					}
				}
				selected[alias] = true
			}
			if count > math.MaxUint64/uint64(len(dimension.selection)) {
				return fmt.Errorf("%s: expanded run count overflows uint64", location)
			}
			count *= uint64(len(dimension.selection))
		}
		if configuration.RunCount > math.MaxUint64-count {
			return fmt.Errorf("%s: total run count overflows uint64", location)
		}
		configuration.RunCount += count
	}
	return nil
}

func repetitions(test benchmarkschema.BenchmarkTest) int {
	if test.Repetitions == nil {
		return 1
	}
	return *test.Repetitions
}

func withOrigin(path string, sources map[string]Source, err error) error {
	for location, source := range sources {
		if strings.HasPrefix(err.Error(), location+".") || strings.HasPrefix(err.Error(), location+" ") {
			return fmt.Errorf("%s: %w", source.File, err)
		}
	}
	return fmt.Errorf("%s: %w", path, err)
}
