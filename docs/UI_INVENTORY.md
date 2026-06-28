# BuchISY UI Inventory

User-facing surface to reproduce in a rebuild. Behavior details live in `FUNCTIONAL_SPEC.md`; this file lists screens, actions, and expected states.

## App Shell

- Startup profile picker: list profiles, create profile, migrate legacy config into a profile when needed.
- Main top bar:
  - view toggle: `Belege` and `Konten`;
  - settings button;
  - overflow/menu button;
  - paste/import buttons;
  - month navigation previous/next and current year/month selector;
  - period lock indicator when current month is locked.
- Global keyboard behavior:
  - table row navigation with arrow keys;
  - Enter opens edit dialog;
  - Delete starts delete flow;
  - Ctrl+Left/Ctrl+Right changes month;
  - Ctrl+S saves in entry/edit dialogs;
  - Esc closes dialogs/windows where supported;
  - Ctrl+Plus/Ctrl+Minus/Ctrl+0 and Ctrl+mouse wheel adjust UI scale with overlay.
- Toasts: non-modal success/info messages, 8-second undo toast for deletes, optional action button.
- Error/info dialogs: modal title + message; no silent failures for user-triggered actions.

## Belege View

- Empty state with import call-to-action.
- Invoice table:
  - configurable columns and order;
  - sortable columns;
  - global search entry;
  - quick-filter chips: attachments, partial payment, outgoing, not booked/open states;
  - sum bar for visible rows;
  - status symbols and legend;
  - tooltips for long cell values;
  - context menu: edit/delete, copy cell, copy row, open original, open attachment, unlink bank statement.
- Global search popup:
  - searches across months;
  - clicking a result jumps to the invoice month and selects/opens context.

## Intake and File Pickers

- Import paths:
  - main file picker;
  - multi-file batch picker;
  - custom sortable picker with search, breadcrumb, Documents/Desktop/Downloads shortcuts;
  - drag/drop if supported by platform;
  - clipboard path/image paste;
  - scan-inbox watcher.
- Multi-file picker:
  - mark one selected file as main with a star;
  - reorder/remove attachments;
  - only supported file types selectable for receipts/attachments.
- Batch flow:
  - process files sequentially;
  - show confirmation modal for manual review unless auto-booking succeeds;
  - show batch result toast for auto-booked vs review count.

## Invoice Confirmation Dialog

- New invoice modal fields:
  - Auftraggeber, Verwendungszweck, Rechnungsnummer, USt-IdNr;
  - Rechnungsdatum with date picker;
  - net/VAT/gross, tax-line editor, Trinkgeld, Rabatt, fee, currency, exchange rate;
  - Gegenkonto picker;
  - payment account, cash-paid shortcut, payment date, partial-payment flag;
  - outgoing-invoice flag;
  - Bewirtung Anlass/Teilnehmer;
  - comments;
  - filename preview/edit;
  - attachment switcher/add/delete.
- Booking area:
  - live auto-booking preview;
  - manual booking editor;
  - reset to auto;
  - balance state visible before save.
- Document preview:
  - PDF/image preview;
  - zoom in/out/reset;
  - page next/previous;
  - pan/scroll;
  - highlight overlays for extracted values;
  - original/attachment switcher.
- Save behavior:
  - duplicate warning/check;
  - file move/copy + DB insert + CSV rewrite;
  - period-lock guard;
  - fallback from failed auto-booking to modal.

## Invoice Edit Dialog

- Same core fields as confirmation dialog.
- Opens the existing filed document and attachments.
- Supports adding/deleting numbered `_Anhang<N>` siblings.
- Supports opening original file in OS.
- Supports "Mit Kontoauszug abgleichen" for eligible bank/credit-card receipts.
- Supports "Als Anlagegut erfassen" / open Anlagen where relevant.
- Save may rename/move the main file and attachments.
- Delete button starts delete confirmation.
- Locked periods block editing with an explanatory message.

## Settings View

- Storage:
  - storage root;
  - scan inbox folder;
  - month subfolders.
- Filename:
  - naming template;
  - decimal separator;
  - default currency.
- Processing:
  - Claude/local mode;
  - model name;
  - API key stored in OS keychain;
  - own VAT-ID exclusion;
  - debug mode.
- Accounts:
  - custom counter-accounts;
  - chart import;
  - SKR03/SKR04 validate/switch;
  - account picker integration;
  - recent/favorite account behavior.
- Payment accounts:
  - up to 30 Zahlungskonten;
  - bank, credit card, cash, payroll;
  - IBAN, settlement account, SKR mapping.
- Booking rules:
  - VAT accounts;
  - Bewirtung split;
  - reverse charge;
  - gifts, travel, vehicle defaults;
  - per-profile rule override.
- Reconciliation:
  - match date window;
  - foreign-currency tolerance.
- Exports:
  - CSV separator;
  - CSV encoding;
  - column order;
  - DATEV Berater/Mandant/Wirtschaftsjahr fields.
- Maintenance:
  - re-book foreign receipts to EUR;
  - wipe database with confirmation.

## Konten View

- Account selector/dropdown with rich labels.
- Create/edit payment accounts through Settings.
- Statement upload for selected payment account.
- Statement list:
  - grouped/folder-aware view;
  - upload date/period/opening/closing metadata;
  - context menu edit, auto-fill metadata, delete;
  - open account folder.
- Auto-fill metadata for one or all statements.
- Belegabgleich action.
- Missing receipts list for unmatched debit statement lines.
- Statement edit dialog:
  - period, opening balance, closing balance, optional note fields as currently supported;
  - writes `metadata.json`.

## Reconciliation Views

- Belegabgleich:
  - candidate list from bank/credit-card statements;
  - scored matches;
  - no silent auto-linking;
  - user confirms every link;
  - grouped and partial-payment candidates;
  - alias learning side effect;
  - unlink support.
- Erlös-Abgleich:
  - outgoing-invoice matching flow;
  - same confirmation-first principle.
- Single-invoice match flow from edit dialog.
- Cash confirmation uses the internal sentinel reference, not an external statement.

## Kassenbuch

- Per cash account/month view.
- Opening balance/carry-in display.
- Deposits/Einlagen table.
- Cash-paid invoice rows as expenses.
- Running balance and coverage checks.
- Save per-month `kassenbuch.json`.
- Kassenbericht PDF export.
- Cash year overview:
  - 12 months;
  - carry-forward balances;
  - PDF export.

## Reports and Exports Menu

- Kassenbuch.
- CSV export by month/range.
- Booking export: DATEV, Lexware, booking journal PDF, include-exported option.
- GoBD/DATEV export package ZIP.
- Controlling.
- Auto-booking rules dialog.
- UStVA: month/quarter/year, PDF, XML.
- ZM: quarter default, PDF, XML.
- Year overview.
- Open items / OPOS.
- SuSa.
- GuV.
- Belegliste PDF.
- Rechnungsausgangsbuch PDF.
- Audit log.
- Lock/unlock current month.
- Belegabgleich.
- Erlös-Abgleich.
- Anlagen.
- Renumber receipt numbers.
- Backup ZIP.
- Verfahrensdokumentation PDF.

## Anlagen

- Asset register list.
- Add/edit asset form.
- Linear AfA and GWG behavior.
- Anlagenspiegel/report export.
- Link/note from invoice edit where relevant.

## Auto-Booking Rules

- Lists learned supplier templates.
- Allows enabling/disabling `autobook`.
- Shows account/category summary.
- Default is off; user explicitly opts in per supplier/rule.

## Visual/State Requirements

- German is primary; English must remain selectable.
- Locked period state must be visible and must block edit/delete paths.
- Duplicate/validation warnings must be visible before save.
- Long labels/cell values must not break table layout.
- Progress and errors during batch import, extraction, and export must be observable.
- All report/export results must show success or a concrete error.
