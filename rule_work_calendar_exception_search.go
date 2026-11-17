package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/storage"
)

// Search es la regla de negocio de leer work_calendar_exception: con staff_id en el filtro, solo las filas de ese profesional.
func (p *WorkCalendarException) Search(a crud.CurrentUser, filter *WorkCalendarException, m *Module) ([]storage.Condition, error) {
	if filter.StaffId == "" {
		return nil, nil
	}
	return []storage.Condition{storage.Eq(WorkCalendarException_.StaffId, filter.StaffId)}, nil
}
