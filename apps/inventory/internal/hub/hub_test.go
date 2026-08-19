package hub_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/portfolio/pf-commerce/apps/inventory/internal/hub"
)

func TestPublishDeliversToSubscriber(t *testing.T) {
	h := hub.New()
	ch := h.Subscribe()
	defer h.Unsubscribe(ch)
	h.Publish(hub.Event{ProductID: "p1", AvailableQty: 3, Reason: "inbound"})
	select {
	case raw := <-ch:
		var ev hub.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.ProductID != "p1" || ev.AvailableQty != 3 {
			t.Fatalf("%+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
