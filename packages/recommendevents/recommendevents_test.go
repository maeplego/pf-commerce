package recommendevents_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/portfolio/pf-commerce/packages/recommendevents"
)

func TestPostPurchaseNoOp(t *testing.T) {
	recommendevents.PostPurchase(context.Background(), "", "alice", []recommendevents.Line{{SKU: "MUG-1"}}, nil)
}

func TestPostPurchasePostsEvents(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]string
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		got = append(got, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	recommendevents.PostPurchase(context.Background(), srv.URL, "alice", []recommendevents.Line{
		{SKU: "MUG-1", Qty: 2},
		{SKU: "", Qty: 1},
	}, srv.Client())

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("events=%d want 2", len(got))
	}
	if got[0]["type"] != "purchase" || got[0]["item_id"] != "MUG-1" || got[0]["user_id"] != "alice" {
		t.Fatalf("first=%v", got[0])
	}
}
