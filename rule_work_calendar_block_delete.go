package appointmentbooking

import "webtyp.com/crud"

// Delete es la regla de negocio de borrar work_calendar_block: recalcula en la misma transacción qué reservas dejan de caber.
// crud ya validó los ids, los límites y el permiso antes de llamarte.
func (p *WorkCalendarBlock) Delete(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	from, to := ruleRange(*p)
	if err := m.recomputeForRule(m.repoOn(tx), p.StaffId, from, to, scheduleChange{dropBlockId: p.Id}); err != nil {
		return crud.RuleAnswer{}, err
	}
	return crud.ApplyAndPublish(), nil
}
