package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
)

// Create es la regla de negocio de crear work_calendar_config: el tope de sobrecupos no es negativo y cada profesional tiene un solo calendario.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (p *WorkCalendarConfig) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	if p.MaxOverbookPerDay < 0 {
		return crud.RuleAnswer{}, &model.FieldError{Field: WorkCalendarConfig_.MaxOverbookPerDay, Err: ErrMissingArgs}
	}
	if _, err := m.repoOn(tx).GetCalendarConfig(p.StaffId); err == nil {
		return crud.RuleAnswer{}, &model.FieldError{Field: WorkCalendarConfig_.StaffId, Err: ErrCalendarConfigExists}
	} else if !isNotFound(err) {
		return crud.RuleAnswer{}, err
	}
	return crud.ApplyAndPublish(), nil
}
