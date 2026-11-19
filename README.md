# cortex9-payments-ledger

Libro mayor de pagos de doble partida, pensado como el nucleo de un wallet
al estilo Mercado Pago: dinero en unidades menores enteras, asientos
inmutables, transferencias intra-shard atomicas y transferencias entre
shards con saga, outbox transaccional e idempotencia.

Es un proyecto de estudio con invariantes comprobables. No es un procesador
de pagos en produccion y no habla con redes de tarjetas ni con Mercado Pago.

Licencia Apache-2.0. Go 1.23, biblioteca estandar en el dominio; `pgx` solo
en el adaptador opcional de Postgres.

## Garantias (cuando los tests y el verificador estan en verde)

- Toda magnitud de dinero es `int64`. No hay flotantes en montos ni saldos.
- No hay tipo de cambio implicito: una transferencia exige la misma moneda.
- Cada asiento esta balanceado (debitos = creditos por moneda). El verificador
  tambien chequea la igualdad global y `activos = pasivos` por moneda.
- Los asientos son inmutables. Una reversa es un asiento nuevo compensatorio.
- El diario de cada shard esta hash-encadenado (SHA-256 canonico). Una
  adulteracion de payload o de orden la detecta el verificador.
- Un wallet no queda bajo `-limite`. Limite 0 = sin descubierto.
- Toda comando aceptado lleva clave de idempotencia. Reenviar la misma clave
  y el mismo payload devuelve el resultado original. Un payload distinto con
  la misma clave es un error tipado (`ErrIdempotencyConflict`).
- El outbox y el estado se commitean juntos. El consumidor usa inbox: un
  mensaje duplicado no vuelve a postear. En una corrida cerrada (outbox
  vacio), toda saga queda en estado terminal (`Settled` o `Released`).
- Un commit con version vieja no pisa el estado: `ErrConflict` y reintento
  del mismo comando.

## Lo que NO se garantiza

- Exactly-once de punta a punta hacia un sistema externo que no sea
  idempotente.
- Durabilidad si el disco miente sobre `fsync`.
- Un cluster multi-nodo, consenso (Raft/Paxos) ni particiones de red reales.
- Un motor de FX, liquidacion, KYC ni integraciones de Mercado Pago.
- Que el modelo TLA+ cubra el codigo Go: cubre el protocolo (saga,
  idempotencia, reenvio). El puente es el CONTRACT y el harness.
- Que el harness de fallos sea exhaustivo. Es un muestreo grande,
  determinista y reproducible, no una prueba formal de todo el espacio.
- El adaptador de Postgres persiste un snapshot transaccional con CAS de
  generacion. No es (todavia) un esquema normalizado de asientos y lineas.

## Como se usa

```bash
# servidor HTTP (memoria, o -file ledger.bin para el almacen append-only)
go run ./cmd/ledgerd -addr :8080

# verificar un archivo
go run ./cmd/ledger verify ledger.bin
```

La API habla JSON y, en error de negocio, `application/problem+json` (RFC 9457).
Cabecera obligatoria en escrituras: `Idempotency-Key`.

```text
POST /accounts
POST /accounts/{id}/fund
POST /accounts/{id}/freeze
POST /transfers
POST /reversals
POST /flush          # entrega el outbox (el servidor tambien lo hace solo)
GET  /accounts/{id}
GET  /accounts/{id}/statement
GET  /sagas/{id}
GET  /verify?closed=1
GET  /health
```

## Arquitectura

```
cmd/ledgerd, cmd/ledger
internal/adapters/httpapi     RFC 9457
internal/adapters/memory      commit optimista por generacion
internal/adapters/filestore   append-only, CRC, fsync, cola rota
internal/adapters/postgres    snapshot transaccional (job de CI)
internal/app                  orquestacion y reintentos
internal/domain               maquina de estados pura, verificador
internal/sim                  harness determinista de fallos
spec/                         TLA+ + TLC
```

Las dependencias apuntan hacia adentro. El dominio no hace I/O.

## Resultados medidos

Numeros de una corrida real en esta maquina (2026-11-19, Go 1.27.3, linux/amd64).
No son una promesa de rendimiento en el tuyo, ni un predictor de contratacion.

| Que | Resultado |
| --- | --- |
| Tests unitarios y de integracion, `go test -race -count=1 -short ./...` | 29 PASS, 2 SKIP (`TestFaultInjection` se saltea con `-short`; `TestPostgresTransferAndIdempotency` se saltea sin `DATABASE_URL`) |
| Estres de transferencias concurrentes (32 workers x 20, `-race`) | conservacion y verificador en verde, sin lost updates |
| Almacen de archivo + cola rota | recupera el ultimo snapshot bueno, trunca y sigue escribiendo |
| Harness de fallos | 2000 semillas, 80 pasos, crashes / duplicados / reorden / demoras; 4m19.552s; 0 fallos. Una semilla se reproduce con `go test ./internal/sim -count=1 -args -seed=N` |
| TLC `Ledger.cfg` (Amounts={1,2}, InitBal=2) | 9841 generados, 345 distintos, profundidad 7, 0 errores, ~0.81 s |
| TLC `Ledger-large.cfg` (Amounts={1,2,3}, InitBal=5) | 41665 generados, 929 distintos, profundidad 7, 0 errores, ~1.05 s |
| `BenchmarkIntraTransfer` | 3751 ns/op, 2682 B/op, 47 allocs/op |
| `BenchmarkHash` | 620.6 ns/op, 312 B/op, 23 allocs/op |
| `BenchmarkVerify` (128 asientos) | 109969 ns/op, 43352 B/op, 3096 allocs/op |
| Fuzz `ComputeHash` (5–6 s) | 833169 execs, 100 interesting, 0 crashes |
| Fuzz `DecodeOpen` (5 s) | 708078 execs, 324 interesting, 0 crashes |
| Fuzz `DecodeTransfer` (5 s) | 688243 execs, 306 interesting, 0 crashes |
| Mutation testing (`gremlins unleash ./internal/domain`, v0.5.1) | 116 killed, 0 lived, 11 not covered; eficacia 100.00 %, cobertura 91.34 %, 9.288 s. Umbral declarado: 80 % / 80 %. Detalle en `docs/MUTATION.md` |

El harness de 2000 semillas usa el mismo formato de record que el archivo
durable, pero abre el almacen con `fsync` desactivado para entrar en el tope
de tiempo de CI. Los tests de cola rota y el `Open` por defecto si hacen
`fsync` en cada commit.

## Como hablar de esto en una entrevista

No recites este README. Elige tres o cuatro decisiones y defiendelas.

1. **Por que doble partida y no un `UPDATE saldo`.** Un wallet es un pasivo.
   El diario es la fuente de verdad; el saldo se deriva de debitos y creditos.
   Asi el verificador puede reconstruir todo y una reversa no reescribe el
   pasado.
2. **Por que no hay FX implicito.** Convertir ARS a USD en silencio inventa
   un tipo de cambio y rompe la conservacion por moneda. Un FX, si existiera,
   seria un asiento explicito contra una cuenta de posicion.
3. **Intra-shard vs saga.** Si las dos cuentas viven en el mismo diario, un
   asiento atomico basta. Si no, reserve → capture → settle (o release), con
   outbox en la misma transaccion e inbox en el consumidor. El at-least-once
   del transporte se vuelve un efecto de una sola vez en el mayor.
4. **Versionado optimista, no un lock global de negocio.** Un conflicto no
   crea un segundo asiento: se relee y se reaplica el mismo comando, o se
   devuelve el resultado de idempotencia si el primero ya habia ganado.
5. **TLA+ y el harness no se sustituyen.** TLC explora un modelo acotado del
   protocolo de forma exhaustiva. El harness pisa miles de semillas sobre el
   codigo Go, con crashes y reorden, y es reproducible. Ninguno de los dos
   prueba al otro; juntos cubren mas que un test de ejemplo.
6. **Limites que conviene decir en voz alta.** Exactamente-una-vez no cruza
   un sistema externo no idempotente. fsync depende del disco. El adaptador
   de Postgres, en este corte, es un snapshot transaccional, no el esquema
   final de un core bancario.

Si te piden un fallo: conta el camino `Released` (destino congelado), o como
una cola de archivo cortada se trunca y no se come el prefijo bueno, o como
una carrera de dos transferencias sobre las mismas cuentas no pierde plata.

Los ADRs en `docs/adr/` son el mapa de esas decisiones.

## Tests locales

```bash
go test -race -count=1 -short ./...
go test -count=1 -timeout 8m ./internal/sim          # 2000 semillas
go test -bench=. -benchmem ./internal/domain
./scripts/tla-check.sh                                # requiere Java 21
```

`CONTRACT.md` es la fuente de verdad del alcance. Si una transaccion rompe un
invariante del verificador, no se "arregla" relajando el invariante.
