package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
)

// showAnlagen opens the asset register (Anlagenverwaltung) window.
func (a *App) showAnlagen() {
	win := a.app.NewWindow(a.bundle.T("anlagen.title"))

	var listBox *fyne.Container
	var refresh func()

	fmtAmt := func(v float64) string {
		return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
	}

	buildList := func() []fyne.CanvasObject {
		year := a.currentYear

		headers := container.NewGridWithColumns(6,
			widget.NewLabelWithStyle(a.bundle.T("anlagen.col.bezeichnung"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(a.bundle.T("anlagen.col.anschaffung"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(a.bundle.T("anlagen.col.ak"), fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(a.bundle.T("anlagen.col.nd"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(a.bundle.T("anlagen.col.afa"), fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(a.bundle.T("anlagen.col.rbw"), fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		)

		rows := []fyne.CanvasObject{headers, widget.NewSeparator()}
		for i, asset := range a.assets {
			idx := i
			afa := core.LinearAfA(asset, year)
			rbw := core.Restbuchwert(asset, year)
			ndText := fmt.Sprintf("%d", asset.NutzungsdauerJahre)

			bezeichnung := widget.NewLabel(asset.Bezeichnung)
			bezeichnung.Wrapping = fyne.TextWrapWord
			anschaffung := widget.NewLabel(asset.Anschaffungsdatum)
			akLbl := widget.NewLabelWithStyle(fmtAmt(asset.Anschaffungswert), fyne.TextAlignTrailing, fyne.TextStyle{})
			ndLbl := widget.NewLabelWithStyle(ndText, fyne.TextAlignCenter, fyne.TextStyle{})
			afaLbl := widget.NewLabelWithStyle(fmtAmt(afa), fyne.TextAlignTrailing, fyne.TextStyle{})
			rbwLbl := widget.NewLabelWithStyle(fmtAmt(rbw), fyne.TextAlignTrailing, fyne.TextStyle{})

			editBtn := widget.NewButton(a.bundle.T("anlagen.edit"), func() {
				a.showAssetForm(win, idx, refresh)
			})
			editBtn.Importance = widget.LowImportance

			row := container.NewGridWithColumns(6,
				bezeichnung, anschaffung, akLbl, ndLbl, afaLbl, rbwLbl,
			)
			rows = append(rows, row)
			rows = append(rows, container.NewHBox(editBtn))
		}
		return rows
	}

	listBox = container.NewVBox()

	refresh = func() {
		listBox.Objects = buildList()
		listBox.Refresh()
	}
	refresh()

	scroll := container.NewVScroll(listBox)
	scroll.SetMinSize(fyne.NewSize(760, 320))

	neuBtn := widget.NewButton(a.bundle.T("anlagen.new"), func() {
		a.showAssetForm(win, -1, refresh)
	})
	neuBtn.Importance = widget.HighImportance

	spiegelBtn := widget.NewButton(a.bundle.T("anlagen.spiegel"), func() {
		rows := core.Anlagenspiegel(a.assets, a.currentYear)
		title := fmt.Sprintf("%s %d", a.bundle.T("anlagen.title"), a.currentYear)
		data, err := core.BuildAnlagenspiegelPDF(rows, a.currentYear, title)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		filename := fmt.Sprintf("Anlagenspiegel_%d.pdf", a.currentYear)
		a.savePDF(filename, data)
	})

	closeBtn := widget.NewButton("Schließen", func() {
		win.Close()
	})
	closeBtn.Importance = widget.LowImportance

	buttons := container.NewHBox(neuBtn, spiegelBtn, widget.NewSeparator(), closeBtn)

	content := container.NewBorder(
		nil,
		container.NewPadded(buttons),
		nil, nil,
		scroll,
	)

	win.SetContent(container.NewPadded(content))
	win.Resize(fyne.NewSize(820, 480))
	win.CenterOnScreen()
	win.Show()
}

// showAssetForm opens the add/edit form for an asset.
// idx == -1 means a new asset; otherwise it is the index in a.assets.
func (a *App) showAssetForm(parent fyne.Window, idx int, onSaved func()) {
	var existing core.Asset
	isNew := idx < 0
	if !isNew && idx < len(a.assets) {
		existing = a.assets[idx]
	}

	title := a.bundle.T("anlagen.form.new")
	if !isNew {
		title = a.bundle.T("anlagen.form.edit")
	}

	bezeichnungEntry := widget.NewEntry()
	bezeichnungEntry.SetPlaceHolder(a.bundle.T("anlagen.form.bezeichnung"))

	datumEntry := widget.NewEntry()
	datumEntry.SetPlaceHolder("DD.MM.YYYY")

	wertEntry := widget.NewEntry()
	wertEntry.SetPlaceHolder("0,00")

	ndEntry := widget.NewEntry()
	ndEntry.SetPlaceHolder("3")

	kontoEntry := widget.NewEntry()
	kontoEntry.SetPlaceHolder("0")

	afaKontoEntry := widget.NewEntry()
	afaKontoEntry.SetPlaceHolder("0")

	if !isNew {
		bezeichnungEntry.SetText(existing.Bezeichnung)
		datumEntry.SetText(existing.Anschaffungsdatum)
		wertEntry.SetText(strings.Replace(fmt.Sprintf("%.2f", existing.Anschaffungswert), ".", ",", 1))
		ndEntry.SetText(fmt.Sprintf("%d", existing.NutzungsdauerJahre))
		kontoEntry.SetText(fmt.Sprintf("%d", existing.Konto))
		afaKontoEntry.SetText(fmt.Sprintf("%d", existing.AfaKonto))
	}

	form := widget.NewForm(
		widget.NewFormItem(a.bundle.T("anlagen.form.bezeichnung"), bezeichnungEntry),
		widget.NewFormItem(a.bundle.T("anlagen.form.datum"), datumEntry),
		widget.NewFormItem(a.bundle.T("anlagen.form.wert"), wertEntry),
		widget.NewFormItem(a.bundle.T("anlagen.form.nd"), ndEntry),
		widget.NewFormItem(a.bundle.T("anlagen.form.konto"), kontoEntry),
		widget.NewFormItem(a.bundle.T("anlagen.form.afakonto"), afaKontoEntry),
	)

	var dlg dialog.Dialog
	saveBtn := widget.NewButton(a.bundle.T("anlagen.form.save"), func() {
		// Parse fields.
		bez := strings.TrimSpace(bezeichnungEntry.Text)
		if bez == "" {
			dialog.ShowInformation(title, a.bundle.T("anlagen.form.err.bezeichnung"), parent)
			return
		}
		datum := strings.TrimSpace(datumEntry.Text)

		wertStr := strings.Replace(strings.TrimSpace(wertEntry.Text), ",", ".", 1)
		wert, err := strconv.ParseFloat(wertStr, 64)
		if err != nil || wert <= 0 {
			dialog.ShowInformation(title, a.bundle.T("anlagen.form.err.wert"), parent)
			return
		}

		nd, err := strconv.Atoi(strings.TrimSpace(ndEntry.Text))
		if err != nil || nd <= 0 {
			dialog.ShowInformation(title, a.bundle.T("anlagen.form.err.nd"), parent)
			return
		}

		konto, _ := strconv.Atoi(strings.TrimSpace(kontoEntry.Text))
		afaKonto, _ := strconv.Atoi(strings.TrimSpace(afaKontoEntry.Text))

		if isNew {
			id := fmt.Sprintf("%d-%s", len(a.assets)+1, sanitizeID(bez))
			asset := core.Asset{
				ID:                id,
				Bezeichnung:       bez,
				Anschaffungsdatum: datum,
				Anschaffungswert:  wert,
				NutzungsdauerJahre: nd,
				Konto:             konto,
				AfaKonto:          afaKonto,
			}
			a.assets = append(a.assets, asset)
		} else {
			a.assets[idx].Bezeichnung = bez
			a.assets[idx].Anschaffungsdatum = datum
			a.assets[idx].Anschaffungswert = wert
			a.assets[idx].NutzungsdauerJahre = nd
			a.assets[idx].Konto = konto
			a.assets[idx].AfaKonto = afaKonto
		}

		if err := core.SaveAssets(a.assetsPath, a.assets); err != nil {
			dialog.ShowError(err, parent)
			return
		}
		if onSaved != nil {
			onSaved()
		}
		dlg.Hide()
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton(a.bundle.T("anlagen.form.cancel"), func() {
		dlg.Hide()
	})
	cancelBtn.Importance = widget.LowImportance

	content := container.NewVBox(
		form,
		container.NewHBox(saveBtn, cancelBtn),
	)

	dlg = dialog.NewCustom(title, " ", content, parent)
	dlg.Resize(fyne.NewSize(440, 320))
	dlg.Show()
}

// sanitizeID strips non-alphanumeric characters for use in an asset ID.
func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if len(result) > 16 {
		result = result[:16]
	}
	return result
}
