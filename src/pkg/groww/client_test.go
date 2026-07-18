package groww_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/groww"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/utils"
)

func TestMain(m *testing.M) {
	utils.LoadEnv()
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mockGrowwServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *groww.Client) {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	t.Setenv("GROWW_BASE_URL", s.URL)
	c := groww.NewClient("test-token")
	return s, c
}

func successResponse(t *testing.T, payload any) string {
	t.Helper()
	wrapper := struct {
		Status  string `json:"status"`
		Payload any    `json:"payload"`
	}{
		Status:  "SUCCESS",
		Payload: payload,
	}
	data, _ := json.Marshal(wrapper)
	return string(data)
}

// ---------------------------------------------------------------------------
// Order tests
// ---------------------------------------------------------------------------

func TestPlaceOrder_Success(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/order/create" {
			t.Fatalf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("bad auth: %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(t, groww.PlaceOrderResponse{
			GrowwOrderID:     "GMK123",
			OrderStatus:      groww.OrderStatusOpen,
			OrderReferenceID: "ref-001",
			Remark:           "OK",
		})))
	})

	resp, err := c.PlaceOrder(&groww.PlaceOrderRequest{
		TradingSymbol:    "RELIANCE",
		Quantity:         10,
		Price:            2500,
		Exchange:         groww.ExchangeNSE,
		Segment:          groww.SegmentCash,
		OrderType:        groww.OrderTypeLimit,
		TransactionType:  groww.TransactionTypeBuy,
		OrderReferenceID: "ref-001",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if resp.GrowwOrderID != "GMK123" {
		t.Fatalf("GrowwOrderID = %q, want %q", resp.GrowwOrderID, "GMK123")
	}
	if resp.OrderStatus != groww.OrderStatusOpen {
		t.Fatalf("OrderStatus = %q, want %q", resp.OrderStatus, groww.OrderStatusOpen)
	}
}

func TestPlaceOrder_Error(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"FAILURE","payload":{"code":"INVALID_SYMBOL","message":"Symbol not found"}}`))
	})

	_, err := c.PlaceOrder(&groww.PlaceOrderRequest{
		TradingSymbol:    "INVALID",
		Quantity:         1,
		Exchange:         groww.ExchangeNSE,
		Segment:          groww.SegmentCash,
		OrderType:        groww.OrderTypeLimit,
		TransactionType:  groww.TransactionTypeBuy,
		OrderReferenceID: "ref-bad",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	t.Logf("got expected error: %v", err)
}

func TestCancelOrder(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/order/cancel" {
			t.Fatalf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(t, groww.CancelOrderResponse{
			GrowwOrderID: "GMK123",
			OrderStatus:  groww.OrderStatusCancelled,
		})))
	})

	resp, err := c.CancelOrder(groww.SegmentCash, "GMK123")
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if resp.OrderStatus != groww.OrderStatusCancelled {
		t.Fatalf("OrderStatus = %q, want %q", resp.OrderStatus, groww.OrderStatusCancelled)
	}
}

func TestGetOrderStatus(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/order/status/GMK123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(t, groww.OrderStatusResponse{
			GrowwOrderID:     "GMK123",
			OrderStatus:      groww.OrderStatusExecuted,
			FilledQuantity:   10,
			OrderReferenceID: "ref-001",
		})))
	})

	resp, err := c.GetOrderStatus(groww.SegmentCash, "GMK123")
	if err != nil {
		t.Fatalf("GetOrderStatus: %v", err)
	}
	if resp.OrderStatus != groww.OrderStatusExecuted {
		t.Fatalf("OrderStatus = %q, want %q", resp.OrderStatus, groww.OrderStatusExecuted)
	}
	if resp.FilledQuantity != 10 {
		t.Fatalf("FilledQuantity = %d, want 10", resp.FilledQuantity)
	}
}

func TestGetOrderList(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(t, map[string][]groww.OrderListEntry{
			"order_list": {
				{OrderDetail: groww.OrderDetail{
					GrowwOrderID:  "GMK1",
					TradingSymbol: "RELIANCE",
					OrderStatus:   groww.OrderStatusExecuted,
					Quantity:      10,
				}},
			},
		})))
	})

	orders, err := c.GetOrderList(groww.SegmentCash, 0, 10)
	if err != nil {
		t.Fatalf("GetOrderList: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("got %d orders, want 1", len(orders))
	}
	if orders[0].GrowwOrderID != "GMK1" {
		t.Fatalf("OrderID = %q, want %q", orders[0].GrowwOrderID, "GMK1")
	}
}

func TestPlaceOrderRequestFromTradeExecution(t *testing.T) {
	exec := &models.TradeExecution{
		Ticker:       "TCS",
		Side:         models.SideLong,
		Quantity:     25,
		Price:        3450,
		ExecutionRef: "exec-001",
	}

	req := groww.PlaceOrderRequestFromTradeExecution(exec)
	if req.TradingSymbol != "TCS" {
		t.Fatalf("TradingSymbol = %q, want %q", req.TradingSymbol, "TCS")
	}
	if req.TransactionType != groww.TransactionTypeBuy {
		t.Fatalf("TransactionType = %q, want %q", req.TransactionType, groww.TransactionTypeBuy)
	}
	if req.Quantity != 25 {
		t.Fatalf("Quantity = %d, want 25", req.Quantity)
	}
	if req.Price != 3450 {
		t.Fatalf("Price = %f, want 3450", req.Price)
	}
	if req.OrderReferenceID != "exec-001" {
		t.Fatalf("OrderReferenceID = %q, want %q", req.OrderReferenceID, "exec-001")
	}
}

func TestPlaceOrderRequestFromTradeExecution_Short(t *testing.T) {
	exec := &models.TradeExecution{
		Ticker:       "INFY",
		Side:         models.SideShort,
		Quantity:     10,
		Price:        1500,
		ExecutionRef: "exec-short",
	}

	req := groww.PlaceOrderRequestFromTradeExecution(exec)
	if req.TransactionType != groww.TransactionTypeSell {
		t.Fatalf("TransactionType = %q, want %q", req.TransactionType, groww.TransactionTypeSell)
	}
}

// ---------------------------------------------------------------------------
// Live data tests
// ---------------------------------------------------------------------------

func TestGetQuote(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live-data/quote" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(t, groww.Quote{
			LastPrice: 150.25,
			DayChange: 2.5,
			Volume:    10000,
		})))
	})

	quote, err := c.GetQuote(groww.ExchangeNSE, groww.SegmentCash, "RELIANCE")
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if quote.LastPrice != 150.25 {
		t.Fatalf("LastPrice = %f, want 150.25", quote.LastPrice)
	}
	if quote.Volume != 10000 {
		t.Fatalf("Volume = %d, want 10000", quote.Volume)
	}
}

func TestGetLTP(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(t, groww.LTPResponse{
			"NSE_RELIANCE": 2334.2,
			"NSE_TCS":      3450.0,
		})))
	})

	ltp, err := c.GetLTP(groww.SegmentCash, []string{"NSE_RELIANCE", "NSE_TCS"})
	if err != nil {
		t.Fatalf("GetLTP: %v", err)
	}
	if ltp["NSE_RELIANCE"] != 2334.2 {
		t.Fatalf("NSE_RELIANCE = %f, want 2334.2", ltp["NSE_RELIANCE"])
	}
	if ltp["NSE_TCS"] != 3450.0 {
		t.Fatalf("NSE_TCS = %f, want 3450", ltp["NSE_TCS"])
	}
}

// ---------------------------------------------------------------------------
// Margin tests
// ---------------------------------------------------------------------------

func TestGetUserMargin(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/margins/detail/user" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(t, groww.MarginResponse{
			ClearCash:     50000,
			NetMarginUsed: 15000,
		})))
	})

	margin, err := c.GetUserMargin()
	if err != nil {
		t.Fatalf("GetUserMargin: %v", err)
	}
	if margin.ClearCash != 50000 {
		t.Fatalf("ClearCash = %f, want 50000", margin.ClearCash)
	}
	if margin.NetMarginUsed != 15000 {
		t.Fatalf("NetMarginUsed = %f, want 15000", margin.NetMarginUsed)
	}
}

// ---------------------------------------------------------------------------
// Error handling tests
// ---------------------------------------------------------------------------

func TestAuthError(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":"FAILURE","payload":{"message":"Token expired"}}`))
	})

	// Without API key+secret, 401 should return ErrAuthExpired.
	_, err := c.GetUserMargin()
	if err == nil {
		t.Fatal("expected error")
	}
	if !groww.IsAuthExpired(err) {
		t.Fatalf("expected auth error, got: %v", err)
	}
}

func TestRateLimitError(t *testing.T) {
	_, c := mockGrowwServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"status":"FAILURE","payload":{"message":"Rate limit exceeded"}}`))
	})

	_, err := c.GetUserMargin()
	if err == nil {
		t.Fatal("expected error")
	}
	if !groww.IsRateLimited(err) {
		t.Fatalf("expected rate-limit error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Utility tests
// ---------------------------------------------------------------------------

func TestOrderReferenceID(t *testing.T) {
	tests := []struct {
		in       string
		min, max int
	}{
		{"exec-001", 8, 20},
		{"short", 8, 20},
		{"a-very-long-execution-reference-id-that-exceeds-twenty", 8, 20},
		{"abcdefgh", 8, 20},
	}
	for _, tc := range tests {
		got := groww.OrderReferenceID(tc.in)
		if len(got) < tc.min || len(got) > tc.max {
			t.Errorf("OrderReferenceID(%q) = %q (len %d), want len in [%d, %d]",
				tc.in, got, len(got), tc.min, tc.max)
		}
		// Verify no more than 2 hyphens.
		hyphens := 0
		for _, c := range got {
			if c == '-' {
				hyphens++
			}
		}
		if hyphens > 2 {
			t.Errorf("OrderReferenceID(%q) = %q has %d hyphens, max 2", tc.in, got, hyphens)
		}
	}
}
