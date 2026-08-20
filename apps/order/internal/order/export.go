package order

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type ExportLine struct {
	OrderID      string `json:"orderId"`
	OrderDate    string `json:"orderDate"`
	ProductID    string `json:"productId"`
	SKU          string `json:"sku"`
	Quantity     int    `json:"quantity"`
	UnitPriceYen int64  `json:"unitPriceYen"`
	BuyerOpaque  string `json:"buyerOpaque"`
	Channel      string `json:"channel"`
}

func opaqueBuyer(sub string) string {
	sum := sha256.Sum256([]byte(sub))
	return hex.EncodeToString(sum[:8])
}

func (s *Service) ExportLines(ctx context.Context, day time.Time) ([]ExportLine, error) {
	all, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	want := day.UTC().Format("2006-01-02")
	var out []ExportLine
	for _, o := range all {
		if o.Status != StatusPaid && o.Status != StatusShipped {
			continue
		}
		orderDate := o.CreatedAt.UTC().Format("2006-01-02")
		if orderDate != want {
			continue
		}
		for _, ln := range o.Lines {
			out = append(out, ExportLine{
				OrderID:      o.ID,
				OrderDate:    orderDate,
				ProductID:    ln.ProductID,
				SKU:          ln.SKU,
				Quantity:     ln.Qty,
				UnitPriceYen: ln.UnitPriceMinor,
				BuyerOpaque:  opaqueBuyer(o.BuyerSub),
				Channel:      "web",
			})
		}
	}
	return out, nil
}
