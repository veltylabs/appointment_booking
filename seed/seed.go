package seed

import (
	"webtyp.com/fmt"
	tinytime "webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
	catalogseed "github.com/veltylabs/item_catalog/seed"
	patientseed "github.com/veltylabs/patient_directory/seed"
	staffseed "github.com/veltylabs/staff_manager/seed"
)

// maxSeedDays es cuántos días hacia adelante busca Load un día hábil que acepte
// las reservas de demo.
const maxSeedDays = 14

// Upstream contiene las estructuras de datos semilla provistas por los módulos
// aguas arriba (staff, catálogo, pacientes).
type Upstream struct {
	Staff    staffseed.Data
	Catalog  catalogseed.Data
	Patients patientseed.Data
}

// Data contiene las filas sembradas por este módulo.
type Data struct {
	ServiceConfigs []ab.EmployeeServiceConfig
	Reservations   []ab.Reservation
}

// Load puebla los datos iniciales de demostración en appointment_booking a través
// de los métodos del módulo.
func Load(m *ab.Module, tenantID string, up Upstream) (Data, error) {
	var data Data

	// 1. Configuración de calendario por cada funcionario.
	for _, sm := range up.Staff.Staff {
		err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
			TenantId: tenantID,
			StaffId:  sm.Id,
			Timezone: "America/Santiago",
			IsActive: true,
		})
		if err != nil {
			return data, err
		}
	}

	// 2. Bloques de atención: Lunes a Viernes (1–5), 09:00–13:00 (540–780) y 15:00–18:00 (900–1080).
	for _, sm := range up.Staff.Staff {
		for dow := 1; dow <= 5; dow++ {
			blocks := []ab.WorkCalendarBlock{
				{TenantId: tenantID, StaffId: sm.Id, DayOfWeek: int64(dow), StartMin: 540, EndMin: 780, IsActive: true},
				{TenantId: tenantID, StaffId: sm.Id, DayOfWeek: int64(dow), StartMin: 900, EndMin: 1080, IsActive: true},
			}
			if err := m.SaveDayBlocks(tenantID, sm.Id, dow, blocks); err != nil {
				return data, err
			}
		}
	}

	// 3. Servicio por funcionario buscando coincidencia entre especialidad y catálogo.
	for _, sm := range up.Staff.Staff {
		var matchedServiceID string
		for _, sp := range up.Catalog.Specialties {
			if sp.Name == sm.Specialty {
				for _, it := range up.Catalog.Items {
					if it.SpecialtyId == sp.Id {
						matchedServiceID = it.Id
						break
					}
				}
				if matchedServiceID != "" {
					break
				}
			}
		}

		if matchedServiceID == "" {
			return data, fmt.Err("no se encontró servicio en catálogo para la especialidad " + sm.Specialty + " del profesional " + sm.Name)
		}

		esc, err := m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{
			TenantId:    tenantID,
			StaffId:     sm.Id,
			ServiceId:   matchedServiceID,
			DurationMin: 30,
			BufferMin:   0,
			IsActive:    true,
		})
		if err != nil {
			return data, err
		}
		data.ServiceConfigs = append(data.ServiceConfigs, esc)
	}

	if len(data.ServiceConfigs) == 0 {
		return data, fmt.Err("no hay configuraciones de servicio creadas")
	}
	if len(up.Patients.Patients) == 0 {
		return data, fmt.Err("no hay pacientes en el módulo de pacientes")
	}

	// 4. Dos reservas para el próximo día hábil que las acepte (09:00 y 09:30
	// Santiago). Se prueba hasta maxSeedDays días hacia adelante, saltando fines
	// de semana: el día siguiente puede ser feriado o cierre del calendario
	// institucional, y la semilla no debe fallar según la fecha en que corre.
	patient1ID := up.Patients.Patients[0].Id
	patient2ID := patient1ID
	if len(up.Patients.Patients) > 1 {
		patient2ID = up.Patients.Patients[1].Id
	}

	nowSec := tinytime.Now() / 1e9
	var res1 ab.Reservation
	var daySec int64
	var lastErr error
	booked := false
	for offset := int64(1); offset <= maxSeedDays && !booked; offset++ {
		candidate := nowSec + offset*86400
		wd := tinytime.Weekday(candidate * 1e9)
		if wd == 0 || wd == 6 {
			continue
		}
		isoDate := tinytime.FormatISO8601(candidate * 1e9)
		if len(isoDate) >= 10 {
			isoDate = isoDate[:10]
		}
		dayNano, err := tinytime.ParseDate(isoDate)
		if err != nil {
			return data, err
		}
		daySec = dayNano / 1e9
		res1, lastErr = m.CreateReservation(ab.CreateReservationCmd{
			TenantId:                tenantID,
			ClientId:                patient1ID,
			EmployeeServiceConfigId: data.ServiceConfigs[0].Id,
			SlotStartUtc:            ab.LocalIntToUnixUTC(daySec, 540, "America/Santiago"), // 09:00
			Origin:                  ab.OriginCounter,
		})
		booked = lastErr == nil
	}
	if !booked {
		return data, fmt.Err("seed: ningún día hábil aceptó la reserva de demo", lastErr)
	}
	slotStartUtc2 := ab.LocalIntToUnixUTC(daySec, 570, "America/Santiago") // 09:30

	// Primera reserva: confirmarla.
	if err := m.ChangeReservationStatus(ab.ChangeStatusCmd{
		TenantId: tenantID,
		Id:       res1.Id,
		Event:    ab.EventConfirm,
		Revision: int(res1.Revision),
	}); err != nil {
		return data, err
	}
	res1Confirmed, err := m.GetReservation(tenantID, res1.Id)
	if err != nil {
		return data, err
	}
	data.Reservations = append(data.Reservations, res1Confirmed)

	// Segunda reserva
	res2, err := m.CreateReservation(ab.CreateReservationCmd{
		TenantId:                tenantID,
		ClientId:                patient2ID,
		EmployeeServiceConfigId: data.ServiceConfigs[0].Id,
		SlotStartUtc:            slotStartUtc2,
		Origin:                  ab.OriginCounter,
	})
	if err != nil {
		return data, err
	}
	if err := m.ChangeReservationStatus(ab.ChangeStatusCmd{
		TenantId: tenantID,
		Id:       res2.Id,
		Event:    ab.EventConfirm,
		Revision: int(res2.Revision),
	}); err != nil {
		return data, err
	}
	res2Confirmed, err := m.GetReservation(tenantID, res2.Id)
	if err != nil {
		return data, err
	}
	data.Reservations = append(data.Reservations, res2Confirmed)

	return data, nil
}
