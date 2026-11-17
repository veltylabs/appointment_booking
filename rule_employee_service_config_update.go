package appointmentbooking

import "webtyp.com/crud"

// Update es la regla de negocio de actualizar employee_service_config: si se cambia la duración, debe ser de al menos 1 minuto.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (next *EmployeeServiceConfig) Update(a crud.CurrentUser, tx crud.Tx, old *EmployeeServiceConfig, fields []string, m *Module) (crud.RuleAnswer, error) {
	if hasField(fields, EmployeeServiceConfig_.DurationMin) {
		if err := checkDuration(next.DurationMin); err != nil {
			return crud.RuleAnswer{}, err
		}
	}
	return crud.ApplyAndPublish(), nil
}

// hasField dice si name está entre los campos que envió quien llama.
func hasField(fields []string, name string) bool {
	for _, f := range fields {
		if f == name {
			return true
		}
	}
	return false
}
