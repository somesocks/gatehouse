package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"
)

func TestAgentRateLimitBucketMath(t *testing.T) {
	bucket := &config.TokenBucket{MinimumBalance: -120, MaximumBalance: 120, RefillPerMinute: 60}
	updated := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	if got := refillCredit(-30, updated, updated.Add(30*time.Second), bucket); got != 0 {
		t.Fatalf("refillCredit() = %v, want 0", got)
	}
	if got := refillCredit(100, updated, updated.Add(30*time.Second), bucket); got != 120 {
		t.Fatalf("refillCredit() capacity = %v, want 120", got)
	}
	if got := refillCredit(200, updated, updated.Add(30*time.Second), bucket); got != 200 {
		t.Fatalf("refillCredit() grandfathered credit = %v, want 200", got)
	}
	if got := debtWait(-30, bucket.RefillPerMinute); got != 30*time.Second {
		t.Fatalf("debtWait() = %s, want 30s", got)
	}
}

func TestAgentRateLimitPostgresClaimQualifiesTargetColumns(t *testing.T) {
	bucket := &config.TokenBucket{MinimumBalance: -120, MaximumBalance: 120, RefillPerMinute: 60}
	expression := agentRateLimitClaimExpression(config.DatabaseKindPostgres, agentRateLimitTargetColumn(config.DatabaseKindPostgres, "available_input_tokens"), bucket, 20, "excluded.accounted_at")
	for _, want := range []string{
		"gatehouse_workspace_agent_rate_buckets.available_input_tokens",
		"gatehouse_workspace_agent_rate_buckets.accounted_at",
	} {
		if !strings.Contains(expression, want) {
			t.Fatalf("PostgreSQL claim expression %q does not qualify %q", expression, want)
		}
	}
}

func TestParseAgentRateLimitBucketTime(t *testing.T) {
	for _, value := range []string{
		"2026-09-22T21:15:15.727872Z",
		"2026-09-22 21:15:15.727872+00",
	} {
		if _, err := parseAgentRateLimitBucketTime(value); err != nil {
			t.Fatalf("parseAgentRateLimitBucketTime(%q): %v", value, err)
		}
	}
}

func TestValidateAgentRateLimitUsage(t *testing.T) {
	claim := AgentRateLimitClaim{OutputTokens: 100}
	validInput, validOutput := 150, 100
	negative := -1
	overReservation := 101
	for name, test := range map[string]struct {
		input, output *int
		wantError     bool
	}{
		"missing usage":       {nil, nil, false},
		"valid usage":         {&validInput, &validOutput, false},
		"negative input":      {&negative, &validOutput, true},
		"negative output":     {&validInput, &negative, true},
		"output over reserve": {&validInput, &overReservation, true},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateAgentRateLimitUsage(claim, test.input, test.output)
			if (err != nil) != test.wantError {
				t.Fatalf("validateAgentRateLimitUsage() error = %v, want error %t", err, test.wantError)
			}
		})
	}
}

func TestAgentRateLimitClaimScopesAndMinimum(t *testing.T) {
	ctx := context.Background()
	err, store := Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ExecContext(ctx, `
		CREATE TABLE gatehouse_workspaces (id TEXT PRIMARY KEY) STRICT;
		CREATE TABLE gatehouse_principals (id TEXT PRIMARY KEY) STRICT;
		CREATE TABLE gatehouse_workspace_agents (workspace_id TEXT NOT NULL, id TEXT NOT NULL, PRIMARY KEY (workspace_id, id)) STRICT;
		CREATE TABLE gatehouse_workspace_agent_rate_buckets (
			workspace_id TEXT NOT NULL REFERENCES gatehouse_workspaces (id),
			binding_id TEXT NOT NULL,
			principal_id TEXT REFERENCES gatehouse_principals (id),
			available_input_tokens REAL NOT NULL,
			available_output_tokens REAL NOT NULL,
			accounted_at TEXT NOT NULL,
			UNIQUE (workspace_id, binding_id, principal_id),
			FOREIGN KEY (workspace_id, binding_id) REFERENCES gatehouse_workspace_agents (workspace_id, id)
		) STRICT;
		CREATE UNIQUE INDEX gatehouse_workspace_agent_rate_buckets_global
			ON gatehouse_workspace_agent_rate_buckets (workspace_id, binding_id)
			WHERE principal_id IS NULL;
		INSERT INTO gatehouse_workspaces (id) VALUES ('wsp_test');
		INSERT INTO gatehouse_principals (id) VALUES ('prn_test');
		INSERT INTO gatehouse_workspace_agents (workspace_id, id) VALUES ('wsp_test', 'wag_test');
	`); err != nil {
		t.Fatal(err)
	}
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-agent-rate-limit-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	dataSource, err := dbos.NewDataSource(dbosContext, store.DB, dbos.WithDataSourceName("gatehouse-rate-limit-test"))
	if err != nil {
		t.Fatal(err)
	}
	selected := WorkspaceAgentModel{
		Ref: model.WorkspaceAgentRef{Workspace: model.WorkspaceRef{Id: "wsp_test"}, Id: "wag_test"},
		RateLimits: &config.WorkspaceAgentRateLimits{
			WorkspaceInput: &config.TokenBucket{MinimumBalance: -50, MaximumBalance: 100, RefillPerMinute: 60},
			UserInput:      &config.TokenBucket{MinimumBalance: -50, MaximumBalance: 100, RefillPerMinute: 60},
		},
	}
	principal := model.PrincipalRef{Id: "prn_test"}
	runner := agentRateLimitTestWorkflow{store: store, dataSource: dataSource, selected: selected, principal: principal}
	dbos.RegisterWorkflow(dbosContext, runner.claim, dbos.WithInstance(&runner), dbos.WithWorkflowName("gatehouse.agent-rate-limit-test"))
	dbos.RegisterWorkflow(dbosContext, runner.settle, dbos.WithInstance(&runner), dbos.WithWorkflowName("gatehouse.agent-rate-limit-settle-test"))
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, time.Second)
	claim := func(tokens int) (AgentRateLimitClaim, error) {
		handle, err := dbos.RunWorkflow(dbosContext, runner.claim, tokens, dbos.WithRunInstance(&runner))
		if err != nil {
			return AgentRateLimitClaim{}, err
		}
		return handle.GetResult()
	}
	first, err := claim(120)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claim(40); !errors.Is(err, ErrAgentRateLimitExceeded) {
		t.Fatalf("second claim error = %v, want minimum-balance rejection", err)
	}
	settle, err := dbos.RunWorkflow(dbosContext, runner.settle, agentRateLimitTestSettleInput{Claim: first, InputTokens: 20}, dbos.WithRunInstance(&runner))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settle.GetResult(); err != nil {
		t.Fatal(err)
	}
	var global, user float64
	if err := store.QueryRowContext(ctx, `SELECT available_input_tokens FROM gatehouse_workspace_agent_rate_buckets WHERE workspace_id = 'wsp_test' AND binding_id = 'wag_test' AND principal_id IS NULL`).Scan(&global); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT available_input_tokens FROM gatehouse_workspace_agent_rate_buckets WHERE workspace_id = 'wsp_test' AND binding_id = 'wag_test' AND principal_id = 'prn_test'`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if global < 79 || global > 81 || user < 79 || user > 81 {
		t.Fatalf("bucket balances = (%v, %v), want approximately (80, 80)", global, user)
	}
}

type agentRateLimitTestWorkflow struct {
	store      *Store
	dataSource *dbos.DataSource
	selected   WorkspaceAgentModel
	principal  model.PrincipalRef
}

func (workflow *agentRateLimitTestWorkflow) ConfigName() string { return "gatehouse-rate-limit-test" }

func (workflow *agentRateLimitTestWorkflow) claim(ctx dbos.Context, tokens int) (AgentRateLimitClaim, error) {
	return dbos.RunAsTransaction(ctx, workflow.dataSource, func(step context.Context, transaction dbos.Tx) (AgentRateLimitClaim, error) {
		claimErr, claim := workflow.store.AgentRateLimitClaimInTransaction(step, transaction, workflow.selected, workflow.principal, tokens, 0)
		return claim, claimErr
	})
}

type agentRateLimitTestSettleInput struct {
	Claim       AgentRateLimitClaim
	InputTokens int
}

func (workflow *agentRateLimitTestWorkflow) settle(ctx dbos.Context, input agentRateLimitTestSettleInput) (struct{}, error) {
	_, err := dbos.RunAsTransaction(ctx, workflow.dataSource, func(step context.Context, transaction dbos.Tx) (struct{}, error) {
		return struct{}{}, workflow.store.AgentRateLimitSettleInTransaction(step, transaction, input.Claim, &input.InputTokens, nil)
	})
	return struct{}{}, err
}
