# Reserva en 6 pasos — servidor

Cada rombo lleva un id. La tabla de trazabilidad de [`docs/PLAN.md`](../PLAN.md) §11 asigna a
cada rama su prueba. Una rama sin prueba es un defecto.

## 1. Precio congelado en la reserva (`priceFor`, lo usan crear, sobrecupo y online)

```mermaid
flowchart TD
    A[config, servicio] --> P1{P1: ¿Catalog.ServiceCard falla?}
    P1 -- sí --> E[devolver el error]
    P1 -- no --> P2{P2: ¿PriceOverride > 0?}
    P2 -- sí --> OV[precio = PriceOverride]
    P2 -- no --> LIST[precio = precio de lista]
    OV --> P3{P3: ¿moneda del catálogo vacía?}
    LIST --> P3
    P3 -- sí --> CLP[moneda = CLP]
    P3 -- no --> CUR[moneda = la del catálogo]
```

## 2. `ListBookableServices(tenant, especialidad)`

"No existe" se pregunta con `StaffExists`/`ServiceExists`, que lo responden como
`(false, nil)`: el `ErrNotFound` de otro módulo es de otro tipo y no se puede
comparar aquí.

```mermaid
flowchart TD
    A[configs activas del tenant] --> B1{B1: ¿error de base?}
    B1 -- sí --> E0[error]
    B1 -- no --> L[para cada config]
    L --> B2{B2: ¿StaffExists / StaffCard?}
    B2 -- no existe --> SKIP1[omitir]
    B2 -- otro error --> E1[error]
    B2 -- ok --> B3{B3: ¿profesional activo y<br>especialidad coincide o filtro vacío?}
    B3 -- no --> SKIP2[omitir]
    B3 -- sí --> B4{B4: ¿ServiceExists / ServiceCard?}
    B4 -- no existe --> SKIP3[omitir]
    B4 -- otro error --> E2[error]
    B4 -- ok --> B5{B5: ¿servicio activo?}
    B5 -- no --> SKIP4[omitir]
    B5 -- sí --> ADD[agregar BookableService<br>con priceFor §1]
    ADD --> ORD[ordenar por especialidad,<br>servicio, profesional]
```

## 3. `ListServiceAvailability(tenant, servicio, profesional, desde, hasta, conTomadas)`

```mermaid
flowchart TD
    A[configs bookables del servicio §2] --> V1{V1: ¿profesional indicado?}
    V1 -- sí --> ONE[solo la config de ese profesional]
    V1 -- no --> ALL[todas, en el orden de §2]
    ONE --> V2{V2: ¿quedó alguna config?}
    ALL --> V2
    V2 -- no --> EMPTY[lista vacía]
    V2 -- sí --> LOOP[ListAvailability por config]
    LOOP --> V3{V3: ¿error?}
    V3 -- sí --> E[error]
    V3 -- no --> V4{V4: ¿conTomadas y<br>profesional indicado?}
    V4 -- sí --> TK[agregar horas del horario ya<br>reservadas, Taken = true]
    V4 -- no --> MRG
    TK --> MRG[unir por hora de inicio:<br>si dos profesionales tienen la misma,<br>queda el primero de la lista]
    MRG --> SORT[ordenar por inicio]
```

## 4. `CreateOnlineReservation(cmd)`

```mermaid
flowchart TD
    A[CreateOnlineReservation] --> O1{O1: ¿Payments configurado?}
    O1 -- no --> E0[ErrPaymentsUnavailable]
    O1 -- sí --> O2{O2: ¿retenciones online activas<br>del paciente ≥ MaxOnlineHolds?}
    O2 -- sí --> E1[ErrTooManyHolds]
    O2 -- no --> CR[CreateReservation con<br>Origin ONLINE, HoldUntil = ahora + OnlineHoldSeconds]
    CR --> O3{O3: ¿error de CreateReservation?}
    O3 -- sí --> E2[devolver el error<br>ErrSlotTaken, ErrIdTaken…]
    O3 -- no --> OK[reserva PENDING retenida]
```

"Retención activa" = reserva del paciente con `Origin ONLINE`, `Status PENDING` y
`HoldUntil > ahora`.

## 5. `Charge(tenant, reserva)` — lo llama payment_manager

```mermaid
flowchart TD
    A[Charge] --> C1{C1: ¿reserva del tenant?}
    C1 -- no --> NF[ErrNotFound]
    C1 -- sí --> C2{C2: ¿PENDING o CONFIRMED,<br>sin PaymentId y precio > 0?}
    C2 -- no --> NP[payable = false]
    C2 -- sí --> C3{C3: ¿ONLINE?}
    C3 -- no --> YES[payable = true]
    C3 -- sí --> C4{C4: ¿quedan ≥ PaymentWindowSeconds<br>de retención?}
    C4 -- no --> NP2[payable = false]
    C4 -- sí --> YES2[payable = true]
    YES --> M{M1: ¿moneda?}
    YES2 --> M
    M -- CLP --> CLP[monto = entero del precio]
    M -- USD --> USD[monto = precio × 100 redondeado]
    M -- otra --> EC[ErrUnsupportedCurrency]
```

## 6. `ConfirmPaidReservation(tenant, reserva, pago)` — la llama la raíz de composición al recibir `payment.paid`

```mermaid
flowchart TD
    A[ConfirmPaidReservation] --> F1{F1: ¿Payments configurado?}
    F1 -- no --> E0[ErrPaymentsUnavailable]
    F1 -- sí --> F2{F2: ¿reserva del tenant?}
    F2 -- no --> NF[ErrNotFound]
    F2 -- sí --> F3{F3: ¿PaymentId?}
    F3 -- "igual a este pago" --> IDEM[nil: ya confirmada]
    F3 -- "otro pago" --> E1[ErrAlreadyPaid]
    F3 -- vacío --> F4{F4: ¿PaidAmount falla o<br>cubre menos que el precio?}
    F4 -- sí --> E2[ErrPaymentNotVerified]
    F4 -- no --> F5{F5: ¿estado?}
    F5 -- PENDING --> CF[CONFIRM con PaymentId,<br>HoldUntil = 0]
    F5 -- CONFIRMED --> SET[guardar PaymentId]
    F5 -- otro --> E3[ErrReservationNotPayable<br>la raíz devuelve el pago]
    CF --> F6{F6: ¿revisión coincide?}
    SET --> F6
    F6 -- no --> E4[ErrConflict]
    F6 -- sí --> EV[publicar evento de confirmación]
```

## 7. `ExpirePendingReservations(tenant, ahora)`

```mermaid
flowchart TD
    A[reservas PENDING del tenant] --> X1{X1: ¿hora de la cita ya pasó?}
    X1 -- sí --> EXP[EXPIRE]
    X1 -- no --> X2{X2: ¿HoldUntil > 0 y<br>HoldUntil < ahora?}
    X2 -- sí --> EXP
    X2 -- no --> KEEP[queda]
```
