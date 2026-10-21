package appointmentbooking

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/router"
	"webtyp.com/time"
)

const (
	OpListBookableServices    = "list_bookable_services"
	OpListServiceAvailability = "list_service_availability"
	OpCreateOnlineReservation = "create_online_reservation"
)

const (
	OpCreateReservation                 = "create_reservation"
	OpCreateOverbooking                 = "create_overbooking"
	OpGetReservation                    = "get_reservation"
	OpListReservationsByStaff           = "list_reservations_by_staff"
	OpListReservationsByClient          = "list_reservations_by_client"
	OpChangeReservationStatus           = "change_reservation_status"
	OpExpirePendingReservations         = "expire_pending_reservations"
	OpGetCalendarConfig                 = "get_calendar_config"
	OpUpsertCalendarConfig              = "upsert_calendar_config"
	OpSaveDayBlocks                     = "save_day_blocks"
	OpSaveDateBlocks                    = "save_date_blocks"
	OpMarkWorkingDays                   = "mark_working_days"
	OpUnmarkWorkingDays                 = "unmark_working_days"
	OpListBlocks                        = "list_blocks"
	OpGetDayBounds                      = "get_day_bounds"
	OpAddCalendarException              = "add_calendar_exception"
	OpRemoveCalendarException           = "remove_calendar_exception"
	OpListAvailability                  = "list_availability"
	OpListExceptions                    = "list_exceptions"
	OpListConflictingReservations       = "list_conflicting_reservations"
	OpRecomputeConflicts                = "recompute_conflicts"
	OpCreateEmployeeServiceConfig       = "create_employee_service_config"
	OpGetEmployeeServiceConfig          = "get_employee_service_config"
	OpListEmployeeServiceConfigsByStaff = "list_employee_service_configs_by_staff"
	OpUpdateEmployeeServiceConfig       = "update_employee_service_config"
)

// ModelName is this module's identity: mcp.HarvestOps qualifies every op
// above as "appointment_booking.<name>" on the wire — the qualification
// that makes this module's own "get_day_bounds" distinct from
// business_calendar's op of the same bare name (the collision this whole
// mechanism exists to make unrepresentable).
//
// Unlike the other modules in this ecosystem, this package builds its own
// Lister/Saver implementations directly (lister.go, schedule_client.go,
// view.go) instead of going through view.NewCallerLister — so it cannot
// lean on view.Ops.Module to compose the qualified name automatically.
// qualifiedOp (below) is the single place that composition happens instead;
// every caller.Call site in this package goes through it.
const ModelName = "appointment_booking"

func (m *Module) ModelName() string { return ModelName }

// qualifiedOp composes the wire name mcp.HarvestOps produces for one of this
// module's own ops (see ModelName's doc above). Every caller.Call site in
// this package calls this instead of passing an Op* constant directly, so
// none hand-writes "appointment_booking." as a literal.
func qualifiedOp(name string) string {
	return ModelName + "." + name
}

func (m *Module) MountOperations(reg router.OperationRegistry) {

	reg.Operation(OpListBookableServices, m.opListBookableServices).Requires(ResourceBookingCatalog, model.Read).Accepts(&ListBookableServicesArgs{})
	reg.Operation(OpListServiceAvailability, m.opListServiceAvailability).Requires(ResourceBookingCatalog, model.Read).Accepts(&ListServiceAvailabilityArgs{})
	reg.Operation(OpCreateOnlineReservation, m.opCreateOnlineReservation).Requires(ResourceReservationOnline, model.Create).Accepts(&CreateOnlineReservationArgs{})

	reg.Operation(OpCreateReservation, m.opCreateReservation).Requires("reservation", model.Create).Accepts(&CreateReservationArgs{})
	reg.Operation(OpCreateOverbooking, m.opCreateOverbooking).Requires("overbooking", model.Create).Accepts(&CreateOverbookingArgs{})
	reg.Operation(OpGetReservation, m.opGetReservation).Requires("reservation", model.Read).Accepts(&GetReservationArgs{})
	reg.Operation(OpListReservationsByStaff, m.opListReservationsByStaff).Requires("reservation", model.Read).Accepts(&ListReservationsByStaffArgs{})
	reg.Operation(OpListReservationsByClient, m.opListReservationsByClient).Requires("reservation", model.Read).Accepts(&ListReservationsByClientArgs{})
	reg.Operation(OpChangeReservationStatus, m.opChangeReservationStatus).Requires("reservation", model.Update).Accepts(&ChangeReservationStatusArgs{})
	reg.Operation(OpExpirePendingReservations, m.opExpirePendingReservations).Requires("reservation", model.Update).Accepts(&ExpirePendingReservationsArgs{})
	reg.Operation(OpGetCalendarConfig, m.opGetCalendarConfig).Requires("calendar", model.Read).Accepts(&GetCalendarConfigArgs{})
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
	reg.Operation(OpCreateEmployeeServiceConfig, m.opCreateEmployeeServiceConfig).Requires("employee_service_config", model.Create).Accepts(&CreateEmployeeServiceConfigArgs{})
	reg.Operation(OpGetEmployeeServiceConfig, m.opGetEmployeeServiceConfig).Requires("employee_service_config", model.Read).Accepts(&GetEmployeeServiceConfigArgs{})
	reg.Operation(OpListEmployeeServiceConfigsByStaff, m.opListEmployeeServiceConfigsByStaff).Requires("employee_service_config", model.Read).Accepts(&ListEmployeeServiceConfigsByStaffArgs{})
	reg.Operation(OpUpdateEmployeeServiceConfig, m.opUpdateEmployeeServiceConfig).Requires("employee_service_config", model.Update).Accepts(&EmployeeServiceConfig{})
}

var _ router.OperationModule = (*Module)(nil)

// writeError mapea los errores sentinela conocidos a un código de estado tipo HTTP y escribe
// err.Error() como cuerpo, preservando (de forma laxa) los mensajes legibles que daba el viejo
// mcp.Result{Content: msg} — router.Context no tiene un envoltorio propio de error-con-mensaje,
// así que esta es la convención mínima propia del módulo. Deliberadamente no es más rica que esto.
//
// Convención de mapeo (la misma para todos los módulos del ecosistema — nunca colapsar todo a
// 500, eso es el "runtime mystery" que CONSTRUCTION_HARNESS prohíbe):
//
//	400 = decode/validación/precondición inválida · 404 = no existe · 409 = conflicto · 500 = resto.
//
// badRequest is a payload the operation could not read, or one that failed the constraints its
// Definition declares: 400, with the decoder's or the validation's own text.
type badRequest struct{ err error }

func (b badRequest) Error() string { return b.err.Error() }

// writeError answers every refusal with its status and its text: a client (rpc, a sync engine's
// conflict tray) shows the text, so an empty body is never an answer.
func writeError(ctx router.Context, err error) {
	ctx.WriteStatus(statusOf(err))
	ctx.Write([]byte(err.Error()))
}

func statusOf(err error) int {
	switch e := err.(type) {
	case badRequest:
		return 400
	case domainError:
		switch e {
		case ErrNotFound, ErrClientNotFound:
			return 404
		case ErrSlotTaken, ErrConflict, ErrBlocksOverlap, ErrIdTaken, ErrOverbookNotAllowed:
			return 409
		case ErrTooManyHolds, ErrPaymentNotVerified, ErrAlreadyPaid, ErrReservationNotPayable:
			return 409
		case ErrPaymentsUnavailable:
			return 500
		case ErrCalendarConfigNotFound, ErrInvalidTransition, ErrInvalidBlock,
			ErrBlockOutsideBusinessHours, ErrBlockOnClosedDay, ErrMissingArgs, ErrNoServiceConfig, ErrIncompleteSlot, ErrSlotNotInSchedule, ErrUnsupportedCurrency:
			return 400
		}
	}
	if orm.IsNotFound(err) {
		return 404
	}
	return 500
}

func (m *Module) opCreateReservation(ctx router.Context) {
	var args CreateReservationArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	// Doctrina fail-closed: decode → validate → servicio. Validate ejecuta las constraints
	// declaradas en la Definition (método generado por ormc — nunca re-implementado a mano).
	if err := args.Validate(model.ActionCreate); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	cmd := CreateReservationCmd{
		Id:                      args.Id,
		TenantId:                args.TenantId,
		ClientId:                args.ClientId,
		CreatorUserId:           args.CreatorUserId,
		EmployeeServiceConfigId: args.EmployeeServiceConfigId,
		SlotStartUtc:            args.SlotStartUtc,
		Notes:                   args.Notes,
		RescheduledFromId:       args.RescheduledFromId,
		Origin:                  args.Origin,
	}
	res, err := m.CreateReservation(cmd)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&res); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opCreateOverbooking(ctx router.Context) {
	var args CreateOverbookingArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	if err := args.Validate(model.ActionCreate); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	cmd := CreateReservationCmd{
		Id:                      args.Id,
		TenantId:                args.TenantId,
		ClientId:                args.ClientId,
		CreatorUserId:           args.CreatorUserId,
		EmployeeServiceConfigId: args.EmployeeServiceConfigId,
		SlotStartUtc:            args.SlotStartUtc,
		Notes:                   args.Notes,
		RescheduledFromId:       args.RescheduledFromId,
		Origin:                  args.Origin,
	}
	res, err := m.CreateOverbooking(cmd)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&res); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opGetReservation(ctx router.Context) {
	var args GetReservationArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	res, err := m.GetReservation(args.TenantId, args.Id)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&res); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opListReservationsByStaff(ctx router.Context) {
	var args ListReservationsByStaffArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	res, err := m.ListReservationsByStaff(args.TenantId, args.StaffId, time.Unix(args.From), time.Unix(args.To))
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(ReservationList, len(res))
	for i := range res {
		list[i] = &res[i]
	}
	if err := ctx.Encode(&list); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opListReservationsByClient(ctx router.Context) {
	var args ListReservationsByClientArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
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
		writeError(ctx, err)
	}
}

func (m *Module) opChangeReservationStatus(ctx router.Context) {
	var args ChangeReservationStatusArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
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
		writeError(ctx, badRequest{err})
		return
	}
	count, err := m.ExpirePendingReservations(args.TenantId, time.Unix(args.Before))
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Write([]byte(fmt.Convert(count).String()))
}

func (m *Module) opCreateEmployeeServiceConfig(ctx router.Context) {
	var args CreateEmployeeServiceConfigArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	cfg, err := m.CreateEmployeeServiceConfig(EmployeeServiceConfig{
		TenantId: args.TenantId, StaffId: args.StaffId, ServiceId: args.ServiceId,
		DurationMin: args.DurationMin, BufferMin: args.BufferMin,
		PriceOverride: args.PriceOverride, PaymentRequired: args.PaymentRequired, IsActive: true,
	})
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&cfg); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opGetEmployeeServiceConfig(ctx router.Context) {
	var args GetEmployeeServiceConfigArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	cfg, err := m.GetEmployeeServiceConfig(args.Id)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&cfg); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opListEmployeeServiceConfigsByStaff(ctx router.Context) {
	var args ListEmployeeServiceConfigsByStaffArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	rows, err := m.ListEmployeeServiceConfigByStaff(args.TenantId, args.StaffId)
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(EmployeeServiceConfigList, len(rows))
	for i := range rows {
		list[i] = &rows[i]
	}
	if err := ctx.Encode(&list); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opUpdateEmployeeServiceConfig(ctx router.Context) {
	var cfg EmployeeServiceConfig
	if err := ctx.Decode(&cfg); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	if err := m.UpdateEmployeeServiceConfig(cfg); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
}

func (m *Module) opGetCalendarConfig(ctx router.Context) {
	var args GetCalendarConfigArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	cfg, err := m.GetCalendarConfig(args.TenantId, args.StaffId)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := ctx.Encode(&cfg); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opUpsertCalendarConfig(ctx router.Context) {
	var args UpsertCalendarConfigArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	cfg := WorkCalendarConfig{
		TenantId:          args.TenantId,
		StaffId:           args.StaffId,
		Timezone:          args.Timezone,
		IsActive:          args.IsActive,
		MaxOverbookPerDay: args.MaxOverbookPerDay,
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
		writeError(ctx, badRequest{err})
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
		writeError(ctx, badRequest{err})
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
		writeError(ctx, badRequest{err})
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
		writeError(ctx, badRequest{err})
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
		writeError(ctx, badRequest{err})
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
		writeError(ctx, err)
	}
}

func (m *Module) opGetDayBounds(ctx router.Context) {
	var args GetDayBoundsArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	b, err := m.GetDayBounds(time.Unix(args.Date))
	if err != nil {
		writeError(ctx, err)
		return
	}
	res := DayBoundsResult{Open: b.Open, OpenMin: int64(b.OpenMin), CloseMin: int64(b.CloseMin)}
	if err := ctx.Encode(&res); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opAddCalendarException(ctx router.Context) {
	var args AddCalendarExceptionArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	exc := WorkCalendarException{
		TenantId: args.TenantId, StaffId: args.StaffId, SpecificDate: time.Unix(args.SpecificDate),
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
		writeError(ctx, badRequest{err})
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
		writeError(ctx, badRequest{err})
		return
	}
	slots, err := m.ListAvailability(args.TenantId, args.StaffId, args.ConfigId, time.Unix(args.From), time.Unix(args.To))
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(TimeSlotList, len(slots))
	for i := range slots {
		list[i] = &slots[i]
	}
	if err := ctx.Encode(&list); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opListExceptions(ctx router.Context) {
	var args ListExceptionsArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	rows, err := m.ListExceptions(args.TenantId, args.StaffId, time.Unix(args.From), time.Unix(args.To))
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(WorkCalendarExceptionList, len(rows))
	for i := range rows {
		list[i] = &rows[i]
	}
	if err := ctx.Encode(&list); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opListConflictingReservations(ctx router.Context) {
	var args ListConflictingReservationsArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	rows, err := m.ListConflictingReservations(args.TenantId, args.StaffId, time.Unix(args.From))
	if err != nil {
		writeError(ctx, err)
		return
	}
	list := make(ConflictingReservationList, len(rows))
	for i := range rows {
		list[i] = &rows[i]
	}
	if err := ctx.Encode(&list); err != nil {
		writeError(ctx, err)
	}
}

func (m *Module) opRecomputeConflicts(ctx router.Context) {
	var args RecomputeConflictsArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	count, err := m.RecomputeConflicts(args.TenantId, time.Unix(args.From), time.Unix(args.To))
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Write([]byte(fmt.Convert(count).String()))
}
func (m *Module) opListBookableServices(ctx router.Context) {
	var args ListBookableServicesArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	if err := args.Validate(model.ActionRead); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	list, err := m.ListBookableServices(args.TenantId, args.Specialty)
	if err != nil {
		writeError(ctx, err)
		return
	}
	listOut := BookableServiceList{}
	for i := range list {
		listOut = append(listOut, &list[i])
	}
	ctx.WriteStatus(200)
	ctx.Encode(&listOut)
}
func (m *Module) opListServiceAvailability(ctx router.Context) {
	var args ListServiceAvailabilityArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	if err := args.Validate(model.ActionRead); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	list, err := m.ListServiceAvailability(args)
	if err != nil {
		writeError(ctx, err)
		return
	}
	listOut := BookableSlotList{}
	for i := range list {
		listOut = append(listOut, &list[i])
	}
	ctx.WriteStatus(200)
	ctx.Encode(&listOut)
}
func (m *Module) opCreateOnlineReservation(ctx router.Context) {
	var args CreateOnlineReservationArgs
	if err := ctx.Decode(&args); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	if err := args.Validate(model.ActionCreate); err != nil {
		writeError(ctx, badRequest{err})
		return
	}
	cmd := CreateReservationCmd{Id: args.Id, TenantId: args.TenantId, ClientId: args.ClientId, EmployeeServiceConfigId: args.EmployeeServiceConfigId, SlotStartUtc: args.SlotStartUtc, Notes: args.Notes}
	res, err := m.CreateOnlineReservation(cmd)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.WriteStatus(200)
	ctx.Encode(&res)
}
