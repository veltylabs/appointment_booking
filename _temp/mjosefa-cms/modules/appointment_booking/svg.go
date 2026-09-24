//go:build !wasm

package appointment_booking

import (
	"webtyp.com/svg"
	"webtyp.com/svg/sprite"
)

// Icons es un tipo que aloja IconID/IconSvg para su descubrimiento y uso en config.
// Reservado ahora para la pantalla Reserva Hora de la Etapa 8.
type Icons struct{}

func (m *Icons) IconSvg() *sprite.Sprite {
	return sprite.NewSprite(
		// Reloj con marca de check: "hora reservada". Un solo path, currentColor.
		sprite.Define(svg.Icon(ID), "0 0 16 16",
			sprite.Path("M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm.5 3v4.3l3 1.8-.5.9-3.5-2.1V4z"),
		),
	)
}
