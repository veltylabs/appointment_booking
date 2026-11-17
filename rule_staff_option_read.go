package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
)

// Read responde la lectura de staff_option: los profesionales que se pueden elegir en una lista.
// crud ya validó los argumentos, aplicó el tope de página y comprobó el permiso antes de llamarte.
func (p *StaffOption) Read(a crud.CurrentUser, filter *StaffOption, args crud.ReadArgs, m *Module) (model.ModelSlice, error) {
	options, err := m.staff.OccupantOptions(m.tenantID)
	if err != nil {
		return nil, err
	}
	list := StaffOptionList{}
	for _, o := range options {
		list = append(list, &StaffOption{Id: o.Key, Name: o.Value})
	}
	return &list, nil
}
