package appointmentbooking

import "webtyp.com/crud"

// Process es la llamada externa de online_reservation (Process): resuelve al paciente por su documento (lo crea si no existe) y, si el
// módulo tiene pasarela de pago, empieza el cobro en línea. La clave de la cola hace idempotentes los ids del paciente y del pago.
// crud guarda los campos devueltos en el registro; si algo falla, la reserva sigue retenida y el registro sigue pendiente.
func (p *OnlineReservation) Process(a crud.CurrentUser, m *Module, key crud.IdempotencyKey) (crud.FieldsToSave, error) {
	id, _, _, _, found, err := m.patients.FindPatient(p.TenantId, p.DocumentNumber)
	if err != nil {
		return nil, err
	}
	if !found {
		if id, err = m.patients.CreatePatient(p.TenantId, string(key), p.DocumentNumber, p.Name, p.Phone, p.Email); err != nil {
			return nil, err
		}
	}
	if err := m.store.UpdateFields(&Reservation{ClientId: id}, p.ReservationId, []string{Reservation_.ClientId}); err != nil {
		return nil, err
	}
	p.PatientId = id
	p.Status = OnlineDone
	if m.payments == nil {
		return crud.Fields(OnlineReservation_.PatientId, OnlineReservation_.Status), nil
	}
	redirectURL, tokenField, token, err := m.payments.StartOnlinePayment(p.TenantId, string(key), "appointment_booking", p.ReservationId, p.ReturnTo)
	if err != nil {
		return nil, err
	}
	p.PaymentId, p.PaymentUrl, p.PayField, p.PayToken = string(key), redirectURL, tokenField, token
	return crud.Fields(OnlineReservation_.PatientId, OnlineReservation_.PaymentId, OnlineReservation_.PaymentUrl, OnlineReservation_.PayField, OnlineReservation_.PayToken, OnlineReservation_.Status), nil
}
