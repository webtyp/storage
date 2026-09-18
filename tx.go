package storage

// TxBoundExecutor represents an executor bound to a transaction.
//
// Commit and Rollback must be safe to call after the transaction has already
// finished, one way or the other — the same contract database/sql's Tx makes,
// specifically so that `defer tx.Rollback()` right after BeginTx is always
// correct: it is a no-op when Commit already succeeded, and the actual
// rollback when it didn't. An implementation that panics or errors loudly on
// the post-Commit Rollback call breaks that idiom for every caller who writes
// it, and it is the idiom every caller should write.
type TxBoundExecutor interface {
	Executor
	Commit() error
	Rollback() error
}

// TxExecutor represents an executor that supports transactions. A Conn optionally implements
// this (type-assert it) to signal transaction support.
type TxExecutor interface {
	Executor
	BeginTx() (TxBoundExecutor, error)
}
