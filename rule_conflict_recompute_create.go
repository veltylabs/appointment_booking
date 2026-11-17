package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/time"
)

// Create es la regla de negocio de crear conflict_recompute: revisa todas las reservas futuras de [from, to] contra la agenda actual
// (to = 0 es todo el horizonte; from = 0 es hoy), marca o libera CONFLICTED y guarda cuántas cambiaron.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (p *ConflictRecompute) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	if p.From == 0 {
		p.From = time.MidnightUTC(time.NowUnix())
	}
	result, err := m.recomputeConflictsFor(m.repoOn(tx), p.From, p.To, "", scheduleChange{})
	if err != nil {
		return crud.RuleAnswer{}, err
	}
	m.announceEstablishment(result, p.From, p.To)
	p.Recomputed = int64(result.totalChanged)
	p.ActorId = a.UserID
	return crud.ApplyAndPublish(), nil
}
