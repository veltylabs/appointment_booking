package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
)

// Read responde la lectura de conflicting_reservation: las reservas del profesional (staff_id) desde from que la agenda actual ya no cubre.
// crud ya validó los argumentos, aplicó el tope de página y comprobó el permiso antes de llamarte.
func (p *ConflictingReservation) Read(a crud.CurrentUser, filter *ConflictingReservation, args crud.ReadArgs, m *Module) (model.ModelSlice, error) {
	rows, err := m.ListConflictingReservations(m.tenantID, filter.StaffId, filter.From)
	if err != nil {
		return nil, err
	}
	list := ConflictingReservationList{}
	for i := range rows {
		list = append(list, &rows[i])
	}
	return &list, nil
}
