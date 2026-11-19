# ADR 0001 — Arquitectura hexagonal y dinero entero

## Contexto

Un ledger de pagos mezcla reglas de negocio (saldos, monedas, asientos) con
I/O (disco, HTTP, Postgres). Si esas reglas viven en los adaptadores, los
invariantes no se pueden probar en aislamiento ni reutilizar en el simulador
de fallos.

## Decision

El dominio (`internal/domain`) no importa `os`, red ni drivers. Los casos de
uso (`internal/app`) orquestan puertos: reloj, identidad, unidad de trabajo.
Los adaptadores implementan esos puertos.

El dinero es un par `(moneda, int64)` en unidades menores. Esta prohibido
usar flotantes para montos, saldos o limites. No hay conversion implicita
entre monedas: una transferencia con monedas distintas es un error tipado.

## Consecuencias

Los tests de dominio y el harness de fallos corren sobre la misma maquina de
estados. El precio es que cada adaptador debe traducir al modelo del dominio
sin "atajos" de persistencia que rompan la doble partida.
