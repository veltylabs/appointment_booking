//go:build wasm

package tests

import (
	"strings"
	"testing"

	"webtyp.com/dom"
	"webtyp.com/fmt"
	"webtyp.com/json"
	"webtyp.com/model"

	ab "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/appointment_booking/ui"
	staffmanager "github.com/veltylabs/staff_manager"
)

const testTenantID = "demo"

type testIDGen struct{ n int }

func (g *testIDGen) NewID() string {
	g.n++
	return "test-id-" + fmt.Convert(g.n).String()
}

type mockCaller struct {
	lastOp   string
	lastArgs model.Encodable
	onCall   func(op string, args model.Encodable, into model.Decodable) error
}

func (m *mockCaller) Call(op string, args model.Encodable, into model.Decodable, done func(err error)) {
	m.lastOp = op
	m.lastArgs = args
	if m.onCall != nil {
		err := m.onCall(op, args, into)
		done(err)
	} else {
		done(nil)
	}
}

func (m *mockCaller) Dispatch(op string, args model.Encodable) {
	m.lastOp = op
	m.lastArgs = args
}

func initView(m interface{ View() dom.Component }) string {
	comp := m.View()
	if initer, ok := comp.(interface{ Init(ctx dom.Ctx) }); ok {
		initer.Init(nil)
	}
	if r, ok := comp.(dom.ViewRenderer); ok {
		return r.Render().String()
	}
	return comp.String()
}

func called(ops []string, want string) bool {
	for _, op := range ops {
		if op == want {
			return true
		}
	}
	return false
}

func TestWASM_AppointmentBooking_ViewBuildsAndLoadsStaff(t *testing.T) {
	var ops []string
	mock := &mockCaller{
		onCall: func(op string, args model.Encodable, into model.Decodable) error {
			ops = append(ops, op)
			if op == staffmanager.ModelName+"."+staffmanager.OpListStaff {
				list := staffmanager.StaffMemberList{
					{Id: "s1", TenantId: testTenantID, UserId: "u1", Rut: "44444444-4", Name: "Dra. Soto", Specialty: "Medicina General", IsActive: true},
				}
				var out []byte
				_ = json.Encode(&list, &out)
				return json.Decode(string(out), into)
			}
			return nil
		},
	}

	m, err := ui.Browser(mock, &testIDGen{}, testTenantID)
	if err != nil {
		t.Fatalf("ui.Browser: %v", err)
	}
	initView(m)

	if !called(ops, staffmanager.ModelName+"."+staffmanager.OpListStaff) {
		t.Errorf("Reserva Hora deriva sus áreas del staff y debe pedirlo al iniciarse, ops=%v", ops)
	}
}

func TestWASM_Personal_ViewBuildsWithItsTabs(t *testing.T) {
	var ops []string
	mock := &mockCaller{
		onCall: func(op string, args model.Encodable, into model.Decodable) error {
			ops = append(ops, op)
			if op == staffmanager.ModelName+"."+staffmanager.OpListStaff {
				list := staffmanager.StaffMemberList{
					{Id: "s1", TenantId: testTenantID, UserId: "u1", Rut: "44444444-4", Name: "Dra. Soto", IsActive: true},
				}
				var out []byte
				_ = json.Encode(&list, &out)
				return json.Decode(string(out), into)
			}
			return nil
		},
	}

	m, err := ui.PersonalBrowser(mock, &testIDGen{}, testTenantID)
	if err != nil {
		t.Fatalf("ui.PersonalBrowser: %v", err)
	}
	html := initView(m)

	for _, label := range []string{"Datos y dispositivos", "Horario", "Servicios"} {
		if !strings.Contains(html, label) {
			t.Errorf("falta la pestaña %q en la pantalla de Personal", label)
		}
	}
	_ = ops
}

var _ = ab.ModelName
