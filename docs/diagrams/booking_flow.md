# Reserva en 6 pasos — pantalla

Dos pantallas con el mismo flujo (`ui/booking_flow.go`): **Reserva Hora** (mesón, `ui.Browser`)
y **Reserva online** (paciente invitado, `ui.OnlineBrowser`). Cada rombo lleva un id; la tabla
de trazabilidad de [`docs/PLAN.md`](../PLAN.md) §11 asigna a cada rama su prueba wasm.

## 1. Pasos (`stepaccordion.StepAccordion`)

```mermaid
flowchart TD
    S1["1 · Especialidad y servicio<br>chipgrid de especialidades + chipgrid de servicios<br>(list_bookable_services)"] --> S2
    S2["2 · Profesional<br>chipgrid: 'Cualquier profesional disponible' + los que ofrecen el servicio"] --> S3
    S3["3 · Fecha y hora<br>calendarslider con ocupación + chipgrid de horas<br>(list_service_availability)"] --> S4
    S4["4 · Paciente<br>mesón: RUT → ficha o modal de registro<br>online: RUT, nombre, correo, teléfono (link_patient)"] --> S5
    S5["5 · Resumen<br>profesional, servicio, día, hora, paciente, precio + notas<br>Continuar crea la reserva"] --> S6
    S6["6 · Pago y confirmación"]
```

Reabrir un paso (`Editar`) borra todo lo elegido en los pasos siguientes (`OnReopen`).
El resumen va después del paciente ("revise sus respuestas"): la reserva se crea
recién ahí, con todo a la vista (decisión del 2026-10-13). Servicio y profesional
se eligen con `chipgrid`. Las ramas M3/M4 y N3 (crear la reserva) ocurren en el
paso 5; M1/M2 y N1/N2 (elegir o vincular al paciente), en el 4.

## 2. Paso 1

```mermaid
flowchart TD
    A[Init] --> U1{U1: ¿list_bookable_services falló?}
    U1 -- sí --> ERR[mensaje visible + Reintentar]
    U1 -- no --> U2{U2: ¿hay servicios?}
    U2 -- no --> NONE[No hay servicios para reservar]
    U2 -- sí --> AREAS[chips de especialidades únicas]
    AREAS --> PICK[elegir especialidad → lista de servicios<br>nombre · duración · precio]
    PICK --> NEXT[elegir servicio → Advance 'Especialidad · Servicio']
```

## 3. Paso 2

```mermaid
flowchart TD
    A[servicio elegido] --> U3{U3: ¿lo ofrece más de un profesional?}
    U3 -- sí --> ANY[primera fila: Cualquier profesional disponible]
    U3 -- no --> ONLY[solo ese profesional]
    ANY --> P[elegir → Advance nombre o 'Cualquiera']
    ONLY --> P
```

## 4. Paso 3

```mermaid
flowchart TD
    A[abrir paso] --> U4{U4: ¿list_service_availability<br>del rango del calendario falló?}
    U4 -- sí --> ERR[mensaje visible]
    U4 -- no --> CAL[ocupación por día al calendario]
    CAL --> DAY[elegir día → horas del día en chipgrid]
    DAY --> U5{U5: ¿hay horas ese día?}
    U5 -- no --> EMPTY[No hay horas disponibles]
    U5 -- sí --> CHIP[elegir hora → Advance 'mar 13 oct · 10:30']
    CHIP --> U6{U6: ¿'Cualquiera'?}
    U6 -- sí --> RESOLVE[se fija el profesional de esa hora]
    U6 -- no --> KEEP[queda el elegido]
    DAY --> U7{U7: ¿mesón y casilla Sobrecupo?}
    U7 -- sí --> OB[chips incluyen horas tomadas;<br>exige profesional fijo]
    U7 -- no --> FREE[solo horas libres]
```

La casilla **Sobrecupo** solo existe en mesón.

## 5. Paso 5 — mesón

```mermaid
flowchart TD
    A[RUT + Buscar] --> M1{M1: ¿find_patient_by_rut?}
    M1 -- encontrado --> CARD[ficha: nombre, teléfono]
    M1 -- 404 --> NEW[botón Registrar paciente → modal<br>RUT, nombre, teléfono, correo]
    M1 -- otro error --> ERR1[mensaje visible]
    NEW --> M2{M2: ¿upsert_patient falló?}
    M2 -- sí --> ERR2[mensaje en el modal, sigue abierto]
    M2 -- no --> CARD
    CARD --> GO[Continuar]
    GO --> M3{M3: ¿Sobrecupo?}
    M3 -- sí --> OVB[create_overbooking]
    M3 -- no --> CRT[create_reservation, Origin COUNTER]
    OVB --> M4{M4: ¿falló?}
    CRT --> M4
    M4 -- "sí: hora tomada (409)" --> TAKEN[aviso y vuelve a abrir el paso 3]
    M4 -- "sí: otro" --> ERR3[mensaje visible]
    M4 -- no --> ADV[Advance nombre del paciente]
```

## 6. Paso 5 — online

```mermaid
flowchart TD
    A[RUT, nombre, correo, teléfono + Continuar] --> N1{N1: ¿el formulario valida?}
    N1 -- no --> INV[errores por campo]
    N1 -- sí --> LINK[link_patient]
    LINK --> N2{N2: ¿falló?}
    N2 -- sí --> ERR1[mensaje visible]
    N2 -- no --> HOLD[create_online_reservation]
    HOLD --> N3{N3: ¿falló?}
    N3 -- "hora tomada (409)" --> TAKEN[aviso y vuelve a abrir el paso 3]
    N3 -- "demasiadas retenciones (409)" --> MANY[aviso: ya tiene horas pendientes de pago]
    N3 -- "otro" --> ERR2[mensaje visible]
    N3 -- no --> ADV[Advance nombre]
```

## 7. Paso 6

```mermaid
flowchart TD
    A[abrir paso] --> Y1{Y1: ¿canal?}
    Y1 -- mesón --> Y2{Y2: ¿PaymentRequired?}
    Y2 -- no --> CONF[Confirmar reserva → change_reservation_status CONFIRM]
    CONF --> Y3{Y3: ¿falló?}
    Y3 -- sí --> ERR1[mensaje visible]
    Y3 -- no --> DONE[panel final: Reserva confirmada]
    Y2 -- sí --> PAYC[payment_manager/ui.PaymentStep mesón]
    PAYC --> PAID[OnPaid → get_reservation]
    PAID --> Y4{Y4: ¿CONFIRMED?}
    Y4 -- sí --> DONE
    Y4 -- no --> WAIT[Pago registrado; confirmando… reintenta 3 veces]
    Y1 -- online --> PAYO[payment_manager/ui.PaymentStep online<br>ReturnTo = OnlineID → sale a Webpay]
```

## 8. Vuelta de Webpay (`OnlineBrowser` al iniciar)

```mermaid
flowchart TD
    A[Init] --> R1{R1: ¿TakeReturn trae pago?}
    R1 -- no --> FLOW[flujo normal desde el paso 1]
    R1 -- sí --> R2{R2: ¿status?}
    R2 -- PAID --> OK[Reserva confirmada + resumen]
    R2 -- "CANCELLED / EXPIRED / FAILED" --> KO[No se completó el pago;<br>la hora se liberará + Empezar de nuevo]
    R2 -- ERROR u otro --> UNK[No pudimos confirmar el pago;<br>llame a la clínica]
```

## 9. Panel final

Muestra profesional, servicio, día, hora, paciente y, si hubo pago, el monto con
`paymentmanager.FormatAmount`. Botón **Nueva reserva** reinicia el flujo.
