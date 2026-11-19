# ADR 0002 — Doble partida, tipos de cuenta y reservas

## Contexto

Hay que bloquear fondos en un pagador (reserve) sin creditar al beneficiario
hasta el capture, y las transferencias entre shards no pueden ser un solo
asiento atomico si cada shard tiene su propio diario.

## Decision

Toda cuenta tiene un tipo:

- Pasivo (wallet, `transit:{saga}`, `due-to`): saldo = creditos - debitos.
- Activo (clearing, `incoming:{saga}`): saldo = debitos - creditos.

Un asiento local de transferencia es `Debit pagador` + `Credit beneficiario`.
Una reserva cross-shard es `Debit pagador` + `Credit transit:{saga}` en el
shard origen. El capture es `Debit incoming:{saga}` + `Credit beneficiario`
en el shard destino. La ecuación `activos = pasivos` se mantiene: el transito
(pasivo) y el incoming (activo) nacen pareados conceptualmente cuando la saga
llega a `Settled`.

Las reversas no editan el asiento original: publican un asiento nuevo con las
lineas invertidas y `Reverses` apuntando al original.

Los wallets tienen limite de descubierto (>= 0). Las cuentas de sistema no
aplican ese tope.

## Consecuencias

El verificador puede chequear cada asiento, la ecuación global y que un
wallet no quede bajo su limite. El dinero en vuelo queda visible en cuentas
`transit:` e `incoming:`, no en un campo paralelo fuera del diario.
