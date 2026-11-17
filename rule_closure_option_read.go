package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/time"
)

// Read responde la lectura de closure_option: los días cerrados del establecimiento entre from y to, feriados primero y luego cierres.
// Sin horario del establecimiento (Deps.Hours vacío) no hay días cerrados.
// crud ya validó los argumentos, aplicó el tope de página y comprobó el permiso antes de llamarte.
func (p *ClosureOption) Read(a crud.CurrentUser, filter *ClosureOption, args crud.ReadArgs, m *Module) (model.ModelSlice, error) {
	list := ClosureOptionList{}
	if m.hours == nil {
		return &list, nil
	}
	holidays, closures, err := m.hours.ClosedDates(time.Unix(filter.From), time.Unix(filter.To))
	if err != nil {
		return nil, err
	}
	for _, d := range holidays {
		list = append(list, &ClosureOption{Date: d, IsHoliday: true})
	}
	for _, d := range closures {
		list = append(list, &ClosureOption{Date: d})
	}
	return &list, nil
}
