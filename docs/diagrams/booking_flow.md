# Diagrama de Flujo de Reserva (Booking Flow) — appointment_booking

Diagrama del embudo de agendamiento en 6 pasos progresivos tipo Acordeón/Wizard.

## 1. Arquitectura del Flujo Progresivo

```mermaid
flowchart TD
    subgraph S1["Paso 1: Especialidad & Servicio"]
        Area["Seleccionar Especialidad"] --> Svc["Seleccionar Servicio<br/>(fija duración y precio)"]
    end

    subgraph S2["Paso 2: Profesional"]
        Svc --> Staff["Seleccionar Profesional<br/>o 'Cualquiera disponible'"]
    end

    subgraph S3["Paso 3: Fecha & Hora"]
        Staff --> Cal["Seleccionar Día en Calendario"]
        Cal --> Slots["Seleccionar Horario Disponible<br/>(Chips táctiles: 09:00, 09:30...)"]
    end

    subgraph S4["Paso 4: Resumen de Cita"]
        Slots --> Summary["Revisar Detalles de Cita<br/>+ Notas / Motivo de consulta"]
    end

    subgraph S5["Paso 5: Identificación / Registro"]
        Summary --> CheckMode{"¿Canal de Reserva?"}
        CheckMode -->|Online Autenticado| PatientAuth["Pre-cargar datos de sesión"]
        CheckMode -->|Online Anónimo / Nuevo| PatientReg["Formulario de Registro Tardío:<br/>RUT, Nombre, Email, Teléfono"]
        CheckMode -->|Mostrador / Recepción| PatientRUT["Búsqueda Reactiva por RUT<br/>(patient_directory)"]
        PatientRUT -->|No existe| PatientModal["Modal: + Nuevo Paciente"]
        PatientRUT -->|Existe| PatientCard["Tarjeta de Paciente Encontrado"]
    end

    subgraph S6["Paso 6: Pago & Confirmación"]
        PatientAuth --> CheckPay{"¿PaymentRequired?"}
        PatientReg --> CheckPay
        PatientModal --> CheckPay
        PatientCard --> CheckPay
        CheckPay -->|Sí| PayStep["Renderizar PaymentHandler<br/>(Interfaz inyectada)"]
        PayStep -->|Pago Exitoso| ConfirmWithPay["Confirmar Reserva con PaymentID"]
        CheckPay -->|No| DirectConfirm["Confirmar Reserva Directa"]
    end
```

## 2. Secuencia Detallada (Paciente Online vs Recepcionista)

```mermaid
sequenceDiagram
    autonumber
    actor User as Usuario (Paciente o Recepcionista)
    participant UI as BookingAccordion UI
    participant AB as appointment_booking
    participant PD as patient_directory
    participant Pay as PaymentHandler (Inyectado)

    User->>UI: 1. Selecciona Especialidad y Servicio
    UI->>UI: Contrae Paso 1, abre Paso 2 (calcula staff filtrado)
    User->>UI: 2. Selecciona Profesional
    UI->>UI: Contrae Paso 2, abre Paso 3
    User->>UI: 3. Elige Fecha en Calendario
    UI->>AB: FreeSlots(staff, config, date)
    AB-->>UI: Retorna slots libres ["09:00", "09:30", "10:30"]
    User->>UI: Toca chip de hora "09:30"
    UI->>UI: Contrae Paso 3, abre Paso 4 (Resumen)
    User->>UI: 4. Ingresa Notas y pulsa Continuar
    UI->>UI: Contrae Paso 4, abre Paso 5 (Identificación)

    alt Modo Mostrador / Recepcionista
        User->>UI: Ingresa RUT
        UI->>PD: Buscar paciente por RUT
        alt Paciente no existe
            UI->>User: Muestra botón "+ Registrar Paciente"
            User->>UI: Abre modal, completa datos y guarda
            UI->>PD: OpCreatePatient
        end
    else Modo Online (Fricción Cero)
        alt Paciente no autenticado / no registrado
            User->>UI: Completa RUT, Nombre, Apellido, Email, Teléfono
            UI->>PD: OpCreatePatient o vinculación
        else Paciente con sesión
            UI-->>User: Muestra datos ya confirmados
        end
    end

    UI->>UI: Contrae Paso 5, abre Paso 6 (Pago)
    alt PaymentRequired == true
        UI->>Pay: RenderPayment(monto, moneda)
        User->>Pay: Efectúa pago
        Pay-->>UI: Retorna paymentID
        UI->>AB: OpCreateReservation / OpChangeReservationStatus(CONFIRM, paymentID)
    else PaymentRequired == false
        UI->>AB: OpCreateReservation (CONFIRMED o PENDING)
    end
    UI-->>User: Despliega Voucher / Comprobante de Reserva
```
