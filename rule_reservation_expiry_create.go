package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/events"
	"webtyp.com/time"
)

// Create es la regla de negocio de crear reservation_expiry: vence las reservas PENDING cuya hora ya pasó o cuya retención online venció, y guarda cuántas fueron.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (p *ReservationExpiry) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	expired, err := m.expireIn(m.repoOn(tx), time.NowUnix())
	if err != nil {
		return crud.RuleAnswer{}, err
	}
	p.Expired = int64(len(expired))
	p.ActorId = a.UserID
	if m.pub != nil {
		for i := range expired {
			m.pub.Publish(events.Event{Topic: EventReservationExpired, Payload: &expired[i]})
		}
	}
	return crud.ApplyAndPublish(), nil
}
