package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/time"
)

// Create es la regla de negocio de crear online_reservation: retiene la hora (reserva PENDING, origin ONLINE, OnlineHoldSeconds de retención)
// dentro de esta transacción y pide la llamada externa Process, que resuelve al paciente y empieza el cobro en la misma petición.
// Un paciente ya registrado no puede tener más de MaxOnlineHolds retenciones vigentes.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (p *OnlineReservation) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	r := m.repoOn(tx)
	now := time.NowUnix()
	clientId := p.DocumentNumber // mientras Process no resuelve al paciente, la reserva lleva su documento
	id, _, _, _, found, err := m.patients.FindPatient(m.tenantID, p.DocumentNumber)
	if err != nil {
		return crud.RuleAnswer{}, err
	}
	if found {
		clientId = id
		mine, err := r.ListReservationsByClient(id)
		if err != nil {
			return crud.RuleAnswer{}, err
		}
		holds := 0
		for _, res := range mine {
			if res.Origin == OriginOnline && res.Status == StatusPending && res.HoldUntil > now {
				holds++
			}
		}
		if holds >= MaxOnlineHolds {
			return crud.RuleAnswer{}, ErrTooManyHolds
		}
	}
	res := Reservation{
		Id: m.ids.NewID(), ClientId: clientId, EmployeeServiceConfigId: p.EmployeeServiceConfigId,
		ReservationTime: int64(p.SlotTimeUtc), Origin: OriginOnline, HoldUntil: now + OnlineHoldSeconds, Notes: p.Notes,
	}
	if err := m.prepareReservation(r, &res, false, false); err != nil {
		return crud.RuleAnswer{}, err
	}
	if err := tx.Create(&res); err != nil {
		return crud.RuleAnswer{}, err
	}
	p.ReservationId = res.Id
	p.Status = OnlinePending
	return crud.ApplyAndPublish(OnlineReservationProcess{}), nil
}
