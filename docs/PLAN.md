---
PLAN: "feat: blob + transaction conformance clauses for vector storage"
TAG: v0.1.0
EXECUTOR: unassigned
REVIEWER: none
---

> Part of the browser-native semantic search effort. Master index:
> https://github.com/webtyp/agent/blob/main/docs/PLAN.md — decisions **D1** and **D2**
> there are the rationale for everything below.

# Plan — prove that a backend can carry a vector

## Why

Storing embeddings means storing `[]byte` and writing many rows atomically. This
repository already has **both contracts**; what it does not have is anything that
proves a backend honours them.

- `ScanAny` (`scan.go`) already handles `*[]byte` completely, in both the NULL and
  non-NULL paths. **No change needed there.**
- `TxExecutor` / `TxBoundExecutor` (`tx.go`) already describe transactions as an
  optional, type-asserted capability. **No change needed there either.**
- But `conformance.Widget` (`conformance/model.go`) has only text/int/bool columns, and
  `conformance.Run` has thirteen clauses, none of which writes a byte slice or opens a
  transaction.

The consequence is concrete: `indexdb` will **crash** — not error — on the first blob
write (`js.ValueOf` panics on `[]byte`, and TinyGo wasm has no `recover()`), and today
the conformance suite passes it anyway. A contract nothing tests is not a contract.

## What does NOT change

`scan.go`, `executor.go`, `conn.go`, `query.go`, `conditions.go`, `compiler.go`,
`execution_plan.go` and `tx.go` are **untouched**. This plan adds test surface and one
new conformance record; it changes no production interface.

In particular, **no streaming-scan API is added.** It would be the obvious thing to reach
for — a kNN query scans every candidate — but decision **D2** stores vectors in shards of
1024, so `vectordb`'s hot read path is a handful of large rows from one table, which
`Query`/`Rows` already serves. Adding a scan API for a caller that does not need one
would be speculative.

`conformance.Widget` is **not** extended. Adding a blob column to the canonical record
would break every backend's table setup at once, including backends outside this plan's
scope. The new clauses use their own record.

## Changes

### 1. `conformance/model.go` — a second canonical record

```go
// Embedding is the canonical record for the blob and transaction clauses. It is
// separate from Widget on purpose: a backend that cannot carry bytes fails only
// the blob clauses instead of every clause at once.
var EmbeddingModel = model.Definition{
	Name: "conformance_embedding",
	Fields: model.Fields{
		{Name: "id",     Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "vec",    Type: model.Vector(4), NotNull: true}, // 16 bytes
		{Name: "loose",  Type: model.Blob()},                   // nullable, no dimension
	},
}

type Embedding struct {
	Id    string
	Vec   []byte
	Loose []byte
}
```

with the usual `ModelName`/`Schema`/`Pointers`/`IsNil`/`EncodeFields`/`DecodeFields`
methods, matching `Widget`'s hand-written style (this package depends on `model`, not
on `ormc`).

`model.Vector(4)` requires `webtyp.com/model` at the tag produced by
https://github.com/webtyp/model/blob/main/docs/PLAN.md — bump it in `go.mod` first.

### 2. `conformance/conformance.go` — four new clauses

Registered in `Run` after the existing thirteen:

| Clause | Asserts |
|---|---|
| `blob_round_trips_byte_for_byte` | a 16-byte `vec` written and read back compares equal byte for byte, including `0x00` bytes in the middle and a `0xFF` terminator — the two values a string-based backend silently truncates |
| `blob_null_scans_as_nil` | `loose` left unset reads back as `nil`, not `[]byte{}` — the same rule `null_scans_as_zero` already enforces for scalars |
| `blob_updates_in_place` | overwriting `vec` with different bytes of the same length leaves no trace of the old value |
| `batch_insert_is_atomic` | **skipped unless the backend implements `TxExecutor`**: 64 rows created inside one transaction are all visible after `Commit`, and a `Rollback` after 64 creates leaves the table empty |

The transaction clause is `t.Skip`-ped with an explicit message when the type assertion
fails, so a backend without transaction support reports "skipped", never a false pass.

### 3. `mem/mem.go` — verify, then fix only if needed

`scanInto` delegates to `storage.ScanAny`, which handles `*[]byte`. `dbRow.set` stores
`any`. So `mem` is expected to pass the blob clauses **unmodified** — run them first and
only touch `mem` if they fail.

`mem` does not implement `TxExecutor` beyond the no-op `BeginTx`/`Commit`/`Rollback`
already on `engine` (`mem.go:80-85`). Those are stubs: `Rollback` discards nothing.
Either implement real snapshot semantics (copy the table slice on `BeginTx`, restore on
`Rollback`) or remove the stubs so the type assertion fails honestly and the clause
skips. **Remove or implement — do not leave a `Rollback` that lies.** Recommendation:
implement it; it is ~15 lines and `mem` is the reference backend.

### 4. `mock/recorders.go`

If the mock records calls per method, add the transaction methods so a test can assert
that a batch used one transaction rather than 64.

## Tests

`tests/mem_conformance_test.go` picks up the new clauses automatically through `Run`.
Add to `tests/mem_extra_test.go`:

| Test | Asserts |
|---|---|
| `TestScanAny_BlobFromString` | a backend handing back `string` for a BLOB column still scans into `*[]byte` (already supported — this pins it) |
| `TestScanAny_NilBlob` | NULL → `nil`, not an empty non-nil slice |
| `TestMem_RollbackDiscards` | only if §3 implements real transactions |

## Acceptance checklist

```bash
grep -n "EmbeddingModel" conformance/model.go       # → 1 match
grep -c "t.Run(\"blob_" conformance/conformance.go  # → 3
grep -n "batch_insert_is_atomic" conformance/conformance.go
go vet ./...
gotest
```

## Downstream note, not this repo's work

`sqlt` and `postgres` need a `FieldBlob` → `BLOB` / `BYTEA` DDL mapping before they pass
the new clauses. If they do not have one, the new clauses will surface it — which is the
point. File it against those repositories; do not widen this plan.

Release after the checklist passes; `indexdb` and `vectordb` both depend on the tag:

```bash
gopush 'feat: blob and transaction conformance clauses'
```
