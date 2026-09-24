// Package config is the demo's composition root. A single Env (env.go) builds
// the shared runtime the real-module wrappers consume: an in-memory orm.DB
// seeded, the veltylabs domain modules mounted on a router/loopback caller,
// and an events broker. css.go carries the visual theme and lang.go the
// Spanish dictionary — this root is one place, not three.
package config

import (
	"webtyp.com/events"
	"webtyp.com/events/mock"
	. "webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/router"
	"webtyp.com/router/loopback"
	"webtyp.com/storage/mem"
	tinytime "webtyp.com/time"
	"webtyp.com/unixid"

	ab "github.com/veltylabs/appointment_booking"
	"github.com/veltylabs/item_catalog"
	workschedule "github.com/veltylabs/work_schedule"
)

// TenantID es el id de tenant único de la demo.
const TenantID = "demo"

// StaffOption es un miembro del staff para los pickers de la demo. La lista no
// vive en una tabla (appointment_booking valida staff por el StaffReader
// inyectado, no por una tabla propia); es configuración de la demo, como lo
// sería un módulo staff en producción.
type StaffOption struct {
	ID            string
	Name          string
	SpecialtySlug string
}

// slugTraumatologia / slugMedicinaGeneral / slugEcografia son los slugs
// canónicos que item_catalog ya define — se derivan de su lista para no
// inventar especialidades (ver DEMO_AGENDA_MASTER_PLAN D6).
func slugTraumatologia() string   { return lookupSlug("traumatologia") }
func slugMedicinaGeneral() string { return lookupSlug("medicina-general") }
func slugEcografia() string       { return lookupSlug("ecografia") }

// lookupSlug devuelve el slug canónico de item_catalog, o "" si no existe.
func lookupSlug(slug string) string {
	for _, cs := range itemcatalog.CanonicalSpecialties {
		if cs.Slug == slug {
			return cs.Slug
		}
	}
	return ""
}

// Specialties son las especialidades (áreas) del catálogo REAL — la fuente
// única de la demo para el selector de área. Se leen por OpListSpecialties
// (el módulo item_catalog montado), no de una constante local.
func (e *Env) Specialties() []string {
	out := &itemcatalog.SpecialtyList{}
	var callErr error
	e.caller.Call(itemcatalog.ModelName + "." + itemcatalog.OpListSpecialties,
		&itemcatalog.ListSpecialtiesArgs{TenantId: TenantID},
		out, func(err error) { callErr = err })
	if callErr != nil {
		return nil
	}
	slugs := make([]string, 0, out.Len())
	for i := 0; i < out.Len(); i++ {
		slugs = append(slugs, out.At(i).(*itemcatalog.Specialty).Slug)
	}
	return slugs
}

// ESCForStaff devuelve el employee_service_config.id primario del médico, o ""
// si no tiene. La demo ofrece UN servicio por médico (no hay picker de servicio
// — el Context del crudview es área+médico). Este id es el que consume
// create_reservation (employee_service_config_id) y list_availability
// (config_id — NO el del work_calendar_config, ver service.go).
func (e *Env) ESCForStaff(staffId string) string {
	switch staffId {
	case "staff-natasha":
		return "escNatashaConsulta"
	case "staff-tony":
		return "escTonyCirugia"
	case "staff-thor":
		return "escThorEcografia"
	}
	return ""
}

// Env es la composición de la demo: un orm.DB en memoria sembrado, los módulos
// de dominio reales montados, un router.Caller in-proc y un events broker.
// Se construye UNA vez en web/client.go y se comparte entre módulos.
type Env struct {
	caller   router.Caller
	broker   events.Broker
	ids      model.IDGenerator
	ab       *ab.Module
	ic       *itemcatalog.Module
	ws       *workschedule.Module
	db       *orm.DB
	staff    []StaffOption
	holidays []string
}

// New construye el entorno: DB en memoria, módulos reales en un loopback.Caller
// y seed realista. Compila a WASM: el Caller vive client-side.
func New() *Env {
	db := orm.New(mem.New())

	ids, err := unixid.NewUnixID()
	if err != nil {
		panic(err)
	}
	broker := &mock.Broker{}

	abMod, err := ab.New(db, ab.Deps{
		// La demo confía en su propio seed (lista de staff en memoria + servicio
		// sembrado) — los readers solo validan existencia al cerrar una reserva,
		// y aquí todo lo que la UI ofrece existe. En un despliegue real estos
		// serían los módulos staff / service_catalog / directory de verdad.
		Staff:     stubStaff{},
		Catalog:   stubCatalog{},
		Directory: stubDirectory{},
		IDs:       ids,
		Publisher: broker,
	})
	if err != nil {
		panic(err)
	}

	icMod, err := itemcatalog.New(db, itemcatalog.Deps{IDs: ids, Publisher: broker})
	if err != nil {
		panic(err)
	}

	wsMod := workschedule.New(db)

	env := &Env{
		caller:   loopback.WithTenant(TenantID, abMod, icMod, wsMod),
		broker:   broker,
		ids:      ids,
		ab:       abMod,
		ic:       icMod,
		ws:       wsMod,
		db:       db,
		holidays: holidaysCL2026(),
		staff: []StaffOption{
			{ID: "staff-tony", Name: "Dr. Tony Stark", SpecialtySlug: slugTraumatologia()},
			{ID: "staff-natasha", Name: "Dra. Natasha Romanoff", SpecialtySlug: slugMedicinaGeneral()},
			{ID: "staff-thor", Name: "Dr. Thor Odinson", SpecialtySlug: slugEcografia()},
		},
	}

	env.seed()
	return env
}

// Caller es el router.Caller in-proc que despacha contra los módulos reales
// montados.
func (e *Env) Caller() router.Caller  { return e.caller }
func (e *Env) Broker() events.Broker  { return e.broker }
func (e *Env) IDs() model.IDGenerator { return e.ids }
func (e *Env) TenantID() string       { return TenantID }
func (e *Env) Staff() []StaffOption   { return e.staff }

// Holidays2026 son los feriados de Chile 2026 ("YYYY-MM-DD"), solo lectura. En
// producción vienen de un servicio de feriados; aquí es una constante.
func (e *Env) Holidays2026() []string { return e.holidays }

// seed puebla la DB en memoria vía las ops REALES de appointment_booking cuando
// existe una op, y con db.Create directo cuando el módulo no la expone (las
// excepciones están documentadas en cada método).
func (e *Env) seed() {
	e.upsertCalendarConfigs()
	e.upsertBlocks()
	e.addExceptions()
	e.seedEmployeeServiceConfig()
	e.seedReservations()
	e.seedCatalog()
	e.seedWorkSchedule()
}

// seedWorkSchedule siembra las tablas staff/workcalendar que el módulo
// work_schedule lee (esquema legacy texto, ownership externo — work_schedule
// solo lee, nunca escribe). Es la fuente de la vista "Horario" del módulo
// agenda. Ids enteros 1..3 en el MISMO orden que e.staff, para traducir el
// StaffOption.ID (string) al staff de work_schedule (int64).
func (e *Env) seedWorkSchedule() {
	if len(e.staff) < 3 {
		return
	}
	names := []string{e.staff[0].Name, e.staff[1].Name, e.staff[2].Name}
	for i, name := range names {
		sid := int64(i + 1)
		e.db.Create(&workschedule.Staff{Id: sid, Name: name, Role: "Médico", Email: "doc" + Sprintf("%d", sid) + "@demo.cl"})
		// Horario legado: igual que la plantilla appointment_booking de Natasha.
		if i == 1 { // staff-natasha
			for _, dow := range []int64{1, 2, 3, 4, 5} {
				e.db.Create(&workschedule.WorkCalendar{Id: sid*10 + dow, StaffId: sid, DayOfWeek: dow, StartTime: "09:00", EndTime: "18:00", IsActive: true})
			}
		} else {
			for _, dow := range []int64{1, 3, 5} {
				e.db.Create(&workschedule.WorkCalendar{Id: sid*10 + dow, StaffId: sid, DayOfWeek: dow, StartTime: "08:00", EndTime: "14:00", IsActive: true})
			}
		}
	}
}

// WorkScheduleStaffID traduce el StaffOption.ID (string) al staff de
// work_schedule (int64) que get_work_schedule espera: la posición en la lista
// de staff, 1-based. "" o desconocido → 0.
func (e *Env) WorkScheduleStaffID(staffID string) int64 {
	for i, so := range e.staff {
		if so.ID == staffID {
			return int64(i + 1)
		}
	}
	return 0
}

func (e *Env) upsertCalendarConfigs() {
	for _, s := range e.staff {
		var doneErr error
		e.caller.Call(ab.ModelName + "." + ab.OpUpsertCalendarConfig, &ab.UpsertCalendarConfigArgs{
			TenantId: TenantID, StaffId: s.ID, Timezone: "America/Santiago", IsActive: true,
		}, nil, func(err error) { doneErr = err })
		if doneErr != nil {
			panic(doneErr)
		}
	}
}

func (e *Env) upsertBlocks() {
	call := func(op string, args model.Encodable) {
		var doneErr error
		e.caller.Call(op, args, nil, func(err error) { doneErr = err })
		if doneErr != nil {
			panic(doneErr)
		}
	}
	// Natasha: Lun–Vie 09:00–18:00 con colación 13:00–14:00 (bloque 1: 09:00–13:00, bloque 2: 14:00–18:00).
	for dow := int64(1); dow <= 5; dow++ {
		call(ab.ModelName + "." + ab.OpSaveDayBlocks, &ab.SaveDayBlocksArgs{
			TenantId: TenantID, StaffId: "staff-natasha", DayOfWeek: dow,
			Blocks: []ab.WorkCalendarBlock{
				{TenantId: TenantID, StaffId: "staff-natasha", DayOfWeek: dow, StartMin: 540, EndMin: 780, IsActive: true},
				{TenantId: TenantID, StaffId: "staff-natasha", DayOfWeek: dow, StartMin: 840, EndMin: 1080, IsActive: true},
			},
		})
	}
	// Tony: Lun/Mié/Vie 08:00–14:00 sin colación.
	for _, dow := range []int64{1, 3, 5} {
		call(ab.ModelName + "." + ab.OpSaveDayBlocks, &ab.SaveDayBlocksArgs{
			TenantId: TenantID, StaffId: "staff-tony", DayOfWeek: dow,
			Blocks: []ab.WorkCalendarBlock{
				{TenantId: TenantID, StaffId: "staff-tony", DayOfWeek: dow, StartMin: 480, EndMin: 840, IsActive: true},
			},
		})
	}
	// Thor: sin filas — agenda "recién creada".
}

func (e *Env) addExceptions() {
	call := func(op string, args model.Encodable) {
		var doneErr error
		e.caller.Call(op, args, nil, func(err error) { doneErr = err })
		if doneErr != nil {
			panic(doneErr)
		}
	}
	// Excepciones de Natasha: 18 y 19 de septiembre 2026 cerrados.
	for _, d := range []string{"2026-09-18", "2026-09-19"} {
		call(ab.ModelName + "." + ab.OpAddCalendarException, &ab.AddCalendarExceptionArgs{
			TenantId: TenantID, StaffId: "staff-natasha",
			SpecificDate: unixDay(d), ExceptionType: ab.ExcHoliday,
		})
	}
}

func (e *Env) seedEmployeeServiceConfig() {
	// employee_service_config NO tiene op en appointment_booking (no hay UI
	// staff↔servicio en este módulo) — se siembra directo con db.Create. Sus
	// ids son estables y los consumen create_reservation / list_availability
	// como config_id. En producción viven en un screen staff↔servicio, fuera
	// de alcance de la demo.
	e.db.Create(&ab.EmployeeServiceConfig{
		Id:          "escNatashaConsulta",
		TenantId:    TenantID,
		StaffId:     "staff-natasha",
		ServiceId:   "svc-consulta",
		DurationMin: 30,
		BufferMin:   0,
		IsActive:    true,
	})
	e.db.Create(&ab.EmployeeServiceConfig{
		Id:          "escTonyCirugia",
		TenantId:    TenantID,
		StaffId:     "staff-tony",
		ServiceId:   "svc-cirugia",
		DurationMin: 30,
		BufferMin:   0,
		IsActive:    true,
	})
	e.db.Create(&ab.EmployeeServiceConfig{
		Id:          "escThorEcografia",
		TenantId:    TenantID,
		StaffId:     "staff-thor",
		ServiceId:   "svc-ecografia",
		DurationMin: 30,
		BufferMin:   0,
		IsActive:    true,
	})
}

// seedReservations siembra reservas CONFIRMED directo con db.Create. La op
// create_reservation valida el slot contra list_availability, lo que hace un
// seed por la op frágil; directo es la forma honesta de sembrar datos
// históricos. Los días caen en la plantilla de Natasha (Lun–Vie) y
// ReservationTime es la hora LOCAL ("HH:MM") convertida a UTC con la zona del
// work_calendar_config de Natasha — igual que hace list_availability, para que
// el motor de disponibilidad realmente las excluya de los huecos libres.
func (e *Env) seedReservations() {
	seeds := []*ab.Reservation{
		{
			Id: "res-1", TenantId: TenantID, ClientId: "12345678-5",
			CreatorUserId: "demo", EmployeeServiceConfigId: "escNatashaConsulta",
			StaffIdsnapshot: "staff-natasha", ServiceIdsnapshot: "svc-consulta",
			DurationMinSnapshot: 30, Status: ab.StatusConfirmed,
			ReservationDate: unixDay("2026-09-10"), ReservationTime: localToUTC(e, "2026-09-10", "09:00"),
			LocalStringDate: "2026-09-10", LocalStringTime: "09:00",
			Notes: "María Gonzalez", UpdatedAt: unixDay("2026-09-09") * 1000000000,
		},
		{
			Id: "res-2", TenantId: TenantID, ClientId: "98765432-5",
			CreatorUserId: "demo", EmployeeServiceConfigId: "escNatashaConsulta",
			StaffIdsnapshot: "staff-natasha", ServiceIdsnapshot: "svc-consulta",
			DurationMinSnapshot: 30, Status: ab.StatusConfirmed,
			ReservationDate: unixDay("2026-09-10"), ReservationTime: localToUTC(e, "2026-09-10", "10:30"),
			LocalStringDate: "2026-09-10", LocalStringTime: "10:30",
			Notes: "Juan Pérez", UpdatedAt: unixDay("2026-09-09") * 1000000000,
		},
		{
			Id: "res-3", TenantId: TenantID, ClientId: "11223344-k",
			CreatorUserId: "demo", EmployeeServiceConfigId: "escNatashaConsulta",
			StaffIdsnapshot: "staff-natasha", ServiceIdsnapshot: "svc-consulta",
			DurationMinSnapshot: 30, Status: ab.StatusConfirmed,
			ReservationDate: unixDay("2026-09-11"), ReservationTime: localToUTC(e, "2026-09-11", "09:00"),
			LocalStringDate: "2026-09-11", LocalStringTime: "09:00",
			Notes: "Ana Silva", UpdatedAt: unixDay("2026-09-10") * 1000000000,
		},
	}
	for _, r := range seeds {
		e.db.Create(r)
	}
}

// localToUTC convierte "YYYY-MM-DD" + "HH:MM" (hora local Santiago) al epoch
// UTC en segundos — la misma conversión que list_availability usa para alinear
// los slots con las reservas. La demo usa America/Santiago (ver upsertCalendarConfigs).
func localToUTC(e *Env, dateStr, hhmm string) int64 {
	// ParseTime devuelve minutos desde medianoche (int16). Se usa webtyp/time,
	// no stdlib strconv.
	minOfDay, err := tinytime.ParseTime(hhmm)
	if err != nil {
		return 0
	}
	return ab.LocalIntToUnixUTC(unixDay(dateStr), int(minOfDay), "America/Santiago")
}

// seedCatalog siembra especialidades + ítems de servicio en el item_catalog
// real, vía sus ops (ejercita el camino de producción). Las especialidades son
// un subconjunto de las canónicas del módulo; los ítems usan sku cuyo prefijo
// case con la especialidad sembrada. Se siembran SIN id (la op create los
// genera) y luego se mapea slug→id para referenciar en los ítems.
func (e *Env) seedCatalog() {
	call := func(op string, args model.Encodable) {
		var doneErr error
		e.caller.Call(op, args, nil, func(err error) { doneErr = err })
		if doneErr != nil {
			panic(doneErr)
		}
	}

	for _, spec := range []itemcatalog.Specialty{
		{TenantId: TenantID, Prefix: "md", Slug: "medicina-general", Name: "Medicina General", Position: 1, IsPublished: true},
		{TenantId: TenantID, Prefix: "do", Slug: "dental", Name: "Dental", Position: 2, IsPublished: true},
		{TenantId: TenantID, Prefix: "tr", Slug: "traumatologia", Name: "Traumatología", Position: 3, IsPublished: true},
		{TenantId: TenantID, Prefix: "ec", Slug: "ecografia", Name: "Ecografía", Position: 4, IsPublished: true},
		{TenantId: TenantID, Prefix: "ra", Slug: "radiologia", Name: "Radiología", Position: 5, IsPublished: true},
	} {
		call(itemcatalog.ModelName + "." + itemcatalog.OpUpsertSpecialty, &spec)
	}

	// Mapear slug → id de las especialidades recién creadas.
	for _, it := range []itemcatalog.CatalogItem{
		{Sku: "md-consulta", Name: "Consulta médica", Type: itemcatalog.ItemTypeService, IsActive: true, Price: 25000, Currency: "CLP"},
		{Sku: "ec-abdominal", Name: "Ecografía abdominal", Type: itemcatalog.ItemTypeService, IsActive: true, Price: 45000, Currency: "CLP"},
		{Sku: "ra-torax", Name: "Radiografía de tórax", Type: itemcatalog.ItemTypeService, IsActive: true, Price: 32000, Currency: "CLP"},
		{Sku: "tr-control", Name: "Control traumatología", Type: itemcatalog.ItemTypeService, IsActive: true, Price: 30000, Currency: "CLP"},
		{Sku: "do-limpieza", Name: "Limpieza dental", Type: itemcatalog.ItemTypeService, IsActive: true, Price: 28000, Currency: "CLP"},
	} {
		it.TenantId = TenantID
		it.SpecialtyId = lookupSpecialtyID(e, slugForSKU(it.Sku))
		call(itemcatalog.ModelName + "." + itemcatalog.OpUpsertItem, &it)
	}
}

// slugForSKU deriva el slug de una especialidad desde el prefijo del sku
// (los dos primeros chars) — la convención canónica de item_catalog.
func slugForSKU(sku string) string {
	prefix := sku
	if len(sku) >= 2 {
		prefix = sku[:2]
	}
	switch prefix {
	case "md":
		return "medicina-general"
	case "do":
		return "dental"
	case "tr":
		return "traumatologia"
	case "po":
		return "podologia"
	case "la":
		return "laboratorio"
	case "ec":
		return "ecografia"
	case "gi":
		return "ginecologia-y-obstetricia"
	case "ca":
		return "cardiologia"
	case "ga":
		return "gastroenterologia"
	case "of":
		return "oftalmologia"
	case "ne":
		return "neurologia"
	case "ps":
		return "psicologia"
	case "de":
		return "dermatologia"
	case "ra":
		return "radiologia"
	}
	return ""
}

// lookupSpecialtyID devuelve el id de la especialidad del slug dado (scan
// lineal sobre la lista corta — la regla "cero map en WASM" no permite map).
func lookupSpecialtyID(e *Env, slug string) string {
	out := &itemcatalog.SpecialtyList{}
	var callErr error
	e.caller.Call(itemcatalog.ModelName + "." + itemcatalog.OpListSpecialties,
		&itemcatalog.ListSpecialtiesArgs{TenantId: TenantID},
		out, func(err error) { callErr = err })
	if callErr != nil {
		panic(callErr)
	}
	for i := 0; i < out.Len(); i++ {
		sp := out.At(i).(*itemcatalog.Specialty)
		if sp.Slug == slug {
			return sp.Id
		}
	}
	return ""
}

// unixDay convierte "YYYY-MM-DD" a medianoche UTC en segundos (la forma que
// work_calendar_exception.specific_date y reservation.reservation_date
// almacenan).
func unixDay(dateStr string) int64 {
	nano, err := tinytime.ParseDate(dateStr)
	if err != nil {
		panic(err)
	}
	return nano / 1000000000
}

// stubStaff/stubCatalog/stubDirectory satisfacen los readers que
// appointment_booking exige: la demo confía en su propio seed, así que todo
// lo que la UI ofrece existe, y las validaciones de fin lo confirman.
type stubStaff struct{}

func (stubStaff) StaffExists(_, _ string) (bool, error) { return true, nil }

type stubCatalog struct{}

func (stubCatalog) ServiceExists(_, _ string) (bool, error) { return true, nil }

type stubDirectory struct{}

func (stubDirectory) ClientExists(_, _ string) (bool, error) { return true, nil }
