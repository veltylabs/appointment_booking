package appointmentbooking

import "webtyp.com/crud"

// Create es la regla de negocio de crear reservation_overbooking: reserva una hora ya ocupada si el profesional lo permite ese día
// (tope MaxOverbookPerDay) y guarda el id de la reserva creada. Tiene su propio permiso: quien puede reservar no puede sobrecupar por eso.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (p *ReservationOverbooking) Create(a crud.CurrentUser, tx crud.Tx, m *Module) (crud.RuleAnswer, error) {
	r := m.repoOn(tx)
	res := Reservation{
		Id: m.ids.NewID(), ClientId: p.ClientId, EmployeeServiceConfigId: p.EmployeeServiceConfigId,
		ReservationTime: int64(p.SlotTimeUtc), Origin: OriginCounter, Notes: p.Notes, CreatorUserId: a.UserID,
	}
	if err := m.prepareReservation(r, &res, true, true); err != nil {
		return crud.RuleAnswer{}, err
	}
	if err := tx.Create(&res); err != nil {
		return crud.RuleAnswer{}, err
	}
	p.ReservationId = res.Id
	p.ActorId = a.UserID
	return crud.ApplyAndPublish(), nil
}
