package demoseed

// Items are fictional demo SKUs. MUG-1 starts at qty 1 for the shortage demo.
var Items = []Item{
	{SKU: "MUG-1", Name: "Demo Mug", Description: "Last-unit demo. Stock starts at 1.", PriceMinor: 1200, Currency: "JPY", ImageURL: "https://placehold.co/400x400?text=Mug", Stock: 1},
	{SKU: "TEE-1", Name: "Demo Tee", Description: "Plenty in stock.", PriceMinor: 3500, Currency: "JPY", ImageURL: "https://placehold.co/400x400?text=Tee", Stock: 20},
	{SKU: "STK-1", Name: "Demo Sticker", Description: "Already sold out.", PriceMinor: 300, Currency: "JPY", ImageURL: "https://placehold.co/400x400?text=Sticker", Stock: 0},
}

type Item struct {
	SKU         string
	Name        string
	Description string
	PriceMinor  int64
	Currency    string
	ImageURL    string
	Stock       int
}
