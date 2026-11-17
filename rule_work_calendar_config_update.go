package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
)

// Update es la regla de negocio de actualizar work_calendar_config: el tope de sobrecupos no puede quedar negativo.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (next *WorkCalendarConfig) Update(a crud.CurrentUser, tx crud.Tx, old *WorkCalendarConfig, fields []string, m *Module) (crud.RuleAnswer, error) {
	if hasField(fields, WorkCalendarConfig_.MaxOverbookPerDay) && next.MaxOverbookPerDay < 0 {
		return crud.RuleAnswer{}, &model.FieldError{Field: WorkCalendarConfig_.MaxOverbookPerDay, Err: ErrMissingArgs}
	}
	return crud.ApplyAndPublish(), nil
}
