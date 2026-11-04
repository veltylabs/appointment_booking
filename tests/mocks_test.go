package tests

import (
	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/events"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/time"
)

type MockStaffReader struct {
	Exists    bool
	Err       error
	Name      string
	Specialty string
	Active    bool
}

func (m *MockStaffReader) StaffExists(tenantID, staffID string) (bool, error) {
	return m.Exists, m.Err
}

func (m *MockStaffReader) StaffCard(tenantId, staffId string) (name, specialty string, active bool, err error) {
	return m.Name, m.Specialty, m.Active, m.Err
}

func (m *MockStaffReader) OccupantOptions(tenantID string) ([]fmt.KeyValue, error) {
	return []fmt.KeyValue{{Key: "s1", Value: "Dr. Mock"}}, nil
}

var _ ab.StaffReader = (*MockStaffReader)(nil)

type MockCatalogReader struct {
	Exists   bool
	Err      error
	CardErr  error
	Name     string
	Price    float64
	Currency string
	Active   bool
}

func (m *MockCatalogReader) ServiceExists(tenantID, serviceID string) (bool, error) {
	return m.Exists, m.Err
}

func (m *MockCatalogReader) ServiceCard(tenantId, serviceId string) (name string, price float64, currency string, active bool, err error) {
	return m.Name, m.Price, m.Currency, m.Active, m.CardErr
}

var _ ab.CatalogReader = (*MockCatalogReader)(nil)

type MockDirectoryReader struct {
	Exists bool
	Err    error
}

func (m *MockDirectoryReader) PatientExists(tenantID, patientID string) (bool, error) {
	return m.Exists, m.Err
}

func (m *MockDirectoryReader) ClientExists(tenantID, clientID string) (bool, error) {
	return m.Exists, m.Err
}

func (m *MockDirectoryReader) FindPatient(tenantID, documentNumber string) (id, name, phone, email string, err error) {
	if !m.Exists {
		return "", "", "", "", fmt.Err("patient not found")
	}
	return "pat-1", "Paciente Demo", "+56911223344", "demo@example.com", m.Err
}

func (m *MockDirectoryReader) RegisterPatient(tenantID, id, documentNumber, name, phone, email string) (patientID string, err error) {
	return id, m.Err
}

var _ ab.PatientDirectory = (*MockDirectoryReader)(nil)

type MockEventPublisher struct {
	PublishedEvents []string
}

func (m *MockEventPublisher) Publish(e events.Event) {
	m.PublishedEvents = append(m.PublishedEvents, e.Topic)
}

var _ events.Publisher = (*MockEventPublisher)(nil)

// DateBound fija los limits de una fecha puntual.
type DateBound struct {
	Date   time.Unix
	Bounds time.DayLimits
}

type WeekdayBound struct {
	DayOfWeek int
	Bounds    time.DayLimits
}

// MockBoundsReader es el fake de ab.OpeningHours.
type MockBoundsReader struct {
	Default          time.DayLimits
	Overrides        []DateBound
	WeekdayOverrides []WeekdayBound
}

func (b *MockBoundsReader) DayLimits(date time.Unix) (time.DayLimits, error) {
	if b == nil {
		return time.Unlimited(), nil
	}
	for _, o := range b.Overrides {
		if o.Date == date {
			return o.Bounds, nil
		}
	}
	return b.Default, nil
}

func (b *MockBoundsReader) WeekdayLimits(dayOfWeek int) (time.DayLimits, error) {
	if b == nil {
		return time.Unlimited(), nil
	}
	for _, o := range b.WeekdayOverrides {
		if o.DayOfWeek == dayOfWeek {
			return o.Bounds, nil
		}
	}
	return b.Default, nil
}

func (b *MockBoundsReader) ClosedDates(from, to time.Unix) (holidays, closures []string, err error) {
	return nil, nil, nil
}

var _ ab.OpeningHours = (*MockBoundsReader)(nil)

// OpenDayBounds es la ventana típica de un establecimiento: 08:00–20:00.
var OpenDayBounds = time.DayLimits{Open: true, OpenMin: 480, CloseMin: 1200}

type fakeIDs struct{ n int }

func (f *fakeIDs) NewID() string {
	f.n++
	return "testid" + fmt.Convert(f.n).String()
}

var _ model.IDGenerator = (*fakeIDs)(nil)

var zoneSantiago, _ = time.LoadZone("America/Santiago")
var zoneUTC, _ = time.LoadZone("UTC")

func SetupDependencies() ab.Deps {
	return ab.Deps{
		Staff:     &MockStaffReader{Exists: true},
		Catalog:   &MockCatalogReader{Exists: true},
		Patients:  &MockDirectoryReader{Exists: true},
		IDs:       &fakeIDs{},
		Publisher: &MockEventPublisher{},
		Zone:      zoneUTC,
		TenantID:  "t1",
	}
}

func SetupDependenciesWithBounds(bounds ab.OpeningHours) ab.Deps {
	deps := SetupDependencies()
	deps.Hours = bounds
	return deps
}
