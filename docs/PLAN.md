---
PLAN: "feat: cláusulas de conformance para blobs y transacciones"
TAG: v0.1.0
EXECUTOR: unassigned
REVIEWER: none
STATUS: review
SESSION: 11829055090585832463
PR: https://github.com/webtyp/storage/pull/4
---

> Parte del esfuerzo de búsqueda semántica nativa en el navegador. Índice maestro:
> https://github.com/webtyp/agent/blob/main/docs/PLAN.md — las decisiones **D1** y **D2** de
> ahí son la justificación de todo lo de abajo.
>
> **Nota de idioma:** la prosa va en español; los bloques de código mantienen sus
> comentarios en inglés, como el resto del código fuente de este repositorio.

# Plan — demostrar que un backend puede transportar un vector

## Por qué

Guardar embeddings significa guardar `[]byte` y escribir muchas filas atómicamente. Este
repositorio ya tiene **ambos contratos**; lo que no tiene es nada que demuestre que un
backend los honra.

- `ScanAny` (`scan.go`) ya maneja `*[]byte` por completo, tanto en el camino NULL como en el
  no-NULL. **No hace falta cambiar nada ahí.**
- `TxExecutor` / `TxBoundExecutor` (`tx.go`) ya describen las transacciones como una
  capacidad opcional con type assertion. **Tampoco hace falta cambiar nada ahí.**
- Pero `conformance.Widget` (`conformance/model.go`) tiene solo columnas de texto, entero y
  booleano, y `conformance.Run` tiene trece cláusulas, ninguna de las cuales escribe un
  slice de bytes ni abre una transacción.

La consecuencia es concreta: `indexdb` va a **crashear** — no dar error — en la primera
escritura de blob (`js.ValueOf` hace pánico con `[]byte`, y TinyGo wasm no tiene
`recover()`), y hoy la suite de conformance lo aprueba igual. Un contrato que nada testea no
es un contrato.

## Lo que NO cambia

`scan.go`, `executor.go`, `conn.go`, `query.go`, `conditions.go`, `compiler.go`,
`execution_plan.go` y `tx.go` quedan **intactos**. Este plan agrega superficie de test y un
record de conformance nuevo; no cambia ninguna interfaz de producción.

En particular, **no se agrega ninguna API de scan en streaming.** Sería lo obvio a lo que
recurrir — una consulta kNN recorre todos los candidatos — pero la decisión **D2** guarda los
vectores en shards de 1024, así que el camino caliente de lectura de `vectordb` es un puñado
de filas grandes de una tabla, que `Query`/`Rows` ya sirve. Agregar una API de scan para un
llamador que no la necesita sería especulativo.

`conformance.Widget` **no** se extiende. Agregarle una columna blob al record canónico
rompería el setup de tablas de todos los backends a la vez, incluidos backends fuera del
alcance de este plan. Las cláusulas nuevas usan su propio record.

## Cambios

### 1. `conformance/model.go` — un segundo record canónico

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

con los métodos habituales `ModelName`/`Schema`/`Pointers`/`IsNil`/`EncodeFields`/
`DecodeFields`, siguiendo el estilo escrito a mano de `Widget` (este paquete depende de
`model`, no de `ormc`).

`model.Vector(4)` requiere `webtyp.com/model` en el tag que produce
https://github.com/webtyp/model/blob/main/docs/PLAN.md — subilo en `go.mod` primero.

### 2. `conformance/conformance.go` — cuatro cláusulas nuevas

Registradas en `Run` después de las trece existentes:

| Cláusula | Verifica |
|---|---|
| `blob_round_trips_byte_for_byte` | un `vec` de 16 bytes escrito y releído compara igual byte a byte, incluyendo bytes `0x00` en el medio y un terminador `0xFF` — los dos valores que un backend basado en strings trunca en silencio |
| `blob_null_scans_as_nil` | un `loose` sin asignar se lee como `nil`, no como `[]byte{}` — la misma regla que `null_scans_as_zero` ya impone para escalares |
| `blob_updates_in_place` | sobrescribir `vec` con bytes distintos del mismo largo no deja rastro del valor anterior |
| `batch_insert_is_atomic` | **se saltea salvo que el backend implemente `TxExecutor`**: 64 filas creadas dentro de una transacción están todas visibles tras `Commit`, y un `Rollback` tras 64 creates deja la tabla vacía |

La cláusula de transacción se saltea con `t.Skip` y un mensaje explícito cuando la type
assertion falla, para que un backend sin soporte de transacciones reporte "salteado", nunca
un falso positivo.

### 3. `mem/mem.go` — verificar, y recién después arreglar si hace falta

`scanInto` delega en `storage.ScanAny`, que maneja `*[]byte`. Así que se espera que `mem`
pase las cláusulas de blob **sin modificación** — corrélas primero y tocá `mem` solo si
fallan.

`mem` no implementa `TxExecutor` más allá de los `BeginTx`/`Commit`/`Rollback` no-op que ya
están en `engine` (`mem.go:80-85`). Esos son stubs: `Rollback` no descarta nada. O bien
implementá semántica real de snapshot (copiar el slice de la tabla en `BeginTx`, restaurar
en `Rollback`), o bien sacá los stubs para que la type assertion falle honestamente y la
cláusula se saltee. **Sacalo o implementalo — no dejes un `Rollback` que miente.**
Recomendación: implementalo; son ~15 líneas y `mem` es el backend de referencia.

### 4. `mock/recorders.go`

Si el mock registra llamadas por método, agregá los métodos de transacción para que un test
pueda verificar que un lote usó una transacción y no 64.

## Tests

`tests/mem_conformance_test.go` recoge las cláusulas nuevas automáticamente a través de
`Run`. Agregar a `tests/mem_extra_test.go`:

| Test | Verifica |
|---|---|
| `TestScanAny_BlobFromString` | un backend que devuelve `string` para una columna BLOB igual escanea a `*[]byte` (ya soportado — esto lo fija) |
| `TestScanAny_NilBlob` | NULL → `nil`, no un slice vacío no-nil |
| `TestMem_RollbackDiscards` | solo si la §3 implementa transacciones reales |

## Checklist de aceptación

```bash
grep -n "EmbeddingModel" conformance/model.go       # → 1 coincidencia
grep -c "t.Run(\"blob_" conformance/conformance.go  # → 3
grep -n "batch_insert_is_atomic" conformance/conformance.go
go vet ./...
gotest
```

## Nota aguas abajo, que no es trabajo de este repositorio

`sqlt` y `postgres` necesitan un mapeo DDL de `FieldBlob` → `BLOB` / `BYTEA` antes de pasar
las cláusulas nuevas. Si no lo tienen, las cláusulas nuevas lo van a sacar a la luz — que es
justamente el punto. Abrí un issue contra esos repositorios; no ensanches este plan.

Liberar cuando el checklist pase; `indexdb` y `vectordb` dependen ambos del tag:

```bash
gopush 'feat: blob and transaction conformance clauses'
```
