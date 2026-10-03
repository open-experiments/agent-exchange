package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
)

func TestMemoryInsertExecutionDuplicateContract(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	if err := st.InsertExecution(ctx, model.Execution{ID: "exec_1", ContractID: "c1"}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := st.InsertExecution(ctx, model.Execution{ID: "exec_2", ContractID: "c1"})
	if !errors.Is(err, ErrExecutionExists) {
		t.Fatalf("second insert for same contract: got %v, want ErrExecutionExists", err)
	}

	got, err := st.GetExecutionByContract(ctx, "c1")
	if err != nil {
		t.Fatalf("GetExecutionByContract: %v", err)
	}
	if got.ID != "exec_1" {
		t.Errorf("execution ID = %s, want exec_1", got.ID)
	}
}

func TestMemoryInsertExecutionConcurrent(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	const n = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	inserted := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := st.InsertExecution(ctx, model.Execution{ID: "exec_" + string(rune('a'+i)), ContractID: "c1"})
			if err == nil {
				mu.Lock()
				inserted++
				mu.Unlock()
			} else if !errors.Is(err, ErrExecutionExists) {
				t.Errorf("insert: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if inserted != 1 {
		t.Fatalf("inserted %d executions for one contract, want 1", inserted)
	}
}

func TestMemoryGetExecutionByContractNotFound(t *testing.T) {
	_, err := NewMemoryStore().GetExecutionByContract(context.Background(), "missing")
	if !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("got %v, want ErrExecutionNotFound", err)
	}
}

func TestMemoryMarkExecutionSettled(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	exec := model.Execution{ID: "exec_1", ContractID: "c1", SettlementStatus: model.SettlementPending}
	if err := st.InsertExecution(ctx, exec); err != nil {
		t.Fatal(err)
	}
	transitioned, err := st.MarkExecutionSettled(ctx, "exec_1", time.Now())
	if err != nil || !transitioned {
		t.Fatalf("MarkExecutionSettled: %v %v, want a transition", transitioned, err)
	}
	got, _ := st.GetExecutionByContract(ctx, "c1")
	if !got.IsSettled() || got.SettledAt == nil {
		t.Errorf("execution not settled: %+v", got)
	}
	if again, err := st.MarkExecutionSettled(ctx, "exec_1", time.Now()); err != nil || again {
		t.Errorf("second mark: %v %v, want no transition", again, err)
	}
	if _, err := st.MarkExecutionSettled(ctx, "missing", time.Now()); !errors.Is(err, ErrExecutionNotFound) {
		t.Errorf("missing execution: got %v, want ErrExecutionNotFound", err)
	}
}

func TestMemoryApplyBalanceOpIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	if _, err := st.IncrementBalance(ctx, "consumer", 1000, "USD"); err != nil {
		t.Fatal(err)
	}

	op := BalanceOp{ID: "c1:debit", TenantID: "consumer", DeltaCents: -300, Currency: "USD"}
	after, err := st.ApplyBalanceOp(ctx, op)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if after != 700 {
		t.Fatalf("balance after = %d, want 700", after)
	}

	// A deposit in between must not change what the replay reports.
	if _, err := st.IncrementBalance(ctx, "consumer", 50, "USD"); err != nil {
		t.Fatal(err)
	}
	again, err := st.ApplyBalanceOp(ctx, op)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if again != 700 {
		t.Errorf("replayed balance after = %d, want the recorded 700", again)
	}
	bal, _ := st.GetBalance(ctx, "consumer")
	if bal.Balance != 750 {
		t.Errorf("balance = %d, want 750 (op applied once)", bal.Balance)
	}
}

func TestMemoryApplyBalanceOpMissingTenant(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	_, err := st.ApplyBalanceOp(ctx, BalanceOp{ID: "c1:debit", TenantID: "ghost", DeltaCents: -100, Currency: "USD"})
	if !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("debit of missing tenant: got %v, want ErrTenantNotFound", err)
	}
	if ok, _ := st.TenantExists(ctx, "ghost"); ok {
		t.Fatal("failed debit created the tenant")
	}

	after, err := st.ApplyBalanceOp(ctx, BalanceOp{ID: "c1:credit", TenantID: "provider", DeltaCents: 85, Currency: "USD", CreateIfMissing: true})
	if err != nil {
		t.Fatalf("credit with CreateIfMissing: %v", err)
	}
	if after != 85 {
		t.Errorf("provider balance after = %d, want 85", after)
	}
}

func TestMemoryAppendLedgerEntryDuplicate(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	entry := model.LedgerEntry{ID: "ledger_c1:debit", TenantID: "consumer", Amount: 100}
	if err := st.AppendLedgerEntry(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendLedgerEntry(ctx, entry); !errors.Is(err, ErrLedgerEntryExists) {
		t.Fatalf("duplicate entry: got %v, want ErrLedgerEntryExists", err)
	}
	entries, _ := st.GetLedgerEntries(ctx, "consumer", 0)
	if len(entries) != 1 {
		t.Errorf("ledger has %d entries, want 1", len(entries))
	}
}

func TestMemoryApplyBalanceOpRemembersOpsRegardlessOfCount(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	if _, err := st.IncrementBalance(ctx, "consumer", 1_000_000, "USD"); err != nil {
		t.Fatal(err)
	}
	pending := BalanceOp{ID: "c0:debit", TenantID: "consumer", DeltaCents: -100, Currency: "USD"}
	if _, err := st.ApplyBalanceOp(ctx, pending); err != nil {
		t.Fatal(err)
	}
	// Far more newer ops than any count-based window would keep.
	for i := 1; i <= 2500; i++ {
		op := BalanceOp{ID: fmt.Sprintf("c%d:debit", i), TenantID: "consumer", DeltaCents: -1, Currency: "USD"}
		if _, err := st.ApplyBalanceOp(ctx, op); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.ApplyBalanceOp(ctx, pending); err != nil {
		t.Fatal(err)
	}
	bal, _ := st.GetBalance(ctx, "consumer")
	if want := int64(1_000_000 - 100 - 2500); bal.Balance != want {
		t.Errorf("balance = %d, want %d (replayed op applied again)", bal.Balance, want)
	}
}

func TestMemoryApplyBalanceOpTrimsByAge(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	st.SetBalanceOpRetention(20 * time.Millisecond)
	op := func(id string) BalanceOp {
		return BalanceOp{ID: id, TenantID: "provider", DeltaCents: 10, Currency: "USD", CreateIfMissing: true}
	}
	if _, err := st.ApplyBalanceOp(ctx, op("old")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := st.ApplyBalanceOp(ctx, op("new")); err != nil {
		t.Fatal(err)
	}
	bal, _ := st.GetBalance(ctx, "provider")
	if len(bal.SettlementOps) != 1 || bal.SettlementOps[0].OpID != "new" {
		t.Errorf("settlement ops = %+v, want only the op inside the retention window", bal.SettlementOps)
	}
}

func TestMemoryReplaceUnchargedFailure(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	failed := model.Execution{ID: "exec_1", ContractID: "c1", Status: "FAILED", SettlementStatus: model.SettlementSettled}
	if err := st.InsertExecution(ctx, failed); err != nil {
		t.Fatal(err)
	}
	upgraded := model.Execution{ID: "exec_1", ContractID: "c1", Status: "COMPLETED", Success: true, Charged: true, SettlementStatus: model.SettlementPending}
	if err := st.ReplaceUnchargedFailure(ctx, upgraded); err != nil {
		t.Fatalf("first upgrade: %v", err)
	}
	if err := st.ReplaceUnchargedFailure(ctx, upgraded); !errors.Is(err, ErrExecutionExists) {
		t.Errorf("second upgrade: got %v, want ErrExecutionExists", err)
	}

	// A legacy execution (no settlement_status, charged unset) is never upgradeable.
	legacy := model.Execution{ID: "exec_2", ContractID: "c2", Status: "COMPLETED", Success: true}
	if err := st.InsertExecution(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	legacyUpgrade := upgraded
	legacyUpgrade.ID, legacyUpgrade.ContractID = "exec_2", "c2"
	if err := st.ReplaceUnchargedFailure(ctx, legacyUpgrade); !errors.Is(err, ErrExecutionExists) {
		t.Errorf("legacy upgrade: got %v, want ErrExecutionExists", err)
	}
	if err := st.ReplaceUnchargedFailure(ctx, model.Execution{ID: "missing", ContractID: "c9"}); !errors.Is(err, ErrExecutionNotFound) {
		t.Errorf("missing: got %v, want ErrExecutionNotFound", err)
	}
}

func TestMemoryRecordExecutionPaymentFirstWins(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	if err := st.InsertExecution(ctx, model.Execution{ID: "exec_1", ContractID: "c1"}); err != nil {
		t.Fatal(err)
	}
	first, err := st.RecordExecutionPayment(ctx, "exec_1", model.PaymentDetails{PaymentProviderID: "p1"})
	if err != nil || !first.PaymentRecorded || first.PaymentProviderID != "p1" {
		t.Fatalf("first record: %+v %v", first, err)
	}
	second, err := st.RecordExecutionPayment(ctx, "exec_1", model.PaymentDetails{PaymentProviderID: "p2"})
	if err != nil || second.PaymentProviderID != "p1" {
		t.Errorf("second record: %+v %v, want the first outcome kept", second, err)
	}
}

func TestMemoryListPendingExecutions(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	now := time.Now().UTC()
	at := func(d time.Duration) *time.Time { t := now.Add(d); return &t }
	for _, e := range []model.Execution{
		{ID: "e_new", ContractID: "c1", SettlementStatus: model.SettlementPending, PendingSince: at(-10 * time.Second)},
		{ID: "e_old", ContractID: "c2", SettlementStatus: model.SettlementPending, PendingSince: at(-10 * time.Minute)},
		{ID: "e_mid", ContractID: "c3", SettlementStatus: model.SettlementPending, PendingSince: at(-5 * time.Minute)},
		{ID: "e_done", ContractID: "c4", SettlementStatus: model.SettlementSettled, PendingSince: at(-time.Hour)},
	} {
		if err := st.InsertExecution(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.ListPendingExecutions(ctx, time.Time{}, now.Add(-time.Minute), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "e_old" || got[1].ID != "e_mid" {
		t.Errorf("pending = %v, want e_old then e_mid", got)
	}
	if limited, _ := st.ListPendingExecutions(ctx, time.Time{}, now, 1); len(limited) != 1 || limited[0].ID != "e_old" {
		t.Errorf("limited = %v, want only e_old", limited)
	}
}
