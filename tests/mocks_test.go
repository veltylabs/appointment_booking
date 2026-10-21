package tests

import (
	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/events"
	"webtyp.com/fmt"
	"webtyp.com/model"
	tinytime "webtyp.com/time"
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

type MockDirectoryReader struct {
	Exists bool
	Err    error
}

func (m *MockDirectoryReader) ClientExists(tenantID, clientID string) (bool, error) {
	return m.Exists, m.Err
}

type MockEventPublisher struct {
	PublishedEvents []string
}

func (m *MockEventPublisher) Publish(e events.Event) {
	m.PublishedEvents = append(m.PublishedEvents, e.Topic)
}

var _ events.Publisher = (*MockEventPublisher)(nil)

// DateBound fija los bounds de una fecha puntual (para simular un feriado o un
// cierre local). Slice de structs, no map — la regla "cero map" de AGENTS.md
// llega también al código de test.
type DateBound struct {
	Date   tinytime.Unix
	Bounds tinytime.DayBounds
}

type WeekdayBound struct {
	DayOfWeek int
	Bounds    tinytime.DayBounds
}

// MockBoundsReader es el fake de ab.BoundsReader: bounds por fecha vía
// Overrides (scan lineal) y un Default para el resto.
type MockBoundsReader struct {
	Default          tinytime.DayBounds
	Overrides        []DateBound
	WeekdayOverrides []WeekdayBound
}

func (b *MockBoundsReader) GetDayBounds(date tinytime.Unix) (tinytime.DayBounds, error) {
	if b == nil {
		return tinytime.Unbounded(), nil
	}
	for _, o := range b.Overrides {
		if o.Date == date {
			return o.Bounds, nil
		}
	}
	return b.Default, nil
}

func (b *MockBoundsReader) GetWeekdayBounds(dayOfWeek int) (tinytime.DayBounds, error) {
	if b == nil {
		return tinytime.Unbounded(), nil
	}
	for _, o := range b.WeekdayOverrides {
		if o.DayOfWeek == dayOfWeek {
			return o.Bounds, nil
		}
	}
	return b.Default, nil
}

var _ ab.BoundsReader = (*MockBoundsReader)(nil)

// OpenDayBounds es la ventana típica de un establecimiento: 08:00–20:00.
var OpenDayBounds = tinytime.DayBounds{Open: true, OpenMin: 480, CloseMin: 1200}

type fakeIDs struct{ n int }

func (f *fakeIDs) NewID() string {
	f.n++
	return "testid" + fmt.Convert(f.n).String()
}

var _ model.IDGenerator = (*fakeIDs)(nil)

func SetupDependencies() ab.Deps {
	return ab.Deps{
		Staff:     &MockStaffReader{Exists: true},
		Catalog:   &MockCatalogReader{Exists: true},
		Directory: &MockDirectoryReader{Exists: true},
		IDs:       &fakeIDs{},
		Publisher: &MockEventPublisher{},
	}
}

func SetupDependenciesWithBounds(bounds ab.BoundsReader) ab.Deps {
	deps := SetupDependencies()
	deps.Bounds = bounds
	return deps
}
