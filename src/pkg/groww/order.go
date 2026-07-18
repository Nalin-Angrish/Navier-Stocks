package groww

import (
	"fmt"
	"net/url"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
)

// PlaceOrder sends a new MIS intraday order to Groww.
// Product is hardcoded to MIS, validity to DAY.
func (c *Client) PlaceOrder(req *PlaceOrderRequest) (*PlaceOrderResponse, error) {
	body := map[string]any{
		"trading_symbol":     req.TradingSymbol,
		"quantity":           req.Quantity,
		"price":              req.Price,
		"trigger_price":      req.TriggerPrice,
		"validity":           "DAY",
		"exchange":           req.Exchange,
		"segment":            req.Segment,
		"product":            "MIS",
		"order_type":         req.OrderType,
		"transaction_type":   req.TransactionType,
		"order_reference_id": req.OrderReferenceID,
	}
	resp, err := c.do("POST", "/order/create", body, c.ordersRL)
	if err != nil {
		return nil, err
	}
	var out PlaceOrderResponse
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyOrder modifies an existing pending/open order.
func (c *Client) ModifyOrder(req *ModifyOrderRequest) (*ModifyOrderResponse, error) {
	resp, err := c.do("POST", "/order/modify", req, c.ordersRL)
	if err != nil {
		return nil, err
	}
	var out ModifyOrderResponse
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelOrder cancels a pending/open order by its Groww order ID.
func (c *Client) CancelOrder(seg Segment, growwOrderID string) (*CancelOrderResponse, error) {
	req := &CancelOrderRequest{
		Segment:      seg,
		GrowwOrderID: growwOrderID,
	}
	resp, err := c.do("POST", "/order/cancel", req, c.ordersRL)
	if err != nil {
		return nil, err
	}
	var out CancelOrderResponse
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOrderStatus fetches the current status of an order.
func (c *Client) GetOrderStatus(seg Segment, growwOrderID string) (*OrderStatusResponse, error) {
	path := fmt.Sprintf("/order/status/%s?segment=%s", url.PathEscape(growwOrderID), string(seg))
	resp, err := c.do("GET", path, nil, c.nonTradingRL)
	if err != nil {
		return nil, err
	}
	var out OrderStatusResponse
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOrderStatusByRef fetches order status using the user-defined reference ID.
func (c *Client) GetOrderStatusByRef(seg Segment, refID string) (*OrderStatusResponse, error) {
	path := fmt.Sprintf("/order/status/reference/%s?segment=%s", url.PathEscape(refID), string(seg))
	resp, err := c.do("GET", path, nil, c.nonTradingRL)
	if err != nil {
		return nil, err
	}
	var out OrderStatusResponse
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOrderList returns the day's order history.
func (c *Client) GetOrderList(seg Segment, page, pageSize int) ([]OrderListEntry, error) {
	path := fmt.Sprintf("/order/list?segment=%s&page=%d&page_size=%d", string(seg), page, pageSize)
	resp, err := c.do("GET", path, nil, c.nonTradingRL)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		OrderList []OrderListEntry `json:"order_list"`
	}
	if err := decodeResponse(resp, &envelope); err != nil {
		return nil, err
	}
	return envelope.OrderList, nil
}

// GetOrderDetail returns full details for a specific order.
func (c *Client) GetOrderDetail(seg Segment, growwOrderID string) (*OrderDetail, error) {
	path := fmt.Sprintf("/order/detail/%s?segment=%s", url.PathEscape(growwOrderID), string(seg))
	resp, err := c.do("GET", path, nil, c.nonTradingRL)
	if err != nil {
		return nil, err
	}
	var out OrderDetail
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOrderTrades returns the list of individual trade executions for an order.
func (c *Client) GetOrderTrades(seg Segment, growwOrderID string, page, pageSize int) ([]Trade, error) {
	path := fmt.Sprintf("/order/trades/%s?segment=%s&page=%d&page_size=%d",
		url.PathEscape(growwOrderID), string(seg), page, pageSize)
	resp, err := c.do("GET", path, nil, c.nonTradingRL)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		TradeList []Trade `json:"trade_list"`
	}
	if err := decodeResponse(resp, &envelope); err != nil {
		return nil, err
	}
	return envelope.TradeList, nil
}

// PlaceOrderRequestFromTradeExecution maps the internal TradeExecution
// model to a Groww PlaceOrderRequest.  It selects sensible defaults:
//   - Exchange: NSE
//   - Segment:  CASH
//   - Product:  MIS (intraday, set internally)
//   - OrderType: LIMIT
func PlaceOrderRequestFromTradeExecution(exec *models.TradeExecution) *PlaceOrderRequest {
	tt := TransactionTypeBuy
	if exec.Side == models.SideShort {
		tt = TransactionTypeSell
	}
	return &PlaceOrderRequest{
		TradingSymbol:    exec.Ticker,
		Quantity:         exec.Quantity,
		Price:            exec.Price,
		Exchange:         ExchangeNSE,
		Segment:          SegmentCash,
		OrderType:        OrderTypeLimit,
		TransactionType:  tt,
		OrderReferenceID: exec.ExecutionRef,
	}
}

// ParseOrderReferenceID is a helper that converts an ExecutionRef string
// to a Groww-compliant reference ID (8–20 alphanumeric chars, max 2 hyphens).
// It truncates or pads the ExecutionRef to fit the constraint.
func ParseOrderReferenceID(ref string) string {
	const maxLen = 20
	const minLen = 8
	if len(ref) > maxLen {
		ref = ref[:maxLen]
	}
	// Count hyphens and truncate further if needed.
	hyphenCount := 0
	keep := make([]byte, 0, len(ref))
	for _, c := range []byte(ref) {
		if c == '-' {
			hyphenCount++
			if hyphenCount > 2 {
				continue
			}
		}
		keep = append(keep, c)
	}
	if len(keep) < minLen {
		// Pad with '0' at the end.
		for len(keep) < minLen {
			keep = append(keep, '0')
		}
	}
	return string(keep)
}

// OrderReferenceID generates a Groww-compliant reference ID from an
// execution reference string.
func OrderReferenceID(execRef string) string {
	return ParseOrderReferenceID(execRef)
}


