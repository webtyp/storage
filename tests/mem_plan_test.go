package tests

import (
	"sync"
	"testing"

	"webtyp.com/model"
	"webtyp.com/storage"
	"webtyp.com/storage/mem"
)

func createPlan(t *testing.T, conn storage.Conn, d *ExtraDummy) storage.Plan {
	t.Helper()
	schema := d.Schema()
	cols := make([]string, len(schema))
	for i, f := range schema {
		cols[i] = f.Name
	}
	plan, err := conn.Compile(storage.Query{
		Action: storage.ActionCreate, Table: d.ModelName(),
		Columns: cols, Values: model.ReadValues(schema, d.Pointers()),
	}, d)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func countRows(t *testing.T, conn storage.Conn) int {
	t.Helper()
	plan, err := conn.Compile(storage.Query{Action: storage.ActionReadAll, Table: ExtraDummyModel.Name}, &ExtraDummy{})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(plan.Query, plan.Args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	return n
}

// A Plan must carry everything its execution needs. A decorator (e.g. a change log) compiles a
// write, then compiles a read of the rows it is about to touch, then executes the write: the
// write must run, not the read compiled last.
func TestMem_ExecRunsThePlanItIsGiven(t *testing.T) {
	conn := mem.New()
	write := createPlan(t, conn, &ExtraDummy{Id: "a", Name: "A"})

	read, err := conn.Compile(storage.Query{Action: storage.ActionReadAll, Table: ExtraDummyModel.Name}, &ExtraDummy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(write.Query, write.Args...); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(read.Query, read.Args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if n != 1 {
		t.Fatalf("the compiled create must have inserted 1 row, got %d", n)
	}
}

// Compile and Exec from several goroutines must not mix plans up.
func TestMem_ConcurrentCompileExec(t *testing.T) {
	conn := mem.New()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				id := string(rune('a'+g)) + "-" + string(rune('A'+i%26)) + string(rune('A'+i/26))
				p := createPlan(t, conn, &ExtraDummy{Id: id, Name: id})
				if err := conn.Exec(p.Query, p.Args...); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	if got := countRows(t, conn); got != 400 {
		t.Fatalf("expected 400 rows after 8×50 concurrent creates, got %d", got)
	}
}

// Executing something that did not come from mem's own Compile is a loud error, never a
// silent no-op on whatever was compiled before.
func TestMem_ExecRejectsForeignPlan(t *testing.T) {
	conn := mem.New()
	if err := conn.Exec("SELECT 1"); err == nil {
		t.Fatal("Exec with a plan not produced by mem.Compile must fail")
	}
}
