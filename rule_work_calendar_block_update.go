package appointmentbooking

import "webtyp.com/crud"

// Update es la regla de negocio de actualizar work_calendar_block: valida el bloque ya modificado y recalcula los conflictos.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (next *WorkCalendarBlock) Update(a crud.CurrentUser, tx crud.Tx, old *WorkCalendarBlock, fields []string, m *Module) (crud.RuleAnswer, error) {
	r := m.repoOn(tx)
	merged := *old
	if hasField(fields, WorkCalendarBlock_.DayOfWeek) {
		merged.DayOfWeek = next.DayOfWeek
	}
	if hasField(fields, WorkCalendarBlock_.SpecificDate) {
		merged.SpecificDate = next.SpecificDate
	}
	if hasField(fields, WorkCalendarBlock_.StartMin) {
		merged.StartMin = next.StartMin
	}
	if hasField(fields, WorkCalendarBlock_.EndMin) {
		merged.EndMin = next.EndMin
	}
	if hasField(fields, WorkCalendarBlock_.IsActive) {
		merged.IsActive = next.IsActive
	}
	if err := m.checkNewBlock(r, &merged, old.Id); err != nil {
		return crud.RuleAnswer{}, err
	}
	next.DayOfWeek = merged.DayOfWeek
	from, to := ruleRange(merged)
	oldFrom, oldTo := ruleRange(*old)
	if oldFrom < from {
		from = oldFrom
	}
	if oldTo > to {
		to = oldTo
	}
	if err := m.recomputeForRule(r, old.StaffId, from, to, scheduleChange{dropBlockId: old.Id, addBlock: &merged}); err != nil {
		return crud.RuleAnswer{}, err
	}
	return crud.ApplyAndPublish(), nil
}
