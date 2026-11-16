package storage

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
)

// ErrTextSearchNoKey: a table whose rows must carry text_search has no primary key, so a row cannot be rewritten alone.
var ErrTextSearchNoKey = fmt.Err("storage: text_search needs a primary key to rewrite a row")

// RewriteTextSearch recomputes model.FieldTextSearch of every row of m's table that matches conds and writes it,
// one UPDATE per row. The value of a field named in fields comes from m (the new value of an UpdateFields that is
// about to run); every other value comes from the row as stored. fields nil reads every value from the row (the
// rebuild of a whole table). m is never changed. A table with no TextSearchFields does nothing. Call it inside the
// transaction of the write, BEFORE the UPDATE of fields: conds may name a column that UPDATE changes.
//
//	err := storage.RewriteTextSearch(tx, rec, []string{"phone"}, []storage.Condition{storage.Eq("id", "p1")})
func RewriteTextSearch(conn Conn, m model.Model, fields []string, conds []Condition) error {
	schema := m.Schema()
	search := model.TextSearchFields(schema)
	if len(search) == 0 {
		return nil
	}
	var keys []int
	for i, f := range schema {
		if f.IsPK() {
			keys = append(keys, i)
		}
	}
	if len(keys) == 0 {
		return ErrTextSearchNoKey
	}
	read := append(make([]int, 0, len(keys)+len(search)), keys...)
	for _, i := range search {
		if !named(fields, schema[i].Name) {
			read = append(read, i)
		}
	}
	given := m.Pointers()
	columns := make([]string, len(read))
	for j, i := range read {
		columns[j] = schema[i].Name
	}
	plan, err := conn.Compile(Query{Action: ActionReadAll, Table: m.ModelName(), Columns: columns, Conditions: conds}, m)
	if err != nil {
		return err
	}
	rows, err := conn.Query(plan.Query, plan.Args...)
	if err != nil {
		return err
	}
	type rewrite struct {
		key  []any
		text string
	}
	var todo []rewrite
	for rows.Next() {
		ptrs := make([]any, len(schema))
		for _, i := range search {
			if named(fields, schema[i].Name) {
				ptrs[i] = given[i]
			}
		}
		dest := make([]any, len(read))
		for j, i := range read {
			if schema[i].Type.Storage() == model.FieldInt {
				dest[j] = new(int64)
			} else {
				dest[j] = new(string)
			}
			ptrs[i] = dest[j]
		}
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return err
		}
		key := make([]any, len(keys))
		for j := range keys {
			switch v := dest[j].(type) {
			case *int64:
				key[j] = *v
			case *string:
				key[j] = *v
			}
		}
		todo = append(todo, rewrite{key: key, text: model.TextSearchOf(schema, ptrs)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, r := range todo {
		where := make([]Condition, len(keys))
		for j, i := range keys {
			where[j] = Eq(schema[i].Name, r.key[j])
		}
		plan, err := conn.Compile(Query{Action: ActionUpdate, Table: m.ModelName(), Columns: []string{model.FieldTextSearch}, Values: []any{r.text}, Conditions: where}, m)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(plan.Query, plan.Args...); err != nil {
			return err
		}
	}
	return nil
}

// named reports whether name is one of fields.
func named(fields []string, name string) bool {
	for _, f := range fields {
		if f == name {
			return true
		}
	}
	return false
}
