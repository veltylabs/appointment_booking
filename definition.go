package appointmentbooking

import (
	"webtyp.com/input"
	"webtyp.com/model"
	"webtyp.com/time"
)

// Origin dice por qué vía se confirma una reserva, y por lo tanto de quién es
// el turno mientras está PENDING. No se deduce de creator_user_id: una
// inferencia miente en silencio el día que una reserva se cree sin usuario por
// otro motivo.
const (
	OriginCounter = "COUNTER" // la crea y la confirma el personal, en el mostrador
	OriginOnline  = "ONLINE"  // la crea el paciente y la confirma él mismo por su vía
)
const (
	OnlineHoldSeconds    time.Unix = 900 // la hora online queda retenida 15 min
	PaymentWindowSeconds time.Unix = 600 // mínimo de retención restante para empezar a pagar
	MaxOnlineHolds                 = 2   // retenciones online activas por paciente
)

// Estados de una reserva online (OnlineReservation.Status).
const (
	OnlinePending = "pending" // la hora está retenida; falta resolver al paciente y el cobro
	OnlineDone    = "done"    // paciente resuelto y, si hay pasarela, el cobro iniciado
)

// Definition es lo que el módulo declara: sus tablas y modelos de lectura. La app lo lista en App.Modules.
var Definition = model.ModuleDefinition{
	Name: "appointment_booking",
	Entities: []model.Definition{
		EmployeeServiceConfigModel, WorkCalendarConfigModel, WorkCalendarBlockModel, WorkCalendarExceptionModel,
		ReservationModel, ReservationOverbookingModel, OnlineReservationModel, ReservationExpiryModel, ConflictRecomputeModel,
		BookableServiceModel, BookableSlotModel, TimeSlotModel, DayLimitsModel, ConflictingReservationModel,
		StaffOptionModel, ClosureOptionModel,
	},
}

// EmployeeServiceConfigModel: que servicios realiza un profesional, durante cuanto
// tiempo y a que precio. Cada columna excepto id/tenant_id es legítimamente
// orientada al usuario, por lo que los widgets van directamente en el modelo persistido.
var EmployeeServiceConfigModel = model.Definition{
	Name:  "employee_service_config",
	Allow: model.Create | model.Read | model.Update,
	Rules: model.Rules{Create: model.AddMyBusinessRule, Read: model.AddMyBusinessRule, Update: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true, ServerOnly: true},
		{Name: "staff_id", Type: input.Text(), NotNull: true, Item: model.ItemDescription},
		{Name: "service_id", Type: input.Text(), NotNull: true, Item: model.ItemLabel},
		{Name: "duration_min", Type: input.Number()},
		{Name: "buffer_min", Type: input.Number()},
		{Name: "price_override", Type: input.Decimal()},
		{Name: "payment_required", Type: input.Checkbox()},
		{Name: "is_active", Type: input.Checkbox()},
	},
}

var WorkCalendarConfigModel = model.Definition{
	Name:  "work_calendar_config",
	Allow: model.Create | model.Read | model.Update,
	Rules: model.Rules{Create: model.AddMyBusinessRule, Read: model.AddMyBusinessRule, Update: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true, ServerOnly: true},
		{Name: "staff_id", Type: model.Text(), NotNull: true},
		{Name: "is_active", Type: model.Bool()},
		{Name: "max_overbook_per_day", Type: model.Int(), OmitEmpty: true},
	},
}

// WorkCalendarBlockModel: one row per block of working time. A day may hold
// several — "morning 09:00–13:00, afternoon 15:00–19:00" is two rows, and the
// lunch break is the GAP between them. There is deliberately no break field:
// a break that is a column can only ever describe one interruption, and the
// gap describes any number.
//
// specific_date == 0 → the block is WEEKLY and applies to day_of_week.
// specific_date  > 0 → the block is DATED and applies to that date only,
//
//	and day_of_week carries no meaning.
//
// A dated block OPENS its day whether or not a weekly block covers that
// weekday. That is what lets an irregular professional mark the days they
// work with no weekly template at all.
var WorkCalendarBlockModel = model.Definition{
	Name:  "work_calendar_block",
	Allow: model.Create | model.Read | model.Update | model.Delete,
	Rules: model.Rules{Create: model.AddMyBusinessRule, Read: model.AddMyBusinessRule, Update: model.AddMyBusinessRule, Delete: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true, ServerOnly: true},
		{Name: "staff_id", Type: model.Text(), NotNull: true},
		{Name: "day_of_week", Type: model.Int(), OmitEmpty: true},     // 0..6, meaningful when specific_date == 0
		{Name: "specific_date", Type: model.UnixTime(), OmitEmpty: true}, // 0 = weekly; else midnight UTC seconds
		{Name: "start_min", Type: model.Int(), OmitEmpty: true},        // minutes from midnight
		{Name: "end_min", Type: model.Int(), NotNull: true},
		{Name: "is_active", Type: model.Bool(), OmitEmpty: true},
	},
}

var WorkCalendarExceptionModel = model.Definition{
	Name:  "work_calendar_exception",
	Allow: model.Create | model.Read | model.Delete,
	Rules: model.Rules{Create: model.AddMyBusinessRule, Read: model.AddMyBusinessRule, Delete: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true, ServerOnly: true},
		{Name: "staff_id", Type: model.Text(), NotNull: true},
		{Name: "specific_date", Type: model.UnixTime()},
		{Name: "exception_type", Type: model.Text()},
		{Name: "start_time", Type: model.Int()},
		{Name: "end_time", Type: model.Int()},
		{Name: "notes", Type: model.Text()},
	},
}

// NOTA: "staff_idsnapshot" / "service_idsnapshot" preservan EXACTAMENTE el nombre de columna
// actual (irregularidad histórica, sin guión bajo) — NO renombrar la columna, la tabla ya existe
// con ese nombre en producción.
//
// La persona que reserva envía el cliente, el servicio, la hora y las notas; todo lo demás lo pone
// el servidor (ServerOnly): quién la creó, las instantáneas de precio y duración, el estado antes
// de un conflicto y las marcas de auditoría. El estado solo cambia por la máquina de estados (Update).
var ReservationModel = model.Definition{
	Name:  "reservation",
	Allow: model.Create | model.Read | model.Update,
	Rules: model.Rules{Create: model.AddMyBusinessRule, Read: model.HaveNoBusinessRule, Update: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true, ServerOnly: true},
		{Name: "client_id", Type: model.Text(), NotNull: true, Item: model.ItemLabel},
		{Name: "creator_user_id", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "employee_service_config_id", Type: model.Text(), NotNull: true, Ref: &EmployeeServiceConfigModel, DB: &model.FieldDB{RefColumn: "id"}},
		{Name: "staff_idsnapshot", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "service_idsnapshot", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "duration_min_snapshot", Type: model.Int(), OmitEmpty: true, ServerOnly: true},
		{Name: "price_snapshot", Type: model.Float(), OmitEmpty: true, ServerOnly: true},
		{Name: "currency_snapshot", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "reservation_time", Type: model.Int(), Item: model.ItemBadge},
		// status: valores válidos = las constantes FSM exportadas de fsm.go — los
		// literales viven SOLO en esas constantes (regla anti magic-string, ver service_catalog).
		// En Create la regla lo fija (PENDING o CONFIRMED); en Update es el único campo que cambia.
		{Name: "status", Type: model.Text(), OmitEmpty: true},
		{Name: "origin", Type: model.Text(), OmitEmpty: true},
		{Name: "rescheduled_from_id", Type: model.Text(), OmitEmpty: true, DB: &model.FieldDB{RefColumn: "id"}},
		// status_before_conflict: el estado anterior cuando Status == CONFLICTED —
		// se escribe al entrar en conflicto y se limpia al salir. Sin él,
		// "des-conflictear" tendría que adivinar entre PENDING y CONFIRMED, y
		// adivinar convierte una reserva confirmada en pendiente a espaldas del
		// paciente.
		{Name: "status_before_conflict", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "payment_id", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "hold_until", Type: model.UnixTime(), OmitEmpty: true, ServerOnly: true},
		{Name: "notes", Type: model.Text(), OmitEmpty: true},
		{Name: "overbooked", Type: model.Bool(), ServerOnly: true},
		{Name: "updated_at", Type: model.NanoTime(), OmitEmpty: true, ServerOnly: true},
		{Name: "updated_by", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "revision", Type: model.Int(), OmitEmpty: true},
	},
}

// ReservationFormModel es la PROYECCIÓN DE FORMULARIO de una reserva: los valores que una persona
// completa o lee al reservar en un mostrador. No es tabla ni se lista en Entities: la pantalla la
// convierte a Reservation. Existe porque los campos de ReservationModel usan tipos base
// (model.Text()) a propósito —es una fila de BD— y webtyp/form omite cualquier campo que no sea
// input.Input. Se mantiene separada para que los campos de auditoría nunca salgan como entradas editables.
var ReservationFormModel = model.Definition{
	Name: "reservation_form",
	Fields: model.Fields{
		{Name: "id", Type: input.Text(), NotNull: true, DB: &model.FieldDB{PK: true}},
		{Name: "client_id", Type: input.Text(), NotNull: true, Item: model.ItemLabel},
		{Name: "day", Type: input.Date(), NotNull: true, Item: model.ItemDescription},
		{Name: "hour", Type: input.Hour(), NotNull: true, Item: model.ItemDescription},
		{Name: "notes", Type: input.Textarea()},
		{Name: "status", Type: model.Text()},
		{Name: "origin", Type: model.Text()},
		{Name: "overbooked", Type: model.Bool()},
	},
}

// ReservationOverbookingModel es el sobrecupo: una reserva sobre un horario ya ocupado. Tiene su
// propio permiso (antes ResourceOverbooking): quien puede reservar no puede sobrecupar por eso.
// Crearlo escribe la Reservation y guarda su id en reservation_id.
var ReservationOverbookingModel = model.Definition{
	Name:  "reservation_overbooking",
	Allow: model.Create | model.Read,
	Rules: model.Rules{Create: model.AddMyBusinessRule, Read: model.HaveNoBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true, ServerOnly: true},
		{Name: "client_id", Type: model.Text(), NotNull: true},
		{Name: "employee_service_config_id", Type: model.Text(), NotNull: true, Ref: &EmployeeServiceConfigModel, DB: &model.FieldDB{RefColumn: "id"}},
		{Name: "slot_time_utc", Type: model.UnixTime(), NotNull: true},
		{Name: "notes", Type: model.Text(), OmitEmpty: true},
		{Name: "reservation_id", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "actor_id", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "created_at", Type: model.NanoTime(), OmitEmpty: true, ServerOnly: true},
	},
}

// OnlineReservationModel es el pedido del visitante: es público (la app lo abre en Backend.Public con
// Create y Read). Crearlo retiene la hora; la llamada externa Process resuelve al paciente por su
// documento y empieza el cobro, todo en la misma petición.
var OnlineReservationModel = model.Definition{
	Name:  "online_reservation",
	Allow: model.Create | model.Read,
	Rules: model.Rules{Create: model.AddMyBusinessRule, Read: model.HaveNoBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true, ServerOnly: true},
		{Name: "employee_service_config_id", Type: model.Text(), NotNull: true, Ref: &EmployeeServiceConfigModel, DB: &model.FieldDB{RefColumn: "id"}},
		{Name: "slot_time_utc", Type: model.UnixTime(), NotNull: true},
		{Name: "document_number", Type: model.Text(), NotNull: true},
		{Name: "name", Type: model.Text(), NotNull: true},
		{Name: "phone", Type: model.Text(), OmitEmpty: true},
		{Name: "email", Type: model.Text(), OmitEmpty: true},
		{Name: "notes", Type: model.Text(), OmitEmpty: true},
		{Name: "return_to", Type: model.Text(), OmitEmpty: true},
		{Name: "reservation_id", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "patient_id", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "payment_id", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "payment_url", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "pay_field", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "pay_token", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "status", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "failure", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "created_at", Type: model.NanoTime(), OmitEmpty: true, ServerOnly: true},
	},
	ExternalCalls: []model.ExternalCallDefinition{
		{Name: "Process", Mode: model.WaitForReply, MaxWaitSeconds: 10},
	},
}

// ReservationExpiryModel es la orden de «vencer las reservas pendientes que pasaron su plazo»: crearla
// las vence y guarda cuántas fueron.
var ReservationExpiryModel = model.Definition{
	Name:  "reservation_expiry",
	Allow: model.Create | model.Read,
	Rules: model.Rules{Create: model.AddMyBusinessRule, Read: model.HaveNoBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true, ServerOnly: true},
		{Name: "expired", Type: model.Int(), OmitEmpty: true, ServerOnly: true},
		{Name: "actor_id", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "created_at", Type: model.NanoTime(), OmitEmpty: true, ServerOnly: true},
	},
}

// ConflictRecomputeModel es la orden de «revisar qué reservas dejaron de caber en la agenda»: crearla
// recalcula los conflictos entre from y to (to = 0 es todo el horizonte) y guarda cuántas reservas quedaron en conflicto.
// Es manual: nada la corre sola al guardar un bloque o una excepción.
var ConflictRecomputeModel = model.Definition{
	Name:  "conflict_recompute",
	Allow: model.Create | model.Read,
	Rules: model.Rules{Create: model.AddMyBusinessRule, Read: model.HaveNoBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}, OmitEmpty: true},
		{Name: "tenant_id", Type: model.Text(), NotNull: true, ServerOnly: true},
		{Name: "from", Type: model.UnixTime(), OmitEmpty: true},
		{Name: "to", Type: model.UnixTime(), OmitEmpty: true},
		{Name: "recomputed", Type: model.Int(), OmitEmpty: true, ServerOnly: true},
		{Name: "actor_id", Type: model.Text(), OmitEmpty: true, ServerOnly: true},
		{Name: "created_at", Type: model.NanoTime(), OmitEmpty: true, ServerOnly: true},
	},
}

// Los modelos de abajo no tienen tabla: son lecturas (D18). Cada uno es su propio filtro (D52): los
// campos que la persona envía (OmitEmpty) acotan la lectura y los demás son la respuesta.

// TimeSlotModel: los horarios libres de un profesional; filtra por staff_id, config_id, from y to.
var TimeSlotModel = model.Definition{
	Name:  "time_slot",
	Allow: model.Read,
	Rules: model.Rules{Read: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "start_utc", Type: model.UnixTime(), OmitEmpty: true},
		{Name: "end_utc", Type: model.UnixTime(), OmitEmpty: true},
		{Name: "staff_id", Type: model.Text(), OmitEmpty: true},
		{Name: "offer_id", Type: model.Text(), OmitEmpty: true},
		{Name: "from", Type: model.UnixTime(), OmitEmpty: true},
		{Name: "to", Type: model.UnixTime(), OmitEmpty: true},
	},
}

// DayLimitsModel: el horario del establecimiento en un día; filtra por date (medianoche UTC) para que el
// editor de bloques acote sus controles.
var DayLimitsModel = model.Definition{
	Name:  "day_limits",
	Allow: model.Read,
	Rules: model.Rules{Read: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "date", Type: model.UnixTime(), OmitEmpty: true},
		{Name: "open", Type: model.Bool(), OmitEmpty: true},
		{Name: "open_min", Type: model.Int(), OmitEmpty: true},
		{Name: "close_min", Type: model.Int(), OmitEmpty: true},
	},
}

// ConflictingReservationModel: las reservas que la agenda actual ya no cubre; filtra por staff_id y from.
var ConflictingReservationModel = model.Definition{
	Name:  "conflicting_reservation",
	Allow: model.Read,
	Rules: model.Rules{Read: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "reservation_id", Type: model.Text(), OmitEmpty: true},
		{Name: "starts_at", Type: model.UnixTime(), OmitEmpty: true},
		{Name: "client_id", Type: model.Text(), OmitEmpty: true},
		{Name: "reason", Type: model.Text(), OmitEmpty: true}, // "OUTSIDE_BLOCKS" | "DAY_CLOSED" | "OUTSIDE_BUSINESS_HOURS"
		{Name: "staff_id", Type: model.Text(), OmitEmpty: true},
		{Name: "from", Type: model.UnixTime(), OmitEmpty: true},
	},
}

// BookableServiceModel: los servicios que un visitante puede reservar (público); filtra por specialty.
var BookableServiceModel = model.Definition{
	Name:  "bookable_service",
	Allow: model.Read,
	Rules: model.Rules{Read: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "offer_id", Type: model.Text(), OmitEmpty: true},
		{Name: "staff_id", Type: model.Text(), OmitEmpty: true},
		{Name: "staff_name", Type: model.Text(), OmitEmpty: true},
		{Name: "specialty", Type: model.Text(), OmitEmpty: true},
		{Name: "service_id", Type: model.Text(), OmitEmpty: true},
		{Name: "service_name", Type: model.Text(), OmitEmpty: true},
		{Name: "duration_min", Type: model.Int(), OmitEmpty: true},
		{Name: "price", Type: model.Float(), OmitEmpty: true},
		{Name: "currency", Type: model.Text(), OmitEmpty: true},
		{Name: "payment_required", Type: model.Bool(), OmitEmpty: true},
	},
}

// BookableSlotModel: las horas de un servicio (público); filtra por service_id, staff_id, from, to e include_taken.
var BookableSlotModel = model.Definition{
	Name:  "bookable_slot",
	Allow: model.Read,
	Rules: model.Rules{Read: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "start_utc", Type: model.UnixTime(), OmitEmpty: true},
		{Name: "end_utc", Type: model.UnixTime(), OmitEmpty: true},
		{Name: "staff_id", Type: model.Text(), OmitEmpty: true},
		{Name: "offer_id", Type: model.Text(), OmitEmpty: true},
		{Name: "taken", Type: model.Bool(), OmitEmpty: true},
		{Name: "service_id", Type: model.Text(), OmitEmpty: true},
		{Name: "from", Type: model.Int(), OmitEmpty: true},
		{Name: "to", Type: model.Int(), OmitEmpty: true},
		{Name: "include_taken", Type: model.Bool(), OmitEmpty: true},
	},
}

// StaffOptionModel: los profesionales para elegir en una lista.
var StaffOptionModel = model.Definition{
	Name:  "staff_option",
	Allow: model.Read,
	Rules: model.Rules{Read: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), OmitEmpty: true},
		{Name: "name", Type: model.Text(), OmitEmpty: true},
	},
}

// ClosureOptionModel: los días cerrados del establecimiento entre from y to (feriados y cierres).
var ClosureOptionModel = model.Definition{
	Name:  "closure_option",
	Allow: model.Read,
	Rules: model.Rules{Read: model.AddMyBusinessRule},
	Fields: model.Fields{
		{Name: "date", Type: model.Text(), OmitEmpty: true},
		{Name: "is_holiday", Type: model.Bool(), OmitEmpty: true},
		{Name: "from", Type: model.Int(), OmitEmpty: true},
		{Name: "to", Type: model.Int(), OmitEmpty: true},
	},
}

func init() {
	for i := range ReservationModel.Fields {
		if ReservationModel.Fields[i].Name == "rescheduled_from_id" {
			ReservationModel.Fields[i].Ref = &ReservationModel
		}
	}
}
