//go:build wasm

package tests

import (
	"strings"
	"syscall/js"
	"testing"

	"webtyp.com/dom"
	"webtyp.com/json"
	"webtyp.com/model"
	tintime "webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/ui"
	staffmanager "github.com/veltylabs/staff_manager"
)

func encodeInto(src model.Encodable, into model.Decodable) error {
	var out []byte
	if err := json.Encode(src, &out); err != nil {
		return err
	}
	return json.Decode(string(out), into)
}

// El calendario de Reserva Hora solo vuelve elegible un día que trae ocupación
// (calendarslider.Occupation). Si la pantalla no se la pasa, ningún día se puede
// elegir y el flujo muere antes de ver las horas libres.
func TestWASM_BookingView_DayWithFreeSlotIsSelectable(t *testing.T) {
	today := tintime.Now() / 1000000000 / 86400 * 86400
	// La hora libre se muestra y se agrupa por día en la zona local del navegador.
	freeAt := today + 15*3600
	todayKey := tintime.FormatDate(freeAt * 1000000000)
	freeHour := tintime.FormatTime(freeAt * 1000000000)[:5]
	mock := &mockCaller{
		onCall: func(op string, args model.Encodable, into model.Decodable) error {
			switch op {
			case staffmanager.ModelName + "." + staffmanager.OpListStaff:
				return encodeInto(&staffmanager.StaffMemberList{
					{Id: "s1", TenantId: testTenantID, UserId: "u1", Rut: "44444444-4", Name: "Dra. Soto", Specialty: "Medicina General", IsActive: true},
				}, into)
			case ab.ModelName + "." + ab.OpListEmployeeServiceConfigsByStaff:
				return encodeInto(&ab.EmployeeServiceConfigList{
					{Id: "c1", TenantId: testTenantID, StaffId: "s1", ServiceId: "i1", DurationMin: 30, IsActive: true},
				}, into)
			case ab.ModelName + "." + ab.OpListReservationsByStaff:
				return encodeInto(&ab.ReservationList{
					{Id: "r-today", TenantId: testTenantID, ClientId: "Ana", StaffIdsnapshot: "s1", ReservationDate: today, ReservationTime: today + 9*3600, DurationMinSnapshot: 30, LocalStringDate: todayKey, LocalStringTime: "09:00", Status: ab.StatusConfirmed},
					{Id: "r-tomorrow", TenantId: testTenantID, ClientId: "Luis", StaffIdsnapshot: "s1", ReservationDate: today + 86400, ReservationTime: today + 86400 + 11*3600, DurationMinSnapshot: 30, LocalStringDate: "otro-dia", LocalStringTime: "11:00", Status: ab.StatusConfirmed},
				}, into)
			case ab.ModelName + "." + ab.OpListAvailability:
				return encodeInto(&ab.TimeSlotList{
					{StartUtc: freeAt, EndUtc: freeAt + 1800},
				}, into)
			}
			return nil
		},
	}

	m, err := ui.Browser(mock, &testIDGen{}, testTenantID)
	if err != nil {
		t.Fatalf("ui.Browser: %v", err)
	}
	doc := js.Global().Get("document")
	host := doc.Call("createElement", "div")
	host.Set("id", "booking-calendar-host")
	doc.Get("body").Call("appendChild", host)
	defer host.Call("remove")
	if err := dom.Render("booking-calendar-host", m.View()); err != nil {
		t.Fatalf("dom.Render: %v", err)
	}

	day := doc.Call("querySelector", "#booking-calendar-host [data-date='"+todayKey+"'] .calendarslider__day-button")
	if day.IsNull() {
		t.Fatal("hoy tiene un hueco libre y el calendario no lo deja elegir: la pantalla no le pasa la ocupación")
	}

	// Elegir el día muestra sus horas: el hueco libre y la reserva de ese día,
	// no la de mañana.
	day.Call("click")
	list := doc.Call("querySelector", "#booking-calendar-host .targethour__list")
	if list.IsNull() {
		t.Fatal("no se encontró la lista de horas")
	}
	if list.Call("querySelector", "[data-free='"+freeHour+"']").IsNull() {
		t.Errorf("al elegir hoy debería aparecer el hueco libre de las %s", freeHour)
	}
	text := list.Get("textContent").String()
	if !strings.Contains(text, "09:00") {
		t.Errorf("al elegir hoy debería aparecer la reserva de las 09:00, lista=%q", text)
	}
	if strings.Contains(text, "11:00") {
		t.Errorf("la reserva de mañana no debería aparecer al elegir hoy, lista=%q", text)
	}
}
