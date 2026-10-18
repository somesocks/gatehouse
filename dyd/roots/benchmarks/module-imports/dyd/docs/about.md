# Weather/travel module-import benchmark

This scenario compares initially bound module functions with discovering and
importing the same functions. It uses `gpt-6-luna` at low and high reasoning effort,
seven questions, three system prompts, two environments, and one repetition:
**1 test, 84 runs**. Medium reasoning is disabled. A run can make
multiple provider requests; 84 is not a request, token, or cost budget. Model
availability is established only by the provider at execution time.

The runner defaults to eight concurrent runs on one execution-local database and
DBOS runtime. Each run still has a fresh session. Pass `--parallel=1` after Dryad's
`--` separator for serial execution, or `--parallel=8` to select the default
explicitly. This flag belongs to Gatehouse, independently of Dryad's parallelism.

## Fixtures and experimental conditions

All data refers to one fixed fictional benchmark day:

| Destination from London | Temperature (C) | Rain expected | Fare (EUR) |
|---|---:|---|---:|
| Athens | 20 | Yes | 150 |
| Berlin | 23 | Yes | 140 |
| Dublin | 17 | No | 110 |
| Helsinki | 16 | Yes | 55 |
| Lisbon | 22 | No | 120 |
| Madrid | 19 | No | 185 |
| Oslo | 29 | Yes | 175 |
| Paris | 18 | Yes | 90 |
| Prague | 21 | No | 80 |
| Rome | 24 | No | 160 |
| Vienna | 26 | Yes | 130 |
| Warsaw | 25 | No | 95 |

`@native:weather/v1` exports `forecast`, taking a case-sensitive city
String and returning a proper association list with symbol keys `temperature_c`
(Integer, Celsius) and `rain` (Boolean). Read fields using
`(list/assoc/require 'temperature_c record)`.
`@native:travel/v1` exports `fare`, taking origin and destination Strings
and returning an Integer fare in EUR. Only the origin `London` and the twelve
destinations in the table are supported. Unsupported inputs raise fixed errors;
function help documents the signatures and errors without providing answers.
There are no live lookups, mutable state, date calculations, or bookings.

The preludes in `preludes/` define the weather and travel modules. Both expose
exactly these two modules through `import/search` and allow imports. The
fixture-definition helper is shadowed after installation. Only `pre-imported`
starts with `weather/forecast` and `travel/fare` bound; `self-import` must import
them inside its programs. Imports are lexical and each tool evaluation is fresh.
Each prompt is used in both import environments with the same questions, help,
and model settings.

## System prompts

| Alias | Contents |
|---|---|
| `detailed` | Grounding instructions and a full Lisp reference with import examples |
| `compact` | Grounding and syntax reference with operation families discovered through help |
| `minimal` | Grounding, stateless execution, and discovery/import examples |

Complete replacement prompts are stored in `prompts/`.

## Build and offline validation

From the garden root:

```sh
dryad root build dyd/roots/benchmarks/module-imports --scope=none --join-stdout --join-stderr
```

The build copies assets unchanged and packages the run command and dependency.
Use `--scope=none` because the dev build scope selects test roots and component
test variants.

Validate the configuration and check fixtures separately, without an API key:

```sh
stem=dyd/sprouts/benchmarks/module-imports/dyd/dependencies/stem
gatehouse="$stem/dyd/dependencies/gatehouse/dyd/commands/dyd-stem-run"
"$gatehouse" benchmark validate "$stem/dyd/assets/tests.yaml"
sh "$stem/dyd/assets/checks/verify.sh" "$stem/dyd/assets" "$gatehouse"
```

The source requirement selects the host Gatehouse build. The built stem declares
`env:OPENAI_API_KEY` as a runtime requirement; neither the source build dependency
graph nor the build reads or embeds its value. Credential sources in the config
remain deferred descriptors. Dryad may resolve the runtime requirement before
launching any command, including `--help`; the commands above invoke the Gatehouse
dependency directly for keyless validation.

## Live execution

Set `OPENAI_API_KEY` in the caller environment using your normal credential setup.
Dryad supplies this specifically declared variable; `--inherit=true` is unnecessary.
Then explicitly start the benchmark, for example:

```sh
dryad sprout run dyd/sprouts/benchmarks/module-imports --scope=none -- \
  --report=results --format=text
```

For full SQLite capture:

```sh
dryad sprout run dyd/sprouts/benchmarks/module-imports --scope=none -- \
  --report=full --format=sqlite --output=/absolute/path/to/module-imports.sqlite
```

Replace the output path with your intended destination. The wrapper forwards
options to `gatehouse benchmark run` with an absolute packaged config path.
Reports are `summary`, `results`, or `full`; formats are `text`, `json`, `jsonl`,
or `sqlite`. SQLite requires `--report=full`.

Dryad documents joined child stdout as going to the parent **stderr**, so prefer
the runner's `--output=<absolute path>` rather than redirecting Dryad stdout as
if it were a raw report. Direct Gatehouse execution supports shell redirection.
Diagnostics keep their configured destinations; choose nonconflicting outputs.

## Grading and interpreting results

The runner compares the final response's `text` field with each question's
configured expectation after trimming surrounding whitespace. Expectations are
captured in results but are not passed to the agent. There are no count-based
grading caps; correctness and efficiency can be inspected separately.

Omitted execution settings use runner defaults: 127 tool-using turns, 16,000
output tokens, and 96/48 KiB MCMTR history/buffer capacities. These limits are
identical across conditions and distinct from optional `expected.max_*` grading
checks. A committed tool-using round counts as one turn, even with multiple tool
calls; final-answer rounds and provider retries do not add turns. One Lisp
invocation is one tool call regardless of module-function calls inside it.

Query a saved full capture with SQLite:

```sql
SELECT model_alias, prompt_alias, environment_alias,
       COUNT(*) AS runs,
       SUM(grade = 'passed') AS passed,
       SUM(output_passed = 1) AS correct_answers,
       AVG(turns) AS mean_tool_turns,
       AVG(tool_calls) AS mean_tool_calls,
       AVG(tool_failures) AS mean_tool_failures,
       SUM(input_tokens) AS observed_input_tokens,
       SUM(input_tokens_observations) AS input_token_observations,
       SUM(provider_outcomes) AS provider_outcomes,
       AVG(elapsed_ms) AS mean_elapsed_ms
FROM run_results
GROUP BY model_alias, prompt_alias, environment_alias
ORDER BY model_alias, prompt_alias, environment_alias;
```

Use metric observation/coverage columns to interpret partial token and provider
time measurements. `elapsed_ms` includes per-run setup and capture overhead;
`request_ms` measures provider time separately. Inspect native tool traces,
including zero-tool answers, before attributing differences to importing.
One repetition is an initial pilot, not evidence of statistical equivalence.
For later runs, increase `repetitions` in the source config and rebuild; there is
no repetitions CLI override. Keep captures and credentials outside source control.
