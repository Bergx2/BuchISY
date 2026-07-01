package ui

import (
	"fyne.io/fyne/v2"

	"github.com/bergx2/buchisy/internal/core"
)

// buildMainMenu builds the native menu bar holding one-shot ACTIONS
// (navigation lives in the sidebar). Every item calls an existing handler.
func (a *App) buildMainMenu() *fyne.MainMenu {
	t := a.bundle.T

	file := fyne.NewMenu(t("menu.file"),
		fyne.NewMenuItem(t("menu.import"), a.importMultiple),
		fyne.NewMenuItem(t("menu.openTarget"), a.openTargetFolder),
		fyne.NewMenuItemSeparator(),
		a.profileMenuItem(),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(t("menu.backup"), a.showBackup),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(t("menu.quit"), a.app.Quit),
	)
	edit := fyne.NewMenu(t("menu.edit"),
		fyne.NewMenuItem(t("menu.renumber"), a.renumberBelegnummern),
		fyne.NewMenuItem(t("menu.autorules"), a.showAutoRulesDialog),
	)
	export := fyne.NewMenu(t("menu.export"),
		fyne.NewMenuItem(t("menu.csvexport"), a.showCSVExportDialog),
		fyne.NewMenuItem(t("menu.bookingexport"), a.showBookingExportDialog),
		fyne.NewMenuItem(t("menu.beleglistepdf"), a.showBelegListePDF),
		fyne.NewMenuItem(t("menu.salesjournalpdf"), a.showSalesJournalPDF),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(t("nav.gobdexport"), a.showExportPackage),
		fyne.NewMenuItem(t("nav.verfahrensdoku"), a.showVerfahrensdokuPDF),
	)
	view := fyne.NewMenu(t("menu.view"),
		fyne.NewMenuItem(t("menu.zoomin"), func() { a.adjustUIScale(UIScaleStep) }),
		fyne.NewMenuItem(t("menu.zoomout"), func() { a.adjustUIScale(-UIScaleStep) }),
		fyne.NewMenuItem(t("menu.zoomreset"), func() { a.setUIScale(1.0) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(t("menu.prevmonth"), func() { a.stepMonth(-1) }),
		fyne.NewMenuItem(t("menu.nextmonth"), func() { a.stepMonth(1) }),
	)
	help := fyne.NewMenu(t("menu.help"),
		fyne.NewMenuItem(t("menu.legend"), a.showLegend),
		fyne.NewMenuItem(t("menu.about"), a.showAbout),
	)
	return fyne.NewMainMenu(file, edit, export, view, help)
}

// profileMenuItem builds the "Profil wechseln" submenu: one entry per company
// profile (the active one checked), switching profiles via startProfile, plus a
// link to the full profile picker for managing / creating profiles.
func (a *App) profileMenuItem() *fyne.MenuItem {
	t := a.bundle.T
	item := fyne.NewMenuItem(t("menu.switchProfile"), nil)

	var children []*fyne.MenuItem
	if profiles, err := core.ListProfiles(); err == nil {
		for _, name := range profiles {
			p := name
			mi := fyne.NewMenuItem(p, func() {
				if p != a.profile {
					a.startProfile(p)
				}
			})
			mi.Checked = p == a.profile
			children = append(children, mi)
		}
	}
	if len(children) > 0 {
		children = append(children, fyne.NewMenuItemSeparator())
	}
	children = append(children, fyne.NewMenuItem(t("menu.profileManage"), a.showProfilePicker))

	item.ChildMenu = fyne.NewMenu("", children...)
	return item
}
