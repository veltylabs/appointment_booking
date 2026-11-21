//go:build !wasm

package appointmentbooking

import "webtyp.com/svg/sprite"

// IconSvg registra el ícono de marca del módulo. webtyp/ssr lo extrae durante SSR y assetmin lo
// inyecta inline en <body> — nunca se llama a mano.
func (m *Module) IconSvg() *sprite.Sprite {
	return sprite.NewSprite(
		sprite.Define(iconAppointmentBooking, "0 0 16 16",
			sprite.Path("m10.29 11.71-3.293-3.293v-4.414h2v3.586l2.707 2.707zm-2.293-11.71c-4.418 0-8 3.582-8 8s3.582 8 8 8 8-3.582 8-8-3.582-8-8-8zm0 14c-3.314 0-6-2.686-6-6s2.686-6 6-6 6 2.686 6 6-2.686 6-6 6z"),
		),
	)
}
