package benchmark

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gatehouse/benchmarkschema"
)

func TestValidateFileLoadsInputsAndExpandsMatrix(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("BENCHMARK_TEST_KEY", "credential-must-not-be-substituted")
	files := map[string]string{
		"providers/openai.yaml": `alias: openai
protocol: openai-responses
endpoint: http://127.0.0.1:1/v1/responses
api_key:
  sources: [env:BENCHMARK_TEST_KEY]
`,
		"models/model.yaml": `alias: model-b
provider: openai
model: model-b
parameters: {}
`,
		"prompts/discovery.yaml": `alias: discovery
mode: replace
text: file:../text/prompt.txt
`,
		"questions/capabilities.yaml": `alias: capabilities
text: file:../text/question.txt
`,
		"environments/custom.yaml": `alias: custom
project_linked: true
prelude: file:../text/prelude.lisp
`,
		"text/prompt.txt":   "file:this-is-literal-prompt-content\n",
		"text/question.txt": "env:THIS_IS_LITERAL_QUESTION_CONTENT\n",
		"text/prelude.lisp": "; comment must keep its newline\n(eval program)\n",
		"tests.yaml": `api_version: v1
agent_providers:
  - file:providers/openai.yaml
agent_models:
  - file:models/model.yaml
  - alias: model-a
    provider: openai
    model: model-a
    parameters: {}
system_prompts:
  - alias: baseline
    mode: builtin
  - file:prompts/discovery.yaml
questions:
  - file:questions/capabilities.yaml
  - alias: tools
    text: What tools do you have?
lisp_environments:
  - alias: session
  - file:environments/custom.yaml
tests:
  - alias: discovery
    matrix:
      models: [model-b, model-a]
      system_prompts: [baseline, discovery]
      questions: [capabilities, tools]
      lisp_environments: [session, custom]
    repetitions: 2
  - alias: default-repetition
    matrix:
      models: [model-a]
      system_prompts: [discovery]
      questions: [tools]
      lisp_environments: [custom]
`,
	}
	for path, contents := range files {
		path = filepath.Join(directory, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err, configuration := ValidateFile(filepath.Join(directory, "tests.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if configuration.RunCount != 33 {
		t.Fatalf("run count = %d, want 33", configuration.RunCount)
	}
	if got := *configuration.Document.SystemPrompts[1].Text; got != files["text/prompt.txt"] {
		t.Fatalf("prompt = %q, want verbatim %q", got, files["text/prompt.txt"])
	}
	if got := configuration.Document.Questions[0].Text; got != files["text/question.txt"] {
		t.Fatalf("question = %q, want verbatim %q", got, files["text/question.txt"])
	}
	if got := *configuration.Document.LispEnvironments[1].Prelude; got != files["text/prelude.lisp"] {
		t.Fatalf("prelude = %q, want verbatim %q", got, files["text/prelude.lisp"])
	}
	if configuration.Document.LispEnvironments[0].Prelude != nil {
		t.Fatal("omitted prelude must remain omitted")
	}
	var document benchmarkschema.GatehouseBenchmarkConfig = configuration.Document
	if got := document.AgentProviders[0].ApiKey.Sources; len(got) != 1 || got[0] != "env:BENCHMARK_TEST_KEY" {
		t.Fatalf("credential descriptors = %v, want deferred environment reference", got)
	}
	if document.AgentModels[0].MaxTurns != nil || document.AgentModels[0].MaxOutputTokens != nil || document.AgentModels[0].Compaction != nil || document.AgentProviders[0].ApiKey.Keychain != nil {
		t.Fatalf("omitted benchmark settings must remain omitted: %#v", document)
	}
	err, encoded := benchmarkschema.EncodeGatehouseBenchmarkConfig(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"agent_models", "agent_providers"} {
		for _, entry := range encoded.(map[string]any)[field].([]any) {
			if _, exists := entry.(map[string]any)["revision"]; exists {
				t.Fatalf("benchmark %s gained a runtime revision", field)
			}
		}
	}
	source := configuration.Sources["system_prompts[1].text"]
	if source.File != filepath.Join(directory, "text/prompt.txt") || source.Reference != "file:../text/prompt.txt" {
		t.Fatalf("prompt provenance = %#v", source)
	}
	var runs []Run
	for run := range configuration.Runs() {
		runs = append(runs, run)
	}
	if len(runs) != 33 || runs[0] != (Run{"discovery", "model-b", "baseline", "capabilities", "session", 1}) || runs[1].Repetition != 2 || runs[16].ModelAlias != "model-a" || runs[32].TestAlias != "default-repetition" || runs[32].Repetition != 1 {
		t.Fatalf("unexpected expansion/order: %#v", runs)
	}
	count := 0
	for range configuration.Runs() {
		count++
		break
	}
	if count != 1 {
		t.Fatalf("early iteration exit produced %d runs", count)
	}
}

func TestValidateFileRejectsRunCountOverflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tests.yaml")
	contents := `api_version: v1
agent_providers:
  - alias: builtin
    protocol: builtin
agent_models:
  - alias: assistant
    provider: builtin
    model: dummy.fixed-reply
    parameters: {text: reply}
system_prompts:
  - alias: baseline
    mode: builtin
questions:
  - alias: question-a
    text: a
  - alias: question-b
    text: b
  - alias: question-c
    text: c
lisp_environments:
  - alias: session
tests:
  - alias: overflow
    matrix:
      models: [assistant]
      system_prompts: [baseline]
      questions: [question-a, question-b, question-c]
      lisp_environments: [session]
    repetitions: 9223372036854775807
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	err, _ := ValidateFile(path)
	if err == nil || !strings.Contains(err.Error(), "run count overflows") {
		t.Fatalf("expected run-count overflow error, got %v", err)
	}
}

func TestValidateFileProviderEndpointEnvironmentReferences(t *testing.T) {
	for _, mode := range []string{"inline", "file"} {
		for _, test := range []struct {
			name, endpoint, value, wantError string
			unset                            bool
		}{
			{name: "resolved", endpoint: "env:BENCH_OPENAI_ENDPOINT", value: "https://provider.invalid/custom/inference?version=1"},
			{name: "literal", endpoint: "http://127.0.0.1:1/v1/responses", value: "not-a-url"},
			{name: "unset", endpoint: "env:BENCH_OPENAI_ENDPOINT", unset: true, wantError: `environment variable "BENCH_OPENAI_ENDPOINT" is unset or empty`},
			{name: "empty", endpoint: "env:BENCH_OPENAI_ENDPOINT", wantError: "unset or empty"},
			{name: "empty name", endpoint: "env:", wantError: "expected env:VARIABLE_NAME"},
			{name: "invalid name", endpoint: "env:BENCH-URL", wantError: "expected env:VARIABLE_NAME"},
			{name: "relative URL", endpoint: "env:BENCH_OPENAI_ENDPOINT", value: "/v1", wantError: "absolute HTTP URL"},
			{name: "unsupported scheme", endpoint: "env:BENCH_OPENAI_ENDPOINT", value: "ftp://provider.invalid/v1", wantError: "absolute HTTP URL"},
			{name: "no host", endpoint: "env:BENCH_OPENAI_ENDPOINT", value: "https:///v1", wantError: "absolute HTTP URL"},
			{name: "nonrecursive", endpoint: "env:BENCH_OPENAI_ENDPOINT", value: "env:OTHER_ENDPOINT", wantError: "absolute HTTP URL"},
		} {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				t.Setenv("BENCH_OPENAI_ENDPOINT", test.value)
				t.Setenv("BENCH_OPENAI_API_KEY", "")
				if test.unset {
					if err := os.Unsetenv("BENCH_OPENAI_ENDPOINT"); err != nil {
						t.Fatal(err)
					}
				}
				directory := t.TempDir()
				path := filepath.Join(directory, "tests.yaml")
				provider := "alias: openai\nprotocol: openai-responses\nendpoint: " + strconv.Quote(test.endpoint) + "\napi_key:\n  sources: [env:BENCH_OPENAI_API_KEY]"
				entry := "  - " + strings.ReplaceAll(provider, "\n", "\n    ")
				origin := path
				if mode == "file" {
					origin = filepath.Join(directory, "provider.yaml")
					if err := os.WriteFile(origin, []byte(provider), 0o600); err != nil {
						t.Fatal(err)
					}
					entry = "  - file:provider.yaml"
				}
				contents := "api_version: v1\nagent_providers:\n" + entry + `
agent_models:
  - alias: assistant
    provider: openai
    model: remote-model
    parameters: {}
system_prompts:
  - alias: baseline
    mode: builtin
questions:
  - alias: question
    text: env:BENCH_OPENAI_ENDPOINT
lisp_environments:
  - alias: session
tests:
  - alias: basic
    matrix:
      models: [assistant]
      system_prompts: [baseline]
      questions: [question]
      lisp_environments: [session]
`
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
				err, configuration := ValidateFile(path)
				if test.wantError != "" {
					if err == nil || !strings.Contains(err.Error(), test.wantError) || !strings.Contains(err.Error(), origin) || !strings.Contains(err.Error(), "agent_providers[0]") {
						t.Fatalf("expected %q at %s agent_providers[0], got %v", test.wantError, origin, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				want := test.endpoint
				if strings.HasPrefix(want, "env:") {
					want = test.value
				}
				providerDefinition := configuration.Document.AgentProviders[0]
				if providerDefinition.Endpoint == nil || *providerDefinition.Endpoint != want || providerDefinition.ApiKey.Sources[0] != "env:BENCH_OPENAI_API_KEY" || configuration.Document.Questions[0].Text != "env:BENCH_OPENAI_ENDPOINT" {
					t.Fatalf("unexpected resolution: %#v", configuration.Document)
				}
				t.Setenv("BENCH_OPENAI_ENDPOINT", "https://changed.invalid/v1/responses")
				prepareErr, prepared := Prepare(configuration, path, "version", "fingerprint")
				if prepareErr != nil {
					t.Fatal(prepareErr)
				}
				if *prepared.Providers[0].Endpoint != want {
					t.Fatal("preparation must capture the loaded URL without resolving the environment again")
				}
			})
		}
	}
}
