package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/time"
)

// Create es la regla de negocio de crear work_calendar_exception: recalcula en la misma transacción qué reservas dejan de caber.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (p *WorkCalendarException) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	from := time.MidnightUTC(p.SpecificDate)
	if err := m.recomputeForRule(m.repoOn(tx), p.StaffId, from, from, scheduleChange{addException: p}); err != nil {
		return crud.RuleAnswer{}, err
	}
	return crud.ApplyAndPublish(), nil
}
