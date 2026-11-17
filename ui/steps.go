package ui

import (
	. "webtyp.com/dom"
	"webtyp.com/view"
)

// PatientStep es el paso «paciente» de la reserva de mesón: buscar o registrar a quien se atiende. Este módulo no
// conoce pacientes; la app lo implementa con la pantalla de patient_directory y lo pasa en Steps.
type PatientStep interface {
	// Render devuelve el paso. Cuando el personal elige a un paciente, llama a choose con su id y su nombre.
	Render(choose func(id, name string)) *Element
	// Reset deja el paso limpio para una reserva nueva.
	Reset()
}

// PaymentStep es el paso «cobro» de la reserva de mesón con pago requerido. Este módulo no conoce pagos; la app lo
// implementa con la pantalla de payment_manager y lo pasa en Steps.
type PaymentStep interface {
	// Render devuelve el cobro de una reserva (el monto lo responde el módulo al gestor de pagos). Cuando el pago
	// queda registrado, llama a paid con su id.
	Render(reservationID string, paid func(paymentID string)) *Element
}

// Steps son los pasos de otros módulos que necesita la reserva de mesón. Patient es obligatorio; Payment puede faltar en
// una app sin cobros.
type Steps struct {
	Patient  PatientStep
	Payment  PaymentStep
	OpenForm func(module string, onSaved func(view.RecordResult), onCancel func()) Component
}
