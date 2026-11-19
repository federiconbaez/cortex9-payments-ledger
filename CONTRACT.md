# CONTRACT — cortex9-payments-ledger

## Proposito

Un libro mayor de pagos de doble partida, en el estilo de un wallet/ledger de
un procesador como Mercado Pago: dinero en unidades menores enteras, asientos
inmutables, transferencias locales atomicas y transferencias entre shards con
saga, outbox transaccional e idempotencia. Es un proyecto de estudio con
invariantes comprobables, no un procesador de pagos en produccion.

## Alcance

Incluye:

- Dominio puro (sin I/O) y arquitectura hexagonal.
- Cuentas de pasivo (wallets) y de activo (clearing / incoming), con limite de
  descubierto opcional.
- Diario hash-encadenado por shard, asientos inmutables y reversas como
  asientos compensatorios.
- Clave de idempotencia en cada comando, con conflicto tipado si el payload
  no coincide.
- Sagas `reserve -> capture -> settle` o `release`, outbox transaccional y
  consumidores con inbox (efecto de una sola vez ante reintentos).
- Versionado optimista por cuenta y pruebas de estres con `-race`.
- Verificador continuo de invariantes.
- Harness determinista de fallos (crash, duplicacion, reorden, delay).
- Especificacion TLA+ del protocolo de saga e idempotencia, con TLC en CI.
- Adaptadores: memoria, archivo append-only con fsync y corte de cola rota,
  API HTTP con RFC 9457. Postgres (pgx) solo si su job de CI esta verde.

No incluye:

- Tipo de cambio implicito ni un motor de FX.
- Liquidacion con redes de tarjetas, KYC, ni integraciones reales de Mercado
  Pago.
- Consenso distribuido multi-nodo (Raft/Paxos) ni un cluster real.
- Garantia de durabilidad frente a un disco que miente sobre fsync.
- Exactly-once de punta a punta hacia un sistema externo no idempotente.

## Modelo de dinero

- Toda magnitud es `int64` en unidades menores (centavos, etc.). Esta prohibido
  `float32`/`float64` para dinero.
- Cada cuenta y cada linea tienen moneda. Una transferencia exige la misma
  moneda en origen, destino y monto. No hay FX implicito.
- Convencion de saldo:
  - Pasivo (wallet, transito, due-to): `saldo = creditos - debitos`.
  - Activo (clearing, incoming): `saldo = debitos - creditos`.
- Un asiento es valido solo si, por moneda, la suma de debitos iguala la suma
  de creditos.
- Un wallet no puede quedar bajo `-limite`. Limite 0 significa sin descubierto.
  Las cuentas de sistema no aplican este tope.

## Asientos y cadena

- Los asientos son inmutables. Una reversa es un asiento nuevo que invierte
  las lineas y referencia al original. No se borra ni se edita el original.
- Cada shard tiene una cadena: `hash = SHA-256(prevHash || canonico(asiento))`.
  El `prevHash` genesis es 32 bytes nulos.
- Alterar un asiento o el orden de la cadena es detectable por el verificador.

## Idempotencia

- Todo comando lleva `Idempotency-Key`.
- Misma clave y mismo fingerprint de payload: se devuelve el resultado
  original, sin un segundo efecto.
- Misma clave y payload distinto: `ErrIdempotencyConflict` (error tipado).
- El fingerprint cubre el tipo de comando y los campos semanticos (cuentas,
  monto, moneda, asiento referenciado), no metadatos irrelevantes.

## Sagas y outbox

- Misma shard: un solo asiento atomico (debito origen, credito destino).
- Distinta shard:
  1. `Reserve` en el origen: debito al pagador, credito a `transit:{saga}`.
  2. Outbox `CapturePayee`. El consumidor, via inbox, hace `Capture` en el
     destino: debito a `incoming:{saga}`, credito al beneficiario.
  3. Outbox `AckCaptured`. El origen marca la saga `Settled`.
  4. Si el capture no puede aplicarse (cuenta inexistente, moneda distinta,
     o politica de rechazo), outbox `ReleasePayer` y el origen revierte el
     transito: debito a `transit:{saga}`, credito al pagador. Terminal:
     `Released`.
- El estado y el outbox se persisten en la misma transaccion.
- El consumidor registra el id del mensaje en el inbox antes de aplicar, o
  en la misma transaccion que el asiento. Un mensaje duplicado no vuelve a
  postear.
- Estados terminales: `Settled`, `Released`, `Failed`.

## Concurrencia

- Cada cuenta tiene un `Version` que incrementa en cada mutacion.
- Un commit con version vieja es `ErrConflict` y se reintenta el comando
  (el de idempotencia, no un segundo efecto).
- Esta prohibido actualizar saldos sin pasar por un asiento.

## Invariantes (verificador)

1. En cada asiento, y en el mayor global, debitos = creditos por moneda.
2. Ningun wallet queda bajo su limite.
3. Toda saga conocida esta en estado terminal al final de una corrida
   cerrada (o sigue pendiente solo si aun hay mensajes en outbox/inbox por
   entregar: entonces no es una corrida cerrada).
4. La cadena de cada shard es valida (prevHash y hash).
5. La ecuacion `suma(activos) = suma(pasivos)` se cumple por moneda.
6. Un comando aceptado no duplica su efecto bajo reintentos, duplicados o
   crashes posteriores a un commit.

## Fallos simulados

El harness es determinista y se reproduce con la semilla. Inyecta crashes
despues de un commit, duplicacion, reorden y demora de mensajes, nunca
"pierde" un mensaje commiteado (el outbox se reentrega). Una semilla que
falla debe volver a fallar con el mismo numero.

## Criterios de aceptacion

- `go test -race ./...` verde, incluido el estres de versiones y el
  verificador.
- Harness de fallos con miles de semillas y tope de tiempo en CI; la semilla
  de fallo se imprime.
- TLC sobre un modelo pequeno en CI, con `tla2tools.jar` pinneado por SHA-256.
- API HTTP responde problemas RFC 9457.
- El archivo sobrevive una cola rota (record incompleto) y reabre el estado.
- README honesto: garantias, no-garantias y una tabla de numeros realmente
  medidos.

## Fuera de contrato

Cualquier cambio que introduzca flotantes para dinero, un FX silencioso, un
force-push, secretos en el repo, o que relaje un invariante del verificador
para "hacer pasar" una semilla, se rechaza.
