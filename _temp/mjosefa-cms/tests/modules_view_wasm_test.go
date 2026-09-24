//go:build wasm

// Carril "vista / componente" de AGENTS.md para los módulos que llegaron con
// la oleada de agendamiento. Corre en navegador real (DOM real) con un
// router.Caller falso: lo que prueba es la CONSTRUCCIÓN de cada pantalla y su
// primera carga, no el dominio — eso ya lo cubre el carril de integración en
// appointment_booking_rules_test.go contra el servidor real.
//
// Por qué vale la pena: el bug "0/0" de esta misma oleada (listas que
// mostraban cero filas con datos en la base) no lo atrapó ningún test de
// dominio, porque el dominio estaba bien; lo que fallaba era la vista al
// pedir su primera página. Un test que sólo construye el módulo y verifica
// que pidió su lista al iniciarse es exactamente lo que faltaba.
package tests

import (
	"strings"
	"testing"

	"webtyp.com/dom"
	"webtyp.com/json"
	"webtyp.com/model"

	ab "github.com/veltylabs/appointment_booking"
	businesscalendar "github.com/veltylabs/business_calendar"
	patientdirectory "github.com/veltylabs/patient_directory"
	staffmanager "github.com/veltylabs/staff_manager"

	"github.com/veltylabs/mjosefa-cms/modules"
	"github.com/veltylabs/mjosefa-cms/modules/appointment_booking"
	"github.com/veltylabs/mjosefa-cms/modules/business_calendar"
	"github.com/veltylabs/mjosefa-cms/modules/patient_directory"
	"github.com/veltylabs/mjosefa-cms/modules/personal"
)

// initView monta la vista del módulo como lo hace el chasis: Init una vez y
// luego Render, y devuelve el HTML resultante.
//
// LÍMITE CONOCIDO, medido, no supuesto: esto alcanza para una pantalla de UN
// crudview — su primer Reload sale en Init — pero NO para una de pestañas.
// decktabs monta cada panel cuando el árbol entra al DOM de verdad, y el Init
// de cada crudview hijo corre ahí (dom.initComponent durante el reconcile).
// Montar de verdad desde este carril (dom.Render sobre "body"/"app") se probó
// y cuelga el test, así que las pantallas de pestañas se verifican por lo que
// sí es observable sin montar — que las pestañas existen y están rotuladas —
// y su primera carga queda cubierta por el navegador real, no por aquí.
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

// called reporta si alguna de las ops pedidas fue invocada.
func called(ops []string, want string) bool {
	for _, op := range ops {
		if op == want {
			return true
		}
	}
	return false
}

func TestWASM_PatientDirectory_ViewListsOnInit(t *testing.T) {
	var ops []string
	mock := &mockCaller{
		onCall: func(op string, args model.Encodable, into model.Decodable) error {
			ops = append(ops, op)
			if op == patientdirectory.ModelName+"."+patientdirectory.OpListPatients {
				list := patientdirectory.PatientList{
					{Id: "p1", TenantId: modules.TenantID, Rut: "11111111-1", Name: "Paciente Uno", IsActive: true},
				}
				var out []byte
				_ = json.Encode(&list, &out)
				return json.Decode(string(out), into)
			}
			return nil
		},
	}

	m, err := patient_directory.Browser(mock, &testIDGen{}, modules.TenantID)
	if err != nil {
		t.Fatalf("patient_directory.Browser: %v", err)
	}
	initView(m)

	if !called(ops, patientdirectory.ModelName+"."+patientdirectory.OpListPatients) {
		t.Errorf("la pantalla de pacientes debe pedir su lista al iniciarse, ops=%v", ops)
	}
}

func TestWASM_BusinessCalendar_ViewListsItsThreeTabsOnInit(t *testing.T) {
	var ops []string
	mock := &mockCaller{
		onCall: func(op string, args model.Encodable, into model.Decodable) error {
			ops = append(ops, op)
			return nil
		},
	}

	m, err := business_calendar.Browser(mock, &testIDGen{}, modules.TenantID)
	if err != nil {
		t.Fatalf("business_calendar.Browser: %v", err)
	}
	html := initView(m)

	// Las tres pestañas son tres crudview independientes sobre los tres
	// presenters de business_calendar. Que estén las tres y rotuladas es lo
	// que este carril puede afirmar (ver initView); una pestaña que se cayera
	// del deck desaparecería de la pantalla sin ningún error.
	for _, label := range []string{"Horario", "Feriados", "Cierres"} {
		if !strings.Contains(html, label) {
			t.Errorf("falta la pestaña %q en la pantalla de Calendario", label)
		}
	}
	_ = businesscalendar.ModelName
}

func TestWASM_AppointmentBooking_ViewBuildsAndLoadsStaff(t *testing.T) {
	var ops []string
	mock := &mockCaller{
		onCall: func(op string, args model.Encodable, into model.Decodable) error {
			ops = append(ops, op)
			if op == staffmanager.ModelName+"."+staffmanager.OpListStaff {
				list := staffmanager.StaffMemberList{
					{Id: "s1", TenantId: modules.TenantID, UserId: "u1", Rut: "44444444-4", Name: "Dra. Soto", Specialty: "General", IsActive: true},
				}
				var out []byte
				_ = json.Encode(&list, &out)
				return json.Decode(string(out), into)
			}
			return nil
		},
	}

	m, err := appointment_booking.Browser(mock, &testIDGen{}, modules.TenantID)
	if err != nil {
		t.Fatalf("appointment_booking.Browser: %v", err)
	}
	initView(m)

	// El área (especialidad) se DERIVA de la lista de funcionarios, nunca se
	// hardcodea — por eso la pantalla de reserva arranca pidiendo el staff.
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
					{Id: "s1", TenantId: modules.TenantID, UserId: "u1", Rut: "44444444-4", Name: "Dra. Soto", IsActive: true},
				}
				var out []byte
				_ = json.Encode(&list, &out)
				return json.Decode(string(out), into)
			}
			return nil
		},
	}

	m, err := personal.Browser(mock, &testIDGen{}, modules.TenantID)
	if err != nil {
		t.Fatalf("personal.Browser: %v", err)
	}
	html := initView(m)

	// Las tres pestañas de la fusión decidida en docs/PLAN_LOCAL.md: datos del
	// funcionario, su horario y sus servicios. "Servicios" llegó después que
	// las otras dos (estaba bloqueada en D12), así que es justo la que una
	// regresión dejaría fuera sin que nada falle.
	for _, label := range []string{"Datos", "Horario", "Servicios"} {
		if !strings.Contains(html, label) {
			t.Errorf("falta la pestaña %q en la pantalla de Personal", label)
		}
	}
	_ = ops
}

// TestWASM_Browser_BuildsEveryModule: el registro completo, la misma llamada
// que config/client.go hace al arrancar. Un módulo que falle al construirse
// desaparece del riel de navegación, y antes eso era una línea de log y un
// ítem ausente en silencio (ver docs/PLAN.md §1.2) — ahora es un error, y
// este test es lo que lo mantiene así.
func TestWASM_Browser_BuildsEveryModule(t *testing.T) {
	mock := &mockCaller{onCall: func(string, model.Encodable, model.Decodable) error { return nil }}

	views, err := modules.Browser(mock, &testIDGen{}, modules.TenantID, "user-1")
	if err != nil {
		t.Fatalf("modules.Browser: %v", err)
	}
	if len(views) != 7 {
		t.Fatalf("el riel de navegación son 7 módulos (catálogo, equipos, ficha, pacientes, calendario, reserva, personal), got %d", len(views))
	}
	for _, v := range views {
		if v.ModelName() == "" {
			t.Error("cada módulo del riel debe declarar su identidad")
		}
	}
}

// El tipo ab se usa sólo para nombrar ops en los comentarios de arriba; este
// guard evita que un import quede huérfano si un caso se reescribe.
var _ = ab.ModelName
