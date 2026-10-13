package ui

import (
	"webtyp.com/components/chipgrid"
	"webtyp.com/lang"

	. "webtyp.com/dom"
)

// anyStaffID es la opción "Cualquier profesional disponible".
const anyStaffID = "any"

// stepStaffPicker es el paso 2 (diagrama §3): los profesionales que ofrecen el
// servicio y, si son más de uno, "Cualquier profesional disponible" primero.
type stepStaffPicker struct {
	Element
	f     *BookingFlow
	staff *chipgrid.ChipGrid
	el    *Element
	chips []chipgrid.Chip
}

func newStepStaffPicker(f *BookingFlow) *stepStaffPicker {
	s := &stepStaffPicker{f: f}
	s.staff = &chipgrid.ChipGrid{OnPick: s.pick}
	s.el = s.staff.Render()
	return s
}

func (s *stepStaffPicker) Render() *Element { return stepBox().Child(s.el) }

func (s *stepStaffPicker) show() {
	offers := s.f.choice.offers
	s.chips = nil
	if len(offers) > 1 { // U3-sí
		s.chips = append(s.chips, chipgrid.Chip{Value: anyStaffID, Label: lang.Translate(textAnyProfessional).String()})
	}
	for _, o := range offers {
		s.chips = append(s.chips, chipgrid.Chip{Value: o.StaffId, Label: o.StaffName})
	}
	s.staff.Selected.Set("")
	s.staff.SetChips(s.chips)
}

func (s *stepStaffPicker) pick(value string) {
	label := ""
	for _, c := range s.chips {
		if c.Value == value {
			label = c.Label
		}
	}
	c := &s.f.choice
	c.anyStaff = value == anyStaffID
	c.staffID = value
	if c.anyStaff {
		c.staffID = ""
	}
	s.f.next(label)
}
