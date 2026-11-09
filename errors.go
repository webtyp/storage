package storage

// noRows is the concrete type of ErrNoRows. IsNoRows recognises it with a type
// assertion: TinyGo compiles that to a type-code comparison, while == between
// two error values (and errors.Is) goes through runtime.interfaceEqual and pulls
// internal/reflectlite into the wasm binary.
type noRows struct{}

func (noRows) Error() string { return "no rows" }

// ErrNoRows is the agnostic sentinel for "query returned no rows". Conn implementations
// (postgres, sqlt) must map their driver-specific no-rows error to this value so callers can
// detect it without importing the std sql package. This is the raw contract sentinel; orm.QB.ReadOne
// translates it to the ergonomic orm.ErrNotFound — db itself never does that translation.
// Detect it with IsNoRows, never with == or errors.Is (both cost reflection under TinyGo).
var ErrNoRows error = noRows{}

// IsNoRows reports whether err is ErrNoRows.
func IsNoRows(err error) bool {
	_, ok := err.(noRows)
	return ok
}

// duplicate is the concrete type behind a unique-key violation. IsDuplicate recognises it with a type assertion, for the reason IsNoRows does.
type duplicate struct{ cause error }

func (d duplicate) Error() string { return d.cause.Error() }

// Unwrap returns the driver's own error, for logs and for a caller that needs its detail.
func (d duplicate) Unwrap() error { return d.cause }

// Duplicate marks cause as a unique-key violation (a primary key or a unique column written twice). Every Conn implementation maps its driver's
// error to it (postgres code 23505, sqlite's unique-constraint codes), so callers detect the violation without importing a driver.
//
//	return nil, storage.Duplicate(err)
func Duplicate(cause error) error { return duplicate{cause: cause} }

// IsDuplicate reports whether err is, or wraps, a unique-key violation made by Duplicate. Never use errors.Is for it (reflection under TinyGo).
func IsDuplicate(err error) bool {
	for err != nil {
		if _, ok := err.(duplicate); ok {
			return true
		}
		wrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = wrapped.Unwrap()
	}
	return false
}
