package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bergx2/buchisy/internal/core"
	"github.com/zalando/go-keyring"
)

// confirmSwitchProfile asks before switching to another company profile and
// saves the current profile's settings first, so an in-session switch (from the
// Datei menu) doesn't lose the current state (e.g. window layout).
func (a *App) confirmSwitchProfile(target string) {
	dialog.ShowConfirm(
		a.bundle.T("profile.switch.title"),
		a.bundle.T("profile.switch.message", target),
		func(ok bool) {
			if !ok {
				return
			}
			if a.settingsMgr != nil {
				if err := a.settingsMgr.Save(a.settings); err != nil && a.logger != nil {
					a.logger.Warn("Saving settings before profile switch failed: %v", err)
				}
			}
			a.startProfile(target)
		}, a.window)
}

// showProfilePicker shows the profile-selection screen as the window
// content. Window size scales with the persisted UI zoom so the picker
// doesn't end up cramped after the user has bumped the theme scale.
func (a *App) showProfilePicker() {
	a.window.SetTitle("BuchISY")
	scale := float32(1.0)
	if a.theme != nil {
		scale = a.theme.Scale()
	}
	a.window.Resize(fyne.NewSize(420*scale, 400*scale))
	a.window.CenterOnScreen()
	a.window.SetContent(a.buildProfilePicker())
}

// buildProfilePicker builds the profile-selection UI as a list of
// inviting cards (one per company) with a primary "+ Neues Profil"
// CTA at the bottom.
func (a *App) buildProfilePicker() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("Firma wählen", fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabelWithStyle(
		"Wähle ein Profil aus.",
		fyne.TextAlignCenter, fyne.TextStyle{})

	list := container.NewVBox()
	profiles, err := core.ListProfiles()
	if err != nil {
		dialog.ShowError(err, a.window)
	}
	for _, name := range profiles {
		p := name
		list.Add(a.profileCard(p))
	}

	// "Neues Profil" is rarely used once set up (a profile is created maybe once
	// every few years), so it's a small, low-key link. Exception: with no
	// profiles yet (first run) it's the only possible action, so keep it a
	// prominent, full-width CTA.
	newBtn := widget.NewButtonWithIcon("Neues Profil",
		theme.ContentAddIcon(), func() { a.promptNewProfile() })
	var footer fyne.CanvasObject
	if len(profiles) == 0 {
		newBtn.Importance = widget.HighImportance
		footer = container.NewPadded(newBtn)
	} else {
		newBtn.Importance = widget.LowImportance
		footer = container.NewCenter(newBtn)
	}

	header := container.NewVBox(title, subtitle, widget.NewSeparator())
	return container.NewBorder(header, footer, nil, nil, container.NewVScroll(list))
}

// profileCard renders one company entry as a clickable card with icon
// + name + hint, picking up the active accent colour via the theme.
func (a *App) profileCard(name string) fyne.CanvasObject {
	icon := widget.NewIcon(theme.AccountIcon())
	nameLbl := widget.NewLabelWithStyle(name,
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hintLbl := widget.NewLabelWithStyle("Profil öffnen",
		fyne.TextAlignLeading, fyne.TextStyle{})
	right := widget.NewIcon(theme.NavigateNextIcon())

	inner := container.NewBorder(nil, nil, icon, right,
		container.NewVBox(nameLbl, hintLbl))

	bg := canvas.NewRectangle(cardBackgroundColor())
	bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	bg.StrokeWidth = 1
	bg.CornerRadius = 8
	card := container.NewStack(bg, container.NewPadded(inner))

	return container.NewPadded(newClickableCard(card, func() {
		a.startProfile(name)
	}))
}

// promptNewProfile asks for a profile name, creates its directory and opens it.
func (a *App) promptNewProfile() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Firmenname")
	dialog.ShowForm("Neues Profil", "Anlegen", "Abbrechen",
		[]*widget.FormItem{widget.NewFormItem("Name", entry)},
		func(ok bool) {
			if !ok {
				return
			}
			name := core.SanitizeFilename(strings.TrimSpace(entry.Text))
			if name == "" {
				dialog.ShowInformation("Ungültiger Name",
					"Bitte einen gültigen Profilnamen eingeben.", a.window)
				return
			}
			existing, _ := core.ListProfiles()
			for _, e := range existing {
				if e == name {
					dialog.ShowInformation("Profil existiert",
						"Ein Profil mit diesem Namen existiert bereits.", a.window)
					return
				}
			}
			dir, err := core.GetProfileConfigDir(name)
			if err != nil {
				dialog.ShowError(err, a.window)
				return
			}
			if err := os.MkdirAll(dir, 0755); err != nil {
				dialog.ShowError(err, a.window)
				return
			}
			a.maybeMigrateLegacyConfig(name, func() { a.startProfile(name) })
		},
		a.window)
}

// maybeMigrateLegacyConfig checks for a pre-profiles configuration directly
// in the config root. If found, it asks whether to assign it to the new
// profile; proceed is called once the decision has been handled.
func (a *App) maybeMigrateLegacyConfig(profile string, proceed func()) {
	root, err := core.GetConfigDir()
	if err != nil {
		proceed()
		return
	}
	if _, err := os.Stat(filepath.Join(root, "settings.json")); err != nil {
		proceed() // no legacy config present
		return
	}
	dialog.ShowConfirm("Bestehende Konfiguration",
		"Es wurde eine bestehende Konfiguration gefunden. Diesem Profil ("+profile+") zuordnen?",
		func(yes bool) {
			if yes {
				if err := migrateLegacyConfig(root, profile); err != nil {
					dialog.ShowError(err, a.window)
				}
			}
			proceed()
		}, a.window)
}

// migrateLegacyConfig moves the legacy config files into the profile
// directory and copies the API key to the profile-scoped keyring account.
func migrateLegacyConfig(root, profile string) error {
	dst, err := core.GetProfileConfigDir(profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, name := range []string{"settings.json", "company_accounts.json", "logs"} {
		src := filepath.Join(root, name)
		if _, err := os.Stat(src); err != nil {
			continue // not present — skip
		}
		if err := os.Rename(src, filepath.Join(dst, name)); err != nil {
			return fmt.Errorf("Verschieben von %s fehlgeschlagen: %w", name, err)
		}
	}
	// Copy the API key from the legacy keyring account ("claude") to the
	// profile-scoped account ("<profile>-claude").
	if val, err := keyring.Get("BuchISY", "claude"); err == nil {
		_ = keyring.Set("BuchISY", profile+"-claude", val)
	}
	return nil
}
