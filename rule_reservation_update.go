package appointmentbooking

import (
	"webtyp.com/crud"
	"webtyp.com/model"
)

// Update es la regla de negocio de actualizar reservation: solo cambia el estado, y solo por una transición que la máquina de estados permite
// (confirmar, cancelar, completar, no-show, vencer). Cualquier otro campo se rechaza.
// crud ya validó el esquema, los campos solo del servidor, los límites y el permiso antes de llamarte.
func (next *Reservation) Update(a crud.CurrentUser, tx crud.Tx, old *Reservation, fields []string, m *Module) (crud.RuleAnswer, error) {
	if !hasField(fields, Reservation_.Status) {
		return crud.RuleAnswer{}, &model.FieldError{Field: Reservation_.Status, Err: ErrStatusRequired}
	}
	for _, f := range fields {
		if f != Reservation_.Status {
			return crud.RuleAnswer{}, &model.FieldError{Field: f, Err: ErrOnlyStatusChanges}
		}
	}
	if _, ok := userEventTo(old.Status, next.Status); !ok {
		return crud.RuleAnswer{}, &model.FieldError{Field: Reservation_.Status, Err: ErrInvalidTransition}
	}
	next.UpdatedBy = a.UserID
	if err := tx.UpdateFields(next, old.Id, []string{Reservation_.UpdatedBy}); err != nil {
		return crud.RuleAnswer{}, err
	}
	return crud.ApplyAndPublish(), nil
}
