# Cómo maneja la hora una reserva

## La regla corta

Una reserva guarda **un instante** y, aparte, **cómo se veía ese instante en el reloj de la clínica**:

| Campo | Qué guarda | Ejemplo |
|---|---|---|
| `reservation_time` | el instante exacto, en segundos desde 1970 en UTC | `1563240600` |
| `reservation_date` | la medianoche UTC de ese día (para buscar por día) | `1563235200` |
| `local_string_date` | la fecha como la vio la clínica | `2019-07-15` |
| `local_string_time` | la hora como la vio la clínica | `21:30` |

El instante es el mismo en todo el mundo y es el que se usa para comparar, ordenar y calcular. Los textos locales son
para mostrar y buscar "las reservas del 15 de julio" sin hacer cuentas.

## Por qué UTC y por qué `webtyp.com/time`

- **UTC** no cambia con el horario de verano ni con el país del servidor: dos instantes se comparan con un `<`.
- **`webtyp.com/time`** es la única librería de tiempo permitida: el código también corre en el navegador
  (WebAssembly) y la librería `time` estándar de Go agrega mucho peso al archivo `.wasm`. `webtyp/time` usa la del
  sistema en el servidor y el reloj del navegador en el cliente, con el mismo código.

## Cuidado: el horario de verano de Chile

Chile usa UTC-3 en verano y UTC-4 en invierno. `webtyp/time` convierte en las dos direcciones de forma distinta:

| Conversión | Función | ¿Respeta el horario de verano? |
|---|---|---|
| hora local → instante | `LocalMinutesToUnixUTC(fecha, minutos, "America/Santiago")` | **Sí**: usa la zona horaria real |
| instante → texto local | `FormatDate(u)`, `FormatTime(u)` | **No**: usa un solo desfase, el del sistema cuando arrancó |

Por eso:

- **Al crear una reserva** (`CreateReservation`) la hora sale del calendario de hoy, así que el desfase de hoy es el
  correcto.
- **Al importar historia** (`ImportReservation`) el sistema anterior ya trae la fecha y la hora locales verdaderas: se
  **conservan** tal cual, y el instante se calcula con `LocalMinutesToUnixUTC` y la zona `America/Santiago`. Solo si
  vienen vacías se calculan con `FormatDate`/`FormatTime`, sabiendo que pueden errar en una hora.

Ejemplo: una reserva del **15-07-2019 a las 21:30** (invierno, UTC-4) es el instante **16-07-2019 01:30 UTC**. Si se
importara en verano (UTC-3) y se recalculara el texto, quedaría "22:30": una hora corrida. Conservando el texto
original, queda "21:30", que es lo que realmente pasó.

## Si el servidor no tiene zona horaria

Un servidor en Docker sin la variable `TZ` arranca en UTC, y entonces `FormatDate`/`FormatTime` dan la hora UTC. La
instalación debe definir `TZ=America/Santiago`. Que la librería tome la zona de la instalación en vez de la del
sistema está anotado como mejora en `webtyp/time`.
