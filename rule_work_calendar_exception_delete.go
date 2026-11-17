package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/time"
)

// Delete es la regla de negocio de borrar work_calendar_exception: recalcula en la misma transacción qué reservas vuelven a caber.
// crud ya validó los ids, los límites y el permiso antes de llamarte.
func (p *WorkCalendarException) Delete(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	from := time.MidnightUTC(p.SpecificDate)
	if err := m.recomputeForRule(m.repoOn(tx), p.StaffId, from, from, scheduleChange{dropExceptionId: p.Id}); err != nil {
		return crud.RuleAnswer{}, err
	}
	return crud.ApplyAndPublish(), nil
}
