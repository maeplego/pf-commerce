package money

import "testing"

func TestNewRejectsNegativeAndBadCurrency(t *testing.T) {
	if _, err := New(-1, "JPY"); err == nil {
		t.Fatal("expected negative to fail")
	}
	if _, err := New(100, "YE"); err == nil {
		t.Fatal("expected non-ISO currency to fail")
	}
	if _, err := New(100, "JP"); err == nil {
		t.Fatal("expected short currency to fail")
	}
}

func TestJPYAddAndMul(t *testing.T) {
	a, err := JPY(1200)
	if err != nil {
		t.Fatal(err)
	}
	b, err := a.MulQty(3)
	if err != nil {
		t.Fatal(err)
	}
	if b.Minor != 3600 || b.Currency != "JPY" {
		t.Fatalf("mul: %+v", b)
	}
	sum, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Minor != 4800 {
		t.Fatalf("add: %d", sum.Minor)
	}
}

func TestAddCurrencyMismatch(t *testing.T) {
	a, _ := JPY(1)
	b, _ := New(1, "USD")
	if _, err := a.Add(b); err != ErrCurrency {
		t.Fatalf("got %v", err)
	}
}

func TestMulOverflow(t *testing.T) {
	a, _ := New(mathMaxInt64(), "JPY")
	if _, err := a.MulQty(2); err != ErrOverflow {
		t.Fatalf("got %v", err)
	}
}

func mathMaxInt64() int64 {
	return int64(^uint64(0) >> 1)
}
