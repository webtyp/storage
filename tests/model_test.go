package tests

import (
	"testing"

	"webtyp.com/model"
	"webtyp.com/storage/conformance"
)

// strCell/intCell/boolCell are linear-scan slice entries, mirroring mem.dbCell — no map
// anywhere in this repo, not even in a test helper (see AGENTS.md).
type strCell struct {
	name string
	val  string
}

type intCell struct {
	name string
	val  int64
}

type boolCell struct {
	name string
	val  bool
}

type bytesCell struct {
	name string
	val  []byte
}

type dummyWriter struct {
	model.FieldWriter
	strings []strCell
	ints    []intCell
	bools   []boolCell
	bytes   []bytesCell
}

func (d *dummyWriter) String(name, val string) {
	d.strings = append(d.strings, strCell{name, val})
}

func (d *dummyWriter) Int(name string, val int64) {
	d.ints = append(d.ints, intCell{name, val})
}

func (d *dummyWriter) Bool(name string, val bool) {
	d.bools = append(d.bools, boolCell{name, val})
}

func (d *dummyWriter) Bytes(name string, val []byte) {
	d.bytes = append(d.bytes, bytesCell{name, val})
}

func (d *dummyWriter) getString(name string) string {
	for _, c := range d.strings {
		if c.name == name {
			return c.val
		}
	}
	return ""
}

func (d *dummyWriter) getInt(name string) int64 {
	for _, c := range d.ints {
		if c.name == name {
			return c.val
		}
	}
	return 0
}

func (d *dummyWriter) getBool(name string) bool {
	for _, c := range d.bools {
		if c.name == name {
			return c.val
		}
	}
	return false
}

func (d *dummyWriter) getBytes(name string) []byte {
	for _, c := range d.bytes {
		if c.name == name {
			return c.val
		}
	}
	return nil
}

type dummyReader struct {
	model.FieldReader
	strings []strCell
	ints    []intCell
	bools   []boolCell
	bytes   []bytesCell
}

func (d *dummyReader) String(name string) (string, bool) {
	for _, c := range d.strings {
		if c.name == name {
			return c.val, true
		}
	}
	return "", false
}

func (d *dummyReader) Int(name string) (int64, bool) {
	for _, c := range d.ints {
		if c.name == name {
			return c.val, true
		}
	}
	return 0, false
}

func (d *dummyReader) Bool(name string) (bool, bool) {
	for _, c := range d.bools {
		if c.name == name {
			return c.val, true
		}
	}
	return false, false
}

func (d *dummyReader) Bytes(name string) ([]byte, bool) {
	for _, c := range d.bytes {
		if c.name == name {
			return c.val, true
		}
	}
	return nil, false
}

func TestWidgetModelExtra(t *testing.T) {
	w := &conformance.Widget{
		Id:     "w1",
		Name:   "widget1",
		Qty:    10,
		Active: true,
	}

	if w.IsNil() {
		t.Error("expected IsNil to be false")
	}

	var nilWidget *conformance.Widget
	if !nilWidget.IsNil() {
		t.Error("expected nil widget to be IsNil true")
	}

	writer := &dummyWriter{}
	w.EncodeFields(writer)

	if writer.getString("id") != "w1" || writer.getString("name") != "widget1" || writer.getInt("qty") != 10 || !writer.getBool("active") {
		t.Errorf("EncodeFields didn't write fields correctly: %+v", writer)
	}

	reader := &dummyReader{
		strings: []strCell{{"id", "w2"}, {"name", "widget2"}},
		ints:    []intCell{{"qty", 20}},
		bools:   []boolCell{{"active", false}},
	}

	var w2 conformance.Widget
	w2.DecodeFields(reader)

	if w2.Id != "w2" || w2.Name != "widget2" || w2.Qty != 20 || w2.Active {
		t.Errorf("DecodeFields didn't read fields correctly: %+v", w2)
	}
}

func TestEmbeddingModelExtra(t *testing.T) {
	e := &conformance.Embedding{
		Id:    "e1",
		Vec:   []byte{1, 2, 3, 4},
		Loose: []byte("loose"),
	}

	if e.ModelName() != "conformance_embedding" {
		t.Errorf("ModelName = %q, want conformance_embedding", e.ModelName())
	}
	if len(e.Schema()) != 3 {
		t.Errorf("Schema len = %d, want 3", len(e.Schema()))
	}
	if len(e.Pointers()) != 3 {
		t.Errorf("Pointers len = %d, want 3", len(e.Pointers()))
	}
	if e.IsNil() {
		t.Error("expected IsNil false")
	}

	var nilEmb *conformance.Embedding
	if !nilEmb.IsNil() {
		t.Error("expected IsNil true for nil Embedding")
	}

	writer := &dummyWriter{}
	e.EncodeFields(writer)
	if writer.getString("id") != "e1" || string(writer.getBytes("vec")) != "\x01\x02\x03\x04" || string(writer.getBytes("loose")) != "loose" {
		t.Errorf("EncodeFields error: %+v", writer)
	}

	reader := &dummyReader{
		strings: []strCell{{"id", "e2"}},
		bytes:   []bytesCell{{"vec", []byte{5, 6, 7, 8}}, {"loose", []byte("updated")}},
	}
	var e2 conformance.Embedding
	e2.DecodeFields(reader)
	if e2.Id != "e2" || string(e2.Vec) != "\x05\x06\x07\x08" || string(e2.Loose) != "updated" {
		t.Errorf("DecodeFields error: %+v", e2)
	}
}
