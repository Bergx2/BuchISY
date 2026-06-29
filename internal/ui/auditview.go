package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showAuditLog opens a read-only dialog showing the last 500 audit log entries
// (newest first). Columns: Zeit · Aktion · Beleg · Details.
func (a *App) showAuditLog() {
	if a.dbRepo == nil {
		a.showError(a.bundle.T("audit.title"), "Datenbank nicht verfügbar.")
		return
	}

	entries, err := a.dbRepo.AuditLog(500)
	if err != nil {
		a.showError(a.bundle.T("audit.title"), err.Error())
		return
	}

	// Map aktion codes to localised labels.
	actionLabel := func(aktion string) string {
		switch aktion {
		case "create":
			return a.bundle.T("audit.create")
		case "update":
			return a.bundle.T("audit.update")
		case "delete":
			return a.bundle.T("audit.delete")
		case "lock":
			return a.bundle.T("audit.lock")
		case "unlock":
			return a.bundle.T("audit.unlock")
		default:
			return aktion
		}
	}

	// Column headers as a non-interactive header row.
	colTime := a.bundle.T("audit.col.time")
	colAction := a.bundle.T("audit.col.action")
	colBeleg := a.bundle.T("audit.col.beleg")
	colDetails := a.bundle.T("audit.col.details")

	// Column widths.
	const (
		wTime    = 160
		wAction  = 80
		wBeleg   = 200
		wDetails = 280
	)

	// header row
	makeHeader := func() fyne.CanvasObject {
		hTime := widget.NewLabelWithStyle(colTime, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		hAction := widget.NewLabelWithStyle(colAction, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		hBeleg := widget.NewLabelWithStyle(colBeleg, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		hDetails := widget.NewLabelWithStyle(colDetails, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		return container.New(
			&auditRowLayout{wTime: wTime, wAction: wAction, wBeleg: wBeleg, wDetails: wDetails},
			hTime, hAction, hBeleg, hDetails,
		)
	}

	// Build an empty placeholder slice so the table can be built even when
	// there are no entries.
	safeGet := func(i int) core.AuditEntry {
		if i < len(entries) {
			return entries[i]
		}
		return core.AuditEntry{}
	}
	rowCount := len(entries)

	tbl := widget.NewTable(
		func() (int, int) { return rowCount, 4 },
		func() fyne.CanvasObject {
			// newHoverLabel already sets Truncation = fyne.TextTruncateEllipsis.
			return newHoverLabel(nil, nil)
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			hl := o.(*hoverLabel)
			// CRITICAL: cells are recycled — reset tooltip on every update.
			// The original callback set no TextStyle or Alignment, so no resets needed.
			hl.tooltip = ""
			e := safeGet(id.Row)
			switch id.Col {
			case 0:
				hl.SetText(e.TS)
			case 1:
				hl.SetText(actionLabel(e.Aktion))
			case 2:
				hl.SetText(e.Schluessel)
			case 3:
				hl.SetText(e.Details)
			}
		},
	)
	tbl.SetColumnWidth(0, wTime)
	tbl.SetColumnWidth(1, wAction)
	tbl.SetColumnWidth(2, wBeleg)
	tbl.SetColumnWidth(3, wDetails)

	content := container.NewBorder(
		container.NewVBox(makeHeader(), widget.NewSeparator()),
		nil, nil, nil,
		container.NewScroll(tbl),
	)

	d := dialog.NewCustom(
		a.bundle.T("audit.title"),
		a.bundle.T("common.close"),
		content,
		a.window,
	)
	d.Resize(fyne.NewSize(760, 460))
	d.Show()
}

// auditRowLayout is a fixed-column layout for the audit header row.
type auditRowLayout struct {
	wTime, wAction, wBeleg, wDetails float32
}

func (l *auditRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.wTime+l.wAction+l.wBeleg+l.wDetails, 28)
}

func (l *auditRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	widths := []float32{l.wTime, l.wAction, l.wBeleg, l.wDetails}
	x := float32(0)
	h := size.Height
	for i, o := range objects {
		if i >= len(widths) {
			break
		}
		w := widths[i]
		o.Resize(fyne.NewSize(w, h))
		o.Move(fyne.NewPos(x, 0))
		x += w
	}
}
