# ADR 0004 — Sagas entre shards y outbox transaccional

## Contexto

Dos cuentas en shards distintas no pueden cerrarse en un solo commit local.
Si el asiento se publica "al lado" de la transaccion, un crash deja un
agujero: dinero reservado sin mensaje, o mensaje sin reserva.

## Decision

Cross-shard usa una saga:

1. Reserve en origen + outbox `CapturePayee` (misma transaccion).
2. Consumidor idempotente (inbox) aplica Capture en destino + outbox
   `AckCaptured`, o `ReleasePayer` si no puede aplicar.
3. El origen, al ver `AckCaptured`, pasa a `Settled`. Al ver
   `ReleasePayer`, publica el asiento compensatorio y pasa a `Released`.

El outbox se reentrega hasta acuse. El inbox garantiza que un mensaje
duplicado no vuelve a postear. Una transferencia intra-shard no abre saga:
un solo asiento atomico basta, tambien bajo clave de idempotencia.

El commit de cuentas usa versionado optimista. Un conflicto no genera un
segundo asiento: se relee y se reaplica el mismo comando, o se devuelve el
resultado de idempotencia si el primero ya habia ganado.

## Consecuencias

Se obtiene at-least-once en el transporte y exactly-once en el efecto,
siempre que todos los participantes usen inbox e idempotencia. No se
pretende exactly-once hacia un sistema externo que no sea idempotente.
