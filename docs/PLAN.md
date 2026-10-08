---
PLAN: "feat(storage): IsNoRows — detect the no-rows sentinel without == between interfaces"
EXECUTOR: jules
REVIEWER: none
---

# Plan — `storage.IsNoRows(err)`

> Master: `webtyp/docs/NO_REFLECTION_MASTER_PLAN.md` (ola 1). `webtyp/orm` (ola 2) depende del tag
> de este plan.

## 1. El problema

Quien detecta "sin filas" hoy escribe `err == storage.ErrNoRows` (o `errors.Is`, en
`conformance/conformance.go:153` y `:344`). `error` es una interfaz: en TinyGo `==` entre interfaces
compila a `runtime.interfaceEqual` → `reflectValueEqual`, y `errors.Is` también usa reflectlite. Las
dos meten `internal/reflectlite` (~7–9 KB) en el binario wasm. La regla del dueño es que el código
que compila a wasm no use reflexión nunca.

## 2. Design gate (api-design)

1. **Antecedentes.** `os.IsNotExist(err)` (biblioteca estándar, el patrón anterior a `errors.Is`);
   `status.Code(err)` de gRPC; `apierrors.IsNotFound(err)` de Kubernetes client-go. `database/sql`
   hace lo contrario (`err == sql.ErrNoRows`, `errors.Is`): exactamente lo que en TinyGo cuesta
   reflexión. Aquí se elige la función de consulta implementada con aserción de tipo, que TinyGo
   compila comparando el código de tipo, sin reflectlite.
2. **Nombre.** `storage.IsNoRows(err)`: "¿es error de sin filas?". Mismo verbo que `os.IsNotExist`.
3. **Balance.** Conceptos +1 · formas de detectar el centinela: hoy 2 (`==`, `errors.Is`), después 1
   (`IsNoRows`; el guardia de `gotest` de la ola 4 prohíbe las otras en código wasm) · call site igual.
4. **Dónde va.** `storage`, que es dueño del centinela.
5. **Qué borra.** Los `errors.Is(err, storage.ErrNoRows)` de `conformance`; el import de `errors`
   si queda sin uso.

## 3. La corrección

En `errors.go`:

```go
// noRows is the concrete type of ErrNoRows. IsNoRows recognises it with a type
// assertion: TinyGo compiles that to a type-code comparison, while == between
// two error values (and errors.Is) goes through runtime.interfaceEqual and pulls
// internal/reflectlite into the wasm binary.
type noRows struct{}

func (noRows) Error() string { return "<texto actual>" }

// ErrNoRows is the agnostic sentinel for "query returned no rows". Conn
// implementations (postgres, sqlt, mem, indexdb) return this value when a query
// yields no rows. Callers detect it with IsNoRows, never with == or errors.Is.
var ErrNoRows error = noRows{}

// IsNoRows reports whether err is ErrNoRows.
func IsNoRows(err error) bool {
	_, ok := err.(noRows)
	return ok
}
```

- `<texto actual>`: el string exacto que devuelve hoy `fmt.Err("no", "rows").Error()`. Medirlo
  **antes** de cambiar nada y fijarlo en un test (los logs y mensajes no deben cambiar).
- Conservar el resto del comentario actual de `ErrNoRows` (la traducción a `orm.ErrNotFound` la hace
  orm).
- `conformance/conformance.go:153` y `:344`: `!errors.Is(err, storage.ErrNoRows)` →
  `!storage.IsNoRows(err)`. Esto además exige a cada implementación devolver el centinela tal cual
  (sin envolverlo), que es el contrato documentado.
- Buscar en todo el módulo otros `==`/`!=`/`switch` sobre errores o interfaces con operandos no nil y
  migrarlos a `IsNoRows` o a una comparación concreta.

## 4. Tests (rojo primero)

- `tests/`: `IsNoRows(storage.ErrNoRows)` → true; `IsNoRows(nil)` → false;
  `IsNoRows(fmt.Err("otro"))` → false; `storage.ErrNoRows.Error()` == `<texto actual>`.
- La conformance de `mem` (`tests/mem_conformance_test.go`) sigue verde con `IsNoRows`.
- `gotest` verde (vet, race, tests, wasm).

## 5. Criterios de aceptación

- `grep -rn 'errors.Is\|== storage.ErrNoRows\|== ErrNoRows' --include=*.go .` → vacío (fuera de
  comentarios).
- Único símbolo exportado nuevo: `IsNoRows`.
- `docs/` y `README.md` mencionan `IsNoRows` donde hoy mencionan `ErrNoRows`.
- `gotest` verde.

## 6. Restricciones

Las de `AGENTS.md`, más: nada de `reflect`, nada de `unsafe`, ningún `==`/`!=`/`switch` entre
valores de interfaz con operandos no nil. No tocar otros repos: `postgres`, `sqlite`, `indexdb`, etc.
solo **devuelven** `storage.ErrNoRows`, y eso sigue siendo correcto.
