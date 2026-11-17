package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/time"
)

// Read responde la lectura de bookable_slot: las horas de un servicio (service_id, requerido) entre from y to, las de un profesional
// (staff_id) o las de todos los que lo ofrecen. Con include_taken y un profesional fijo agrega sus horas ya reservadas. Es público.
// crud ya validó los argumentos, aplicó el tope de página y comprobó el permiso antes de llamarte.
func (p *BookableSlot) Read(a crud.CurrentUser, filter *BookableSlot, args crud.ReadArgs, m *Module) (model.ModelSlice, error) {
	if filter.ServiceId == "" {
		return nil, &model.FieldError{Field: "service_id", Err: ErrMissingArgs}
	}
	rows, err := m.ListServiceAvailability(m.tenantID, filter.ServiceId, filter.StaffId, time.Unix(filter.From), time.Unix(filter.To), filter.IncludeTaken)
	if err != nil {
		return nil, err
	}
	list := BookableSlotList{}
	for i := range rows {
		list = append(list, &rows[i])
	}
	return &list, nil
}
