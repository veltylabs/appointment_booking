package seed

import (
	"webtyp.com/fmt"
	"webtyp.com/time"

	ab "github.com/veltylabs/appointment_booking"
)

// Constantes para IDs determinísticos de demo
const (
	DemoReservation1ID = "demo-reservation-1"
	DemoReservation2ID = "demo-reservation-2"
)

// maxSeedDays es cuántos días hacia adelante busca Load un día hábil que acepte
// las reservas de demo.
const maxSeedDays = 14

type StaffSeed struct {
	ID        string
	Name      string
	Specialty string
}

type ServiceSeed struct {
	ID        string
	Specialty string
}

// LoadOptions contiene los identificadores y opciones sembradas por la app.
type LoadOptions struct {
	Staff      []StaffSeed
	Services   []ServiceSeed
	PatientIDs []string
}

// Upstream es un alias retrocompatible para LoadOptions.
type Upstream = LoadOptions

// Data contiene las filas sembradas por este módulo.
type Data struct {
	ServiceConfigs []ab.EmployeeServiceConfig
	Reservations   []ab.Reservation
}

// Load puebla los datos iniciales de demostración en appointment_booking a través
// de los métodos del módulo.
func Load(m *ab.Module, tenantID string, up LoadOptions) (Data, error) {
	var data Data

	// 1. Configuración de calendario por cada funcionario.
	for _, sm := range up.Staff {
		err := m.UpsertCalendarConfig(ab.WorkCalendarConfig{
			TenantId: tenantID,
			StaffId:  sm.ID,
			IsActive: true,
		})
		if err != nil {
			return data, err
		}
	}

	// 2. Bloques de atención: Lunes a Viernes (1–5), 09:00–13:00 (540–780) y 15:00–18:00 (900–1080).
	for _, sm := range up.Staff {
		for dow := 1; dow <= 5; dow++ {
			blocks := []ab.WorkCalendarBlock{
				{TenantId: tenantID, StaffId: sm.ID, DayOfWeek: int64(dow), StartMin: 540, EndMin: 780, IsActive: true},
				{TenantId: tenantID, StaffId: sm.ID, DayOfWeek: int64(dow), StartMin: 900, EndMin: 1080, IsActive: true},
			}
			if err := m.SaveDayBlocks(tenantID, sm.ID, dow, blocks); err != nil {
				return data, err
			}
		}
	}

	// 3. Servicio por funcionario buscando coincidencia entre especialidad y catálogo.
	for _, sm := range up.Staff {
		var matchedServiceID string
		for _, s := range up.Services {
			if s.Specialty == sm.Specialty {
				matchedServiceID = s.ID
				break
			}
		}

		if matchedServiceID == "" && len(up.Services) > 0 {
			matchedServiceID = up.Services[0].ID
		}

		if matchedServiceID == "" {
			return data, fmt.Err("no se encontró servicio en catálogo para la especialidad " + sm.Specialty + " del profesional " + sm.Name)
		}

		// El primer profesional cobra en mesón antes de confirmar: la demo
		// recorre así el pago del paso 6.
		esc, err := m.CreateEmployeeServiceConfig(ab.EmployeeServiceConfig{
			TenantId:        tenantID,
			StaffId:         sm.ID,
			ServiceId:       matchedServiceID,
			DurationMin:     30,
			BufferMin:       0,
			PaymentRequired: len(data.ServiceConfigs) == 0,
			IsActive:        true,
		})
		if err != nil {
			return data, err
		}
		data.ServiceConfigs = append(data.ServiceConfigs, esc)
	}

	if len(data.ServiceConfigs) == 0 {
		return data, fmt.Err("no service configurations created")
	}
	if len(up.PatientIDs) == 0 {
		return data, fmt.Err("no patients found for seed")
	}

	patient1ID := up.PatientIDs[0]
	patient2ID := patient1ID
	if len(up.PatientIDs) > 1 {
		patient2ID = up.PatientIDs[1]
	}

	zone := m.Zone()
	nowSec := time.NowUnix()
	var res1 ab.Reservation
	var targetDay string
	var lastErr error
	booked := false
	for offset := int64(1); offset <= maxSeedDays && !booked; offset++ {
		candidate := nowSec + time.Unix(offset)*time.Day
		wd := zone.Weekday(candidate)
		if wd == 0 || wd == 6 {
			continue
		}
		dayStr := zone.Date(candidate)
		slotStartUtc, err := zone.At(dayStr, 540) // 09:00
		if err != nil {
			continue
		}
		res1, lastErr = m.CreateReservation(ab.CreateReservationCmd{
			Id:                      DemoReservation1ID,
			TenantId:                tenantID,
			ClientId:                patient1ID,
			EmployeeServiceConfigId: data.ServiceConfigs[0].Id,
			SlotStartUtc:            slotStartUtc,
			Origin:                  ab.OriginCounter,
		})
		if lastErr == nil {
			booked = true
			targetDay = dayStr
		}
	}
	if !booked {
		return data, fmt.Err("seed: ningún día hábil aceptó la reserva de demo: " + lastErr.Error())
	}
	slotStartUtc2, _ := zone.At(targetDay, 570) // 09:30

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
		Id:                      DemoReservation2ID,
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
