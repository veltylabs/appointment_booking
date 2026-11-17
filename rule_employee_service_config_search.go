package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/storage"
)

// Search es la regla de negocio de leer employee_service_config: con staff_id en el filtro, solo las filas de ese profesional.
func (p *EmployeeServiceConfig) Search(a crud.CurrentUser, filter *EmployeeServiceConfig, m *Module) ([]storage.Condition, error) {
	if filter.StaffId == "" {
		return nil, nil
	}
	return []storage.Condition{storage.Eq(EmployeeServiceConfig_.StaffId, filter.StaffId)}, nil
}
