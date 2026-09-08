package appointmentbooking

import "webtyp.com/fmt"

// Estados
const (
	StatusPending     = "PENDING"
	StatusConfirmed   = "CONFIRMED"
	StatusCancelled   = "CANCELLED"
	StatusCompleted   = "COMPLETED"
	StatusNoShow      = "NO_SHOW"
	StatusExpired     = "EXPIRED"     // reserva no pagada que expiró (disparador: scheduler externo vía MCP)
	StatusRescheduled = "RESCHEDULED" // reserva original reemplazada por una nueva (registro de auditoría)
	StatusConflicted  = "CONFLICTED"  // la agenda actual ya no cubre esta reserva — NO es terminal
)

// Eventos
const (
	EventConfirm    = "CONFIRM"
	EventCancel     = "CANCEL"
	EventComplete   = "COMPLETE"
	EventNoShow     = "NO_SHOW_EVENT"
	EventExpire     = "EXPIRE"
	EventReschedule = "RESCHEDULE" // marca la original como RESCHEDULED; la nueva reserva se crea atómicamente
	EventConflict   = "CONFLICT"   // una recomputación pone la reserva en conflicto
	EventResolve    = "RESOLVE"    // una recomputación la saca del conflicto
)

// transition es una fila de la tabla de transiciones FSM: (estado actual, evento) -> estado siguiente.
// Slice, no map — la regla "cero map" de AGENTS.md no tiene excepciones; esta tabla tiene 9 filas,
// un scan lineal no cuesta nada medible.
type transition struct {
	From  string
	Event string
	To    string
}

var transitions = []transition{
	{StatusPending, EventConfirm, StatusConfirmed},
	{StatusPending, EventCancel, StatusCancelled},
	{StatusPending, EventExpire, StatusExpired},
	{StatusPending, EventReschedule, StatusRescheduled},
	{StatusConfirmed, EventCancel, StatusCancelled},
	{StatusConfirmed, EventComplete, StatusCompleted},
	{StatusConfirmed, EventNoShow, StatusNoShow},
	{StatusConfirmed, EventReschedule, StatusRescheduled},
	// CONFLICTED se entra desde PENDING o CONFIRMED (vía EventConflict, siempre
	// por una recomputación, nunca por un event de usuario) y se sale "hacia
	// cualquiera de los dos" (EventResolve): el destino real NO lo decide esta
	// tabla sino StatusBeforeConflict — la recomputación restaura el estado
	// previo (fuente única, §8.5). Las dos filas declaradas documentan que ambos
	// son sucesores legales; Transition() no se usa para elegir el destino.
	{StatusPending, EventConflict, StatusConflicted},
	{StatusConfirmed, EventConflict, StatusConflicted},
	{StatusConflicted, EventResolve, StatusPending},
	{StatusConflicted, EventResolve, StatusConfirmed},
	// CANCELLED, COMPLETED, NO_SHOW, EXPIRED, RESCHEDULED son terminales — sin transiciones salientes
}

// ErrInvalidTransition se devuelve cuando una transición no está permitida.
var ErrInvalidTransition = fmt.Err("invalid", "transition")

// Transition devuelve el siguiente estado, o un error si la transición no es válida.
func Transition(current, event string) (string, error) {
	if IsTerminal(current) {
		return "", ErrInvalidTransition
	}
	for _, t := range transitions {
		if t.From == current && t.Event == event {
			return t.To, nil
		}
	}
	return "", ErrInvalidTransition
}

// IsTerminal devuelve true si el estado no tiene transiciones salientes.
func IsTerminal(status string) bool {
	switch status {
	case StatusCancelled, StatusCompleted, StatusNoShow, StatusExpired, StatusRescheduled:
		return true
	default:
		return false
	}
}
