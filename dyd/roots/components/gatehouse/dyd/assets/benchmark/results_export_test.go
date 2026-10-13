package benchmark

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func openResultsFileForTest(t *testing.T, path string) *sql.DB {
	t.Helper()
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	return db
}

func resultsFileRows(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query("SELECT * FROM " + quoteResultsIdentifier(table) + " ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	values, pointers := make([]any, len(columns)), make([]any, len(columns))
	for index := range values {
		pointers[index] = &values[index]
	}
	var records []string
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		records = append(records, resultJSON(t, values))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func assertResultsFileEqual(t *testing.T, source, destination *sql.DB) {
	t.Helper()
	for _, table := range resultsTables {
		want, got := resultsFileRows(t, source, table), resultsFileRows(t, destination, table)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s changed on file output\ngot: %v\nwant: %v", table, got, want)
		}
	}
	var version int
	if err := destination.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("file schema version = %d, want 1", version)
	}
	rows, err := destination.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("file output contains a foreign-key violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestExportResultsRoundTrip(t *testing.T) {
	for _, appendOutput := range []bool{false, true} {
		name := "create"
		if appendOutput {
			name = "append-creates-missing-file"
		}
		t.Run(name, func(t *testing.T) {
			source := openTestResults(t)
			first, second := newResultsFixture(t, "first"), newResultsFixture(t, "second")
			second.env.Prelude, second.env.ProjectLinked, second.prompt.Text = nil, nil, nil
			second.prompt.Mode = "builtin"
			second.provider.ApiKeySources, second.provider.BaseUrl = nil, nil
			second.provider.Protocol = "builtin"
			second.model.Model = "dummy.fixed-reply"
			second.model.Parameters = map[string]interface{}{"text": "reply"}
			second.model.CompactionAlgorithm, second.model.HistoryBytes = nil, nil
			second.events[len(second.events)-1].Metrics = nil
			insertResultFixture(t, source, first)
			insertResultFixture(t, source, second)
			path := filepath.Join(t.TempDir(), "capture ? # results.sqlite")
			if err := ExportResults(context.Background(), source, path, appendOutput); err != nil {
				t.Fatal(err)
			}
			// Reopen through the ordinary SQLite driver, with no Gatehouse setup.
			file := openResultsFileForTest(t, path)
			assertResultsFileEqual(t, source, file)
			var question, answer string
			if err := file.QueryRow(`SELECT q.text, json_extract(e.payload, '$.text')
				FROM runs r JOIN questions q ON q.id = r.question_id
				JOIN session_events e ON e.run_id = r.id AND e.id = r.terminal_event_id
				WHERE r.id = ?`, first.run.Id).Scan(&question, &answer); err != nil {
				t.Fatal(err)
			}
			if question != first.question.Text || answer != "Capabilities" {
				t.Fatal("file output lost input/run/event relationships")
			}
			stat, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if stat.Mode().Perm() != 0o600 {
				t.Fatalf("output permissions = %o, want 600", stat.Mode().Perm())
			}
		})
	}
}

func TestExportResultsAppendPreservesExecutions(t *testing.T) {
	firstSource, secondSource, expected := openTestResults(t), openTestResults(t), openTestResults(t)
	first, second := newResultsFixture(t, "original question"), newResultsFixture(t, "changed question")
	insertResultFixture(t, firstSource, first)
	insertResultFixture(t, secondSource, second)
	insertResultFixture(t, expected, first)
	insertResultFixture(t, expected, second)
	path := filepath.Join(t.TempDir(), "results.sqlite")
	if err := ExportResults(context.Background(), firstSource, path, false); err != nil {
		t.Fatal(err)
	}
	if err := ExportResults(context.Background(), secondSource, path, true); err != nil {
		t.Fatal(err)
	}
	file := openResultsFileForTest(t, path)
	assertResultsFileEqual(t, expected, file)
	for _, fixture := range []resultsFixture{first, second} {
		var text string
		if err := file.QueryRow(`SELECT q.text FROM runs r JOIN questions q
			ON q.execution_id = r.execution_id AND q.id = r.question_id WHERE r.id = ?`, fixture.run.Id).Scan(&text); err != nil {
			t.Fatal(err)
		}
		if text != fixture.question.Text {
			t.Fatal("reused aliases replaced an earlier execution's input")
		}
	}
}

func TestExportResultsRequiresExplicitAppend(t *testing.T) {
	source := openTestResults(t)
	insertResultFixture(t, source, newResultsFixture(t, "original"))
	path := filepath.Join(t.TempDir(), "results.sqlite")
	if err := ExportResults(context.Background(), source, path, false); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExportResults(context.Background(), source, path, false); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing output error = %v, want os.ErrExist", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("non-append export modified an existing file")
	}
}

func TestExportResultsFailedAppendRollsBack(t *testing.T) {
	firstSource, secondSource := openTestResults(t), openTestResults(t)
	first, second := newResultsFixture(t, "existing"), newResultsFixture(t, "new")
	insertResultFixture(t, firstSource, first)
	// Collide in the final table, after the execution, inputs, and run have
	// already been inserted into the destination transaction.
	second.events[len(second.events)-1].Id = *first.run.TerminalEventId
	second.run.TerminalEventId = first.run.TerminalEventId
	insertResultFixture(t, secondSource, second)
	path := filepath.Join(t.TempDir(), "results.sqlite")
	if err := ExportResults(context.Background(), firstSource, path, false); err != nil {
		t.Fatal(err)
	}
	if err := ExportResults(context.Background(), secondSource, path, true); err == nil {
		t.Fatal("duplicate captured identity was silently accepted")
	}
	assertResultsFileEqual(t, firstSource, openResultsFileForTest(t, path))
	if err := ExportResults(context.Background(), openTestResults(t), path, true); err != nil {
		t.Fatalf("failed append left an unusable transaction: %v", err)
	}
}

func TestExportResultsRejectsIncompatibleOutput(t *testing.T) {
	for _, version := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			source := openTestResults(t)
			insertResultFixture(t, source, newResultsFixture(t, "source"))
			path := filepath.Join(t.TempDir(), "other.sqlite")
			file := openResultsFileForTest(t, path)
			if _, err := file.Exec(`CREATE TABLE other (value TEXT); INSERT INTO other VALUES ('keep')`); err != nil {
				t.Fatal(err)
			}
			if _, err := file.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := ExportResults(context.Background(), source, path, true); err == nil {
				t.Fatal("incompatible output was accepted")
			}
			file = openResultsFileForTest(t, path)
			var text string
			if err := file.QueryRow(`SELECT value FROM other`).Scan(&text); err != nil {
				t.Fatal(err)
			}
			if text != "keep" {
				t.Fatal("incompatible output was modified")
			}
		})
	}
}

func TestExportResultsNewFailureRemovesFile(t *testing.T) {
	source := openTestResults(t)
	insertResultFixture(t, source, newResultsFixture(t, "source"))
	if _, err := source.Exec(`ALTER TABLE questions ADD COLUMN incompatible TEXT`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "results.sqlite")
	if err := ExportResults(context.Background(), source, path, false); err == nil {
		t.Fatal("incompatible source column was accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed new export left an output file: %v", err)
	}
	if err := source.QueryRow(`SELECT COUNT(*) FROM executions`).Scan(new(int)); err != nil {
		t.Fatalf("export failure closed its caller-owned source: %v", err)
	}
}
