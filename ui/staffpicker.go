package ui

import (
	"webtyp.com/router"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	ab "github.com/veltylabs/appointment_booking"
)

// staffPicker es el selector de profesional que Horario, Servicios y Reserva
// Hora utilizan para cargar la lista de personal y mantener sus opciones.
type staffPicker struct {
	caller   router.Caller
	tenantID string
	sel      *SignalString
	opts     *SignalNodes
	staff    []ab.StaffOption
	FilterFn func(ab.StaffOption) bool
	onChange func(id string)
}

// newStaffPicker construye el picker. onChange, si no es nil, se ejecuta
// después de que el usuario elige otra opción.
func newStaffPicker(caller router.Caller, tenantID string, onChange func(id string)) *staffPicker {
	return &staffPicker{caller: caller, tenantID: tenantID, sel: NewString(""), opts: NewNodes(), onChange: onChange}
}

// load trae la lista completa de opciones de profesionales, construye las opciones y
// selecciona la primera disponible si no había ninguna seleccionada.
func (p *staffPicker) load(then func()) {
	out := &ab.StaffOptionList{}
	p.caller.Call(ab.ModelName+"."+ab.OpListStaffOptions, &ab.ListStaffOptionsArgs{TenantId: p.tenantID}, out,
		func(err error) {
			if err != nil {
				return
			}
			p.staff = make([]ab.StaffOption, 0, len(*out))
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

func (p *staffPicker) visible() []ab.StaffOption {
	if p.FilterFn == nil {
		return p.staff
	}
	out := make([]ab.StaffOption, 0, len(p.staff))
	for _, sm := range p.staff {
		if p.FilterFn(sm) {
			out = append(out, sm)
		}
	}
	return out
}

func (p *staffPicker) firstVisible() (ab.StaffOption, bool) {
	v := p.visible()
	if len(v) == 0 {
		return ab.StaffOption{}, false
	}
	return v[0], true
}

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
