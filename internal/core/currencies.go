package core

import (
	"sort"
	"strings"
)

// Currency is an ISO 4217 currency: 3-letter code + English name.
type Currency struct {
	Code string
	Name string
}

// ISOCurrencies is the list of active ISO 4217 circulating national currencies.
var ISOCurrencies = []Currency{
	{Code: "AED", Name: "UAE Dirham"},
	{Code: "AFN", Name: "Afghan Afghani"},
	{Code: "ALL", Name: "Albanian Lek"},
	{Code: "AMD", Name: "Armenian Dram"},
	{Code: "ANG", Name: "Netherlands Antillean Guilder"},
	{Code: "AOA", Name: "Angolan Kwanza"},
	{Code: "ARS", Name: "Argentine Peso"},
	{Code: "AUD", Name: "Australian Dollar"},
	{Code: "AWG", Name: "Aruban Florin"},
	{Code: "AZN", Name: "Azerbaijani Manat"},
	{Code: "BAM", Name: "Bosnia-Herzegovina Convertible Mark"},
	{Code: "BBD", Name: "Barbadian Dollar"},
	{Code: "BDT", Name: "Bangladeshi Taka"},
	{Code: "BGN", Name: "Bulgarian Lev"},
	{Code: "BHD", Name: "Bahraini Dinar"},
	{Code: "BIF", Name: "Burundian Franc"},
	{Code: "BMD", Name: "Bermudian Dollar"},
	{Code: "BND", Name: "Brunei Dollar"},
	{Code: "BOB", Name: "Bolivian Boliviano"},
	{Code: "BRL", Name: "Brazilian Real"},
	{Code: "BSD", Name: "Bahamian Dollar"},
	{Code: "BTN", Name: "Bhutanese Ngultrum"},
	{Code: "BWP", Name: "Botswanan Pula"},
	{Code: "BYN", Name: "Belarusian Ruble"},
	{Code: "BZD", Name: "Belize Dollar"},
	{Code: "CAD", Name: "Canadian Dollar"},
	{Code: "CDF", Name: "Congolese Franc"},
	{Code: "CHF", Name: "Swiss Franc"},
	{Code: "CLP", Name: "Chilean Peso"},
	{Code: "CNY", Name: "Chinese Yuan"},
	{Code: "COP", Name: "Colombian Peso"},
	{Code: "CRC", Name: "Costa Rican Colón"},
	{Code: "CVE", Name: "Cape Verdean Escudo"},
	{Code: "CZK", Name: "Czech Koruna"},
	{Code: "DJF", Name: "Djiboutian Franc"},
	{Code: "DKK", Name: "Danish Krone"},
	{Code: "DOP", Name: "Dominican Peso"},
	{Code: "DZD", Name: "Algerian Dinar"},
	{Code: "EGP", Name: "Egyptian Pound"},
	{Code: "ERN", Name: "Eritrean Nakfa"},
	{Code: "ETB", Name: "Ethiopian Birr"},
	{Code: "EUR", Name: "Euro"},
	{Code: "FJD", Name: "Fijian Dollar"},
	{Code: "GBP", Name: "British Pound Sterling"},
	{Code: "GEL", Name: "Georgian Lari"},
	{Code: "GHS", Name: "Ghanaian Cedi"},
	{Code: "GIP", Name: "Gibraltar Pound"},
	{Code: "GMD", Name: "Gambian Dalasi"},
	{Code: "GNF", Name: "Guinean Franc"},
	{Code: "GTQ", Name: "Guatemalan Quetzal"},
	{Code: "GYD", Name: "Guyanaese Dollar"},
	{Code: "HKD", Name: "Hong Kong Dollar"},
	{Code: "HNL", Name: "Honduran Lempira"},
	{Code: "HTG", Name: "Haitian Gourde"},
	{Code: "HUF", Name: "Hungarian Forint"},
	{Code: "IDR", Name: "Indonesian Rupiah"},
	{Code: "ILS", Name: "Israeli New Shekel"},
	{Code: "INR", Name: "Indian Rupee"},
	{Code: "IQD", Name: "Iraqi Dinar"},
	{Code: "IRR", Name: "Iranian Rial"},
	{Code: "ISK", Name: "Icelandic Króna"},
	{Code: "JMD", Name: "Jamaican Dollar"},
	{Code: "JOD", Name: "Jordanian Dinar"},
	{Code: "JPY", Name: "Japanese Yen"},
	{Code: "KES", Name: "Kenyan Shilling"},
	{Code: "KGS", Name: "Kyrgystani Som"},
	{Code: "KHR", Name: "Cambodian Riel"},
	{Code: "KMF", Name: "Comorian Franc"},
	{Code: "KPW", Name: "North Korean Won"},
	{Code: "KRW", Name: "South Korean Won"},
	{Code: "KWD", Name: "Kuwaiti Dinar"},
	{Code: "KYD", Name: "Cayman Islands Dollar"},
	{Code: "KZT", Name: "Kazakhstani Tenge"},
	{Code: "LAK", Name: "Laotian Kip"},
	{Code: "LBP", Name: "Lebanese Pound"},
	{Code: "LKR", Name: "Sri Lankan Rupee"},
	{Code: "LRD", Name: "Liberian Dollar"},
	{Code: "LSL", Name: "Lesotho Loti"},
	{Code: "LYD", Name: "Libyan Dinar"},
	{Code: "MAD", Name: "Moroccan Dirham"},
	{Code: "MDL", Name: "Moldovan Leu"},
	{Code: "MGA", Name: "Malagasy Ariary"},
	{Code: "MKD", Name: "Macedonian Denar"},
	{Code: "MMK", Name: "Myanmar Kyat"},
	{Code: "MNT", Name: "Mongolian Tögrög"},
	{Code: "MOP", Name: "Macanese Pataca"},
	{Code: "MRU", Name: "Mauritanian Ouguiya"},
	{Code: "MUR", Name: "Mauritian Rupee"},
	{Code: "MVR", Name: "Maldivian Rufiyaa"},
	{Code: "MWK", Name: "Malawian Kwacha"},
	{Code: "MXN", Name: "Mexican Peso"},
	{Code: "MYR", Name: "Malaysian Ringgit"},
	{Code: "MZN", Name: "Mozambican Metical"},
	{Code: "NAD", Name: "Namibian Dollar"},
	{Code: "NGN", Name: "Nigerian Naira"},
	{Code: "NIO", Name: "Nicaraguan Córdoba"},
	{Code: "NOK", Name: "Norwegian Krone"},
	{Code: "NPR", Name: "Nepalese Rupee"},
	{Code: "NZD", Name: "New Zealand Dollar"},
	{Code: "OMR", Name: "Omani Rial"},
	{Code: "PAB", Name: "Panamanian Balboa"},
	{Code: "PEN", Name: "Peruvian Sol"},
	{Code: "PGK", Name: "Papua New Guinean Kina"},
	{Code: "PHP", Name: "Philippine Peso"},
	{Code: "PKR", Name: "Pakistani Rupee"},
	{Code: "PLN", Name: "Polish Zloty"},
	{Code: "PYG", Name: "Paraguayan Guaraní"},
	{Code: "QAR", Name: "Qatari Riyal"},
	{Code: "RON", Name: "Romanian Leu"},
	{Code: "RSD", Name: "Serbian Dinar"},
	{Code: "RUB", Name: "Russian Ruble"},
	{Code: "RWF", Name: "Rwandan Franc"},
	{Code: "SAR", Name: "Saudi Riyal"},
	{Code: "SBD", Name: "Solomon Islands Dollar"},
	{Code: "SCR", Name: "Seychellois Rupee"},
	{Code: "SDG", Name: "Sudanese Pound"},
	{Code: "SEK", Name: "Swedish Krona"},
	{Code: "SGD", Name: "Singapore Dollar"},
	{Code: "SLL", Name: "Sierra Leonean Leone"},
	{Code: "SOS", Name: "Somali Shilling"},
	{Code: "SRD", Name: "Surinamese Dollar"},
	{Code: "SSP", Name: "South Sudanese Pound"},
	{Code: "STN", Name: "São Tomé and Príncipe Dobra"},
	{Code: "SVC", Name: "Salvadoran Colón"},
	{Code: "SYP", Name: "Syrian Pound"},
	{Code: "SZL", Name: "Swazi Lilangeni"},
	{Code: "THB", Name: "Thai Baht"},
	{Code: "TJS", Name: "Tajikistani Somoni"},
	{Code: "TMT", Name: "Turkmenistani Manat"},
	{Code: "TND", Name: "Tunisian Dinar"},
	{Code: "TOP", Name: "Tongan Paʻanga"},
	{Code: "TRY", Name: "Turkish Lira"},
	{Code: "TTD", Name: "Trinidad and Tobago Dollar"},
	{Code: "TWD", Name: "New Taiwan Dollar"},
	{Code: "TZS", Name: "Tanzanian Shilling"},
	{Code: "UAH", Name: "Ukrainian Hryvnia"},
	{Code: "UGX", Name: "Ugandan Shilling"},
	{Code: "USD", Name: "United States Dollar"},
	{Code: "UYU", Name: "Uruguayan Peso"},
	{Code: "UZS", Name: "Uzbekistani Som"},
	{Code: "VES", Name: "Venezuelan Bolívar Soberano"},
	{Code: "VND", Name: "Vietnamese Dong"},
	{Code: "WST", Name: "Samoan Tālā"},
	{Code: "XAF", Name: "Central African CFA Franc"},
	{Code: "XCD", Name: "East Caribbean Dollar"},
	{Code: "XOF", Name: "West African CFA Franc"},
	{Code: "XPF", Name: "CFP Franc"},
	{Code: "YER", Name: "Yemeni Rial"},
	{Code: "ZAR", Name: "South African Rand"},
	{Code: "ZMW", Name: "Zambian Kwacha"},
	{Code: "ZWL", Name: "Zimbabwean Dollar"},
}

// topCurrencies are shown first in dropdowns, in this order.
var topCurrencies = []string{"EUR", "USD", "CAD", "AUD"}

// CurrencyOptions returns "CODE — Name" strings: the top currencies first,
// then the rest sorted by code.
func CurrencyOptions() []string {
	byCode := map[string]Currency{}
	for _, c := range ISOCurrencies {
		byCode[c.Code] = c
	}
	isTop := map[string]bool{}
	var opts []string
	for _, code := range topCurrencies {
		if c, ok := byCode[code]; ok {
			opts = append(opts, c.Code+" — "+c.Name)
			isTop[code] = true
		}
	}
	rest := make([]Currency, 0, len(ISOCurrencies))
	for _, c := range ISOCurrencies {
		if !isTop[c.Code] {
			rest = append(rest, c)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].Code < rest[j].Code })
	for _, c := range rest {
		opts = append(opts, c.Code+" — "+c.Name)
	}
	return opts
}

// CurrencyOptionForCode returns the option string for a code (bare code if unknown).
func CurrencyOptionForCode(code string) string {
	for _, c := range ISOCurrencies {
		if c.Code == code {
			return c.Code + " — " + c.Name
		}
	}
	return code
}

// CurrencyCodeFromOption parses the 3-letter code from a "CODE — Name" option.
func CurrencyCodeFromOption(opt string) string {
	if i := strings.Index(opt, " — "); i >= 0 {
		return strings.TrimSpace(opt[:i])
	}
	return strings.TrimSpace(opt)
}
