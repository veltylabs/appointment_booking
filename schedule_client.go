package appointmentbooking

import "webtyp.com/router"

// ScheduleClient es la vista caller-side de la agenda de UN profesional: la
// contraparte tipada de las ops de calendario, para que una app adapte a
// scheduleeditor sin importar un transport. Importa solo router + los tipos
// de este paquete — este módulo no puede importar components/layout (blacklist
// de renderer).
type ScheduleClient struct {
	caller   router.Caller
	tenantId string
	staffId  string
}

// NewScheduleClient construye el cliente para la agenda de (tenantId, staffId).
func NewScheduleClient(caller router.Caller, tenantId, staffId string) *ScheduleClient {
	return &ScheduleClient{caller: caller, tenantId: tenantId, staffId: staffId}
}

// Blocks carga todos los bloques del profesional (semanales y datados).
func (c *ScheduleClient) Blocks(done func([]WorkCalendarBlock, error)) {
	out := &WorkCalendarBlockList{}
	c.caller.Call(
		OpListBlocks,
		&ListBlocksArgs{TenantId: c.tenantId, StaffId: c.staffId},
		out,
		func(err error) {
			if err != nil {
				done(nil, err)
				return
			}
			rows := make([]WorkCalendarBlock, 0, out.Len())
			for i := 0; i < out.Len(); i++ {
				rows = append(rows, *out.At(i).(*WorkCalendarBlock))
			}
			done(rows, nil)
		},
	)
}

// Exceptions carga las excepciones del rango [from, to] (medianoche UTC, inclusive).
func (c *ScheduleClient) Exceptions(from, to int64, done func([]WorkCalendarException, error)) {
	out := &WorkCalendarExceptionList{}
	c.caller.Call(
		OpListExceptions,
		&ListExceptionsArgs{TenantId: c.tenantId, StaffId: c.staffId, From: from, To: to},
		out,
		func(err error) {
			if err != nil {
				done(nil, err)
				return
			}
			rows := make([]WorkCalendarException, 0, out.Len())
			for i := 0; i < out.Len(); i++ {
				rows = append(rows, *out.At(i).(*WorkCalendarException))
			}
			done(rows, nil)
		},
	)
}

// SaveDayBlocks pisa todos los bloques semanales del weekday dado.
func (c *ScheduleClient) SaveDayBlocks(dayOfWeek int, blocks []WorkCalendarBlock, done func(error)) {
	c.caller.Call(
		OpSaveDayBlocks,
		&SaveDayBlocksArgs{
			TenantId:  c.tenantId,
			StaffId:   c.staffId,
			DayOfWeek: int64(dayOfWeek),
			Blocks:    blocks,
		},
		nil,
		done,
	)
}

// AddException da de alta una excepción de calendario.
func (c *ScheduleClient) AddException(exc WorkCalendarException, done func(error)) {
	c.caller.Call(
		OpAddCalendarException,
		&AddCalendarExceptionArgs{
			TenantId:      c.tenantId,
			StaffId:       c.staffId,
			SpecificDate:  exc.SpecificDate,
			ExceptionType: exc.ExceptionType,
			StartTime:     exc.StartTime,
			EndTime:       exc.EndTime,
			Notes:         exc.Notes,
		},
		nil,
		done,
	)
}

// RemoveException da de baja una excepción por id.
func (c *ScheduleClient) RemoveException(exceptionId string, done func(error)) {
	c.caller.Call(
		OpRemoveCalendarException,
		&RemoveCalendarExceptionArgs{TenantId: c.tenantId, ExceptionId: exceptionId},
		nil,
		done,
	)
}
