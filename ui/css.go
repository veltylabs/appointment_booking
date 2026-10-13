//go:build !wasm

package ui

import (
	"webtyp.com/css"
	"webtyp.com/widget"
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

// RenderCSS es la reserva en 6 pasos: columna con aire, error y aviso
// visibles, formularios en columna, botón principal y panel final.
func (f *BookingFlow) RenderCSS() *css.Stylesheet {
	return style.For(f).
		// KeepSize: dentro del panel con scroll del shell, la reserva crece con
		// su contenido; si se encogiera, el calendario taparía los pasos de abajo.
		Root(style.Stack(style.Space4), style.Pad(style.Space4), style.KeepSize()).
		Part(PartFlowStep, style.Stack(style.Space3)).
		Part(PartFlowError, style.As(style.DangerWash), style.FontSize(style.TextSm)).
		Part(PartFlowNotice, style.FontSize(style.TextSm)).
		Part(PartFlowField, style.Stack(style.Space1)).
		Part(PartFlowCheck, style.Row(style.Space2)).
		Part(PartFlowInput, style.As(style.Field), style.Round(style.RadiusMd), style.PadInline(style.Space3), style.ControlBox()).
		Part(PartFlowActions, style.Row(style.Space2)).
		Part(PartFlowAction, style.Button(style.Primary)).
		When(widget.Busy, PartFlowAction, style.As(style.Inactive)).
		Part(PartFlowTitle, style.FontSize(style.TextLg), style.FontWeight(style.WeightBold)).
		Part(PartFlowFinal, style.Stack(style.Space3), style.As(style.Panel), style.Pad(style.Space4)).
		Stylesheet()
}
