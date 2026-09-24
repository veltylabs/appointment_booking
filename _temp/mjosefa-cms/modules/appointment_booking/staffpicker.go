package appointment_booking

import (
	"webtyp.com/router"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	staffmanager "github.com/veltylabs/staff_manager"
)

// staffPicker es el selector de profesional que Horario, Servicios y Reserva
// Hora repetían cada uno por su cuenta — un único lugar para cargar
// list_staff y mantener las <option>, en vez de tres copias casi idénticas.
//
// FilterFn, si no es nil, acota qué profesionales se ofrecen (Reserva Hora lo
// usa para el filtro de área/especialidad); Horario y Servicios lo dejan nil
// y ven la lista completa.
type staffPicker struct {
	caller   router.Caller
	sel      *SignalString
	opts     *SignalNodes
	staff    []staffmanager.StaffMember
	FilterFn func(staffmanager.StaffMember) bool
	onChange func(id string)
}

// newStaffPicker construye el picker. onChange, si no es nil, corre después
// de que el usuario elige otra opción (nunca durante la carga inicial).
func newStaffPicker(caller router.Caller, onChange func(id string)) *staffPicker {
	return &staffPicker{caller: caller, sel: NewString(""), opts: NewNodes(), onChange: onChange}
}

// load trae la lista completa de profesionales, arma las opciones, elige la
// primera disponible si no había ninguna elegida, y corre then al terminar.
func (p *staffPicker) load(then func()) {
	out := &staffmanager.StaffMemberList{}
	p.caller.Call(staffmanager.ModelName+"."+staffmanager.OpListStaff, &staffmanager.ListStaffArgs{}, out,
		func(err error) {
			if err != nil {
				return
			}
			p.staff = make([]staffmanager.StaffMember, 0, len(*out))
			for _, sm := range *out {
				p.staff = append(p.staff, *sm)
			}
			p.rebuildOptions()
			if p.sel.Get() == "" {
				if first, ok := p.firstVisible(); ok {
					p.sel.Set(first.Id)
				}
			}
			if then != nil {
				then()
			}
		})
}

func (p *staffPicker) visible() []staffmanager.StaffMember {
	if p.FilterFn == nil {
		return p.staff
	}
	out := make([]staffmanager.StaffMember, 0, len(p.staff))
	for _, sm := range p.staff {
		if p.FilterFn(sm) {
			out = append(out, sm)
		}
	}
	return out
}

func (p *staffPicker) firstVisible() (staffmanager.StaffMember, bool) {
	v := p.visible()
	if len(v) == 0 {
		return staffmanager.StaffMember{}, false
	}
	return v[0], true
}

// rebuildOptions repinta las <option> según FilterFn y la selección actual —
// se llama tras cargar, tras elegir otra opción, y tras cambiar el filtro
// (Reserva Hora la llama de nuevo al cambiar de área).
func (p *staffPicker) rebuildOptions() {
	visible := p.visible()
	opts := make([]*Element, 0, len(visible))
	for _, sm := range visible {
		if sm.Id == p.sel.Get() {
			opts = append(opts, SelectedOption(sm.Id, sm.Name))
		} else {
			opts = append(opts, Option(sm.Id, sm.Name))
		}
	}
	p.opts.Set(opts)
}

// refreshAfterFilterChange se llama cuando FilterFn cambió por fuera (Reserva
// Hora, al cambiar de área): si la selección actual quedó fuera del filtro,
// elige la primera visible (o ninguna); siempre repinta.
func (p *staffPicker) refreshAfterFilterChange() {
	stillVisible := false
	for _, sm := range p.visible() {
		if sm.Id == p.sel.Get() {
			stillVisible = true
			break
		}
	}
	if !stillVisible {
		if first, ok := p.firstVisible(); ok {
			p.sel.Set(first.Id)
		} else {
			p.sel.Set("")
		}
	}
	p.rebuildOptions()
}

func (p *staffPicker) onSelectChange(id string) {
	p.sel.Set(id)
	p.rebuildOptions()
	if p.onChange != nil {
		p.onChange(id)
	}
}

// nameOf busca el nombre de un profesional por id en la lista ya cargada —
// scan lineal sobre una lista corta, mismo trade-off que el resto del
// ecosistema hace para listas de este tamaño.
func (p *staffPicker) nameOf(id string) string {
	for _, sm := range p.staff {
		if sm.Id == id {
			return sm.Name
		}
	}
	return ""
}

func (p *staffPicker) render(selectName string) *Element {
	return Div().
		Child(Label().Text("Profesional")).
		Child(NewElement("select").Attr("name", selectName).BindChildren(p.opts).
			OnChange(func(e Event) { p.onSelectChange(e.TargetValue()) }))
}
