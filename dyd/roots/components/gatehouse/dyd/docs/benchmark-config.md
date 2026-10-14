# Benchmark configuration

`gatehouse benchmark validate tests.yaml` loads and validates benchmark inputs
and reports the number of expanded runs. Validation makes no network requests:
provider endpoint environment references are resolved locally, while credential
sources are retained for runtime use.

```yaml
api_version: v1

agent_providers:
  - alias: openai
    protocol: openai-responses
    endpoint: env:BENCH_OPENAI_ENDPOINT
    api_key:
      sources: [env:BENCH_OPENAI_API_KEY]

agent_models:
  - alias: assistant
    provider: openai
    model: your-model-id
    parameters:
      reasoning_effort: low

system_prompts:
  - alias: baseline
    mode: builtin
  - alias: discovery
    mode: append
    text: file:./prompts/discovery.txt

questions:
  - alias: capabilities
    text: "hey, what can you do?"
  - alias: tools
    text: file:./questions/tools.txt

lisp_environments:
  - alias: session
  - alias: project
    project_linked: true

tests:
  - alias: capability-discovery
    matrix:
      models: [assistant]
      system_prompts: [baseline, discovery]
      questions: [capabilities, tools]
      lisp_environments: [session, project]
    repetitions: 3
```

This matrix describes 24 runs: one model, two prompts, two questions, two
environments, and three repetitions. All six collections are required and
nonempty. Aliases match `[a-z][a-z0-9_-]*` and are unique within each collection.
Each matrix dimension is nonempty, has unique selections, and references known
aliases. Repetitions default to one and must be positive. Expansion follows test
order, then model/prompt/question/environment selection order, then repetitions.

For this example, set `BENCH_OPENAI_ENDPOINT=https://api.openai.com/v1/responses` before
validation or execution. `BENCH_OPENAI_API_KEY` is needed only for execution.

## Individual definition files

An entry in `agent_providers`, `agent_models`, `system_prompts`, `questions`, or
`lisp_environments` may be a `file:` reference instead of an inline record:

```yaml
agent_models:
  - file:./models/assistant.yaml
```

The referenced file supplies one record, including its alias, without a collection
wrapper:

```yaml
alias: assistant
provider: openai
model: your-model-id
parameters: {}
```

Definition files must contain a record, not an array or another reference. The
source schema permits records or strings at these entry positions; the loader
requires reference strings to use `file:<nonempty path>`. The loaded schema
requires records. Whole collections and test entries are inline arrays/records.

Prompt `text`, question `text`, and environment `prelude` may also use `file:`.
Those files are read verbatim, preserving whitespace and Lisp comments, with no
further interpretation of their contents. Relative references are resolved against
the file containing the reference, including inside imported definitions. Other
fields are not file-reference locations.

## Input definitions

- **Providers and models:** are independently defined by the v1 benchmark format.
  Revisions are not part of this format. Supported protocols are `builtin`,
  `openai-chat-completions`, and `openai-responses`. OpenAI providers require an
  absolute HTTP(S) `endpoint` or an `env:VARIABLE_NAME` reference, and nonempty
  credential sources using `env:`/`stdin:`. Endpoint references are resolved once
  during loading, for inline and file-based providers. The variable name must
  match `[A-Za-z_][A-Za-z0-9_]*`; unset or empty variables are errors, and their
  values must be absolute HTTP(S) URLs. The loaded configuration and captured
  provider snapshot contain the resolved URL. Literal URLs are also accepted.
  Gatehouse POSTs to the complete endpoint URL without appending a route or
  modifying its path or query. Protocol selects only the request/response format.
  Environment resolution applies only to provider `endpoint`; other fields are
  not recursively substituted.
  Validation retains credential descriptors without reading credentials. Builtin
  providers omit the URL and credentials. A selected model and its provider must
  be enabled; an omitted `enabled` is treated as true.
- **Model settings:** `parameters` is an object. OpenAI parameters permit optional
  `reasoning_effort` (`none`, `low`, `medium`, `high`, `xhigh`, or `max`); Chat
  Completions permits only `none`. Builtin models use `dummy.fixed-reply` with one
  nonblank `text` parameter. Supplied turn/token limits and compaction capacities
  must be positive; when both compaction capacities are supplied, the buffer must
  not exceed half the history. Omitted settings stay omitted in the loaded config;
  validation does not import operational defaults or model-specific profiles.
- **Prompts:** `mode` is `builtin`, `append`, or `replace`. Builtin mode omits
  `text`; append and replace require it. These describe the intended runtime
  prompt composition.
- **Questions:** require nonblank `text`.
- **Lisp environments:** `project_linked` defaults to false. An omitted `prelude`
  selects the normal Gatehouse prelude; an explicitly empty prelude stays empty.

The benchmark's own Dhall definitions generate source and loaded JSON Schemas and
a Go codec that decodes `GatehouseBenchmarkConfig`. Only the two benchmark
representations share definitions. Benchmark provider/model validation is owned
by the benchmark package; operational schema, resolver, and default changes do not
automatically change this file format. Conversion into runtime configuration
belongs at the execution boundary.
