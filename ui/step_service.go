package ui

import (
	"webtyp.com/components/chipgrid"
	"webtyp.com/fmt"
	"webtyp.com/lang"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	ab "github.com/veltylabs/appointment_booking"
)

// stepServicePicker es el paso 1: chips de especialidades y la lista de sus
// servicios (nombre · duración · precio). Diagrama booking_flow.md §2.
type stepServicePicker struct {
	Element
	f      *BookingFlow
	errMsg *SignalString
	body   *SignalNodes

	specialties *chipgrid.ChipGrid
	services    *chipgrid.ChipGrid
	chipsEl     *Element
	listEl      *Element

	all []ab.BookableService
}

func newStepServicePicker(f *BookingFlow) *stepServicePicker {
	s := &stepServicePicker{f: f, errMsg: NewString(""), body: NewNodes()}
	s.specialties = &chipgrid.ChipGrid{OnPick: s.pickSpecialty}
	s.services = &chipgrid.ChipGrid{OnPick: s.pickService}
	s.chipsEl = s.specialties.Render()
	s.listEl = s.services.Render()
	return s
}

func (s *stepServicePicker) Render() *Element {
	return stepBox().Child(errorLine(s.errMsg), stepBox().BindChildren(s.body))
}

// load pide los servicios reservables (U1, U2).
func (s *stepServicePicker) load() {
	s.errMsg.Set("")
	s.body.Set(nil)
	list := ab.BookableServiceList{}
	s.f.caller.Call(ab.ModelName+"."+ab.OpListBookableServices,
		&ab.ListBookableServicesArgs{TenantId: s.f.tenantID}, &list,
		func(err error) {
			if err != nil { // U1-sí
				s.errMsg.Set(err.Error())
				s.body.Set([]*Element{actionButton(textRetry, nil, s.load)})
				return
			}
			s.all = s.all[:0]
			for _, it := range list {
				s.all = append(s.all, *it)
			}
			if len(s.all) == 0 { // U2-no
				s.body.Set([]*Element{P().Text(lang.Translate(textNoServicesToBook).String())})
				return
			}
			s.specialties.SetChips(specialtyChips(s.all))
			s.services.SetChips(nil)
			s.body.Set([]*Element{s.chipsEl})
		})
}

// specialtyChips son las especialidades sin repetir, en el orden de la lista
// (ya viene ordenada por especialidad).
func specialtyChips(all []ab.BookableService) []chipgrid.Chip {
	var chips []chipgrid.Chip
	for _, svc := range all {
		if len(chips) == 0 || chips[len(chips)-1].Value != svc.Specialty {
			chips = append(chips, chipgrid.Chip{Value: svc.Specialty, Label: svc.Specialty})
		}
	}
	return chips
}

// pickSpecialty muestra los servicios de esa especialidad, uno por servicio
// aunque lo ofrezcan varios profesionales: "Consulta · 30 min · $25.000".
func (s *stepServicePicker) pickSpecialty(specialty string) {
	s.f.choice.specialty = specialty
	var chips []chipgrid.Chip
	for _, svc := range s.all {
		if svc.Specialty != specialty || hasChip(chips, svc.ServiceId) {
			continue
		}
		chips = append(chips, chipgrid.Chip{
			Value: svc.ServiceId,
			Label: svc.ServiceName + " · " + fmt.Convert(svc.DurationMin).String() + " min · " + priceLabel(svc.Price, svc.Currency),
		})
	}
	s.services.SetChips(chips)
	s.body.Set([]*Element{s.chipsEl, s.listEl})
}

func hasChip(chips []chipgrid.Chip, value string) bool {
	for _, c := range chips {
		if c.Value == value {
			return true
		}
	}
	return false
}

// pickService guarda las configuraciones que ofrecen el servicio y avanza con
// "Especialidad · Servicio".
func (s *stepServicePicker) pickService(serviceID string) {
	c := &s.f.choice
	c.offers = nil
	for _, svc := range s.all {
		if svc.ServiceId == serviceID && svc.Specialty == c.specialty {
			c.offers = append(c.offers, svc)
			c.serviceName = svc.ServiceName
		}
	}
	s.f.next(c.specialty + " · " + c.serviceName)
}
