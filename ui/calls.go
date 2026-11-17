package ui

import (
	ab "github.com/veltylabs/appointment_booking"
	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/router"
)

// operation arma la operación de una entidad del módulo.
func operation(entity model.Model, action model.Action) router.Operation {
	return router.Operation{Module: ab.ModelName, Entity: entity.ModelName(), Action: action}
}

// readFiltered lee las filas de una entidad que cumplen el filtro (la entidad es su propio filtro).
func readFiltered(c router.Caller, entity, filter model.Model, into model.Decodable, done func(error)) {
	c.Call(operation(entity, model.Read), &crud.ReadArgs{Limit: crud.MaxPage, Filter: filter}, into, done)
}

// createOne crea una fila y responde con ella.
func createOne(c router.Caller, rec model.Model, into model.Decodable, done func(error)) {
	c.Call(operation(rec, model.Create), &crud.CreateArgs{Records: []model.Model{rec}}, into, done)
}

// deleteOne borra una fila por id.
func deleteOne(c router.Caller, entity model.Model, id string, done func(error)) {
	c.Call(operation(entity, model.Delete), &crud.DeleteArgs{IDs: []string{id}}, nil, done)
}

// step es una llamada que avisa al terminar; runSteps las encadena y se detiene en el primer error.
type step func(done func(error))

func runSteps(steps []step, done func(error)) {
	i := 0
	var next func(error)
	next = func(err error) {
		if err != nil || i == len(steps) {
			done(err)
			return
		}
		s := steps[i]
		i++
		s(next)
	}
	next(nil)
}
