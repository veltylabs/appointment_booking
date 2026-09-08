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
	OpSaveDayBlocks              = "save_day_blocks"
	OpSaveDateBlocks             = "save_date_blocks"
	OpMarkWorkingDays            = "mark_working_days"
	OpUnmarkWorkingDays          = "unmark_working_days"
	OpListBlocks                 = "list_blocks"
	OpGetDayBounds               = "get_day_bounds"
	OpAddCalendarException       = "add_calendar_exception"
	OpRemoveCalendarException    = "remove_calendar_exception"
	OpListAvailability           = "list_availability"
	OpListExceptions             = "list_exceptions"
	OpListConflictingReservations = "list_conflicting_reservations"
	OpRecomputeConflicts         = "recompute_conflicts"
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
	reg.Operation(OpSaveDayBlocks, m.opSaveDayBlocks).Requires("calendar", model.Create|model.Update).Accepts(&SaveDayBlocksArgs{})
	reg.Operation(OpSaveDateBlocks, m.opSaveDateBlocks).Requires("calendar", model.Create|model.Update).Accepts(&SaveDateBlocksArgs{})
	reg.Operation(OpMarkWorkingDays, m.opMarkWorkingDays).Requires("calendar", model.Create|model.Update).Accepts(&MarkWorkingDaysArgs{})
	reg.Operation(OpUnmarkWorkingDays, m.opUnmarkWorkingDays).Requires("calendar", model.Delete).Accepts(&UnmarkWorkingDaysArgs{})
	reg.Operation(OpListBlocks, m.opListBlocks).Requires("calendar", model.Read).Accepts(&ListBlocksArgs{})
	reg.Operation(OpGetDayBounds, m.opGetDayBounds).Requires("calendar", model.Read).Accepts(&GetDayBoundsArgs{})
	reg.Operation(OpAddCalendarException, m.opAddCalendarException).Requires("calendar", model.Create).Accepts(&AddCalendarExceptionArgs{})
	reg.Operation(OpRemoveCalendarException, m.opRemoveCalendarException).Requires("calendar", model.Delete).Accepts(&RemoveCalendarExceptionArgs{})
	reg.Operation(OpListAvailability, m.opListAvailability).Requires("calendar", model.Read).Accepts(&ListAvailabilityArgs{})
	reg.Operation(OpListExceptions, m.opListExceptions).Requires("calendar", model.Read).Accepts(&ListExceptionsArgs{})
	reg.Operation(OpListConflictingReservations, m.opListConflictingReservations).Requires("reservation", model.Read).Accepts(&ListConflictingReservationsArgs{})
	reg.Operation(OpRecomputeConflicts, m.opRecomputeConflicts).Requires("reservation", model.Update).Accepts(&RecomputeConflictsArgs{})
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
	case ErrSlotTaken, ErrConflict, ErrBlocksOverlap:
		ctx.WriteStatus(409)
	case ErrCalendarConfigNotFound, ErrInvalidTransition, ErrInvalidBlock,
		ErrBlockOutsideBusinessHours, ErrBlockOnClosedDay:
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

func (m *Module) opSaveDayBlocks(ctx router.Context) {
	var args SaveDayBlocksArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	if err := m.SaveDayBlocks(args.TenantId, args.StaffId, int(args.DayOfWeek), args.Blocks); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opSaveDateBlocks(ctx router.Context) {
	var args SaveDateBlocksArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	if err := m.SaveDateBlocks(args.TenantId, args.StaffId, args.SpecificDate, args.Blocks); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opMarkWorkingDays(ctx router.Context) {
	var args MarkWorkingDaysArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	dates := make([]int64, len(args.Dates))
	for i, d := range args.Dates {
		dates[i] = int64(d)
	}
	if err := m.MarkWorkingDays(args.TenantId, args.StaffId, dates, int(args.StartMin), int(args.EndMin)); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opUnmarkWorkingDays(ctx router.Context) {
	var args UnmarkWorkingDaysArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	dates := make([]int64, len(args.Dates))
	for i, d := range args.Dates {
		dates[i] = int64(d)
	}
	if err := m.UnmarkWorkingDays(args.TenantId, args.StaffId, dates); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opListBlocks(ctx router.Context) {
	var args ListBlocksArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	rows, err := m.ListBlocks(args.TenantId, args.StaffId)
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(WorkCalendarBlockList, len(rows))
	for i := range rows {
		list[i] = &rows[i]
	}
	if err := ctx.Encode(&list); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opGetDayBounds(ctx router.Context) {
	var args GetDayBoundsArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	b, err := m.GetDayBounds(args.Date)
	if err != nil {
		writeError(ctx, err)
		return
	}
	res := DayBoundsResult{Open: b.Open, OpenMin: int64(b.OpenMin), CloseMin: int64(b.CloseMin)}
	if err := ctx.Encode(&res); err != nil {
		ctx.WriteStatus(500)
	}
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

func (m *Module) opListExceptions(ctx router.Context) {
	var args ListExceptionsArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	rows, err := m.ListExceptions(args.TenantId, args.StaffId, args.From, args.To)
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(WorkCalendarExceptionList, len(rows))
	for i := range rows {
		list[i] = &rows[i]
	}
	if err := ctx.Encode(&list); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opListConflictingReservations(ctx router.Context) {
	var args ListConflictingReservationsArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	rows, err := m.ListConflictingReservations(args.TenantId, args.StaffId, args.From)
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(ConflictingReservationList, len(rows))
	for i := range rows {
		list[i] = &rows[i]
	}
	if err := ctx.Encode(&list); err != nil {
		ctx.WriteStatus(500)
	}
}

func (m *Module) opRecomputeConflicts(ctx router.Context) {
	var args RecomputeConflictsArgs
	if err := ctx.Decode(&args); err != nil {
		ctx.WriteStatus(400)
		return
	}
	count, err := m.RecomputeConflicts(args.TenantId, args.From, args.To)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Write([]byte(fmt.Convert(count).String()))
}