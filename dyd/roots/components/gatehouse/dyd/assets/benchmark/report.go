package benchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

func ValidateOutput(report, format string) error {
	switch report {
	case "summary", "results", "full":
	default:
		return fmt.Errorf("unknown report %q; expected summary, results, or full", report)
	}
	switch format {
	case "text", "json", "jsonl", "sqlite":
	default:
		return fmt.Errorf("unknown format %q; expected text, json, jsonl, or sqlite", format)
	}
	if format == "sqlite" && report != "full" {
		return fmt.Errorf("SQLite output requires --report=full")
	}
	return nil
}

// Report reads only the capture database. Output selection never changes what
// Execute captures or how the SQL views grade it.
func Report(ctx context.Context, capture *sql.DB, report, format string, output io.Writer) error {
	if err := ValidateOutput(report, format); err != nil {
		return err
	}
	if format == "sqlite" {
		directory, err := os.MkdirTemp("", "gatehouse-benchmark-export-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(directory)
		path := filepath.Join(directory, "results.sqlite")
		if err := ExportResults(ctx, capture, path, false); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(output, file)
		return err
	}
	summary, err := resultRows(ctx, capture, `SELECT * FROM execution_summary ORDER BY execution_id`)
	if err != nil {
		return err
	}
	groups, err := resultRows(ctx, capture, `SELECT execution_id, test_alias, model_alias, prompt_alias, environment_alias,
        COUNT(*) AS runs, SUM(grade = 'passed') AS passed, SUM(grade = 'failed') AS failed,
        SUM(grade = 'ungraded') AS ungraded, SUM(grade = 'error') AS errors, SUM(grade = 'cancelled') AS cancelled, SUM(grade = 'pending') AS pending,
        SUM(input_tokens) AS input_tokens, SUM(output_tokens) AS output_tokens,
        SUM(reasoning_tokens) AS reasoning_tokens, SUM(cached_input_tokens) AS cached_input_tokens, SUM(total_tokens) AS total_tokens,
        SUM(tool_calls) AS tool_calls, SUM(tool_failures) AS tool_failures, SUM(turns) AS turns,
        SUM(elapsed_ms) AS run_ms, AVG(elapsed_ms) AS mean_run_ms, SUM(request_ms) AS request_ms,
        SUM(provider_outcomes) AS provider_outcomes, SUM(input_tokens_observations) AS input_tokens_observations,
        SUM(output_tokens_observations) AS output_tokens_observations,
        SUM(reasoning_tokens_observations) AS reasoning_tokens_observations,
        SUM(cached_input_tokens_observations) AS cached_input_tokens_observations,
        SUM(total_tokens_observations) AS total_tokens_observations,
        SUM(request_ms_observations) AS request_ms_observations,
        SUM(outcomes_with_metrics) AS outcomes_with_metrics
        FROM run_results GROUP BY execution_id, test_alias, model_alias, prompt_alias, environment_alias
        ORDER BY execution_id, test_alias, model_alias, prompt_alias, environment_alias`)
	if err != nil {
		return err
	}
	document := map[string]interface{}{"schema_version": 3, "report": report, "summary": summary, "groups": groups}
	if report != "summary" {
		runs, err := resultRows(ctx, capture, `SELECT * FROM run_results ORDER BY execution_id, started_at, id`)
		if err != nil {
			return err
		}
		document["results"] = runs
	}
	if report == "full" {
		data := map[string]interface{}{}
		for _, table := range resultsTables {
			rows, err := resultRows(ctx, capture, "SELECT * FROM "+quoteResultsIdentifier(table)+" ORDER BY rowid")
			if err != nil {
				return err
			}
			data[table] = rows
		}
		document["data"], document["schema"] = data, resultsSchema
	}
	encoder := json.NewEncoder(output)
	if format == "json" {
		encoder.SetIndent("", "  ")
		return encoder.Encode(document)
	}
	if format == "jsonl" {
		if report == "full" {
			if err := encoder.Encode(map[string]interface{}{"type": "schema", "schema_version": 3, "sql": resultsSchema}); err != nil {
				return err
			}
			data := document["data"].(map[string]interface{})
			for _, table := range resultsTables {
				for _, row := range data[table].([]map[string]interface{}) {
					if err := encoder.Encode(map[string]interface{}{"type": table, "data": row}); err != nil {
						return err
					}
				}
			}
		}
		if report != "summary" {
			for _, row := range document["results"].([]map[string]interface{}) {
				if err := encoder.Encode(map[string]interface{}{"type": "result", "data": row}); err != nil {
					return err
				}
			}
		}
		for _, group := range groups {
			if err := encoder.Encode(map[string]interface{}{"type": "group", "data": group}); err != nil {
				return err
			}
		}
		for _, row := range summary {
			if err := encoder.Encode(map[string]interface{}{"type": "summary", "data": row}); err != nil {
				return err
			}
		}
		return nil
	}
	if report == "full" {
		// A readable, complete representation, including base64 artifact contents.
		encoder.SetIndent("", "  ")
		return encoder.Encode(document)
	}
	runs, _ := document["results"].([]map[string]interface{})
	return textReport(output, summary, groups, runs)
}

func textReport(output io.Writer, summary, groups, runs []map[string]interface{}) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	var writeErr error
	write := func(format string, values ...interface{}) {
		if writeErr == nil {
			_, writeErr = fmt.Fprintf(writer, format, values...)
		}
	}
	measurements := func(row map[string]interface{}) {
		for _, metric := range []struct{ label, key string }{
			{"Input tokens", "input_tokens"}, {"Output tokens", "output_tokens"},
			{"Reasoning tokens", "reasoning_tokens"}, {"Cached input tokens", "cached_input_tokens"},
			{"Total tokens", "total_tokens"}, {"Provider request ms", "request_ms"},
		} {
			write("%s\t%v (%v/%v outcomes reported)\n", metric.label, display(row[metric.key]), display(row[metric.key+"_observations"]), display(row["provider_outcomes"]))
		}
	}
	for _, row := range summary {
		write("Execution\t%v\nStatus\t%v\nPlanned runs\t%v\nCaptured runs\t%v\nPassed\t%v\nFailed\t%v\nUngraded\t%v\nErrors\t%v\nCancelled\t%v\nPending\t%v\nElapsed ms\t%v\nTool calls\t%v\nTool failures\t%v\nTool-using turns\t%v\n",
			row["execution_id"], row["status"], display(row["planned_runs"]), row["materialized_runs"], row["passed"], row["failed"], row["ungraded"], row["errors"], row["cancelled"], row["pending"], display(row["elapsed_ms"]), row["tool_calls"], row["tool_failures"], row["turns"])
		measurements(row)
	}
	write("\nExecution\tTest\tModel\tPrompt\tEnvironment\tRuns\tPassed\tFailed\tUngraded\tErrors\tCancelled\tPending\n")
	for _, row := range groups {
		write("%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\n", row["execution_id"], row["test_alias"], row["model_alias"], row["prompt_alias"], row["environment_alias"], row["runs"], row["passed"], row["failed"], row["ungraded"], row["errors"], row["cancelled"], row["pending"])
	}
	for _, row := range runs {
		write("\nRun\t%v\nExecution\t%v\nTest\t%v\nQuestion\t%v\nModel\t%v\nPrompt\t%v\nEnvironment\t%v\nRepetition\t%v\nStatus\t%v\nGrade\t%v\n",
			row["id"], row["execution_id"], row["test_alias"], row["question_alias"], row["model_alias"], row["prompt_alias"], row["environment_alias"], row["repetition"], row["status"], row["grade"])
		expected := "not specified"
		if row["expected_output"] != nil {
			expected = quoted(row["expected_output"])
		}
		write("Output\t%s expected %s, got %s\n", checkStatus(row["output_passed"]), expected, quoted(row["output"]))
		for _, check := range []struct{ label, actual, expected, passed string }{
			{"Turns", "turns", "expected_max_turns", "turns_passed"},
			{"Tool failures", "tool_failures", "expected_max_tool_failures", "tool_failures_passed"},
			{"Tool calls", "tool_calls", "expected_max_tool_calls", "tool_calls_passed"},
		} {
			maximum := display(row[check.expected])
			if row[check.expected] == nil {
				maximum = "not specified"
			}
			write("%s\t%s actual %v, maximum %v\n", check.label, checkStatus(row[check.passed]), row[check.actual], maximum)
		}
		if row["error"] != nil {
			write("Error\t%s\n", quoted(row["error"]))
		}
		write("Elapsed ms\t%v\n", display(row["elapsed_ms"]))
		measurements(row)
	}
	if writeErr != nil {
		return writeErr
	}
	return writer.Flush()
}

func checkStatus(value interface{}) string {
	switch value {
	case nil:
		return "unchecked"
	case int64(0), false:
		return "FAIL"
	case int64(1), true:
		return "PASS"
	default:
		return fmt.Sprint(value)
	}
}

func quoted(value interface{}) string {
	text, _ := json.Marshal(value)
	return string(text)
}

func display(value interface{}) interface{} {
	if value == nil {
		return "unknown"
	}
	return value
}

// Successful reports and exit status use the same SQL grading projection.
func ResultsFailed(ctx context.Context, capture *sql.DB) (bool, error) {
	var failed bool
	err := capture.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM run_results WHERE grade IN ('failed', 'error', 'cancelled', 'pending'))
        OR EXISTS(SELECT 1 FROM executions WHERE status <> 'completed')`).Scan(&failed)
	return failed, err
}

func resultRows(ctx context.Context, db *sql.DB, query string, arguments ...interface{}) ([]map[string]interface{}, error) {
	rows, err := db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := []map[string]interface{}{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for index := range pointers {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		record := map[string]interface{}{}
		for index, column := range columns {
			value := values[index]
			if text, ok := value.(string); ok {
				switch column {
				case "configuration", "sources", "parameters", "payload", "metrics", "metadata", "api_key_sources":
					decoder := json.NewDecoder(strings.NewReader(text))
					decoder.UseNumber()
					if err := decoder.Decode(&value); err != nil {
						return nil, fmt.Errorf("decode captured %s: %w", column, err)
					}
				case "planned_runs", "run_count":
					value = json.Number(text)
				}
			}
			record[column] = value
		}
		result = append(result, record)
	}
	return result, rows.Err()
}
