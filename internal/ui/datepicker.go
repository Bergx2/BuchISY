package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// showDatePicker shows a month-calendar date picker. Clicking a day selects
// that date and closes the window; the month arrows navigate without
// selecting. The selected date is delivered to onSelect as "DD.MM.YYYY".
func (a *App) showDatePicker(parent fyne.Window, initialDate string, onSelect func(string)) {
	// Selected date: the initial date if valid, otherwise today.
	now := time.Now()
	selDay, selMonth, selYear := now.Day(), int(now.Month()), now.Year()
	if parts := strings.Split(initialDate, "."); len(parts) == 3 {
		d, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		m, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		y, e3 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if e1 == nil && e2 == nil && e3 == nil &&
			d >= 1 && d <= 31 && m >= 1 && m <= 12 && y > 0 {
			selDay, selMonth, selYear = d, m, y
		}
	}

	win := a.app.NewWindow("Datum wählen")
	viewYear, viewMonth := selYear, selMonth

	var render func()
	render = func() {
		prevBtn := widget.NewButton("‹", func() {
			viewMonth--
			if viewMonth < 1 {
				viewMonth = 12
				viewYear--
			}
			render()
		})
		nextBtn := widget.NewButton("›", func() {
			viewMonth++
			if viewMonth > 12 {
				viewMonth = 1
				viewYear++
			}
			render()
		})
		title := widget.NewLabelWithStyle(
			fmt.Sprintf("%s %d", a.bundle.T(fmt.Sprintf("month.%02d", viewMonth)), viewYear),
			fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		header := container.NewBorder(nil, nil, prevBtn, nextBtn, title)

		weekdays := container.NewGridWithColumns(7)
		for _, wd := range []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"} {
			weekdays.Add(widget.NewLabelWithStyle(wd, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
		}

		grid := container.NewGridWithColumns(7)
		first := time.Date(viewYear, time.Month(viewMonth), 1, 0, 0, 0, 0, time.Local)
		lead := (int(first.Weekday()) + 6) % 7 // leading blanks, Monday-first
		for i := 0; i < lead; i++ {
			grid.Add(widget.NewLabel(""))
		}
		daysInMonth := time.Date(viewYear, time.Month(viewMonth)+1, 0, 0, 0, 0, 0, time.Local).Day()
		for d := 1; d <= daysInMonth; d++ {
			day := d
			btn := widget.NewButton(strconv.Itoa(d), func() {
				onSelect(fmt.Sprintf("%02d.%02d.%04d", day, viewMonth, viewYear))
				win.Close()
			})
			if day == selDay && viewMonth == selMonth && viewYear == selYear {
				btn.Importance = widget.HighImportance // current date — clearly marked
			} else if day == now.Day() && viewMonth == int(now.Month()) && viewYear == now.Year() {
				btn.Importance = widget.SuccessImportance // today — subtly marked
			}
			grid.Add(btn)
		}

		cancelBtn := widget.NewButton(a.bundle.T("btn.cancel"), func() { win.Close() })

		win.SetContent(container.NewVBox(header, weekdays, grid, widget.NewSeparator(), cancelBtn))
	}
	render()

	win.Resize(fyne.NewSize(340, 380))
	win.CenterOnScreen()
	win.Show()
}
