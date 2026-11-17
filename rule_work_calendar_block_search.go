package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/storage"
)

// Search es la regla de negocio de leer work_calendar_block: con staff_id en el filtro, solo las filas de ese profesional.
func (p *WorkCalendarBlock) Search(a crud.CurrentUser, filter *WorkCalendarBlock, m *Module) ([]storage.Condition, error) {
	if filter.StaffId == "" {
		return nil, nil
	}
	return []storage.Condition{storage.Eq(WorkCalendarBlock_.StaffId, filter.StaffId)}, nil
}
