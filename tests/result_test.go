package tests

import (
	"testing"

	"webtyp.com/fmt"
	"webtyp.com/storage"
	"webtyp.com/storage/mock"
)

type wrappedError struct{ inner error }

func (w wrappedError) Error() string { return "wrapped: " + w.inner.Error() }
func (w wrappedError) Unwrap() error { return w.inner }

func TestResult_AffectedReportsItsCount(t *testing.T) {
	var r storage.Result = storage.Affected(3)
	if got := r.RowsAffected(); got != 3 {
		t.Fatalf("RowsAffected() = %d, want 3", got)
	}
}

func TestDuplicate_KeepsTheDriversMessage(t *testing.T) {
	cause := fmt.Err("driver says: duplicate key")
	if got := storage.Duplicate(cause).Error(); got != cause.Error() {
		t.Fatalf("Error() = %q, want %q", got, cause.Error())
	}
}

func TestDuplicate_IsRecognisedDirectlyAndWhenWrapped(t *testing.T) {
	dup := storage.Duplicate(fmt.Err("x"))
	if !storage.IsDuplicate(dup) {
		t.Error("IsDuplicate(Duplicate(err)) must be true")
	}
	if !storage.IsDuplicate(wrappedError{inner: dup}) {
		t.Error("IsDuplicate must see through a wrapper that has Unwrap")
	}
}

func TestDuplicate_OtherErrorsAreNotDuplicates(t *testing.T) {
	if storage.IsDuplicate(nil) {
		t.Error("IsDuplicate(nil) must be false")
	}
	if storage.IsDuplicate(fmt.Err("plain")) {
		t.Error("a plain error is not a duplicate")
	}
	if storage.IsDuplicate(wrappedError{inner: fmt.Err("plain")}) {
		t.Error("a wrapper around a plain error is not a duplicate")
	}
	if storage.IsDuplicate(storage.ErrNoRows) {
		t.Error("ErrNoRows is not a duplicate")
	}
}

func TestMockExecutor_ReportsZeroRowsUnlessTold(t *testing.T) {
	exec := &mock.Executor{}
	res, err := exec.Exec("INSERT")
	if err != nil || res.RowsAffected() != 0 {
		t.Fatalf("default: got (%v, %v), want (0 rows, nil)", res, err)
	}
	exec.ReturnExecResult = storage.Affected(2)
	res, _ = exec.Exec("UPDATE")
	if res.RowsAffected() != 2 {
		t.Fatalf("configured: got %d rows, want 2", res.RowsAffected())
	}
}
