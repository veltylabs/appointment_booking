package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/storage"
)

// Search es la regla de negocio de leer work_calendar_config: con staff_id en el filtro, solo las filas de ese profesional.
func (p *WorkCalendarConfig) Search(a crud.CurrentUser, filter *WorkCalendarConfig, m *Module) ([]storage.Condition, error) {
	if filter.StaffId == "" {
		return nil, nil
	}
	return []storage.Condition{storage.Eq(WorkCalendarConfig_.StaffId, filter.StaffId)}, nil
}
