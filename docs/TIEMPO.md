# Cómo maneja la hora una reserva

## La regla: un solo instante en UTC

Una reserva guarda **únicamente el instante UTC** en que ocurre:

| Campo | Qué guarda | Ejemplo |
|---|---|---|
| `reservation_time` | el instante exacto, en segundos desde 1970 en UTC (`int64`) | `1563240600` |

Se eliminaron los campos derivados y redundantes (`reservation_date`, `local_string_date`, `local_string_time`). La fecha y hora local se derivan bajo demanda utilizando la zona horaria inyectada en el módulo (`m.zone.Date(u)` y `m.zone.Clock(u)`).

## Inyección explícita de `time.Zone`

- El módulo recibe `Deps.Zone` (`time.Zone`) obligatoria en `New()`.
- La UI recibe `zone time.Zone` en `Screens` y `NewBookingView`.
- Se usa exclusivamente `webtyp.com/time` importado directamente como `time` (sin alias `tinytime`).
- La conversión entre minutos locales de un día y el instante UTC exacto se realiza con `m.zone.At(date, minutes)`.
- El horario de verano de Chile (UTC-3 / UTC-4) es manejado dinámicamente por la zona IANA `America/Santiago` de `webtyp.com/time`.
