package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/events"
)

// Create es la regla de negocio de crear reservation: la crea el personal en el mostrador (origin COUNTER), la hora debe estar libre dentro
// de esta transacción (sin doble reserva) y quedan congelados el precio y la duración. Quien la crea es quien llama (nunca el cliente).
// Si reemplaza a otra (rescheduled_from_id), la original pasa a RESCHEDULED en la misma transacción.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (p *Reservation) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	r := m.repoOn(tx)
	p.CreatorUserId = a.UserID
	p.Origin = OriginCounter
	p.HoldUntil = 0
	if err := m.prepareReservation(r, p, false, true); err != nil {
		return crud.RuleAnswer{}, err
	}
	original, err := m.rescheduleOriginal(r, p)
	if err != nil {
		return crud.RuleAnswer{}, err
	}
	if original != nil && m.pub != nil {
		m.pub.Publish(events.Event{Topic: EventReservationRescheduled, Payload: original})
	}
	return crud.ApplyAndPublish(), nil
}
