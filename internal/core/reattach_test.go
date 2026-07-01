package core

import "testing"

func TestReattachInvoiceRefs_ByStableIdentity(t *testing.T) {
	ref := &InvoiceRef{Filename: "inv.pdf"}
	old := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "08.06.2026", Betrag: 309.39},
		{Page: 0, LineIdx: 2, Date: "08.06.2026", Betrag: 119.00},
		{Page: 0, LineIdx: 3, Date: "10.06.2026", Betrag: 434.35, InvoiceRef: ref}, // linked
	}
	// A parser fix dropped a spurious booking, so the 434.35 booking is now at
	// LineIdx 2 instead of 3. The link must follow the booking, not the index.
	parsed := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "08.06.2026", Betrag: 309.39},
		{Page: 0, LineIdx: 2, Date: "10.06.2026", Betrag: 434.35},
	}
	reattachInvoiceRefs(old, parsed)
	if parsed[1].InvoiceRef != ref {
		t.Errorf("ref should re-attach to the 434.35 booking by identity despite the index change")
	}
	if parsed[0].InvoiceRef != nil {
		t.Errorf("the unrelated 309.39 booking must not receive a ref")
	}
}

func TestReattachInvoiceRefs_DuplicatesInOrder(t *testing.T) {
	r1, r2 := &InvoiceRef{Filename: "a.pdf"}, &InvoiceRef{Filename: "b.pdf"}
	old := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "05.01.2026", Betrag: 10.00, InvoiceRef: r1},
		{Page: 0, LineIdx: 2, Date: "05.01.2026", Betrag: 10.00, InvoiceRef: r2},
	}
	parsed := []StatementBooking{
		{Page: 0, LineIdx: 1, Date: "05.01.2026", Betrag: 10.00},
		{Page: 0, LineIdx: 2, Date: "05.01.2026", Betrag: 10.00},
	}
	reattachInvoiceRefs(old, parsed)
	if parsed[0].InvoiceRef != r1 || parsed[1].InvoiceRef != r2 {
		t.Errorf("same-identity duplicates must re-attach in order: got %v, %v", parsed[0].InvoiceRef, parsed[1].InvoiceRef)
	}
}
