package conformance

import "webtyp.com/model"

// Widget is the canonical record every backend is driven with. Its schema carries real DB
// metadata (types + PK) so SQL backends can CREATE TABLE it; mem ignores the metadata and
// stores by column name. Hand-written (conformance depends only on model, not ormc).
var WidgetModel = model.Definition{
	Name: "conformance_widget",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.Text(), NotNull: true},
		{Name: "qty", Type: model.Int(), NotNull: true},
		{Name: "active", Type: model.Bool(), NotNull: true},
		{Name: "note", Type: model.Text()}, // nullable ON PURPOSE: proves NULL scans as ""
	},
}

type Widget struct {
	Id     string
	Name   string
	Qty    int64
	Active bool
	Note   string
}

func (w *Widget) ModelName() string     { return WidgetModel.Name }
func (w *Widget) Schema() []model.Field { return WidgetModel.Fields }
func (w *Widget) Pointers() []any {
	return []any{&w.Id, &w.Name, &w.Qty, &w.Active, &w.Note}
}
func (w *Widget) IsNil() bool { return w == nil }
func (w *Widget) EncodeFields(wr model.FieldWriter) {
	wr.String("id", w.Id)
	wr.String("name", w.Name)
	wr.Int("qty", w.Qty)
	wr.Bool("active", w.Active)
	wr.String("note", w.Note)
}
func (w *Widget) DecodeFields(r model.FieldReader) {
	if v, ok := r.String("id"); ok {
		w.Id = v
	}
	if v, ok := r.String("name"); ok {
		w.Name = v
	}
	if v, ok := r.Int("qty"); ok {
		w.Qty = v
	}
	if v, ok := r.Bool("active"); ok {
		w.Active = v
	}
	if v, ok := r.String("note"); ok {
		w.Note = v
	}
}

var _ model.Model = (*Widget)(nil)

// Embedding is the canonical record for the blob and transaction clauses. It is
// separate from Widget on purpose: a backend that cannot carry bytes fails only
// the blob clauses instead of every clause at once.
var EmbeddingModel = model.Definition{
	Name: "conformance_embedding",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "vec", Type: model.Vector(4), NotNull: true}, // 16 bytes
		{Name: "loose", Type: model.Blob()},                 // nullable, no dimension
	},
}

type Embedding struct {
	Id    string
	Vec   []byte
	Loose []byte
}

func (e *Embedding) ModelName() string     { return EmbeddingModel.Name }
func (e *Embedding) Schema() []model.Field { return EmbeddingModel.Fields }
func (e *Embedding) Pointers() []any {
	return []any{&e.Id, &e.Vec, &e.Loose}
}
func (e *Embedding) IsNil() bool { return e == nil }
func (e *Embedding) EncodeFields(wr model.FieldWriter) {
	wr.String("id", e.Id)
	wr.Bytes("vec", e.Vec)
	wr.Bytes("loose", e.Loose)
}
func (e *Embedding) DecodeFields(r model.FieldReader) {
	if v, ok := r.String("id"); ok {
		e.Id = v
	}
	if v, ok := r.Bytes("vec"); ok {
		e.Vec = v
	}
	if v, ok := r.Bytes("loose"); ok {
		e.Loose = v
	}
}

var _ model.Model = (*Embedding)(nil)
