package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/store"
)

// faultStore wraps the memory store and injects failures.
type faultStore struct {
	*store.MemoryStore
	lookupErr     error
	failCredits   atomic.Int32 // fail this many provider credits
	failMarks     atomic.Int32 // fail this many MarkExecutionSettled calls
	transitions   atomic.Int32 // PENDING -> SETTLED transitions made
	insertBarrier chan struct{}
}

func (f *faultStore) GetExecutionByContract(ctx context.Context, contractID string) (model.Execution, error) {
	if f.lookupErr != nil {
		return model.Execution{}, f.lookupErr
	}
	return f.MemoryStore.GetExecutionByContract(ctx, contractID)
}

func (f *faultStore) InsertExecution(ctx context.Context, execution model.Execution) error {
	if f.insertBarrier != nil {
		<-f.insertBarrier
	}
	return f.MemoryStore.InsertExecution(ctx, execution)
}

func (f *faultStore) ApplyBalanceOp(ctx context.Context, op store.BalanceOp) (int64, error) {
	if op.DeltaCents > 0 && f.failCredits.Add(-1) >= 0 {
		return 0, errors.New("injected credit failure")
	}
	return f.MemoryStore.ApplyBalanceOp(ctx, op)
}

func (f *faultStore) MarkExecutionSettled(ctx context.Context, executionID string, settledAt time.Time) (bool, error) {
	if f.failMarks.Add(-1) >= 0 {
		return false, errors.New("injected mark failure")
	}
	transitioned, err := f.MemoryStore.MarkExecutionSettled(ctx, executionID, settledAt)
	if transitioned {
		f.transitions.Add(1)
	}
	return transitioned, err
}

func newTestService(t *testing.T, st store.SettlementStore) *Service {
	t.Helper()
	// No payment provider agents or AP2 in unit tests.
	t.Setenv("PAYMENT_PROVIDER_URLS", "[]")
	t.Setenv("AP2_ENABLED", "false")
	return New(st, nil)
}

func completionEvent(contractID string) model.ContractCompletedEvent {
	now := time.Now().UTC()
	return model.ContractCompletedEvent{
		ContractID:  contractID,
		WorkID:      "work_1",
		AgentID:     "agent_1",
		ConsumerID:  "consumer",
		ProviderID:  "provider",
		Domain:      "general",
		StartedAt:   now.Add(-time.Minute),
		CompletedAt: now,
		Success:     true,
		AgreedPrice: "100.00",
	}
}

func fund(t *testing.T, st store.SettlementStore, tenantID string, cents int64) {
	t.Helper()
	if _, err := st.IncrementBalance(context.Background(), tenantID, cents, "USD"); err != nil {
		t.Fatal(err)
	}
}

func balanceOf(t *testing.T, st store.SettlementStore, tenantID string) int64 {
	t.Helper()
	bal, err := st.GetBalance(context.Background(), tenantID)
	if err != nil {
		t.Fatal(err)
	}
	return bal.Balance
}

func TestProcessContractCompletionSettles(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	res, err := svc.ProcessContractCompletion(ctx, completionEvent("c1"))
	if err != nil {
		t.Fatalf("ProcessContractCompletion: %v", err)
	}
	if res.Status != "settled" || !res.Charged || res.ExecutionID == "" {
		t.Errorf("result = %+v, want settled and charged", res)
	}
	if got := balanceOf(t, st, "consumer"); got != 40000 {
		t.Errorf("consumer balance = %d, want 40000", got)
	}
	if got := balanceOf(t, st, "provider"); got != 8500 {
		t.Errorf("provider balance = %d, want 8500", got)
	}
	exec, _ := st.GetExecutionByContract(ctx, "c1")
	if exec.SettlementStatus != model.SettlementSettled {
		t.Errorf("settlement status = %q, want SETTLED", exec.SettlementStatus)
	}

	// A repeat is a duplicate and moves no money.
	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); !errors.Is(err, ErrExecutionExists) {
		t.Fatalf("repeat: got %v, want ErrExecutionExists", err)
	}
	if got := balanceOf(t, st, "consumer"); got != 40000 {
		t.Errorf("consumer balance after repeat = %d, want 40000", got)
	}
}

func TestProcessContractCompletionConcurrentSettlesOnce(t *testing.T) {
	ctx := context.Background()
	st := &faultStore{MemoryStore: store.NewMemoryStore(), insertBarrier: make(chan struct{})}
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	const n = 10
	var wg sync.WaitGroup
	var ok, dup atomic.Int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.ProcessContractCompletion(ctx, completionEvent("c1"))
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrExecutionExists):
				dup.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	// Release every request into InsertExecution at once, after all of them
	// have passed the existence lookup.
	time.Sleep(50 * time.Millisecond)
	close(st.insertBarrier)
	wg.Wait()

	if ok.Load() < 1 || ok.Load()+dup.Load() != n {
		t.Errorf("ok=%d dup=%d, want at least one success and the rest duplicates", ok.Load(), dup.Load())
	}
	if got := balanceOf(t, st, "consumer"); got != 40000 {
		t.Errorf("consumer balance = %d, want 40000 (debited once)", got)
	}
	if got := balanceOf(t, st, "provider"); got != 8500 {
		t.Errorf("provider balance = %d, want 8500 (credited once)", got)
	}
	consumerEntries, _ := st.GetLedgerEntries(ctx, "consumer", 0)
	debits := 0
	for _, e := range consumerEntries {
		if e.EntryType == "DEBIT" {
			debits++
		}
	}
	if debits != 1 {
		t.Errorf("consumer has %d DEBIT ledger entries, want 1", debits)
	}
}

func TestProcessContractCompletionLookupErrorDoesNotSettle(t *testing.T) {
	ctx := context.Background()
	st := &faultStore{MemoryStore: store.NewMemoryStore(), lookupErr: errors.New("mongo timeout")}
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	_, err := svc.ProcessContractCompletion(ctx, completionEvent("c1"))
	if err == nil || errors.Is(err, ErrExecutionExists) {
		t.Fatalf("got %v, want an internal error", err)
	}
	var vErr *ValidationError
	if errors.As(err, &vErr) {
		t.Fatalf("got validation error %v, want an internal error", err)
	}
	if _, err := st.MemoryStore.GetExecutionByContract(ctx, "c1"); !errors.Is(err, store.ErrExecutionNotFound) {
		t.Errorf("execution recorded despite lookup failure")
	}
	if got := balanceOf(t, st, "consumer"); got != 50000 {
		t.Errorf("consumer balance = %d, want 50000 (untouched)", got)
	}
}

func TestProcessContractCompletionRetryAfterBalanceFailure(t *testing.T) {
	ctx := context.Background()
	st := &faultStore{MemoryStore: store.NewMemoryStore()}
	st.failCredits.Store(1)
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	// The consumer debit succeeds, the provider credit fails.
	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); err == nil || errors.Is(err, ErrExecutionExists) {
		t.Fatalf("first attempt: got %v, want an internal error", err)
	}
	exec, err := st.GetExecutionByContract(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if exec.SettlementStatus != model.SettlementPending {
		t.Fatalf("settlement status = %q, want PENDING", exec.SettlementStatus)
	}

	// The retry must finish the settlement, not answer 409.
	res, err := svc.ProcessContractCompletion(ctx, completionEvent("c1"))
	if err != nil {
		t.Fatalf("retry: got %v, want the settlement to complete", err)
	}
	if res.Status != "settled" || res.ExecutionID != exec.ID {
		t.Errorf("retry result = %+v, want settled for %s", res, exec.ID)
	}
	if got := balanceOf(t, st, "consumer"); got != 40000 {
		t.Errorf("consumer balance = %d, want 40000 (debited once)", got)
	}
	if got := balanceOf(t, st, "provider"); got != 8500 {
		t.Errorf("provider balance = %d, want 8500", got)
	}
	entries, _ := st.GetLedgerEntries(ctx, "consumer", 0)
	if len(entries) != 1 {
		t.Errorf("consumer ledger has %d entries, want 1", len(entries))
	}

	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); !errors.Is(err, ErrExecutionExists) {
		t.Errorf("after settling: got %v, want ErrExecutionExists", err)
	}
}

func TestProcessContractCompletionRetryAfterMarkFailure(t *testing.T) {
	ctx := context.Background()
	st := &faultStore{MemoryStore: store.NewMemoryStore()}
	st.failMarks.Store(1)
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); err == nil {
		t.Fatal("first attempt: want an error")
	}
	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := balanceOf(t, st, "consumer"); got != 40000 {
		t.Errorf("consumer balance = %d, want 40000 (debited once)", got)
	}
	if got := balanceOf(t, st, "provider"); got != 8500 {
		t.Errorf("provider balance = %d, want 8500 (credited once)", got)
	}
}

func TestProcessContractCompletionRejectsInvalidParties(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*model.ContractCompletedEvent)
		wantCode string
	}{
		{"empty consumer", func(e *model.ContractCompletedEvent) { e.ConsumerID = "" }, "INVALID_CONSUMER_ID"},
		{"unknown consumer", func(e *model.ContractCompletedEvent) { e.ConsumerID = "unknown" }, "INVALID_CONSUMER_ID"},
		{"empty provider", func(e *model.ContractCompletedEvent) { e.ProviderID = "" }, "PROVIDER_ID_REQUIRED"},
		{"empty contract", func(e *model.ContractCompletedEvent) { e.ContractID = "" }, "CONTRACT_ID_REQUIRED"},
		{"bad price", func(e *model.ContractCompletedEvent) { e.AgreedPrice = "abc" }, "INVALID_AGREED_PRICE"},
		{"negative price", func(e *model.ContractCompletedEvent) { e.AgreedPrice = "-5" }, "INVALID_AGREED_PRICE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := store.NewMemoryStore()
			fund(t, st, "consumer", 50000)
			svc := newTestService(t, st)

			event := completionEvent("c1")
			tt.mutate(&event)
			_, err := svc.ProcessContractCompletion(context.Background(), event)
			var vErr *ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("got %v, want ValidationError", err)
			}
			if vErr.Code != tt.wantCode {
				t.Errorf("code = %s, want %s", vErr.Code, tt.wantCode)
			}
			if got := balanceOf(t, st, "consumer"); got != 50000 {
				t.Errorf("consumer balance = %d, want 50000", got)
			}
		})
	}
}

func TestProcessContractCompletionUnsuccessfulMovesNoMoney(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	event := completionEvent("c1")
	event.Success = false
	res, err := svc.ProcessContractCompletion(ctx, event)
	if err != nil {
		t.Fatalf("ProcessContractCompletion: %v", err)
	}
	if res.Status != "recorded" || res.Charged {
		t.Errorf("result = %+v, want recorded and not charged", res)
	}
	if got := balanceOf(t, st, "consumer"); got != 50000 {
		t.Errorf("consumer balance = %d, want 50000", got)
	}
	if ok, _ := st.TenantExists(ctx, "provider"); ok {
		t.Error("provider balance account created for an unsuccessful contract")
	}
	exec, err := st.GetExecutionByContract(ctx, "c1")
	if err != nil {
		t.Fatalf("execution not recorded: %v", err)
	}
	if exec.Charged || exec.Status != "FAILED" || !exec.IsSettled() {
		t.Errorf("execution = %+v, want FAILED, settled, not charged", exec)
	}

	// Recorded once: a repeat is a duplicate.
	if _, err := svc.ProcessContractCompletion(ctx, event); !errors.Is(err, ErrExecutionExists) {
		t.Errorf("repeat: got %v, want ErrExecutionExists", err)
	}
}

func TestProcessContractCompletionUnfundedConsumer(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	svc := newTestService(t, st)

	_, err := svc.ProcessContractCompletion(ctx, completionEvent("c1"))
	if !errors.Is(err, ErrConsumerAccountNotFound) {
		t.Fatalf("got %v, want ErrConsumerAccountNotFound", err)
	}
	if ok, _ := st.TenantExists(ctx, "consumer"); ok {
		t.Error("consumer balance account was created")
	}
	if ok, _ := st.TenantExists(ctx, "provider"); ok {
		t.Error("provider was credited")
	}
	if _, err := st.GetExecutionByContract(ctx, "c1"); !errors.Is(err, store.ErrExecutionNotFound) {
		t.Error("execution recorded for an unfunded consumer")
	}

	// Once the consumer deposits, the same completion settles.
	if _, err := svc.ProcessDeposit(ctx, "consumer", "500.00"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); err != nil {
		t.Fatalf("after deposit: %v", err)
	}
	if got := balanceOf(t, st, "consumer"); got != 40000 {
		t.Errorf("consumer balance = %d, want 40000", got)
	}
}
