---
PLAN: "feat: storage.ILike — a text match that ignores case, guaranteed by the conformance suite of every backend"
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work. Commit after every stage.
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.
> **Publishing:** you publish only by OPENING THE PULL REQUEST. Submit after Stage 1 and keep committing.
> **Never ask a yes/no question about progress.**
> **Before you submit:** `git status` must be clean and `git diff --stat main` must list the files of the stages.

# Plan: `storage.ILike`

## Why (read once)

Searching a name by text must find «Ana» when the person types «ana». `storage.Like` does not promise that: on Postgres `LIKE` is case-sensitive, on SQLite it ignores case for ASCII, and `storage/mem` ignores it (`likeMatch` lowercases both sides): the same query answers differently per backend, and `crud` works around it by lowercasing the search term, which only finds data that was stored in lowercase. Decision (owner, 2026-11-11, U6 of the usability master): add `ILike`, whose meaning is the same everywhere — it ignores case — and make the conformance suite fail any backend that does not honour it. `Like` stays as it is (its case behaviour depends on the backend: do not rely on it for text search).

## Ecosystem search

`likeMatch` of `storage/mem` already ignores case: reuse it. SQL backends (`postgres/translate.go:348`, `sqlt/translate.go:336`) and `indexdb/execute.go:661` handle their own operators in later plans; this plan owns only the contract, the in-memory backend and the conformance clause.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```
No new dependency, no `replace`. Read files by ranges (`sed -n 1,80p file`).

## Stage 1 — `conditions.go` and `mem/mem.go`

After `Like` in `conditions.go` add:

```go
// ILike creates a condition that matches a pattern ignoring upper and lower case ("%an%" finds "Ana" and "ana"). Every
// backend honours it (the conformance suite checks it); Like does not promise that.
//
//	storage.ILike("name", "%"+text+"%")
func ILike(field string, value any) Condition {
	return Condition{field: field, operator: "ILIKE", value: value, logic: "AND"}
}
```

In `mem/mem.go`, function `evalCond`, add next to the `"LIKE"` case:

```go
	case "ILIKE":
		return likeMatch(toStr(v), toStr(c.Value()))
```

## Stage 2 — conformance and tests

In `conformance/conformance.go` register the clause in `Run`, after `in_operator_filters`:

```go
	t.Run("ilike_ignores_case", func(t *testing.T) { ilikeIgnoresCase(t, f) })
```

and add the clause (same shape as `inOperatorFilters`):

```go
func ilikeIgnoresCase(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "a", Name: "Ana", Qty: 1, Active: true},
		&Widget{Id: "b", Name: "ana maría", Qty: 2, Active: true},
		&Widget{Id: "c", Name: "Bob", Qty: 3, Active: true},
	)
	got, err := readAll(conn, &Widget{}, []storage.Condition{storage.ILike("name", "%AN%")}, nil, 0, 0)
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(got) != 2 || got[0].Id != "a" || got[1].Id != "b" {
		t.Errorf("ILike %%AN%% must find Ana and ana maría whatever the case; got %+v", got)
	}
	none, err := readAll(conn, &Widget{}, []storage.Condition{storage.ILike("name", "%zzz%")}, nil, 0, 0)
	if err != nil || len(none) != 0 {
		t.Errorf("ILike with no match: got %+v, %v", none, err)
	}
}
```

`tests/mem_conformance_test.go` already runs `conformance.Run` over `mem`: no edit needed; check that the new clause passes there. Add to `tests/mem_extra_test.go`:

```go
func TestILike_ConditionShape(t *testing.T) {
	c := storage.ILike("name", "%an%")
	if c.Operator() != "ILIKE" || c.Field() != "name" || c.Logic() != "AND" {
		t.Fatalf("unexpected condition %+v", c)
	}
}
```
(Add the import `webtyp.com/storage` to that file if it lacks it.) Rows in `docs/TESTS.md` (`| [Name](../tests/<file>) | rule |`; the file may not exist or be incomplete: `gotest` prints `missing row for TestX (path)` for each: add exactly those rows, header `| Test | Rule |`).

## Stage 3 — documentation (English, `webtyp/*`)

`README.md` (≤150 lines): one paragraph in the condition list: «`ILike` ignores case on every backend; `Like` does not promise it».

## Delivery check

`gotest` passes on every destination; the clause `ilike_ignores_case` passes on `mem`; the pull request is open. A tree that does not compile or whose tests did not run is a failed delivery.
