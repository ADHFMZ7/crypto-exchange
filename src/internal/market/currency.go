package market

// TODO: Use this model when implementing multi-currency support
type Currency struct {
	Code     string `json:"code"`     // e.g. USD, BTC
	Name     string `json:"name"`     // e.g. US Dollar, Bitcoin
	Exponent int    `json:"exponent"` // number of decimal places
}

func (c Currency) Scale() int64 {
	scale := int64(1)
	for i := 0; i < c.Exponent; i++ {
		scale *= 10
	}
	return scale
}
