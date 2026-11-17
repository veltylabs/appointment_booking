package appointmentbooking

import "webtyp.com/crud"

// StoreEmployeeServiceConfig is the tenant-sealed store of employee_service_config.
func (m *Module) StoreEmployeeServiceConfig() crud.Store { return m.store }

// StoreWorkCalendarConfig is the tenant-sealed store of work_calendar_config.
func (m *Module) StoreWorkCalendarConfig() crud.Store { return m.store }

// StoreWorkCalendarBlock is the tenant-sealed store of work_calendar_block.
func (m *Module) StoreWorkCalendarBlock() crud.Store { return m.store }

// StoreWorkCalendarException is the tenant-sealed store of work_calendar_exception.
func (m *Module) StoreWorkCalendarException() crud.Store { return m.store }

// StoreReservation is the tenant-sealed store of reservation.
func (m *Module) StoreReservation() crud.Store { return m.store }

// StoreReservationOverbooking is the tenant-sealed store of reservation_overbooking.
func (m *Module) StoreReservationOverbooking() crud.Store { return m.store }

// StoreOnlineReservation is the tenant-sealed store of online_reservation.
func (m *Module) StoreOnlineReservation() crud.Store { return m.store }

// StoreReservationExpiry is the tenant-sealed store of reservation_expiry.
func (m *Module) StoreReservationExpiry() crud.Store { return m.store }

// StoreConflictRecompute is the tenant-sealed store of conflict_recompute.
func (m *Module) StoreConflictRecompute() crud.Store { return m.store }
