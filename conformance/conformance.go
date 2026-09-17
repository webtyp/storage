package conformance

import (
	"bytes"
	"errors"
	"testing"

	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/storage"
)

// Factory builds, for ONE clause, a fresh storage.Conn whose Widget table already exists and is
// EMPTY. Schema setup is the backend's job, done OUTSIDE this DML contract (this suite never
// builds a CreateTable Query):
//   - mem:              auto-creates the table on first Create — New just returns mem.New().
//   - sqlite/postgres:  New runs ddlc.ExportDDL(models) (or ddl.CreateTable) before returning.
//   - indexdb:          New declares `models` as IndexedDB object stores up front.
//
// models are the record types the suite will exercise. Called once per clause → no cross-clause
// bleed.
type Factory struct {
	Name string
	New  func(t *testing.T, models ...model.Model) storage.Conn
}

func Run(t *testing.T, f Factory) {
	if f.New == nil {
		t.Fatal("conformance: Factory.New is required")
	}
	t.Run("create_then_read_one_by_pk", func(t *testing.T) { createThenReadOneByPK(t, f) })
	t.Run("read_one_no_match_is_not_found", func(t *testing.T) { readOneNoMatchIsNotFound(t, f) })
	t.Run("read_all_returns_every_row", func(t *testing.T) { readAllReturnsEveryRow(t, f) })
	t.Run("read_all_filters_by_eq", func(t *testing.T) { readAllFiltersByEq(t, f) })
	t.Run("read_all_ands_two_conditions", func(t *testing.T) { readAllAndsTwoConditions(t, f) })
	t.Run("read_all_ors_conditions", func(t *testing.T) { readAllOrsConditions(t, f) })
	t.Run("read_all_orders_asc_and_desc", func(t *testing.T) { readAllOrdersAscDesc(t, f) })
	t.Run("read_all_applies_limit_and_offset", func(t *testing.T) { readAllLimitOffset(t, f) })
	t.Run("comparison_operators_filter", func(t *testing.T) { comparisonOperatorsFilter(t, f) })
	t.Run("in_operator_filters", func(t *testing.T) { inOperatorFilters(t, f) })
	t.Run("update_changes_matched_rows_only", func(t *testing.T) { updateChangesMatchedOnly(t, f) })
	t.Run("delete_removes_matched_rows_only", func(t *testing.T) { deleteRemovesMatchedOnly(t, f) })
	t.Run("null_scans_as_zero", func(t *testing.T) { nullScansAsZero(t, f) })
	t.Run("blob_round_trips_byte_for_byte", func(t *testing.T) { blobRoundTripsByteForByte(t, f) })
	t.Run("blob_null_scans_as_nil", func(t *testing.T) { blobNullScansAsNil(t, f) })
	t.Run("blob_updates_in_place", func(t *testing.T) { blobUpdatesInPlace(t, f) })
	t.Run("batch_insert_is_atomic", func(t *testing.T) { batchInsertIsAtomic(t, f) })
}

func setup(t *testing.T, f Factory, seed ...*Widget) storage.Conn {
	t.Helper()
	conn := f.New(t, &Widget{}) // table already exists & empty — backend set it up, not this suite
	for _, w := range seed {
		if err := create(conn, w); err != nil {
			t.Fatalf("seed create(%+v): %v", w, err)
		}
	}
	return conn
}

// create mirrors what orm.DB.Create builds, minus the autoincrement-PK-skip branch (Widget's PK
// is a plain Text, never AutoInc — that branch is orm's concern and is tested there, not here).
func create(conn storage.Conn, w *Widget) error {
	schema := w.Schema()
	values := model.ReadValues(schema, w.Pointers())
	columns := make([]string, len(schema))
	for i, f := range schema {
		columns[i] = f.Name
	}
	q := storage.Query{Action: storage.ActionCreate, Table: w.ModelName(), Columns: columns, Values: values}
	plan, err := conn.Compile(q, w)
	if err != nil {
		return err
	}
	return conn.Exec(plan.Query, plan.Args...)
}

func readOne(conn storage.Conn, w *Widget, conds ...storage.Condition) error {
	q := storage.Query{Action: storage.ActionReadOne, Table: w.ModelName(), Conditions: conds, Limit: 1}
	plan, err := conn.Compile(q, w)
	if err != nil {
		return err
	}
	return conn.QueryRow(plan.Query, plan.Args...).Scan(w.Pointers()...)
}

func readAll(conn storage.Conn, w *Widget, conds []storage.Condition, order []storage.Order, limit, offset int) ([]*Widget, error) {
	q := storage.Query{
		Action: storage.ActionReadAll, Table: w.ModelName(),
		Conditions: conds, OrderBy: order, Limit: limit, Offset: offset,
	}
	plan, err := conn.Compile(q, w)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(plan.Query, plan.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Widget
	for rows.Next() {
		var got Widget
		if err := rows.Scan(got.Pointers()...); err != nil {
			return nil, err
		}
		out = append(out, &got)
	}
	return out, rows.Err()
}

func update(conn storage.Conn, w *Widget, conds ...storage.Condition) error {
	schema := w.Schema()
	columns := make([]string, len(schema))
	for i, f := range schema {
		columns[i] = f.Name
	}
	q := storage.Query{
		Action: storage.ActionUpdate, Table: w.ModelName(),
		Columns: columns, Values: model.ReadValues(schema, w.Pointers()), Conditions: conds,
	}
	plan, err := conn.Compile(q, w)
	if err != nil {
		return err
	}
	return conn.Exec(plan.Query, plan.Args...)
}

func deleteRow(conn storage.Conn, w *Widget, conds ...storage.Condition) error {
	q := storage.Query{Action: storage.ActionDelete, Table: w.ModelName(), Conditions: conds}
	plan, err := conn.Compile(q, w)
	if err != nil {
		return err
	}
	return conn.Exec(plan.Query, plan.Args...)
}

func createThenReadOneByPK(t *testing.T, f Factory) {
	conn := setup(t, f, &Widget{Id: "w1", Name: "alpha", Qty: 3, Active: true})
	var got Widget
	if err := readOne(conn, &got, storage.Eq("id", "w1")); err != nil {
		t.Fatalf("readOne: %v", err)
	}
	if got.Name != "alpha" || got.Qty != 3 || got.Active != true {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func readOneNoMatchIsNotFound(t *testing.T, f Factory) {
	conn := setup(t, f)
	var got Widget
	err := readOne(conn, &got, storage.Eq("id", "nonexistent"))
	if !errors.Is(err, storage.ErrNoRows) {
		t.Errorf("expected storage.ErrNoRows, got %v", err)
	}
}

func readAllReturnsEveryRow(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 3, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 4, Active: false},
	)
	got, err := readAll(conn, &Widget{}, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 widgets, got %d", len(got))
	}
}

func readAllFiltersByEq(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 3, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 4, Active: false},
	)
	got, err := readAll(conn, &Widget{}, []storage.Condition{storage.Eq("name", "alpha")}, nil, 0, 0)
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(got) != 1 || got[0].Id != "w1" {
		t.Errorf("expected only w1, got %+v", got)
	}
}

func readAllAndsTwoConditions(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "a", Name: "x", Qty: 1, Active: true},
		&Widget{Id: "b", Name: "x", Qty: 1, Active: false},
		&Widget{Id: "c", Name: "y", Qty: 1, Active: true},
	)
	conds := []storage.Condition{storage.Eq("name", "x"), storage.Eq("active", true)}
	got, err := readAll(conn, &Widget{}, conds, nil, 0, 0)
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(got) != 1 || got[0].Id != "a" {
		t.Errorf("AND of two conditions must return only {a}; got %+v", got)
	}
}

func readAllOrsConditions(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "a", Name: "x", Qty: 1, Active: true},
		&Widget{Id: "b", Name: "y", Qty: 2, Active: false},
		&Widget{Id: "c", Name: "z", Qty: 3, Active: true},
	)
	conds := []storage.Condition{storage.Eq("name", "x"), storage.Or(storage.Eq("name", "y"))}
	got, err := readAll(conn, &Widget{}, conds, nil, 0, 0)
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
	if got[0].Id != "a" || got[1].Id != "b" {
		t.Errorf("expected a and b, got %+v", got)
	}
}

func readAllOrdersAscDesc(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 5, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
		&Widget{Id: "w3", Name: "gamma", Qty: 8, Active: true},
	)
	gotAsc, err := readAll(conn, &Widget{}, nil, []storage.Order{storage.Asc("qty")}, 0, 0)
	if err != nil {
		t.Fatalf("readAll asc: %v", err)
	}
	if len(gotAsc) != 3 || gotAsc[0].Id != "w2" || gotAsc[1].Id != "w1" || gotAsc[2].Id != "w3" {
		t.Errorf("expected w2, w1, w3; got %+v", gotAsc)
	}
	gotDesc, err := readAll(conn, &Widget{}, nil, []storage.Order{storage.Desc("qty")}, 0, 0)
	if err != nil {
		t.Fatalf("readAll desc: %v", err)
	}
	if len(gotDesc) != 3 || gotDesc[0].Id != "w3" || gotDesc[1].Id != "w1" || gotDesc[2].Id != "w2" {
		t.Errorf("expected w3, w1, w2; got %+v", gotDesc)
	}
}

func readAllLimitOffset(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
		&Widget{Id: "w3", Name: "gamma", Qty: 3, Active: true},
		&Widget{Id: "w4", Name: "delta", Qty: 4, Active: false},
	)
	got, err := readAll(conn, &Widget{}, nil, []storage.Order{storage.Asc("qty")}, 2, 1)
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(got) != 2 || got[0].Id != "w2" || got[1].Id != "w3" {
		t.Errorf("expected w2, w3; got %+v", got)
	}
}

func comparisonOperatorsFilter(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
		&Widget{Id: "w3", Name: "gamma", Qty: 3, Active: true},
	)
	cases := []struct {
		name string
		cond storage.Condition
		want []string
	}{
		{"Neq", storage.Neq("qty", 2), []string{"w1", "w3"}},
		{"Gt", storage.Gt("qty", 1), []string{"w2", "w3"}},
		{"Gte", storage.Gte("qty", 2), []string{"w2", "w3"}},
		{"Lt", storage.Lt("qty", 3), []string{"w1", "w2"}},
		{"Lte", storage.Lte("qty", 2), []string{"w1", "w2"}},
	}
	for _, c := range cases {
		got, err := readAll(conn, &Widget{}, []storage.Condition{c.cond}, nil, 0, 0)
		if err != nil {
			t.Fatalf("%s: readAll: %v", c.name, err)
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: expected %d rows, got %d (%+v)", c.name, len(c.want), len(got), got)
			continue
		}
		for i, id := range c.want {
			if got[i].Id != id {
				t.Errorf("%s: expected %v, got %+v", c.name, c.want, got)
				break
			}
		}
	}
}

func inOperatorFilters(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "a", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "b", Name: "beta", Qty: 2, Active: false},
		&Widget{Id: "c", Name: "gamma", Qty: 3, Active: true},
	)
	got, err := readAll(conn, &Widget{}, []storage.Condition{storage.In("id", []any{"a", "c"})}, nil, 0, 0)
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(got) != 2 || got[0].Id != "a" || got[1].Id != "c" {
		t.Errorf("In: expected a, c; got %+v", got)
	}
}

func updateChangesMatchedOnly(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
	)
	m := &Widget{Name: "updated", Qty: 99, Active: true}
	if err := update(conn, m, storage.Eq("id", "w1")); err != nil {
		t.Fatalf("update: %v", err)
	}
	var got1 Widget
	if err := readOne(conn, &got1, storage.Eq("id", "w1")); err != nil {
		t.Fatalf("readOne w1: %v", err)
	}
	if got1.Name != "updated" || got1.Qty != 99 || got1.Active != true {
		t.Errorf("w1 was not correctly updated: %+v", got1)
	}
	var got2 Widget
	if err := readOne(conn, &got2, storage.Eq("id", "w2")); err != nil {
		t.Fatalf("readOne w2: %v", err)
	}
	if got2.Name != "beta" || got2.Qty != 2 || got2.Active != false {
		t.Errorf("w2 was modified but shouldn't have been: %+v", got2)
	}
}

func deleteRemovesMatchedOnly(t *testing.T, f Factory) {
	conn := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
	)
	if err := deleteRow(conn, &Widget{}, storage.Eq("id", "w1")); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var got1 Widget
	err := readOne(conn, &got1, storage.Eq("id", "w1"))
	if !errors.Is(err, storage.ErrNoRows) {
		t.Errorf("expected w1 to be deleted/not found, got err: %v", err)
	}
	var got2 Widget
	if err := readOne(conn, &got2, storage.Eq("id", "w2")); err != nil {
		t.Errorf("expected w2 to still exist, got: %v", err)
	}
}

func nullScansAsZero(t *testing.T, f Factory) {
	conn := f.New(t, &Widget{})
	w := &Widget{Id: "w1", Name: "alpha", Qty: 3, Active: true}
	q := storage.Query{Action: storage.ActionCreate, Table: w.ModelName(), Columns: []string{"id", "name", "qty", "active"}, Values: []any{w.Id, w.Name, w.Qty, w.Active}}
	plan, err := conn.Compile(q, w)
	if err != nil {
		t.Fatalf("create without note: %v", err)
	}
	if err := conn.Exec(plan.Query, plan.Args...); err != nil {
		t.Fatalf("exec without note: %v", err)
	}
	got := Widget{Note: "sentinel"}
	if err := readOne(conn, &got, storage.Eq("id", "w1")); err != nil {
		t.Fatalf("readOne: %v", err)
	}
	if got.Note != "" {
		t.Errorf("NullScansAsZero: note = %q, want \"\" (a NULL column must scan as the Go zero value)", got.Note)
	}
}

func setupEmbedding(t *testing.T, f Factory, seed ...*Embedding) storage.Conn {
	t.Helper()
	conn := f.New(t, &Embedding{})
	for _, e := range seed {
		if err := createEmbedding(conn, e); err != nil {
			t.Fatalf("seed createEmbedding(%+v): %v", e, err)
		}
	}
	return conn
}

func createEmbedding(conn storage.Conn, e *Embedding) error {
	schema := e.Schema()
	values := model.ReadValues(schema, e.Pointers())
	columns := make([]string, len(schema))
	for i, f := range schema {
		columns[i] = f.Name
	}
	q := storage.Query{Action: storage.ActionCreate, Table: e.ModelName(), Columns: columns, Values: values}
	plan, err := conn.Compile(q, e)
	if err != nil {
		return err
	}
	return conn.Exec(plan.Query, plan.Args...)
}

func readOneEmbedding(conn storage.Conn, e *Embedding, conds ...storage.Condition) error {
	q := storage.Query{Action: storage.ActionReadOne, Table: e.ModelName(), Conditions: conds, Limit: 1}
	plan, err := conn.Compile(q, e)
	if err != nil {
		return err
	}
	return conn.QueryRow(plan.Query, plan.Args...).Scan(e.Pointers()...)
}

func readAllEmbeddings(conn storage.Conn, e *Embedding, conds []storage.Condition, order []storage.Order, limit, offset int) ([]*Embedding, error) {
	q := storage.Query{
		Action: storage.ActionReadAll, Table: e.ModelName(),
		Conditions: conds, OrderBy: order, Limit: limit, Offset: offset,
	}
	plan, err := conn.Compile(q, e)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(plan.Query, plan.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Embedding
	for rows.Next() {
		var got Embedding
		if err := rows.Scan(got.Pointers()...); err != nil {
			return nil, err
		}
		out = append(out, &got)
	}
	return out, rows.Err()
}

func updateEmbedding(conn storage.Conn, e *Embedding, conds ...storage.Condition) error {
	schema := e.Schema()
	columns := make([]string, len(schema))
	for i, f := range schema {
		columns[i] = f.Name
	}
	q := storage.Query{
		Action: storage.ActionUpdate, Table: e.ModelName(),
		Columns: columns, Values: model.ReadValues(schema, e.Pointers()), Conditions: conds,
	}
	plan, err := conn.Compile(q, e)
	if err != nil {
		return err
	}
	return conn.Exec(plan.Query, plan.Args...)
}

func blobRoundTripsByteForByte(t *testing.T, f Factory) {
	vec := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0xFF}
	conn := setupEmbedding(t, f, &Embedding{Id: "e1", Vec: vec, Loose: []byte("hello")})
	var got Embedding
	if err := readOneEmbedding(conn, &got, storage.Eq("id", "e1")); err != nil {
		t.Fatalf("readOneEmbedding: %v", err)
	}
	if !bytes.Equal(got.Vec, vec) {
		t.Errorf("Vec round-trip mismatch: got %v, want %v", got.Vec, vec)
	}
	if !bytes.Equal(got.Loose, []byte("hello")) {
		t.Errorf("Loose round-trip mismatch: got %v, want %v", got.Loose, []byte("hello"))
	}
}

func blobNullScansAsNil(t *testing.T, f Factory) {
	conn := f.New(t, &Embedding{})
	e := &Embedding{Id: "e1", Vec: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F}}
	q := storage.Query{Action: storage.ActionCreate, Table: e.ModelName(), Columns: []string{"id", "vec"}, Values: []any{e.Id, e.Vec}}
	plan, err := conn.Compile(q, e)
	if err != nil {
		t.Fatalf("compile create without loose: %v", err)
	}
	if err := conn.Exec(plan.Query, plan.Args...); err != nil {
		t.Fatalf("exec create without loose: %v", err)
	}
	got := Embedding{Loose: []byte("sentinel")}
	if err := readOneEmbedding(conn, &got, storage.Eq("id", "e1")); err != nil {
		t.Fatalf("readOneEmbedding: %v", err)
	}
	if got.Loose != nil {
		t.Errorf("blob_null_scans_as_nil: loose = %v, want nil (a NULL blob column must scan as nil)", got.Loose)
	}
}

func blobUpdatesInPlace(t *testing.T, f Factory) {
	vec1 := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0xFF}
	vec2 := []byte{0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8, 0xF7, 0xF6, 0xF5, 0xF4, 0xF3, 0xF2, 0xF1, 0xF0, 0x00}
	conn := setupEmbedding(t, f, &Embedding{Id: "e1", Vec: vec1})

	m := &Embedding{Vec: vec2}
	if err := updateEmbedding(conn, m, storage.Eq("id", "e1")); err != nil {
		t.Fatalf("updateEmbedding: %v", err)
	}

	var got Embedding
	if err := readOneEmbedding(conn, &got, storage.Eq("id", "e1")); err != nil {
		t.Fatalf("readOneEmbedding: %v", err)
	}
	if !bytes.Equal(got.Vec, vec2) {
		t.Errorf("blob_updates_in_place: got %v, want %v", got.Vec, vec2)
	}
}

func batchInsertIsAtomic(t *testing.T, f Factory) {
	conn := f.New(t, &Embedding{})
	txExec, ok := conn.(storage.TxExecutor)
	if !ok {
		t.Skip("backend does not implement storage.TxExecutor")
	}

	tx, err := txExec.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	vec := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	for i := 0; i < 64; i++ {
		e := &Embedding{Id: fmt.Sprintf("e%d", i), Vec: vec}
		schema := e.Schema()
		values := model.ReadValues(schema, e.Pointers())
		columns := make([]string, len(schema))
		for ci, field := range schema {
			columns[ci] = field.Name
		}
		q := storage.Query{Action: storage.ActionCreate, Table: e.ModelName(), Columns: columns, Values: values}
		plan, err := conn.Compile(q, e)
		if err != nil {
			t.Fatalf("compile create e%d: %v", i, err)
		}
		if err := tx.Exec(plan.Query, plan.Args...); err != nil {
			t.Fatalf("tx.Exec create e%d: %v", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	rows, err := readAllEmbeddings(conn, &Embedding{}, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("readAllEmbeddings post-commit: %v", err)
	}
	if len(rows) != 64 {
		t.Fatalf("expected 64 rows after commit, got %d", len(rows))
	}

	// Now start a second transaction, create 64 more, then rollback.
	tx2, err := txExec.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx tx2: %v", err)
	}

	for i := 64; i < 128; i++ {
		e := &Embedding{Id: fmt.Sprintf("e%d", i), Vec: vec}
		schema := e.Schema()
		values := model.ReadValues(schema, e.Pointers())
		columns := make([]string, len(schema))
		for ci, field := range schema {
			columns[ci] = field.Name
		}
		q := storage.Query{Action: storage.ActionCreate, Table: e.ModelName(), Columns: columns, Values: values}
		plan, err := conn.Compile(q, e)
		if err != nil {
			t.Fatalf("compile create e%d: %v", i, err)
		}
		if err := tx2.Exec(plan.Query, plan.Args...); err != nil {
			t.Fatalf("tx2.Exec create e%d: %v", i, err)
		}
	}

	if err := tx2.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	rowsAfterRollback, err := readAllEmbeddings(conn, &Embedding{}, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("readAllEmbeddings post-rollback: %v", err)
	}
	if len(rowsAfterRollback) != 64 {
		t.Errorf("expected 64 rows after rollback, got %d", len(rowsAfterRollback))
	}
}
