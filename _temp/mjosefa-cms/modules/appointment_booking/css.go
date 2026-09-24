//go:build !wasm

package appointment_booking

import (
	"webtyp.com/css"
	"webtyp.com/widget/style"
)

// Las tres pantallas de este módulo comparten el mismo reparto: rightpanel
// pone el título, el panel y la región con scroll; cada hoja solo ajusta su
// propia fila de controles. Mismo patrón que webtyp/app-demo/modules/agenda.

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

// RenderCSS es el chrome de la pantalla "Reserva Hora". Su fila de
// controles es más larga (área + profesional + servicio + confirmar), así
// que envuelve — Row con wrap, no una sola línea rígida.
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
