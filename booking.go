package appointmentbooking

import (
	"webtyp.com/events"
	"webtyp.com/orm"
	tinytime "webtyp.com/time"
)

// Reserva en 6 pasos, mesón y online. Cada función sigue su diagrama en
// docs/diagrams/booking_backend.md; los ids de rama (P1, B2, …) están en los
// comentarios.

// CurrencyCLP es la moneda por defecto cuando el catálogo no la informa.
const CurrencyCLP = "CLP"

// CurrencyUSD tiene 2 decimales; CLP, ninguno.
const CurrencyUSD = "USD"

func nowSeconds() int64 { return tinytime.Now() / 1_000_000_000 }

// priceFor congela el precio de una reserva (§1): la sobrescritura de la
// configuración gana; si no, el precio de lista del catálogo; la moneda es la
// del catálogo, o CLP si no la informa.
func (m *Module) priceFor(tenantId string, cfg EmployeeServiceConfig) (float64, string, error) {
	_, listPrice, currency, _, err := m.catalog.ServiceCard(tenantId, cfg.ServiceId)
	if err != nil { // P1
		return 0, "", err
	}
	price, currency := effectivePrice(cfg, listPrice, currency)
	return price, currency, nil
}

func effectivePrice(cfg EmployeeServiceConfig, listPrice float64, currency string) (float64, string) {
	price := listPrice
	if cfg.PriceOverride > 0 { // P2
		price = cfg.PriceOverride
	}
	if currency == "" { // P3
		currency = CurrencyCLP
	}
	return price, currency
}

// ListBookableServices lista lo que se puede reservar: configuraciones activas
// de profesionales activos (de la especialidad pedida, o todas) con un servicio
// activo del catálogo; ordenado por especialidad, servicio y profesional (§2).
func (m *Module) ListBookableServices(tenantId, specialty string) ([]BookableService, error) {
	cfgs, err := ReadAllEmployeeServiceConfig(m.db.Query(&EmployeeServiceConfig{}).
		Where(EmployeeServiceConfig_.TenantId).Eq(tenantId).
		Where(EmployeeServiceConfig_.IsActive).Eq(true))
	if err != nil { // B1
		return nil, err
	}
	out := []BookableService{}
	for _, cfg := range cfgs {
		// "No existe" se pregunta con StaffExists/ServiceExists, que lo responden
		// como (false, nil): un error de otro módulo no se puede comparar aquí.
		exists, err := m.staff.StaffExists(tenantId, cfg.StaffId)
		if err != nil { // B2 otro error
			return nil, err
		}
		if !exists { // B2 no encontrado
			continue
		}
		staffName, staffSpecialty, staffActive, err := m.staff.StaffCard(tenantId, cfg.StaffId)
		if err != nil {
			return nil, err
		}
		if !staffActive || (specialty != "" && staffSpecialty != specialty) { // B3
			continue
		}
		exists, err = m.catalog.ServiceExists(tenantId, cfg.ServiceId)
		if err != nil { // B4 otro error
			return nil, err
		}
		if !exists { // B4 no encontrado
			continue
		}
		serviceName, listPrice, currency, serviceActive, err := m.catalog.ServiceCard(tenantId, cfg.ServiceId)
		if err != nil {
			return nil, err
		}
		if !serviceActive { // B5
			continue
		}
		price, currency := effectivePrice(*cfg, listPrice, currency)
		out = append(out, BookableService{
			ConfigId: cfg.Id, StaffId: cfg.StaffId, StaffName: staffName, Specialty: staffSpecialty,
			ServiceId: cfg.ServiceId, ServiceName: serviceName, DurationMin: cfg.DurationMin,
			Price: price, Currency: currency, PaymentRequired: cfg.PaymentRequired,
		})
	}
	// Orden de inserción (lista chica, sin el paquete sort en el binario wasm).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && bookableLess(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func bookableLess(a, b BookableService) bool {
	if a.Specialty != b.Specialty {
		return a.Specialty < b.Specialty
	}
	if a.ServiceName != b.ServiceName {
		return a.ServiceName < b.ServiceName
	}
	return a.StaffName < b.StaffName
}

// ListServiceAvailability devuelve las horas de un servicio (§3): las de un
// profesional, o las de todos los que lo ofrecen ("cualquiera"); si dos tienen
// la misma hora queda el primero en el orden de ListBookableServices. Con
// IncludeTaken y un profesional fijo agrega sus horas ya reservadas (sobrecupo).
func (m *Module) ListServiceAvailability(a ListServiceAvailabilityArgs) ([]BookableSlot, error) {
	bookable, err := m.ListBookableServices(a.TenantId, "")
	if err != nil {
		return nil, err
	}
	out := []BookableSlot{}
	for _, svc := range bookable {
		if svc.ServiceId != a.ServiceId || (a.StaffId != "" && svc.StaffId != a.StaffId) { // V1, V2
			continue
		}
		free, err := m.ListAvailability(a.TenantId, svc.StaffId, svc.ConfigId, a.From, a.To)
		if err != nil { // V3
			return nil, err
		}
		for _, s := range free {
			out = addSlot(out, BookableSlot{StartUtc: s.StartUtc, EndUtc: s.EndUtc, StaffId: svc.StaffId, ConfigId: svc.ConfigId})
		}
		if a.IncludeTaken && a.StaffId != "" { // V4
			reserved, err := m.repo.ListReservationsByStaff(a.TenantId, svc.StaffId, a.From, a.To)
			if err != nil {
				return nil, err
			}
			for _, r := range reserved {
				if !holdsSlot(r.Status) {
					continue
				}
				out = addSlot(out, BookableSlot{StartUtc: r.ReservationTime, EndUtc: r.ReservationTime + r.DurationMinSnapshot*60,
					StaffId: svc.StaffId, ConfigId: svc.ConfigId, Taken: true})
			}
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].StartUtc < out[j-1].StartUtc; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// addSlot agrega s salvo que ya haya una hora con el mismo inicio.
func addSlot(out []BookableSlot, s BookableSlot) []BookableSlot {
	for _, have := range out {
		if have.StartUtc == s.StartUtc {
			return out
		}
	}
	return append(out, s)
}

// holdsSlot: una reserva en este estado ocupa su hora.
func holdsSlot(status string) bool {
	return status != StatusCancelled && status != StatusRescheduled && status != StatusExpired
}

// CreateOnlineReservation crea una reserva online retenida OnlineHoldSeconds
// hasta que se pague (§4). Un paciente no puede tener más de MaxOnlineHolds
// retenciones vigentes.
func (m *Module) CreateOnlineReservation(cmd CreateReservationCmd) (Reservation, error) {
	if m.payments == nil { // O1
		return Reservation{}, ErrPaymentsUnavailable
	}
	now := nowSeconds()
	mine, err := m.repo.ListReservationsByClient(cmd.TenantId, cmd.ClientId)
	if err != nil {
		return Reservation{}, err
	}
	holds := 0
	for _, r := range mine {
		if r.Origin == OriginOnline && r.Status == StatusPending && r.HoldUntil > now {
			holds++
		}
	}
	if holds >= MaxOnlineHolds { // O2
		return Reservation{}, ErrTooManyHolds
	}
	cmd.Origin = OriginOnline
	cmd.HoldUntil = now + OnlineHoldSeconds
	return m.CreateReservation(cmd) // O3
}

// Charge responde cuánto se cobra por una reserva (lo llama payment_manager vía
// su ChargeReader). Solo es pagable una reserva PENDING o CONFIRMED, sin pago y
// con precio; una online, solo si le quedan PaymentWindowSeconds de retención (§5).
func (m *Module) Charge(tenantId, reservationId string) (amount int64, currency string, payable bool, err error) {
	res, err := m.GetReservation(tenantId, reservationId)
	if err != nil { // C1
		return 0, "", false, err
	}
	if (res.Status != StatusPending && res.Status != StatusConfirmed) || res.PaymentId != "" || res.PriceSnapshot <= 0 { // C2
		return 0, "", false, nil
	}
	if res.Origin == OriginOnline && res.HoldUntil-nowSeconds() < PaymentWindowSeconds { // C3, C4
		return 0, "", false, nil
	}
	amount, err = MinorUnits(res.PriceSnapshot, res.CurrencySnapshot)
	if err != nil {
		return 0, "", false, err
	}
	return amount, res.CurrencySnapshot, true, nil
}

// MinorUnits convierte un precio a unidades mínimas (M1): CLP sin decimales,
// USD en centavos. Es el monto que cobra payment_manager; la pantalla lo usa
// para mostrar el precio con paymentmanager.FormatAmount.
func MinorUnits(price float64, currency string) (int64, error) {
	switch currency {
	case CurrencyCLP:
		return int64(price + 0.5), nil
	case CurrencyUSD:
		return int64(price*100 + 0.5), nil
	}
	return 0, ErrUnsupportedCurrency
}

// ConfirmPaidReservation confirma la reserva que se pagó con paymentId (§6).
// La llama la raíz de composición al recibir payment.paid; es idempotente.
// El pago se verifica con PaymentReader contra el precio congelado, sin la
// ventana de pago: un pago aprobado dentro de la retención vale.
func (m *Module) ConfirmPaidReservation(tenantId, reservationId, paymentId string) error {
	if m.payments == nil { // F1
		return ErrPaymentsUnavailable
	}
	res, err := m.GetReservation(tenantId, reservationId)
	if err != nil { // F2
		return err
	}
	switch res.PaymentId { // F3
	case paymentId:
		return nil
	case "":
	default:
		return ErrAlreadyPaid
	}
	price, err := MinorUnits(res.PriceSnapshot, res.CurrencySnapshot)
	if err != nil {
		return err
	}
	paid, currency, err := m.payments.PaidAmount(tenantId, paymentId, ModelName, reservationId)
	if err != nil || currency != res.CurrencySnapshot || paid < price { // F4
		return ErrPaymentNotVerified
	}
	if res.Status != StatusPending && res.Status != StatusConfirmed { // F5 otro
		return ErrReservationNotPayable
	}

	var updated Reservation
	err = m.db.Tx(func(tx *orm.DB) error {
		current := &Reservation{}
		got, err := ReadOneReservation(tx.Query(current).Where(Reservation_.Id).Eq(reservationId).Where(Reservation_.TenantId).Eq(tenantId), current)
		if err != nil {
			return err
		}
		if got.Revision != res.Revision { // F6
			return ErrConflict
		}
		if got.Status == StatusPending { // F5 PENDING
			got.Status = StatusConfirmed
		}
		got.PaymentId = paymentId
		got.HoldUntil = 0
		got.UpdatedAt = tinytime.Now()
		got.UpdatedBy = ActorPayments
		got.Revision++
		updated = *got
		return tx.Update(got, orm.Eq(Reservation_.Id, got.Id), orm.Eq(Reservation_.TenantId, tenantId))
	})
	if err != nil {
		return err
	}
	if m.pub != nil {
		m.pub.Publish(events.Event{Topic: EventReservationConfirmed, Payload: &updated})
	}
	return nil
}

// ActorPayments es el autor que queda en una reserva confirmada por su pago.
const ActorPayments = "payment_manager"

// ActorSystem es el autor de los vencimientos automáticos.
const ActorSystem = "system"

// ExpirePendingReservations vence las reservas PENDING cuya hora ya pasó o
// cuya retención online venció (§7). Devuelve cuántas venció.
func (m *Module) ExpirePendingReservations(tenantId string, now int64) (int, error) {
	pending, err := ReadAllReservation(m.db.Query(&Reservation{}).
		Where(Reservation_.TenantId).Eq(tenantId).
		Where(Reservation_.Status).Eq(StatusPending))
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, r := range pending {
		pastAppointment := r.ReservationTime < now         // X1
		lapsedHold := r.HoldUntil > 0 && r.HoldUntil < now // X2
		if !pastAppointment && !lapsedHold {
			continue
		}
		if err := m.ChangeReservationStatus(ChangeStatusCmd{
			TenantId: tenantId, Id: r.Id, Event: EventExpire, ActorId: ActorSystem, Revision: int(r.Revision),
		}); err != nil {
			return expired, err
		}
		expired++
	}
	return expired, nil
}
