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
