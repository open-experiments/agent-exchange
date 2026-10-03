package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
	"github.com/parlakisik/agent-exchange/aex-settlement/internal/store"
	"github.com/shopspring/decimal"
)

// ledgerCounts returns how many DEBIT and CREDIT ledger entries tenantID has.
func ledgerCounts(t *testing.T, st store.SettlementStore, tenantID string) (debits, credits int) {
	t.Helper()
	entries, err := st.GetLedgerEntries(context.Background(), tenantID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		switch e.EntryType {
		case "DEBIT":
			debits++
		case "CREDIT":
			credits++
		}
	}
	return debits, credits
}

// assertSettledOnce checks the c1 settlement of completionEvent moved the
// money exactly once.
func assertSettledOnce(t *testing.T, st *faultStore) {
	t.Helper()
	if got := balanceOf(t, st, "consumer"); got != 40000 {
		t.Errorf("consumer balance = %d, want 40000 (debited once)", got)
	}
	if got := balanceOf(t, st, "provider"); got != 8500 {
		t.Errorf("provider balance = %d, want 8500 (credited once)", got)
	}
	if d, _ := ledgerCounts(t, st, "consumer"); d != 1 {
		t.Errorf("consumer has %d DEBIT entries, want 1", d)
	}
	if _, c := ledgerCounts(t, st, "provider"); c != 1 {
		t.Errorf("provider has %d CREDIT entries, want 1", c)
	}
	if got := st.transitions.Load(); got != 1 {
		t.Errorf("%d PENDING->SETTLED transitions, want 1", got)
	}
}

// strandAfterDebit records c1 as PENDING with the consumer debited and the
// provider not yet credited, as if the request had died mid-settlement.
func strandAfterDebit(t *testing.T, svc *Service, st *faultStore) model.Execution {
	t.Helper()
	st.failCredits.Store(1)
	if _, err := svc.ProcessContractCompletion(context.Background(), completionEvent("c1")); err == nil {
		t.Fatal("stranding attempt: want an error")
	}
	exec, err := st.GetExecutionByContract(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if exec.SettlementStatus != model.SettlementPending || exec.PendingSince == nil {
		t.Fatalf("execution = %+v, want PENDING with pending_since", exec)
	}
	if got := balanceOf(t, st, "consumer"); got != 40000 {
		t.Fatalf("consumer balance = %d, want 40000 (debit applied before the failure)", got)
	}
	if got := balanceOf(t, st, "provider"); got != 0 {
		t.Fatalf("provider balance = %d, want 0 (credit failed)", got)
	}
	return exec
}

func TestResumerCompletesStrandedSettlement(t *testing.T) {
	ctx := context.Background()
	st := &faultStore{MemoryStore: store.NewMemoryStore()}
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)
	exec := strandAfterDebit(t, svc, st)

	r := NewResumer(svc, ResumerConfig{Grace: time.Minute})

	// Still inside the grace period: left to the request path.
	if n := r.ResumeOnce(ctx, time.Now().UTC()); n != 0 {
		t.Fatalf("resumed %d executions inside the grace period, want 0", n)
	}

	later := exec.PendingSince.Add(2 * time.Minute)
	if n := r.ResumeOnce(ctx, later); n != 1 {
		t.Fatalf("resumed %d executions, want 1", n)
	}
	got, _ := st.GetExecutionByContract(ctx, "c1")
	if got.SettlementStatus != model.SettlementSettled || got.SettledAt == nil {
		t.Errorf("execution = %+v, want SETTLED", got)
	}
	assertSettledOnce(t, st)

	// Nothing left to resume, and the contract now answers 409.
	if n := r.ResumeOnce(ctx, later); n != 0 {
		t.Errorf("second scan resumed %d executions, want 0", n)
	}
	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); !errors.Is(err, ErrExecutionExists) {
		t.Errorf("after resume: got %v, want ErrExecutionExists", err)
	}
	assertSettledOnce(t, st)
}

func TestResumerConcurrentWithRequestsSettlesOnce(t *testing.T) {
	for run := 0; run < 20; run++ {
		ctx := context.Background()
		st := &faultStore{MemoryStore: store.NewMemoryStore()}
		fund(t, st, "consumer", 50000)
		svc := newTestService(t, st)
		exec := strandAfterDebit(t, svc, st)
		later := exec.PendingSince.Add(2 * time.Minute)

		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 4; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				NewResumer(svc, ResumerConfig{}).ResumeOnce(ctx, later)
			}()
			go func() {
				defer wg.Done()
				<-start
				_, err := svc.ProcessContractCompletion(ctx, completionEvent("c1"))
				if err != nil && !errors.Is(err, ErrExecutionExists) {
					t.Errorf("request: %v", err)
				}
			}()
		}
		close(start)
		wg.Wait()

		got, _ := st.GetExecutionByContract(ctx, "c1")
		if got.SettlementStatus != model.SettlementSettled {
			t.Fatalf("execution = %+v, want SETTLED", got)
		}
		assertSettledOnce(t, st)
		if t.Failed() {
			return
		}
	}
}

func TestResumerRefusesTooOldPending(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	mem.SetBalanceOpRetention(2 * time.Hour)
	st := &faultStore{MemoryStore: mem}
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	// PENDING for longer than half the op retention: its ops may be
	// forgotten by the time it is replayed, so it must not be replayed.
	pendingSince := time.Now().UTC().Add(-90 * time.Minute)
	exec := svc.newExecution(completionEvent("c1"), mustDecimal(t, "100.00"), "exec_old", pendingSince)
	exec.PendingSince = &pendingSince
	if err := st.InsertExecution(ctx, exec); err != nil {
		t.Fatal(err)
	}

	if n := NewResumer(svc, ResumerConfig{}).ResumeOnce(ctx, time.Now().UTC()); n != 0 {
		t.Errorf("resumed %d executions, want 0", n)
	}
	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); !errors.Is(err, ErrSettlementTooOld) {
		t.Errorf("request: got %v, want ErrSettlementTooOld", err)
	}
	if got := balanceOf(t, st, "consumer"); got != 50000 {
		t.Errorf("consumer balance = %d, want 50000 (untouched)", got)
	}
}

func TestResumerTooOldPendingDoesNotStarveNewer(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	mem.SetBalanceOpRetention(2 * time.Hour)
	st := &faultStore{MemoryStore: mem}
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	// A full batch of executions that can never be replayed sits at the
	// front of the oldest-first order.
	old := time.Now().UTC().Add(-90 * time.Minute)
	for _, id := range []string{"old1", "old2"} {
		exec := svc.newExecution(completionEvent(id), mustDecimal(t, "100.00"), "exec_"+id, old)
		exec.PendingSince = &old
		if err := st.InsertExecution(ctx, exec); err != nil {
			t.Fatal(err)
		}
	}
	exec := strandAfterDebit(t, svc, st)

	r := NewResumer(svc, ResumerConfig{BatchSize: 2})
	if n := r.ResumeOnce(ctx, exec.PendingSince.Add(2*time.Minute)); n != 1 {
		t.Fatalf("resumed %d executions, want 1 (the replayable one)", n)
	}
	assertSettledOnce(t, st)
}

func TestProcessContractCompletionSurvivesCallerCancellation(t *testing.T) {
	st := &faultStore{MemoryStore: store.NewMemoryStore()}
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := svc.ProcessContractCompletion(ctx, completionEvent("c1"))
	if err != nil {
		t.Fatalf("got %v, want the settlement to complete despite the cancelled caller", err)
	}
	if res.Status != "settled" {
		t.Errorf("result = %+v, want settled", res)
	}
	assertSettledOnce(t, st)
}

func TestProcessContractCompletionFailedThenSuccessSettlesOnce(t *testing.T) {
	ctx := context.Background()
	st := &faultStore{MemoryStore: store.NewMemoryStore()}
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	failed := completionEvent("c1")
	failed.Success = false
	first, err := svc.ProcessContractCompletion(ctx, failed)
	if err != nil || first.Status != "recorded" {
		t.Fatalf("failure: %+v %v, want recorded", first, err)
	}

	// A repeated failure is a duplicate.
	if _, err := svc.ProcessContractCompletion(ctx, failed); !errors.Is(err, ErrExecutionExists) {
		t.Fatalf("repeated failure: got %v, want ErrExecutionExists", err)
	}

	// Concurrent successes upgrade the failure exactly once.
	const n = 8
	var wg sync.WaitGroup
	var ok atomic.Int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := svc.ProcessContractCompletion(ctx, completionEvent("c1"))
			switch {
			case err == nil:
				ok.Add(1)
				if res.Status != "settled" || res.ExecutionID != first.ExecutionID {
					t.Errorf("result = %+v, want settled for %s", res, first.ExecutionID)
				}
			case errors.Is(err, ErrExecutionExists):
			default:
				t.Errorf("success: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() < 1 {
		t.Fatal("no success settled the upgraded execution")
	}

	exec, _ := st.GetExecutionByContract(ctx, "c1")
	if exec.Status != "COMPLETED" || !exec.Charged || exec.SettlementStatus != model.SettlementSettled {
		t.Errorf("execution = %+v, want COMPLETED, charged, SETTLED", exec)
	}
	assertSettledOnce(t, st)

	for _, success := range []bool{true, false} {
		event := completionEvent("c1")
		event.Success = success
		if _, err := svc.ProcessContractCompletion(ctx, event); !errors.Is(err, ErrExecutionExists) {
			t.Errorf("after settling, success=%v: got %v, want ErrExecutionExists", success, err)
		}
	}
	assertSettledOnce(t, st)
}

func TestProcessContractCompletionFailedThenSuccessUnfundedConsumer(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	svc := newTestService(t, st)

	failed := completionEvent("c1")
	failed.Success = false
	if _, err := svc.ProcessContractCompletion(ctx, failed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); !errors.Is(err, ErrConsumerAccountNotFound) {
		t.Fatalf("got %v, want ErrConsumerAccountNotFound", err)
	}
	exec, _ := st.GetExecutionByContract(ctx, "c1")
	if !exec.IsUnchargedFailure() {
		t.Errorf("execution = %+v, want it left as an uncharged failure", exec)
	}
}

func TestProcessContractCompletionNormalizesIDs(t *testing.T) {
	ctx := context.Background()
	st := &faultStore{MemoryStore: store.NewMemoryStore()}
	fund(t, st, "consumer", 50000)
	svc := newTestService(t, st)

	event := completionEvent(" c1\t")
	event.ConsumerID = "  consumer "
	event.ProviderID = "\nprovider "
	if _, err := svc.ProcessContractCompletion(ctx, event); err != nil {
		t.Fatalf("ProcessContractCompletion: %v", err)
	}
	exec, err := st.GetExecutionByContract(ctx, "c1")
	if err != nil {
		t.Fatalf("execution not stored under the trimmed contract_id: %v", err)
	}
	if exec.ContractID != "c1" || exec.ConsumerID != "consumer" || exec.ProviderID != "provider" {
		t.Errorf("ids = %q %q %q, want trimmed", exec.ContractID, exec.ConsumerID, exec.ProviderID)
	}
	if ok, _ := st.TenantExists(ctx, " consumer "); ok {
		t.Error("balance account created for an untrimmed id")
	}
	assertSettledOnce(t, st)

	if _, err := svc.ProcessContractCompletion(ctx, completionEvent("c1")); !errors.Is(err, ErrExecutionExists) {
		t.Errorf("trimmed duplicate: got %v, want ErrExecutionExists", err)
	}
}

func TestPaymentStepNotRedoneOnResume(t *testing.T) {
	ctx := context.Background()
	var bidRequests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bidRequests.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	st := &faultStore{MemoryStore: store.NewMemoryStore()}
	fund(t, st, "consumer", 50000)
	t.Setenv("AP2_ENABLED", "false")
	providers, _ := json.Marshal([]map[string]string{{"id": "p1", "name": "P1", "endpoint": srv.URL}})
	t.Setenv("PAYMENT_PROVIDER_URLS", string(providers))
	svc := New(st, nil)

	exec := strandAfterDebit(t, svc, st)
	if bidRequests.Load() != 1 {
		t.Fatalf("bid requests = %d, want 1", bidRequests.Load())
	}
	if !exec.PaymentRecorded || exec.PaymentProviderID == "" {
		t.Fatalf("execution = %+v, want the payment outcome recorded before the money moved", exec)
	}

	if n := NewResumer(svc, ResumerConfig{}).ResumeOnce(ctx, exec.PendingSince.Add(time.Hour)); n != 1 {
		t.Fatalf("resumed %d executions, want 1", n)
	}
	if bidRequests.Load() != 1 {
		t.Errorf("bid requests = %d after resume, want 1 (payment step not redone)", bidRequests.Load())
	}
	got, _ := st.GetExecutionByContract(ctx, "c1")
	if got.PaymentProviderID != exec.PaymentProviderID {
		t.Errorf("payment provider = %q, want %q", got.PaymentProviderID, exec.PaymentProviderID)
	}
	assertSettledOnce(t, st)
}

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
