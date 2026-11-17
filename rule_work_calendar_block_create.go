package appointmentbooking

import "webtyp.com/crud"

// Create es la regla de negocio de crear work_calendar_block: valida el bloque (rango, horas del establecimiento, sin solape) y
// recalcula en la misma transacción qué reservas dejan de caber, para avisar a sus pacientes (CU-19).
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (p *WorkCalendarBlock) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	r := m.repoOn(tx)
	p.IsActive = true
	if err := m.checkNewBlock(r, p, ""); err != nil {
		return crud.RuleAnswer{}, err
	}
	from, to := ruleRange(*p)
	if err := m.recomputeForRule(r, p.StaffId, from, to, scheduleChange{addBlock: p}); err != nil {
		return crud.RuleAnswer{}, err
	}
	return crud.ApplyAndPublish(), nil
}
