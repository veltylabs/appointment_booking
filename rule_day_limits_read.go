package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
)

// Read responde la lectura de day_limits: si el establecimiento abre en date (medianoche UTC) y entre qué minutos.
// crud ya validó los argumentos, aplicó el tope de página y comprobó el permiso antes de llamarte.
func (p *DayLimits) Read(a crud.CurrentUser, filter *DayLimits, args crud.ReadArgs, m *Module) (model.ModelSlice, error) {
	limits, err := m.GetDayLimits(filter.Date)
	if err != nil {
		return nil, err
	}
	return &DayLimitsList{{Date: filter.Date, Open: limits.Open, OpenMin: int64(limits.OpenMin), CloseMin: int64(limits.CloseMin)}}, nil
}
