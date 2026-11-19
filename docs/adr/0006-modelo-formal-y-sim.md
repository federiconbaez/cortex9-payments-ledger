# ADR 0006 — Modelo formal y simulacion de fallos

## Contexto

Los tests de ejemplo no alcanzan para colisiones de reintento, reorden de
mensajes y crashes. Hace falta un modelo pequeno exhaustivo y un espacio
grande muestreado, ambos reproducibles.

## Decision

- Una especificacion TLA+ (`spec/Ledger.tla`) del protocolo de saga e
  idempotencia, model-checked con TLC. En CI corre un modelo acotado
  (pocas cuentas, pocos comandos, mensajes duplicables). Corridas mas
  grandes se documentan en `spec/README.md` si el tiempo no entra en CI.
- `tla2tools.jar` se descarga de la release estable v1.7.4 de tlaplus y se
  verifica un SHA-256 pinneado. No se usa v1.8.0: ese artefacto se
  republico varias veces con el mismo tag.
- Un harness determinista, en Go, pisa miles de semillas con crashes,
  duplicados, reorden y demoras. Si una semilla falla, el test imprime el
  numero y `go test -run ... -args -seed=N` la reproduce.

## Consecuencias

TLC no prueba el codigo Go; prueba el protocolo. El harness no es
exhaustivo; cubre mucho mas que un test de ejemplo y es reproducible. El
puente entre ambos es el CONTRACT: los mismos estados terminales y las
mismas reglas de idempotencia.
