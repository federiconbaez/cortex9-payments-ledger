# ADR 0003 — Idempotencia y conflicto de payload

## Contexto

Pagos, reintentos de red y consumidores de cola reenvian el mismo comando.
"Reintentar" no puede crear un segundo asiento. Tampoco puede un cliente
reutilizar una clave con otro monto y que el servidor finja que es lo mismo.

## Decision

Cada comando lleva una clave de idempotencia y un fingerprint canonico del
payload semantico. El registro se escribe en la misma transaccion que el
asiento (o que el rechazo durable, si se decide persistir rechazos).

- Clave nueva: se ejecuta y se guarda `(clave, fingerprint, resultado)`.
- Misma clave, mismo fingerprint: se devuelve el resultado guardado.
- Misma clave, fingerprint distinto: `ErrIdempotencyConflict`.

El fingerprint no incluye marcas de tiempo ni ids generados por el servidor.

## Consecuencias

El at-least-once del transporte se vuelve un efecto de una sola vez en el
ledger. El cliente debe no reciclar claves. El almacenamiento de
idempotencia crece con los comandos y, en este proyecto, no tiene TTL.
