# Especificacion TLA+ del protocolo

`Ledger.tla` modela la saga, la idempotencia y el reenvio de mensajes entre
dos cuentas. El dinero en transito se representa con `held`, que es una
abstraccion de las cuentas `transit:` e `incoming:` del codigo Go. Las
reglas de estados (`reserved`, `settled`, `released`), de clave de
idempotencia y de inbox coinciden con el CONTRACT.

## CI (modelo acotado)

```text
constantes: 2 cuentas, montos {1,2}, saldo inicial 2, 2 claves
invariantes: TypeOK, conservacion, sin descubierto,
             sagas terminales cuando no hay mensajes pendientes,
             a lo sumo un efecto por clave
```

Herramienta: `tla2tools.jar` de la release estable **v1.7.4**
(`936a262061c914694dfd669a543be24573c45d5aa0ff20a8b96b23d01e050e88`).
No se usa v1.8.0: el artefacto de ese tag se republico varias veces.

```bash
./scripts/tla-check.sh
```

## Resultados medidos (2026-11-19, TLC 2.19, tla2tools v1.7.4, Temurin 21)

Modelo CI (`spec/Ledger.cfg`, Amounts={1,2}, InitBal=2):

- 9841 estados generados, 345 distintos, profundidad 7, 0 errores, ~0.81 s

Modelo grande (`spec/Ledger-large.cfg`, Amounts={1,2,3}, InitBal=5):

- 41665 estados generados, 929 distintos, profundidad 7, 0 errores, ~1.05 s

Ambos se corren en CI (`./scripts/tla-check.sh`). La profundidad 7 es el
diametro completo, no un corte artificial: el protocolo de una saga es corto
(reserve / capture|release / ack).
