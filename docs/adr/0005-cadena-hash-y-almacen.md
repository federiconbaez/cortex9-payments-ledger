# ADR 0005 — Cadena de hash y almacen durable

## Contexto

Un diario append-only puede corromperse por una colision de bits, un bug
que reescribe un asiento, o una cola de archivo cortada a mitad de un
record. Hay que detectarlo y, en el caso de la cola rota, recuperarse.

## Decision

Cada shard mantiene una cadena SHA-256: el asiento N incluye el hash de
N-1 y su propio hash se calcula sobre una forma canonica (campos en orden,
sin JSON). El verificador recorre la cadena.

El adaptador de archivo escribe records `{len u32 LE}{crc32 IEEE}{payload}`.
Un record final incompleto o con CRC invalido se trunca (torn tail) y se
reprocesa el prefijo bueno. Cada transaccion (asientos + outbox + inbox +
idempotencia + cuentas) es un record y se hace `fsync` antes de confirmar.

El adaptador en memoria sirve para tests. Postgres, si esta verde en CI,
usa una transaccion SQL real para el mismo agregado.

## Consecuencias

La cadena detecta adulteraciones; no es una blockchain publica ni un ancla
timestamped. fsync depende del contrato del sistema operativo y del disco.
La cola rota se repara; un CRC valido con payload semanticamente mentiroso
solo lo pesca el verificador de invariantes, no el CRC.
