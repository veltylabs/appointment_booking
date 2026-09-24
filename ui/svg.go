//go:build !wasm

package ui

import (
	"webtyp.com/svg"
	"webtyp.com/svg/sprite"
)

// Icons es un tipo que aloja IconID/IconSvg para su descubrimiento y uso en config.
type Icons struct{}

func (m *Icons) IconSvg() *sprite.Sprite {
	return sprite.NewSprite(
		// Reloj con marca de check: "hora reservada" (Reserva Hora).
		sprite.Define(svg.Icon(ID), "0 0 16 16",
			sprite.Path("M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm.5 3v4.3l3 1.8-.5.9-3.5-2.1V4z"),
		),
		// Dos personas: "personal" / equipo (Personal).
		sprite.Define(svg.Icon(PersonalID), "0 0 16 16",
			sprite.Path("M5.5 7a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5zm5-1a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM0 14c0-2.67 3.58-4 5.5-4S11 11.33 11 14v1H0v-1zm11.5-3c1.6 0 4.5 1.1 4.5 3.5V15h-3.55c.03-.16.05-.33.05-.5 0-1.9-1.06-3.38-2.5-4.31.5-.13 1-.19 1.5-.19z"),
		),
	)
}
