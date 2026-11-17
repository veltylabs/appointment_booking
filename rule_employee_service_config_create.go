package appointmentbooking

import "webtyp.com/crud"

// Create es la regla de negocio de crear employee_service_config: la duración debe ser de al menos 1 minuto.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (p *EmployeeServiceConfig) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	if err := checkDuration(p.DurationMin); err != nil {
		return crud.RuleAnswer{}, err
	}
	return crud.ApplyAndPublish(), nil
}
