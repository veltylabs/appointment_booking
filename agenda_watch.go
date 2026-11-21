package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/events"
	"webtyp.com/model"
)

// WatchStaffAgenda llama a fn con el profesional cada vez que cambia una reserva de su agenda: creada, confirmada,
// cancelada, completada, reagendada, expirada o en conflicto, venga de una operación crud o del flujo de reserva.
// Necesita Deps.Subscriber; sin él no llama nunca. fn corre fuera de la escritura que la causó.
// Ejemplo: m.WatchStaffAgenda(func(staffID string) { avisarAlProfesional(staffID) })
func (m *Module) WatchStaffAgenda(fn func(staffID string)) {
	m.watchMu.Lock()
	m.watchers = append(m.watchers, fn)
	m.watchMu.Unlock()
}

// agendaTopics son los eventos que mueven la agenda de un profesional.
func agendaTopics() []string {
	return []string{
		crud.Topic(Definition.Name, ReservationModel.Name, model.Create),
		crud.Topic(Definition.Name, ReservationModel.Name, model.Update),
		crud.Topic(Definition.Name, ReservationModel.Name, model.Delete),
		EventReservationCreated, EventReservationConfirmed, EventReservationCancelled, EventReservationCompleted,
		EventReservationNoShow, EventReservationExpired, EventReservationRescheduled, EventReservationConflicted,
	}
}

func (m *Module) listenAgenda(sub events.Subscriber) {
	for _, topic := range agendaTopics() {
		sub.Subscribe(topic, func(e events.Event) {
			r := reservationInEvent(e.Payload)
			if r == nil {
				return
			}
			go m.agendaChanged(*r)
		})
	}
}

// agendaChanged avisa a los observadores. Una actualización crud trae solo los campos escritos: sin profesional, se
// lee la reserva completa.
func (m *Module) agendaChanged(r Reservation) {
	staffID := r.StaffIdsnapshot
	if staffID == "" && r.Id != "" {
		var full Reservation
		if err := m.store.ReadOne(&full, r.Id); err == nil {
			staffID = full.StaffIdsnapshot
		}
	}
	if staffID == "" {
		return
	}
	m.watchMu.Lock()
	watchers := append([]func(string){}, m.watchers...)
	m.watchMu.Unlock()
	for _, fn := range watchers {
		fn(staffID)
	}
}

// reservationInEvent saca la reserva del payload de un evento: la reserva misma (flujo de reserva) o el registro de un
// crud.Change (operaciones crud).
func reservationInEvent(p model.Encodable) *Reservation {
	switch v := p.(type) {
	case *Reservation:
		return v
	case *crud.Change:
		if r, ok := v.Record.(*Reservation); ok {
			return r
		}
	}
	return nil
}
