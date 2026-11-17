package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
)

// Read responde la lectura de bookable_service: lo que un visitante puede reservar, de la especialidad pedida o de todas. Es público.
// crud ya validó los argumentos, aplicó el tope de página y comprobó el permiso antes de llamarte.
func (p *BookableService) Read(a crud.CurrentUser, filter *BookableService, args crud.ReadArgs, m *Module) (model.ModelSlice, error) {
	rows, err := m.ListBookableServices(m.tenantID, filter.Specialty)
	if err != nil {
		return nil, err
	}
	list := BookableServiceList{}
	for i := range rows {
		list = append(list, &rows[i])
	}
	return &list, nil
}
