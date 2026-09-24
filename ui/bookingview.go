package ui

import (
	"webtyp.com/components/calendarslider"
	"webtyp.com/components/targethour"
	"webtyp.com/fmt"
	"webtyp.com/layout/crudview"
	"webtyp.com/layout/rightpanel"
	"webtyp.com/model"
	"webtyp.com/router"
	tintime "webtyp.com/time"
	"webtyp.com/unixid"
	"webtyp.com/view"
	"webtyp.com/widget"

	. "webtyp.com/dom"
	. "webtyp.com/html"

	ab "github.com/veltylabs/appointment_booking"
	itemcatalog "github.com/veltylabs/item_catalog"
	staffmanager "github.com/veltylabs/staff_manager"
)

// NameBookingView es la identidad de widget de esta pantalla.
const NameBookingView = widget.Name("bookingview")

const PartBookingHeader = widget.Part("header")

var (
	clsBookingRoot   = NameBookingView.Root()
	clsBookingHeader = NameBookingView.Class(PartBookingHeader)
)

func (v *BookingView) WidgetName() widget.Name { return NameBookingView }
func (v *BookingView) WidgetKind() widget.Kind { return widget.Region }

const (
	bookingWindowPastDays   = 30
	bookingWindowFutureDays = 365
)

// NewBookingView construye la pantalla "Reserva Hora": área (especialidad) →
// profesional → servicio → día → hueco.
func NewBookingView(caller router.Caller, tenantID string) Component {
	v := &BookingView{
		caller:   caller,
		tenantID: tenantID,
		area:     NewString(""),
		service:  NewString(""),
		day:      NewString(""),
	}
	v.areaOpts = NewNodes()
	v.serviceOpts = NewNodes()
	v.panel = NewNodes()
	v.picker = newStaffPicker(caller, func(string) { v.reloadServices() })
	v.picker.FilterFn = v.staffInArea
	return v
}

type BookingView struct {
	Element
	caller   router.Caller
	tenantID string

	area     *SignalString
	areaOpts *SignalNodes

	picker *staffPicker

	serviceConfigs []ab.EmployeeServiceConfig
	catalogNames   []fmt.KeyValue
	service        *SignalString
	serviceOpts    *SignalNodes

	day *SignalString

	panel  *SignalNodes
	target *targethour.TargetHour
}

func (v *BookingView) Init(_ Ctx) {
	v.picker.load(func() {
		v.rebuildAreaOptions()
		v.reloadServices()
	})
}

func (v *BookingView) staffInArea(sm staffmanager.StaffMember) bool {
	return v.area.Get() == "" || sm.Specialty == v.area.Get()
}

func (v *BookingView) rebuildAreaOptions() {
	var areas []string
	for _, sm := range v.picker.staff {
		if sm.Specialty == "" {
			continue
		}
		found := false
		for _, a := range areas {
			if a == sm.Specialty {
				found = true
				break
			}
		}
		if !found {
			areas = append(areas, sm.Specialty)
		}
	}

	opts := make([]*Element, 0, len(areas)+1)
	if v.area.Get() == "" {
		opts = append(opts, SelectedOption("", "Todas"))
	} else {
		opts = append(opts, Option("", "Todas"))
	}
	for _, a := range areas {
		if a == v.area.Get() {
			opts = append(opts, SelectedOption(a, a))
		} else {
			opts = append(opts, Option(a, a))
		}
	}
	v.areaOpts.Set(opts)
}

func (v *BookingView) onAreaChange(area string) {
	v.area.Set(area)
	v.picker.refreshAfterFilterChange()
	v.reloadServices()
}

func (v *BookingView) reloadServices() {
	staffID := v.picker.sel.Get()
	if staffID == "" {
		v.serviceConfigs = nil
		v.service.Set("")
		v.rebuildServiceOptions()
		v.rebuildPanel()
		return
	}

	var configs ab.EmployeeServiceConfigList
	var items itemcatalog.CatalogItemList
	pending := 2
	done := func() {
		pending--
		if pending != 0 {
			return
		}
		v.serviceConfigs = make([]ab.EmployeeServiceConfig, len(configs))
		for i, c := range configs {
			v.serviceConfigs[i] = *c
		}
		v.catalogNames = make([]fmt.KeyValue, len(items))
		for i, it := range items {
			v.catalogNames[i] = fmt.KeyValue{Key: it.Id, Value: it.Name}
		}
		if v.service.Get() == "" && len(v.serviceConfigs) > 0 {
			v.service.Set(v.serviceConfigs[0].Id)
		}
		v.rebuildServiceOptions()
		v.rebuildPanel()
	}

	v.caller.Call(ab.ModelName+"."+ab.OpListEmployeeServiceConfigsByStaff,
		&ab.ListEmployeeServiceConfigsByStaffArgs{TenantId: v.tenantID, StaffId: staffID}, &configs,
		func(error) { done() })
	v.caller.Call(itemcatalog.ModelName+"."+itemcatalog.OpListItems,
		&itemcatalog.ListItemsArgs{TenantId: v.tenantID, ActiveOnly: true}, &items,
		func(error) { done() })
}

func (v *BookingView) catalogName(serviceID string) string {
	for _, kv := range v.catalogNames {
		if kv.Key == serviceID {
			return kv.Value
		}
	}
	return ""
}

func (v *BookingView) rebuildServiceOptions() {
	opts := make([]*Element, 0, len(v.serviceConfigs))
	for _, cfg := range v.serviceConfigs {
		label := v.catalogName(cfg.ServiceId)
		if label == "" {
			label = cfg.ServiceId
		}
		if cfg.Id == v.service.Get() {
			opts = append(opts, SelectedOption(cfg.Id, label))
		} else {
			opts = append(opts, Option(cfg.Id, label))
		}
	}
	v.serviceOpts.Set(opts)
}

func (v *BookingView) onServiceChange(id string) {
	v.service.Set(id)
	v.rebuildServiceOptions()
	v.rebuildPanel()
}

func (v *BookingView) rebuildPanel() {
	staffID := v.picker.sel.Get()
	serviceConfigID := v.service.Get()
	if staffID == "" || serviceConfigID == "" {
		v.panel.Set([]*Element{Div().Text("Elija un profesional con al menos un servicio configurado.")})
		return
	}

	ids, err := unixid.NewUnixID()
	if err != nil {
		v.panel.Set([]*Element{Div().Text("Error interno: " + err.Error())})
		return
	}

	now := tintime.Now() / 1000000000
	cfg := ab.FormConfig{
		TenantId:        v.tenantID,
		StaffId:         staffID,
		ServiceConfigId: serviceConfigID,
		Timezone:        timezoneClinic,
		From:            now - bookingWindowPastDays*86400,
		To:              now + bookingWindowFutureDays*86400,
	}

	cal := &calendarslider.CalendarSlider{NumMonths: 3}
	cal.OnFilterChange(func(day string) {
		v.day.Set(day)
		v.refreshFreeSlots(cfg)
	})

	cv, err := crudview.New(crudview.Config{
		ParentID:  ID + ".booking." + staffID + "." + serviceConfigID,
		Presenter: byDay{Presenter: ab.NewFormView(v.caller, cfg)},
		IDs:       ids,
		Filter:    cal,
		List: func(selected *SignalString, onSelect func(view.Item)) crudview.ListView {
			v.target = &targethour.TargetHour{
				Selected: selected,
				OnSelect: onSelect,
				StatusOf: func(it view.Item) targethour.Status {
					switch it.Description {
					case ab.StatusConfirmed:
						return targethour.StatusConfirmed
					case ab.StatusCompleted, ab.StatusNoShow:
						return targethour.StatusAttended
					}
					return targethour.StatusPending
				},
			}
			return v.target
		},
		OnAfterReload: func(list crudview.ListView) {
			if t, ok := list.(*targethour.TargetHour); ok {
				freeSlotsCache(v.caller, cfg, v.day.Get(), func(slots []string) {
					t.FreeSlots = slots
					t.SetItems(t.Items())
				})
			}
		},
	})
	if err != nil {
		v.panel.Set([]*Element{Div().Text("Error interno: " + err.Error())})
		return
	}

	v.panel.Set([]*Element{Div().Child(cv)})
}

func freeSlotsCache(caller router.Caller, cfg ab.FormConfig, day string, done func(slots []string)) {
	if day == "" {
		done(nil)
		return
	}
	ab.FreeSlots(caller, cfg, day, func(slots []string, err error) {
		if err != nil {
			done(nil)
			return
		}
		done(slots)
	})
}

func (v *BookingView) refreshFreeSlots(cfg ab.FormConfig) {
	if v.target == nil {
		return
	}
	target := v.target
	freeSlotsCache(v.caller, cfg, v.day.Get(), func(slots []string) {
		target.FreeSlots = slots
		target.SetItems(target.Items())
	})
}

func (v *BookingView) confirmSelected() {
	if v.target == nil {
		return
	}
	id := v.target.Selected.Get()
	if id == "" {
		return
	}
	var res ab.Reservation
	v.caller.Call(ab.ModelName+"."+ab.OpGetReservation, &ab.GetReservationArgs{TenantId: v.tenantID, Id: id}, &res,
		func(err error) {
			if err != nil {
				return
			}
			v.caller.Call(ab.ModelName+"."+ab.OpChangeReservationStatus, &ab.ChangeReservationStatusArgs{
				TenantId: v.tenantID, Id: id, Event: ab.EventConfirm, Revision: res.Revision,
			}, nil, func(error) {
				v.rebuildPanel()
			})
		})
}

func (v *BookingView) Render() *Element {
	controls := Div().Set(clsBookingHeader.AsAttr()).
		Child(Label().Text("Área")).
		Child(NewElement("select").Attr("name", "booking-area").BindChildren(v.areaOpts).
			OnChange(func(e Event) { v.onAreaChange(e.TargetValue()) })).
		Child(v.picker.render("booking-staff")).
		Child(Label().Text("Servicio")).
		Child(NewElement("select").Attr("name", "booking-service").BindChildren(v.serviceOpts).
			OnChange(func(e Event) { v.onServiceChange(e.TargetValue()) })).
		Child(Button().Attr("type", "button").Text("Confirmar reserva elegida").
			OnClick(func(Event) { v.confirmSelected() }))

	panel := &rightpanel.RightPanel{
		Title:        NavLabel,
		HeadControls: controls,
		Article:      Div().BindChildren(v.panel),
	}
	return Div().Set(clsBookingRoot.AsAttr()).Child(panel.Render())
}

type byDay struct {
	view.Presenter
}

func (p byDay) Filter(term string) []view.Item {
	if term == "" {
		return nil
	}
	var items []view.Item
	for _, it := range p.Presenter.Items() {
		if it.ID != "" && it.LeadMain != "" {
			items = append(items, it)
		}
	}
	return items
}

func (p byDay) Save(recs []model.Model, done func(error)) {
	if s, ok := p.Presenter.(view.Saver); ok {
		s.Save(recs, done)
		return
	}
	done(fmt.Err("byDay: underlying presenter cannot save"))
}
