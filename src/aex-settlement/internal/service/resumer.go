package service

import (
	"context"
	"log/slog"
	"time"
)

// Resumer defaults. A settlement normally finishes within one request, so
// an execution still PENDING after the grace period was stranded by a crash,
// a store error or a timeout.
const (
	DefaultResumeInterval  = 30 * time.Second
	DefaultResumeGrace     = time.Minute
	DefaultPendingAlertAge = 15 * time.Minute
	defaultResumeBatchSize = 100
)

// ResumerConfig configures a Resumer. Zero values take the defaults.
type ResumerConfig struct {
	// Interval is how often the resumer scans for stranded executions.
	Interval time.Duration
	// Grace is how long an execution must have been PENDING before the
	// resumer picks it up, so it rarely races an in-flight request.
	Grace time.Duration
	// AlertAge is how long an execution may stay PENDING before every
	// failed resume attempt is logged at ERROR.
	AlertAge time.Duration
	// BatchSize caps how many executions one scan resumes.
	BatchSize int
}

func (c ResumerConfig) withDefaults() ResumerConfig {
	if c.Interval <= 0 {
		c.Interval = DefaultResumeInterval
	}
	if c.Grace <= 0 {
		c.Grace = DefaultResumeGrace
	}
	if c.AlertAge <= 0 {
		c.AlertAge = DefaultPendingAlertAge
	}
	if c.BatchSize <= 0 {
		c.BatchSize = defaultResumeBatchSize
	}
	return c
}

// Resumer drives PENDING executions that no request finished to SETTLED. It
// runs the same idempotent steps as the request path (settlePending), so it
// is safe to run while requests retry the same contract and on every
// replica at once.
type Resumer struct {
	svc *Service
	cfg ResumerConfig
}

// NewResumer returns a Resumer for svc.
func NewResumer(svc *Service, cfg ResumerConfig) *Resumer {
	return &Resumer{svc: svc, cfg: cfg.withDefaults()}
}

// Run scans once immediately, then every Interval, until ctx is done.
func (r *Resumer) Run(ctx context.Context) {
	slog.InfoContext(ctx, "settlement resumer started",
		"interval", r.cfg.Interval.String(),
		"grace", r.cfg.Grace.String(),
		"alert_age", r.cfg.AlertAge.String(),
	)
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	for {
		r.ResumeOnce(ctx, time.Now().UTC())
		select {
		case <-ctx.Done():
			slog.Info("settlement resumer stopped")
			return
		case <-ticker.C:
		}
	}
}

// ResumeOnce settles the PENDING executions that have been pending since
// before now minus the grace period and returns how many it settled.
// Executions too old to replay safely are left out of the scan, so they
// can never fill the batch and starve newer ones; they are reported
// separately for manual reconciliation.
func (r *Resumer) ResumeOnce(ctx context.Context, now time.Time) int {
	replayFloor := now.Add(-r.svc.store.BalanceOpRetention() / 2)
	r.reportTooOld(ctx, now, replayFloor)

	pending, err := r.svc.store.ListPendingExecutions(ctx, replayFloor, now.Add(-r.cfg.Grace), r.cfg.BatchSize)
	if err != nil {
		if ctx.Err() == nil {
			slog.ErrorContext(ctx, "list pending settlements failed", "error", err)
		}
		return 0
	}

	settled := 0
	for _, execution := range pending {
		if ctx.Err() != nil {
			break
		}
		age := now.Sub(*execution.PendingSince)
		if age > r.cfg.AlertAge {
			slog.ErrorContext(ctx, "settlement_pending_overdue",
				"execution_id", execution.ID,
				"contract_id", execution.ContractID,
				"consumer_id", execution.ConsumerID,
				"provider_id", execution.ProviderID,
				"pending_for", age.String(),
			)
		}

		execCtx, cancel := context.WithTimeout(ctx, r.svc.settleTimeout)
		_, err := r.svc.settlePending(execCtx, execution)
		cancel()
		if err != nil {
			level := slog.LevelWarn
			if age > r.cfg.AlertAge {
				level = slog.LevelError
			}
			slog.Log(ctx, level, "resume pending settlement failed",
				"execution_id", execution.ID,
				"contract_id", execution.ContractID,
				"pending_for", age.String(),
				"error", err,
			)
			continue
		}
		settled++
		slog.InfoContext(ctx, "pending settlement resumed",
			"execution_id", execution.ID,
			"contract_id", execution.ContractID,
			"pending_for", age.String(),
		)
	}
	return settled
}

// reportTooOld logs, at ERROR, PENDING executions that are past the replay
// limit. They are never resumed automatically and need an operator.
func (r *Resumer) reportTooOld(ctx context.Context, now, replayFloor time.Time) {
	stuck, err := r.svc.store.ListPendingExecutions(ctx, time.Time{}, replayFloor, r.cfg.BatchSize)
	if err != nil {
		if ctx.Err() == nil {
			slog.ErrorContext(ctx, "list too-old pending settlements failed", "error", err)
		}
		return
	}
	for _, execution := range stuck {
		slog.ErrorContext(ctx, "settlement_pending_too_old_to_replay",
			"execution_id", execution.ID,
			"contract_id", execution.ContractID,
			"consumer_id", execution.ConsumerID,
			"provider_id", execution.ProviderID,
			"pending_for", now.Sub(*execution.PendingSince).String(),
		)
	}
}
