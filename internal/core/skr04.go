package core

import (
	"sort"
	"strconv"
	"strings"
)

// SKRAccount is one account of an SKR04-style chart of accounts.
type SKRAccount struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	Type   string `json:"type"`             // e.g. "expense", "revenue", "asset", "vat"
	TaxKey string `json:"tax_key,omitempty"` // optional DATEV BU-/Steuerschlüssel
}

// ChartOfAccounts is an indexed, searchable set of SKRAccounts.
type ChartOfAccounts struct {
	accounts []SKRAccount
	byNumber map[int]SKRAccount
}

// NewChartOfAccounts builds a chart from accounts (later entries win on
// duplicate number, so an imported override can replace a bundled default).
func NewChartOfAccounts(accs []SKRAccount) *ChartOfAccounts {
	byNumber := make(map[int]SKRAccount, len(accs))
	for _, a := range accs {
		byNumber[a.Number] = a
	}
	out := make([]SKRAccount, 0, len(byNumber))
	for _, a := range byNumber {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return &ChartOfAccounts{accounts: out, byNumber: byNumber}
}

// Find returns the account with the given number.
func (c *ChartOfAccounts) Find(number int) (SKRAccount, bool) {
	a, ok := c.byNumber[number]
	return a, ok
}

// All returns every account, sorted by number.
func (c *ChartOfAccounts) All() []SKRAccount {
	return append([]SKRAccount{}, c.accounts...)
}

// Search returns accounts whose number text or name contains q
// (case-insensitive), sorted by number.
func (c *ChartOfAccounts) Search(q string) []SKRAccount {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return c.All()
	}
	var out []SKRAccount
	for _, a := range c.accounts {
		if strings.Contains(strconv.Itoa(a.Number), q) || strings.Contains(strings.ToLower(a.Name), q) {
			out = append(out, a)
		}
	}
	return out
}
