package appointmentbooking

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router"
)

const (
	OpCreateReservation          = "create_reservation"
	OpGetReservation             = "get_reservation"
	OpListReservationsByStaff    = "list_reservations_by_staff"
	OpListReservationsByClient   = "list_reservations_by_client"
	OpChangeReservationStatus    = "change_reservation_status"
	OpExpirePendingReservations  = "expire_pending_reservations"
	OpUpsertCalendarConfig       = "upsert_calendar_config"
	OpUpsertWeeklyCalendar       = "upsert_weekly_calendar"
	OpAddCalendarException       = "add_calendar_exception"
	OpRemoveCalendarException    = "remove_calendar_exception"
	OpListAvailability           = "list_availability"
)

func (m *Module) ModelName() string { return "appointment_booking" }

func (m *Module) MountOperations(reg router.OperationRegistry) {
	reg.Operation(OpCreateReservation, m.opCreateReservation).Requires("reservation", model.Create).Accepts(&CreateReservationArgs{})
	reg.Operation(OpGetReservation, m.opGetReservation).Requires("reservation", model.Read).Accepts(&GetReservationArgs{})
	reg.Operation(OpListReservationsByStaff, m.opListReservationsByStaff).Requires("reservation", model.Read).Accepts(&ListReservationsByStaffArgs{})
	reg.Operation(OpListReservationsByClient, m.opListReservationsByClient).Requires("reservation", model.Read).Accepts(&ListReservationsByClientArgs{})
	reg.Operation(OpChangeReservationStatus, m.opChangeReservationStatus).Requires("reservation", model.Update).Accepts(&ChangeReservationStatusArgs{})
	reg.Operation(OpExpirePendingReservations, m.opExpirePendingReservations).Requires("reservation", model.Update).Accepts(&ExpirePendingReservationsArgs{})
	// Upserts: crean en la rama not-found Y actualizan en la otra — el op exige TODAS las
	// acciones que realmente puede ejecutar (model.Action es bitmask). Declarar solo Update
	// dejaría a un principal update-only creando filas (violación de closed-by-default).
	reg.Operation(OpUpsertCalendarConfig, m.opUpsertCalendarConfig).Requires("calendar", model.Create|model.Update).Accepts(&UpsertCalendarConfigArgs{})
	reg.Operation(OpUpsertWeeklyCalendar, m.opUpsertWeeklyCalendar).Requires("calendar", model.Create|model.Update).Accepts(&UpsertWeeklyCalendarArgs{})
	reg.Operation(OpAddCalendarException, m.opAddCalendarException).Requires("calendar", model.Create).Accepts(&AddCalendarExceptionArgs{})
	reg.Operation(OpRemoveCalendarException, m.opRemoveCalendarException).Requires("calendar", model.Delete).Accepts(&RemoveCalendarExceptionArgs{})
	reg.Operation(OpListAvailability, m.opListAvailability).Requires("calendar", model.Read).Accepts(&ListAvailabilityArgs{})
}

var _ router.OperationModule = (*Module)(nil)

// writeError mapea los errores sentinela conocidos a un código de estado tipo HTTP y escribe
// err.Error() como cuerpo, preservando (de forma laxa) los mensajes legibles que daba el viejo
// mcp.Result{Content: msg} — router.Context no tiene un envoltorio propio de error-con-mensaje,
// así que esta es la convención mínima propia del módulo. Deliberadamente no es más rica que esto.
//
// Convención de mapeo (la misma para todos los módulos del ecosistema — nunca colapsar todo a
// 500, eso es el "runtime mystery" que CONSTRUCTION_HARNESS prohíbe):
//   400 = decode/validación/precondición inválida · 404 = no existe · 409 = conflicto · 500 = resto.
func writeError(ctx router.Context, err error) {
	switch err {
	case ErrNotFound:
		ctx.WriteStatus(404)
	case ErrSlotTaken, ErrConflict:
		ctx.WriteStatus(409)
	case ErrCalendarConfigNotFound, ErrInvalidTransition:
		ctx.WriteStatus(400)
	default:
		ctx.WriteStatus(500)
	}
	ctx.Write([]byte(err.Error()))
}

func (m *Module) opCreateReservation(ctx router.Context) {
	var args CreateReservationArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	// Doctrina fail-closed: decode → validate → servicio. Validate ejecuta las constraints
	// declaradas en la Definition (método generado por ormc — nunca re-implementado a mano).
	// Aplica este mismo patrón en los 11 handlers: todo op que decodifica args valida antes
	// de llamar al método de negocio; error de validación ⇒ 400.
	if err := args.Validate(model.ActionCreate); err != nil {
		ctx.WriteStatus(400)
		return
	}
	cmd := CreateReservationCmd{
		TenantId:                args.TenantId,
		ClientId:                args.ClientId,
		CreatorUserId:           args.CreatorUserId,
		EmployeeServiceConfigId: args.EmployeeServiceConfigId,
		SlotStartUtc:            args.SlotStartUtc,
		Notes:                   args.Notes,
		RescheduledFromId:       args.RescheduledFromId,
	}
	res, err := m.CreateReservation(cmd)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&res); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opGetReservation(ctx router.Context) {
	var args GetReservationArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	res, err := m.GetReservation(args.TenantId, args.Id)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&res); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opListReservationsByStaff(ctx router.Context) {
	var args ListReservationsByStaffArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	res, err := m.ListReservationsByStaff(args.TenantId, args.StaffId, args.From, args.To)
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(ReservationList, len(res))
	for i := range res {
		list[i] = &res[i]
	}
	if err := ctx.Encode(&list); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opListReservationsByClient(ctx router.Context) {
	var args ListReservationsByClientArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	res, err := m.ListReservationsByClient(args.TenantId, args.ClientId)
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(ReservationList, len(res))
	for i := range res {
		list[i] = &res[i]
	}
	if err := ctx.Encode(&list); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opChangeReservationStatus(ctx router.Context) {
	var args ChangeReservationStatusArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	cmd := ChangeStatusCmd{
		TenantId:  args.TenantId,
		Id:        args.Id,
		Event:     args.Event,
		ActorId:   args.ActorId,
		PaymentId: args.PaymentId,
		Revision:  int(args.Revision),
	}
	if err := m.ChangeReservationStatus(cmd); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opExpirePendingReservations(ctx router.Context) {
	var args ExpirePendingReservationsArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	count, err := m.ExpirePendingReservations(args.TenantId, args.Before)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Write([]byte(fmt.Convert(count).String()))
}

func (m *Module) opUpsertCalendarConfig(ctx router.Context) {
	var args UpsertCalendarConfigArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	cfg := WorkCalendarConfig{
		TenantId: args.TenantId, StaffId: args.StaffId,
		Timezone: args.Timezone, IsActive: args.IsActive,
	}
	if err := m.UpsertCalendarConfig(cfg); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opUpsertWeeklyCalendar(ctx router.Context) {
	var args UpsertWeeklyCalendarArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	cal := WorkCalendarWeekly{
		TenantId: args.TenantId, StaffId: args.StaffId, DayOfWeek: args.DayOfWeek,
		WorkStart: args.WorkStart, WorkFinish: args.WorkFinish,
		BreakStart: args.BreakStart, BreakFinish: args.BreakFinish, IsActive: args.IsActive,
	}
	if err := m.UpsertWeeklyCalendar(cal); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opAddCalendarException(ctx router.Context) {
	var args AddCalendarExceptionArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	exc := WorkCalendarException{
		TenantId: args.TenantId, StaffId: args.StaffId, SpecificDate: args.SpecificDate,
		ExceptionType: args.ExceptionType, StartTime: args.StartTime, EndTime: args.EndTime,
		Notes: args.Notes,
	}
	if err := m.AddException(exc); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opRemoveCalendarException(ctx router.Context) {
	var args RemoveCalendarExceptionArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	if err := m.RemoveException(args.TenantId, args.ExceptionId); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opListAvailability(ctx router.Context) {
	var args ListAvailabilityArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	slots, err := m.ListAvailability(args.TenantId, args.StaffId, args.ConfigId, args.From, args.To)
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(TimeSlotList, len(slots))
	for i := range slots {
		list[i] = &slots[i]
	}
	if err := ctx.Encode(&list); err != nil {
		ctx.WriteStatus(500)
	}
}
