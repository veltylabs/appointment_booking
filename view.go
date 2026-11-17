package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/view"
)

// staffCaller acota las llamadas de una pantalla a un profesional: lo que lee lleva su staff_id como filtro (la
// entidad es su propio filtro) y lo que crea queda a su nombre. Así la pantalla de servicios muestra y guarda solo
// las configuraciones del profesional elegido, sin un lister propio.
type staffCaller struct {
	router.Caller
	staffId string
}

func (c staffCaller) Call(op router.Operation, args model.Encodable, into model.Decodable, done func(error)) {
	if op.Entity == (&EmployeeServiceConfig{}).ModelName() {
		switch a := args.(type) {
		case *crud.ReadArgs:
			a.Filter = &EmployeeServiceConfig{StaffId: c.staffId}
		case *crud.CreateArgs:
			for _, rec := range a.Records {
				if cfg, ok := rec.(*EmployeeServiceConfig); ok {
					cfg.StaffId = c.staffId
				}
			}
		}
	}
	c.Caller.Call(op, args, into, done)
}

// NewEmployeeServiceConfigView construye el Presenter de los servicios que realiza un profesional.
//
// Ejemplo: p := appointmentbooking.NewEmployeeServiceConfigView(caller, staffId)
func NewEmployeeServiceConfigView(caller router.Caller, staffId string) view.Presenter {
	lister := view.NewCallerLister(view.NewRecords().Caller(staffCaller{Caller: caller, staffId: staffId}),
		view.Ops{Module: ModelName, Page: view.Page{First: view.All}},
		func() model.TableSlice { return &EmployeeServiceConfigList{} })
	return view.New(lister, &EmployeeServiceConfig{}, view.WithTitle("Services"))
}
