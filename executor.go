package storage

// Executor represents the database connection abstraction. It must remain compatible with
// sql.DB, sql.Tx, mocks, and WASM drivers.
type Executor interface {
	// Exec runs a statement that writes (insert, update, delete, DDL) and reports how many rows it changed. A unique-key violation is returned
	// as storage.Duplicate(cause), so a caller tells it apart with IsDuplicate without knowing the driver.
	Exec(query string, args ...any) (Result, error)
	QueryRow(query string, args ...any) Scanner
	Query(query string, args ...any) (Rows, error)
	Close() error
}

// Result is what a statement that writes reports.
type Result interface {
	// RowsAffected is how many rows the statement inserted, updated or deleted; 0 for a statement that changes no row (DDL, an update that matched nothing).
	RowsAffected() int64
}

// Affected is the Result of a statement that changed n rows. Every driver returns it, so none defines its own result type.
//
//	return storage.Affected(res.RowsAffected()), nil
type Affected int64

// RowsAffected returns n.
func (a Affected) RowsAffected() int64 { return int64(a) }

// Scanner represents a single row scanner.
type Scanner interface {
	Scan(dest ...any) error
}

// Rows represents an iterator over query results.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Columns() ([]string, error)
	Close() error
	Err() error
}
