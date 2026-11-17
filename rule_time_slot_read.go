package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
)

// Read responde la lectura de time_slot: los horarios libres del profesional (staff_id) para una configuración de servicio (config_id) entre from y to.
// crud ya validó los argumentos, aplicó el tope de página y comprobó el permiso antes de llamarte.
func (p *TimeSlot) Read(a crud.CurrentUser, filter *TimeSlot, args crud.ReadArgs, m *Module) (model.ModelSlice, error) {
	slots, err := m.availability(m.repoOn(m.store), filter.StaffId, filter.OfferId, filter.From, filter.To)
	if err != nil {
		return nil, err
	}
	list := TimeSlotList{}
	for i := range slots {
		list = append(list, &slots[i])
	}
	return &list, nil
}
