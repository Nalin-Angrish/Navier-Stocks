package groww

// GetUserMargin retrieves margin details for the authenticated user.
func (c *Client) GetUserMargin() (*MarginResponse, error) {
	resp, err := c.do("GET", "/margins/detail/user", nil, c.nonTradingRL)
	if err != nil {
		return nil, err
	}
	var out MarginResponse
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CalculateMargin calculates the margin required for a single order or
// basket of MIS orders.  Product is hardcoded to MIS internally.
func (c *Client) CalculateMargin(orders []MarginCalcRequest) (*MarginCalcResponse, error) {
	type marginCalcBody struct {
		TradingSymbol   string          `json:"trading_symbol"`
		TransactionType TransactionType `json:"transaction_type"`
		Quantity        int             `json:"quantity"`
		Price           float64         `json:"price,omitempty"`
		OrderType       OrderType       `json:"order_type"`
		Product         string          `json:"product"`
		Exchange        Exchange        `json:"exchange"`
	}
	body := make([]marginCalcBody, len(orders))
	for i, o := range orders {
		body[i] = marginCalcBody{
			TradingSymbol:   o.TradingSymbol,
			TransactionType: o.TransactionType,
			Quantity:        o.Quantity,
			Price:           o.Price,
			OrderType:       o.OrderType,
			Product:         "MIS",
			Exchange:        o.Exchange,
		}
	}
	resp, err := c.do("POST", "/margins/detail/orders", body, c.nonTradingRL)
	if err != nil {
		return nil, err
	}
	var out MarginCalcResponse
	if err := decodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
