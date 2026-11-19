# Mutation testing

Herramienta: `gremlins` v0.5.1 (`github.com/go-gremlins/gremlins`), corrida
real el 2026-11-19 sobre `./internal/domain` (el nucleo puro, sin I/O).

```bash
gremlins unleash ./internal/domain --workers 4 --test-cpu 1
```

Resultado medido (9.288 s):

| Metrica | Valor |
| --- | --- |
| Killed | 116 |
| Lived | 0 |
| Not covered | 11 |
| Timed out / not viable / skipped | 0 / 0 / 0 |
| Test efficacy (killed / killed+lived) | 100.00 % |
| Mutator coverage (killed+lived / total) | 91.34 % |

Umbral declarado para este paquete: eficacia >= 80 % y cobertura de mutantes
>= 80 %. Esta corrida lo cumple. No se metio a la CI porque instala un
binario extra y alarga el job; el numero de arriba se puede rehacer en
local con el mismo comando.

Los 11 mutantes no cubiertos viven en ramas defensivas (aritmetica de
indices, IDs y un camino del verificador) que los tests actuales no pisan.
No se ajusto el umbral para esconderlos.
