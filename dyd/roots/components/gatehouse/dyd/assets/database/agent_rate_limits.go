package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"gatehouse/config"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

var ErrAgentRateLimitExceeded = errors.New("agent rate-limit reservation exceeds minimum balance")

// AgentRateLimitClaim is one provider turn's durable token debit.
type AgentRateLimitClaim struct {
	Workspace       model.WorkspaceAgentRef
	Principal       model.PrincipalRef
	InputTokens     float64
	OutputTokens    float64
	WorkspaceInput  *config.TokenBucket
	WorkspaceOutput *config.TokenBucket
	UserInput       *config.TokenBucket
	UserOutput      *config.TokenBucket
	ReservedAt      time.Time
	Wait            time.Duration
}

type agentRateLimitBucket struct {
	Input       float64
	Output      float64
	AccountedAt time.Time
}

// AgentRateLimitClaimInTransaction atomically refills and debits every
// applicable bucket. Claims below a bucket's minimum balance are rejected.
func (store *Store) AgentRateLimitClaimInTransaction(ctx context.Context, transaction dbos.Tx, selected WorkspaceAgentModel, principal model.PrincipalRef, inputTokens, outputTokens int) (error, AgentRateLimitClaim) {
	if transaction == nil {
		return fmt.Errorf("claim agent rate limit: transaction is required"), AgentRateLimitClaim{}
	}
	limits := selected.RateLimits
	if limits == nil || limits.Empty() {
		return nil, AgentRateLimitClaim{}
	}
	if principal.Id == "" || inputTokens < 0 || outputTokens < 0 {
		return fmt.Errorf("claim agent rate limit: invalid token claim"), AgentRateLimitClaim{}
	}
	claim := AgentRateLimitClaim{
		Workspace: selected.Ref, Principal: principal, InputTokens: float64(inputTokens), OutputTokens: float64(outputTokens),
		WorkspaceInput: limits.WorkspaceInput, WorkspaceOutput: limits.WorkspaceOutput, UserInput: limits.UserInput, UserOutput: limits.UserOutput,
		ReservedAt: time.Now().UTC(),
	}
	user := principal.Id
	for _, scope := range []struct {
		principal *string
		input     *config.TokenBucket
		output    *config.TokenBucket
	}{
		{input: claim.WorkspaceInput, output: claim.WorkspaceOutput},
		{principal: &user, input: claim.UserInput, output: claim.UserOutput},
	} {
		if scope.input == nil && scope.output == nil {
			continue
		}
		state, err := store.agentRateLimitBucketClaim(ctx, transaction, claim.Workspace, scope.principal, scope.input, scope.output, claim.InputTokens, claim.OutputTokens, claim.ReservedAt)
		if err != nil {
			return err, AgentRateLimitClaim{}
		}
		if scope.input != nil {
			if state.Input < float64(scope.input.MinimumBalance) {
				return fmt.Errorf("%w: input", ErrAgentRateLimitExceeded), AgentRateLimitClaim{}
			}
			claim.Wait = maxDuration(claim.Wait, debtWait(state.Input, scope.input.RefillPerMinute))
		}
		if scope.output != nil {
			if state.Output < float64(scope.output.MinimumBalance) {
				return fmt.Errorf("%w: output", ErrAgentRateLimitExceeded), AgentRateLimitClaim{}
			}
			claim.Wait = maxDuration(claim.Wait, debtWait(state.Output, scope.output.RefillPerMinute))
		}
	}
	return nil, claim
}

// AgentRateLimitSettleInTransaction reconciles a claim with actual provider
// usage. A nil usage retains that dimension's reservation.
func (store *Store) AgentRateLimitSettleInTransaction(ctx context.Context, transaction dbos.Tx, claim AgentRateLimitClaim, inputTokens, outputTokens *int) error {
	if transaction == nil {
		return fmt.Errorf("settle agent rate limit: transaction is required")
	}
	if claim.Workspace.Id == "" || (inputTokens == nil && outputTokens == nil) {
		return nil
	}
	if err := validateAgentRateLimitUsage(claim, inputTokens, outputTokens); err != nil {
		return err
	}
	now := time.Now().UTC()
	user := claim.Principal.Id
	for _, scope := range []struct {
		principal *string
		input     *config.TokenBucket
		output    *config.TokenBucket
	}{
		{input: claim.WorkspaceInput, output: claim.WorkspaceOutput},
		{principal: &user, input: claim.UserInput, output: claim.UserOutput},
	} {
		if scope.input == nil && scope.output == nil {
			continue
		}
		if _, err := store.agentRateLimitBucketSettle(ctx, transaction, claim.Workspace, scope.principal, scope.input, scope.output, claim.InputTokens, claim.OutputTokens, inputTokens, outputTokens, now); err != nil {
			return err
		}
	}
	return nil
}

func validateAgentRateLimitUsage(claim AgentRateLimitClaim, inputTokens, outputTokens *int) error {
	if inputTokens != nil && *inputTokens < 0 {
		return fmt.Errorf("settle agent rate limit: input usage is negative")
	}
	if outputTokens != nil && *outputTokens < 0 {
		return fmt.Errorf("settle agent rate limit: output usage is negative")
	}
	if outputTokens != nil && float64(*outputTokens) > claim.OutputTokens {
		return fmt.Errorf("settle agent rate limit: output usage exceeds reservation")
	}
	return nil
}

func (store *Store) agentRateLimitBucketClaim(ctx context.Context, transaction dbos.Tx, agent model.WorkspaceAgentRef, principal *string, input, output *config.TokenBucket, inputTokens, outputTokens float64, now time.Time) (agentRateLimitBucket, error) {
	nowText := now.Format(time.RFC3339Nano)
	inputInitial, outputInitial := bucketMaximum(input)-inputTokens, bucketMaximum(output)-outputTokens
	query := `INSERT INTO gatehouse_workspace_agent_rate_buckets (workspace_id, binding_id, principal_id, available_input_tokens, available_output_tokens, accounted_at)
		VALUES (` + rateLimitParameter(store.kind, 1) + `, ` + rateLimitParameter(store.kind, 2) + `, ` + rateLimitParameter(store.kind, 3) + `, ` + rateLimitParameter(store.kind, 4) + `, ` + rateLimitParameter(store.kind, 5) + `, ` + rateLimitParameter(store.kind, 6) + `)
		ON CONFLICT ` + agentRateLimitConflictTarget(principal) + ` DO UPDATE SET
		available_input_tokens = ` + agentRateLimitClaimExpression(store.kind, agentRateLimitTargetColumn(store.kind, "available_input_tokens"), input, inputTokens, "excluded.accounted_at") + `,
			available_output_tokens = ` + agentRateLimitClaimExpression(store.kind, agentRateLimitTargetColumn(store.kind, "available_output_tokens"), output, outputTokens, "excluded.accounted_at") + `,
			accounted_at = excluded.accounted_at
		RETURNING available_input_tokens, available_output_tokens, accounted_at`
	var principalValue any
	if principal != nil {
		principalValue = *principal
	}
	state, err := agentRateLimitBucketScan(transaction.QueryRow(ctx, query, agent.Workspace.Id, agent.Id, principalValue, inputInitial, outputInitial, nowText))
	if err != nil {
		return agentRateLimitBucket{}, fmt.Errorf("claim agent rate-limit bucket: %w", err)
	}
	return state, nil
}

func (store *Store) agentRateLimitBucketSettle(ctx context.Context, transaction dbos.Tx, agent model.WorkspaceAgentRef, principal *string, input, output *config.TokenBucket, reservedInput, reservedOutput float64, actualInput, actualOutput *int, now time.Time) (agentRateLimitBucket, error) {
	nowParameter := rateLimitParameter(store.kind, 1)
	if store.kind == config.DatabaseKindPostgres {
		// Keep the stored clock in its RFC3339 text form even though the refill
		// expression also casts this parameter to timestamptz.
		nowParameter += `::text`
	}
	query := `UPDATE gatehouse_workspace_agent_rate_buckets SET
		available_input_tokens = ` + agentRateLimitSettleExpression(store.kind, "available_input_tokens", input, reservedInput, actualInput, nowParameter) + `,
		available_output_tokens = ` + agentRateLimitSettleExpression(store.kind, "available_output_tokens", output, reservedOutput, actualOutput, nowParameter) + `,
		accounted_at = ` + nowParameter + `
		WHERE workspace_id = ` + rateLimitParameter(store.kind, 2) + ` AND binding_id = ` + rateLimitParameter(store.kind, 3) + agentRateLimitPrincipalCondition(principal, store.kind, 4) + `
		RETURNING available_input_tokens, available_output_tokens, accounted_at`
	args := []any{now.Format(time.RFC3339Nano), agent.Workspace.Id, agent.Id}
	if principal != nil {
		args = append(args, *principal)
	}
	state, err := agentRateLimitBucketScan(transaction.QueryRow(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return agentRateLimitBucket{}, fmt.Errorf("settle agent rate-limit bucket: bucket is unavailable")
	}
	if err != nil {
		return agentRateLimitBucket{}, fmt.Errorf("settle agent rate-limit bucket: %w", err)
	}
	return state, nil
}

func agentRateLimitBucketScan(row interface{ Scan(...any) error }) (agentRateLimitBucket, error) {
	state := agentRateLimitBucket{}
	var accountedAt string
	if err := row.Scan(&state.Input, &state.Output, &accountedAt); err != nil {
		return agentRateLimitBucket{}, err
	}
	parsed, err := parseAgentRateLimitBucketTime(accountedAt)
	if err != nil {
		return agentRateLimitBucket{}, err
	}
	state.AccountedAt = parsed
	return state, nil
}

func parseAgentRateLimitBucketTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("decode agent rate-limit bucket time %q", value)
}

func agentRateLimitConflictTarget(principal *string) string {
	if principal == nil {
		return `(workspace_id, binding_id) WHERE principal_id IS NULL`
	}
	return `(workspace_id, binding_id, principal_id)`
}

func agentRateLimitPrincipalCondition(principal *string, kind config.DatabaseKind, index int) string {
	if principal == nil {
		return ` AND principal_id IS NULL`
	}
	return ` AND principal_id = ` + rateLimitParameter(kind, index)
}

func agentRateLimitClaimExpression(kind config.DatabaseKind, column string, bucket *config.TokenBucket, tokens float64, now string) string {
	if bucket == nil {
		return column
	}
	return agentRateLimitRefillExpression(kind, column, bucket, now) + ` - ` + rateLimitNumber(tokens)
}

func agentRateLimitSettleExpression(kind config.DatabaseKind, column string, bucket *config.TokenBucket, reserved float64, actual *int, now string) string {
	if bucket == nil {
		return column
	}
	refilled := agentRateLimitRefillExpression(kind, column, bucket, now)
	if actual == nil {
		return refilled
	}
	adjustment := reserved - float64(*actual)
	if adjustment <= 0 {
		return `(` + refilled + ` + ` + rateLimitNumber(adjustment) + `)`
	}
	maximum := rateLimitNumber(float64(bucket.MaximumBalance))
	return `CASE WHEN (` + refilled + `) >= ` + maximum + ` THEN (` + refilled + `) ELSE ` + agentRateLimitLeast(kind, maximum, `(`+refilled+` + `+rateLimitNumber(adjustment)+`)`) + ` END`
}

func agentRateLimitRefillExpression(kind config.DatabaseKind, column string, bucket *config.TokenBucket, now string) string {
	maximum := rateLimitNumber(float64(bucket.MaximumBalance))
	elapsed := agentRateLimitElapsedExpression(kind, now, agentRateLimitTargetColumn(kind, "accounted_at"))
	refilled := `(` + column + ` + (` + elapsed + ` * ` + rateLimitNumber(float64(bucket.RefillPerMinute)) + ` / 60.0))`
	return `CASE WHEN ` + column + ` >= ` + maximum + ` THEN ` + column + ` ELSE ` + agentRateLimitLeast(kind, maximum, refilled) + ` END`
}

func agentRateLimitElapsedExpression(kind config.DatabaseKind, now, accountedAt string) string {
	if kind == config.DatabaseKindPostgres {
		return `EXTRACT(EPOCH FROM (` + now + `::timestamptz - ` + accountedAt + `::timestamptz))`
	}
	return `((julianday(` + now + `) - julianday(` + accountedAt + `)) * 86400.0)`
}

func agentRateLimitTargetColumn(kind config.DatabaseKind, column string) string {
	if kind == config.DatabaseKindPostgres {
		return `gatehouse_workspace_agent_rate_buckets.` + column
	}
	return column
}

func agentRateLimitLeast(kind config.DatabaseKind, left, right string) string {
	if kind == config.DatabaseKindPostgres {
		return `LEAST(` + left + `, ` + right + `)`
	}
	return `MIN(` + left + `, ` + right + `)`
}

func rateLimitNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func rateLimitParameter(kind config.DatabaseKind, index int) string {
	if kind == config.DatabaseKindPostgres {
		return `$` + strconv.Itoa(index)
	}
	return `?` + strconv.Itoa(index)
}

func bucketMaximum(bucket *config.TokenBucket) float64 {
	if bucket == nil {
		return 0
	}
	return float64(bucket.MaximumBalance)
}

func refillCredit(tokens float64, accountedAt, now time.Time, bucket *config.TokenBucket) float64 {
	if bucket == nil || tokens >= float64(bucket.MaximumBalance) {
		return tokens
	}
	elapsed := now.Sub(accountedAt).Seconds()
	return math.Min(float64(bucket.MaximumBalance), tokens+elapsed*float64(bucket.RefillPerMinute)/60)
}

func debtWait(tokens float64, refillPerMinute int) time.Duration {
	if tokens >= 0 {
		return 0
	}
	milliseconds := math.Ceil((-tokens * 60_000) / float64(refillPerMinute))
	return time.Duration(milliseconds) * time.Millisecond
}

func maxDuration(left, right time.Duration) time.Duration {
	if right > left {
		return right
	}
	return left
}
