package tests

import (
	"testing"

	"webtyp.com/model"
	"webtyp.com/storage"
	"webtyp.com/storage/mem"
)

// searchKind is a text kind that enters text_search (webtyp.com/input does this for Text, Phone, Email, Rut).
type searchKind struct{ model.Kind }

func (searchKind) SearchableText() {}

type person struct {
	ID, Name, Phone, Notes string
}

func (p *person) ModelName() string { return "person" }
func (p *person) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: searchKind{model.Text()}},
		{Name: "phone", Type: searchKind{model.Text()}},
		{Name: "notes", Type: model.Text()},
	}
}
func (p *person) Pointers() []any                { return []any{&p.ID, &p.Name, &p.Phone, &p.Notes} }
func (p *person) IsNil() bool                    { return p == nil }
func (p *person) EncodeFields(model.FieldWriter) {}
func (p *person) DecodeFields(model.FieldReader) {}

// insert writes a row with the columns given, as orm does (text_search included when given).
func insert(t *testing.T, conn storage.Conn, cols []string, vals []any) {
	t.Helper()
	plan, err := conn.Compile(storage.Query{Action: storage.ActionCreate, Table: "person", Columns: cols, Values: vals}, &person{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(plan.Query, plan.Args...); err != nil {
		t.Fatal(err)
	}
}

// textSearchOf reads the text_search column of row id.
func textSearchOf(t *testing.T, conn storage.Conn, id string) string {
	t.Helper()
	plan, err := conn.Compile(storage.Query{Action: storage.ActionReadOne, Table: "person", Columns: []string{model.FieldTextSearch}, Conditions: []storage.Condition{storage.Eq("id", id)}, Limit: 1}, &person{})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if err := conn.QueryRow(plan.Query, plan.Args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTextSearch_RewriteFromStoredRow(t *testing.T) {
	conn := mem.New()
	insert(t, conn, []string{"id", "name", "phone", "notes", model.FieldTextSearch}, []any{"p1", "Lucía Pérez", "+56912345678", "Ficha 12", "stale"})
	if err := storage.RewriteTextSearch(conn, &person{}, nil, []storage.Condition{storage.Eq("id", "p1")}); err != nil {
		t.Fatal(err)
	}
	if got := textSearchOf(t, conn, "p1"); got != "lucia perez 56912345678" {
		t.Errorf("text_search = %q, want %q", got, "lucia perez 56912345678")
	}
}

func TestTextSearch_RewriteOnlyMatchingRows(t *testing.T) {
	conn := mem.New()
	insert(t, conn, []string{"id", "name", model.FieldTextSearch}, []any{"p1", "Ana", "old1"})
	insert(t, conn, []string{"id", "name", model.FieldTextSearch}, []any{"p2", "Bruno", "old2"})
	if err := storage.RewriteTextSearch(conn, &person{}, nil, []storage.Condition{storage.Eq("id", "p1")}); err != nil {
		t.Fatal(err)
	}
	if got := textSearchOf(t, conn, "p1"); got != "ana" {
		t.Errorf("p1 = %q, want ana", got)
	}
	if got := textSearchOf(t, conn, "p2"); got != "old2" {
		t.Errorf("p2 = %q, want old2 (not matched, not touched)", got)
	}
}

func TestTextSearch_RewriteMixesNewValuesWithStoredOnes(t *testing.T) {
	conn := mem.New()
	insert(t, conn, []string{"id", "name", "phone", model.FieldTextSearch}, []any{"p1", "Ana", "+56911111111", "ana 56911111111"})
	next := &person{ID: "p1", Phone: "+56 9 2222 2222"} // an UpdateFields of phone only: Name is empty in the record
	if err := storage.RewriteTextSearch(conn, next, []string{"phone"}, []storage.Condition{storage.Eq("id", "p1")}); err != nil {
		t.Fatal(err)
	}
	if got := textSearchOf(t, conn, "p1"); got != "ana 56922222222" {
		t.Errorf("text_search = %q, want %q", got, "ana 56922222222")
	}
}
