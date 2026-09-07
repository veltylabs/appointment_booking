package appointmentbooking

import (
	"webtyp.com/input"
	"webtyp.com/model"
)

var EmployeeServiceConfigModel = model.Definition{
	Name: "employee_service_config",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "tenant_id", Type: model.Text(), NotNull: true},
		{Name: "staff_id", Type: model.Text(), NotNull: true},
		{Name: "service_id", Type: model.Text(), NotNull: true},
		{Name: "duration_min", Type: model.Int()},
		{Name: "buffer_min", Type: model.Int()},
		{Name: "price_override", Type: model.Float()},
		{Name: "payment_required", Type: model.Bool()},
		{Name: "is_active", Type: model.Bool()},
	},
}

var WorkCalendarConfigModel = model.Definition{
	Name: "work_calendar_config",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "tenant_id", Type: model.Text(), NotNull: true},
		{Name: "staff_id", Type: model.Text(), NotNull: true},
		{Name: "timezone", Type: model.Text(), NotNull: true},
		{Name: "is_active", Type: model.Bool()},
	},
}

var WorkCalendarWeeklyModel = model.Definition{
	Name: "work_calendar_weekly",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "tenant_id", Type: model.Text(), NotNull: true},
		{Name: "staff_id", Type: model.Text(), NotNull: true},
		{Name: "day_of_week", Type: model.Int()},
		{Name: "work_start", Type: model.Int()},
		{Name: "work_finish", Type: model.Int()},
		{Name: "break_start", Type: model.Int()},
		{Name: "break_finish", Type: model.Int()},
		{Name: "is_active", Type: model.Bool()},
	},
}

var WorkCalendarExceptionModel = model.Definition{
	Name: "work_calendar_exception",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "tenant_id", Type: model.Text(), NotNull: true},
		{Name: "staff_id", Type: model.Text(), NotNull: true},
		{Name: "specific_date", Type: model.Int()},
		{Name: "exception_type", Type: model.Text()},
		{Name: "start_time", Type: model.Int()},
		{Name: "end_time", Type: model.Int()},
		{Name: "notes", Type: model.Text()},
	},
}

// NOTA: "staff_idsnapshot" / "service_idsnapshot" preservan EXACTAMENTE el nombre de columna
// actual (irregularidad histórica, sin guión bajo) — NO renombrar la columna, la tabla ya existe
// con ese nombre en producción.
var ReservationModel = model.Definition{
	Name: "reservation",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "tenant_id", Type: model.Text(), NotNull: true},
		{Name: "client_id", Type: model.Text(), NotNull: true},
		{Name: "creator_user_id", Type: model.Text()},
		{Name: "employee_service_config_id", Type: model.Text(), NotNull: true},
		{Name: "staff_idsnapshot", Type: model.Text()},
		{Name: "service_idsnapshot", Type: model.Text()},
		{Name: "duration_min_snapshot", Type: model.Int()},
		{Name: "price_snapshot", Type: model.Float()},
		{Name: "currency_snapshot", Type: model.Text()},
		{Name: "reservation_date", Type: model.Int()},
		{Name: "reservation_time", Type: model.Int()},
		{Name: "local_string_date", Type: model.Text()},
		{Name: "local_string_time", Type: model.Text()},
		// status: valores válidos = las constantes FSM exportadas de fsm.go/service.go — los
		// literales viven SOLO en esas constantes (regla anti magic-string, ver item_catalog).
		{Name: "status", Type: model.Text(), NotNull: true},
		{Name: "rescheduled_from_id", Type: model.Text()},
		{Name: "payment_id", Type: model.Text()},
		{Name: "notes", Type: model.Text()},
		{Name: "updated_at", Type: model.Int()},
		{Name: "updated_by", Type: model.Text()},
		{Name: "revision", Type: model.Int()},
	},
}

// Las 12 Definitions de abajo son transport-only. Política de widgets POR ROL (no "lo que el
// model_orm.go viejo tuviera" — ese archivo ponía widget en todo campo transport, un defecto que
// esta migración corrige): input.X() SOLO en campos que un usuario edita en un form; kinds base
// (model.X()) en campos machine-supplied (tenant_id) y en modelos de SALIDA (TimeSlot es
// resultado de list_availability — nunca se renderiza como form editable). No dejes caer un
// widget de un campo genuinamente editable: form.New() saldría vacío en silencio.

var TimeSlotModel = model.Definition{
	Name: "time_slot",
	Fields: model.Fields{
		{Name: "start_utc", Type: model.Int()},
		{Name: "end_utc", Type: model.Int()},
	},
}

var CreateReservationArgsModel = model.Definition{
	Name: "create_reservation_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "client_id", Type: input.Text()},
		{Name: "creator_user_id", Type: input.Text()},
		{Name: "employee_service_config_id", Type: input.Text()},
		{Name: "slot_start_utc", Type: input.Number()},
		{Name: "notes", Type: input.Text()},
		{Name: "rescheduled_from_id", Type: input.Text()},
	},
}

var GetReservationArgsModel = model.Definition{
	Name: "get_reservation_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "id", Type: input.Text()},
	},
}

var ListReservationsByStaffArgsModel = model.Definition{
	Name: "list_reservations_by_staff_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "staff_id", Type: input.Text()},
		{Name: "from", Type: input.Number()},
		{Name: "to", Type: input.Number()},
	},
}

var ListReservationsByClientArgsModel = model.Definition{
	Name: "list_reservations_by_client_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "client_id", Type: input.Text()},
	},
}

var ChangeReservationStatusArgsModel = model.Definition{
	Name: "change_reservation_status_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "id", Type: input.Text()},
		{Name: "event", Type: input.Text()},
		{Name: "actor_id", Type: input.Text()},
		{Name: "payment_id", Type: input.Text()},
		{Name: "revision", Type: input.Number()},
	},
}

var ExpirePendingReservationsArgsModel = model.Definition{
	Name: "expire_pending_reservations_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "before", Type: input.Number()},
	},
}

var UpsertCalendarConfigArgsModel = model.Definition{
	Name: "upsert_calendar_config_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "staff_id", Type: input.Text()},
		{Name: "timezone", Type: input.Text()},
		{Name: "is_active", Type: input.Checkbox()},
	},
}

var UpsertWeeklyCalendarArgsModel = model.Definition{
	Name: "upsert_weekly_calendar_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "staff_id", Type: input.Text()},
		{Name: "day_of_week", Type: input.Number()},
		{Name: "work_start", Type: input.Number()},
		{Name: "work_finish", Type: input.Number()},
		{Name: "break_start", Type: input.Number()},
		{Name: "break_finish", Type: input.Number()},
		{Name: "is_active", Type: input.Checkbox()},
	},
}

var AddCalendarExceptionArgsModel = model.Definition{
	Name: "add_calendar_exception_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "staff_id", Type: input.Text()},
		{Name: "specific_date", Type: input.Number()},
		{Name: "exception_type", Type: input.Text()},
		{Name: "start_time", Type: input.Number()},
		{Name: "end_time", Type: input.Number()},
		{Name: "notes", Type: input.Text()},
	},
}

var RemoveCalendarExceptionArgsModel = model.Definition{
	Name: "remove_calendar_exception_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "exception_id", Type: input.Text()},
	},
}

var ListAvailabilityArgsModel = model.Definition{
	Name: "list_availability_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "staff_id", Type: input.Text()},
		{Name: "config_id", Type: input.Text()},
		{Name: "from", Type: input.Number()},
		{Name: "to", Type: input.Number()},
	},
}

var ListWeeklyCalendarArgsModel = model.Definition{
	Name: "list_weekly_calendar_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "staff_id", Type: input.Text()},
	},
}

var ListExceptionsArgsModel = model.Definition{
	Name: "list_exceptions_args",
	Fields: model.Fields{
		{Name: "tenant_id", Type: model.Text()}, // machine-supplied — never a form input
		{Name: "staff_id", Type: input.Text()},
		{Name: "from", Type: input.Number()},
		{Name: "to", Type: input.Number()},
	},
}
