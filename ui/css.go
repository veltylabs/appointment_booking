//go:build !wasm

package ui

import (
	"webtyp.com/css"
	"webtyp.com/widget/style"
)

// RenderCSS es el chrome de la pestaña "Horario".
func (s *ScheduleView) RenderCSS() *css.Stylesheet {
	return style.For(s).
		Root(
			style.Fill(),
		).
		Part(PartHeader,
			style.Row(style.Space2),
			style.CenterContent(),
		).
		Stylesheet()
}

// RenderCSS es el chrome de la pestaña "Servicios".
func (s *ServiceConfigView) RenderCSS() *css.Stylesheet {
	return style.For(s).
		Root(
			style.Fill(),
		).
		Part(PartServiceHeader,
			style.Row(style.Space2),
			style.CenterContent(),
		).
		Stylesheet()
}

// RenderCSS es el chrome de la pantalla "Reserva Hora".
func (v *BookingView) RenderCSS() *css.Stylesheet {
	return style.For(v).
		Root(
			style.Fill(),
		).
		Part(PartBookingHeader,
			style.Row(style.Space3),
			style.CenterContent(),
		).
		Stylesheet()
}
