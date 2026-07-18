// Package groww implements a Go client for the Groww Trading API
// (https://groww.in/trade-api).  It covers MIS (intraday) order management,
// margin checks, live market data (REST + WebSocket), historical candles,
// and instrument-master CSV parsing.
//
// The package is used by the Trader Gateway (GrowwTrader backend) and the
// Quantitative Scout (WebSocket market-data connector).  Neither analyst
// package is modified; they import this library.
package groww

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// Exchange represents a stock exchange.
type Exchange string

const (
	ExchangeNSE Exchange = "NSE"
	ExchangeBSE Exchange = "BSE"
	ExchangeMCX Exchange = "MCX"
)

// Segment represents a trading segment.
type Segment string

const (
	SegmentCash      Segment = "CASH"
	SegmentFNO       Segment = "FNO"
	SegmentCommodity Segment = "COMMODITY"
)

// Product represents a product type for an order.
// Only MIS (intraday) is supported.
type Product string

const (
	ProductMIS Product = "MIS"
)

// OrderType represents the type of an order.
type OrderType string

const (
	OrderTypeLimit       OrderType = "LIMIT"
	OrderTypeMarket      OrderType = "MARKET"
	OrderTypeStopLoss    OrderType = "SL"
	OrderTypeStopLossMkt OrderType = "SL_M"
)

// TransactionType represents buy or sell.
type TransactionType string

const (
	TransactionTypeBuy  TransactionType = "BUY"
	TransactionTypeSell TransactionType = "SELL"
)

// OrderStatus represents the lifecycle status of an order.
type OrderStatus string

const (
	OrderStatusNew             OrderStatus = "NEW"
	OrderStatusAcked           OrderStatus = "ACKED"
	OrderStatusOpen            OrderStatus = "OPEN"
	OrderStatusTriggerPending  OrderStatus = "TRIGGER_PENDING"
	OrderStatusApproved        OrderStatus = "APPROVED"
	OrderStatusRejected        OrderStatus = "REJECTED"
	OrderStatusFailed          OrderStatus = "FAILED"
	OrderStatusExecuted        OrderStatus = "EXECUTED"
	OrderStatusDeliveryAwaited OrderStatus = "DELIVERY_AWAITED"
	OrderStatusCancelled       OrderStatus = "CANCELLED"
	OrderStatusCancellationReq OrderStatus = "CANCELLATION_REQUESTED"
	OrderStatusModificationReq OrderStatus = "MODIFICATION_REQUESTED"
	OrderStatusCompleted       OrderStatus = "COMPLETED"
)

// InstrumentType represents the type of an instrument in the CSV master.
type InstrumentType string

const (
	InstrumentTypeEQ  InstrumentType = "EQ"
	InstrumentTypeIDX InstrumentType = "IDX"
	InstrumentTypeFUT InstrumentType = "FUT"
	InstrumentTypeCE  InstrumentType = "CE"
	InstrumentTypePE  InstrumentType = "PE"
)

// CandleInterval represents a candle aggregation interval in minutes,
// as expected by the Groww /historical/candle/range endpoint.
type CandleInterval string

const (
	CandleInterval1Min  CandleInterval = "1"
	CandleInterval5Min  CandleInterval = "5"
	CandleInterval10Min CandleInterval = "10"
	CandleInterval15Min CandleInterval = "15"
	CandleInterval30Min CandleInterval = "30"
	CandleInterval1Hr   CandleInterval = "60"
	CandleInterval4Hr   CandleInterval = "240"
	CandleInterval1Day  CandleInterval = "1440"
	CandleInterval1Week CandleInterval = "10080"
)

// ---------------------------------------------------------------------------
// API status constants used in the standard Groww JSON envelope.
const (
	APIStatusSuccess = "SUCCESS"
	APIStatusFailure = "FAILURE"
)

// ---------------------------------------------------------------------------
// Orders
// ---------------------------------------------------------------------------

// PlaceOrderRequest is the body for POST /v1/order/create.
// Product is always MIS, validity is always DAY — set internally.
type PlaceOrderRequest struct {
	TradingSymbol    string          `json:"trading_symbol"`
	Quantity         int             `json:"quantity"`
	Price            float64         `json:"price,omitempty"`
	TriggerPrice     float64         `json:"trigger_price,omitempty"`
	Exchange         Exchange        `json:"exchange"`
	Segment          Segment         `json:"segment"`
	OrderType        OrderType       `json:"order_type"`
	TransactionType  TransactionType `json:"transaction_type"`
	OrderReferenceID string          `json:"order_reference_id"`
}

// PlaceOrderResponse is returned by POST /v1/order/create.
type PlaceOrderResponse struct {
	GrowwOrderID     string      `json:"groww_order_id"`
	OrderStatus      OrderStatus `json:"order_status"`
	OrderReferenceID string      `json:"order_reference_id"`
	Remark           string      `json:"remark"`
}

// ModifyOrderRequest is the body for POST /v1/order/modify.
type ModifyOrderRequest struct {
	GrowwOrderID string    `json:"groww_order_id"`
	Quantity     int       `json:"quantity,omitempty"`
	Price        float64   `json:"price,omitempty"`
	TriggerPrice float64   `json:"trigger_price,omitempty"`
	OrderType    OrderType `json:"order_type"`
	Segment      Segment   `json:"segment"`
}

// ModifyOrderResponse is returned by POST /v1/order/modify.
type ModifyOrderResponse struct {
	GrowwOrderID string      `json:"groww_order_id"`
	OrderStatus  OrderStatus `json:"order_status"`
}

// CancelOrderRequest is the body for POST /v1/order/cancel.
type CancelOrderRequest struct {
	Segment      Segment `json:"segment"`
	GrowwOrderID string  `json:"groww_order_id"`
}

// CancelOrderResponse is returned by POST /v1/order/cancel.
type CancelOrderResponse struct {
	GrowwOrderID string      `json:"groww_order_id"`
	OrderStatus  OrderStatus `json:"order_status"`
}

// OrderStatusResponse is returned by GET /v1/order/status/{id}.
type OrderStatusResponse struct {
	GrowwOrderID     string      `json:"groww_order_id"`
	OrderStatus      OrderStatus `json:"order_status"`
	Remark           string      `json:"remark"`
	FilledQuantity   int         `json:"filled_quantity"`
	OrderReferenceID string      `json:"order_reference_id"`
}

// OrderDetail is the full detail for a single order.
type OrderDetail struct {
	GrowwOrderID     string          `json:"groww_order_id"`
	TradingSymbol    string          `json:"trading_symbol"`
	OrderStatus      OrderStatus     `json:"order_status"`
	Remark           string          `json:"remark"`
	Quantity         int             `json:"quantity"`
	Price            float64         `json:"price"`
	TriggerPrice     float64         `json:"trigger_price"`
	FilledQuantity   int             `json:"filled_quantity"`
	RemainingQty     int             `json:"remaining_quantity"`
	AverageFillPrice float64         `json:"average_fill_price"`
	DeliverableQty   int             `json:"deliverable_quantity"`
	AMOStatus        string          `json:"amo_status"`
	Exchange         Exchange        `json:"exchange"`
	OrderType        OrderType       `json:"order_type"`
	TransactionType  TransactionType `json:"transaction_type"`
	Segment          Segment         `json:"segment"`
	Product          Product         `json:"product"`
	CreatedAt        string          `json:"created_at"`
	ExchangeTime     string          `json:"exchange_time"`
	TradeDate        string          `json:"trade_date"`
	OrderReferenceID string          `json:"order_reference_id"`
}

// Trade represents a single execution trade within an order.
type Trade struct {
	Price           float64         `json:"price"`
	ISIN            string          `json:"isin"`
	Quantity        int             `json:"quantity"`
	GrowwOrderID    string          `json:"groww_order_id"`
	GrowwTradeID    string          `json:"groww_trade_id"`
	ExchangeTradeID string          `json:"exchange_trade_id"`
	ExchangeOrderID string          `json:"exchange_order_id"`
	TradeStatus     string          `json:"trade_status"`
	TradingSymbol   string          `json:"trading_symbol"`
	Remark          string          `json:"remark"`
	Exchange        Exchange        `json:"exchange"`
	Segment         Segment         `json:"segment"`
	Product         Product         `json:"product"`
	TransactionType TransactionType `json:"transaction_type"`
	CreatedAt       string          `json:"created_at"`
	TradeDateTime   string          `json:"trade_date_time"`
	SettlementNum   string          `json:"settlement_number"`
}

// OrderListEntry is a single entry in the order list response.
type OrderListEntry struct {
	OrderDetail
}

// ---------------------------------------------------------------------------
// Margin
// ---------------------------------------------------------------------------

// MarginResponse is returned by GET /v1/margins/detail/user.
type MarginResponse struct {
	ClearCash           float64              `json:"clear_cash"`
	NetMarginUsed       float64              `json:"net_margin_used"`
	BrokerageAndCharges float64              `json:"brokerage_and_charges"`
	CollateralUsed      float64              `json:"collateral_used"`
	CollateralAvailable float64              `json:"collateral_available"`
	AdhocMargin         float64              `json:"adhoc_margin"`
	FNOMarginDetails    *FNOMarginDetails    `json:"fno_margin_details,omitempty"`
	EquityMarginDetails *EquityMarginDetails `json:"equity_margin_details,omitempty"`
}

// FNOMarginDetails holds FNO-specific margin fields.
type FNOMarginDetails struct {
	NetFNOMarginUsed       float64 `json:"net_fno_margin_used"`
	SpanMarginUsed         float64 `json:"span_margin_used"`
	ExposureMarginUsed     float64 `json:"exposure_margin_used"`
	FutureBalanceAvail     float64 `json:"future_balance_available"`
	OptionBuyBalanceAvail  float64 `json:"option_buy_balance_available"`
	OptionSellBalanceAvail float64 `json:"option_sell_balance_available"`
}

// EquityMarginDetails holds equity-specific margin fields for MIS trading.
type EquityMarginDetails struct {
	NetEquityMarginUsed float64 `json:"net_equity_margin_used"`
	MISMarginUsed       float64 `json:"mis_margin_used"`
	MISBalanceAvail     float64 `json:"mis_balance_available"`
}

// MarginCalcRequest is the body for POST /v1/margins/detail/orders.
// Product is always MIS — set internally.
type MarginCalcRequest struct {
	TradingSymbol   string          `json:"trading_symbol"`
	TransactionType TransactionType `json:"transaction_type"`
	Quantity        int             `json:"quantity"`
	Price           float64         `json:"price,omitempty"`
	OrderType       OrderType       `json:"order_type"`
	Exchange        Exchange        `json:"exchange"`
}

// MarginCalcResponse is returned by POST /v1/margins/detail/orders.
type MarginCalcResponse struct {
	ExposureRequired      float64 `json:"exposure_required"`
	SpanRequired          float64 `json:"span_required"`
	OptionBuyPremium      float64 `json:"option_buy_premium"`
	BrokerageAndCharges   float64 `json:"brokerage_and_charges"`
	TotalRequirement      float64 `json:"total_requirement"`
	CashMISMarginRequired float64 `json:"cash_mis_margin_required"`
}

// ---------------------------------------------------------------------------
// Live data
// ---------------------------------------------------------------------------

// Quote is returned by GET /v1/live-data/quote.
type Quote struct {
	AveragePrice      float64      `json:"average_price"`
	BidQuantity       int          `json:"bid_quantity"`
	BidPrice          float64      `json:"bid_price"`
	DayChange         float64      `json:"day_change"`
	DayChangePerc     float64      `json:"day_change_perc"`
	UpperCircuitLimit float64      `json:"upper_circuit_limit"`
	LowerCircuitLimit float64      `json:"lower_circuit_limit"`
	OHLC              OHLC         `json:"ohlc"`
	Depth             *MarketDepth `json:"depth,omitempty"`
	HighTradeRange    float64      `json:"high_trade_range"`
	ImpliedVolatility float64      `json:"implied_volatility,omitempty"`
	LastTradeQuantity int          `json:"last_trade_quantity"`
	LastTradeTime     int64        `json:"last_trade_time"`
	LowTradeRange     float64      `json:"low_trade_range"`
	LastPrice         float64      `json:"last_price"`
	MarketCap         float64      `json:"market_cap"`
	OfferPrice        float64      `json:"offer_price"`
	OfferQuantity     int          `json:"offer_quantity"`
	OIDayChange       float64      `json:"oi_day_change,omitempty"`
	OIDayChangePerc   float64      `json:"oi_day_change_percentage,omitempty"`
	OpenInterest      float64      `json:"open_interest,omitempty"`
	PrevOpenInterest  float64      `json:"previous_open_interest,omitempty"`
	TotalBuyQuantity  float64      `json:"total_buy_quantity"`
	TotalSellQuantity float64      `json:"total_sell_quantity"`
	Volume            int          `json:"volume"`
	Week52High        float64      `json:"week_52_high"`
	Week52Low         float64      `json:"week_52_low"`
}

// MarketDepth holds the buy and sell book entries.
type MarketDepth struct {
	Buy  []DepthEntry `json:"buy"`
	Sell []DepthEntry `json:"sell"`
}

// DepthEntry is a single price level in the market depth.
type DepthEntry struct {
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

// OHLC holds open, high, low, close prices.
type OHLC struct {
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

// LTPResponse maps exchange_symbol → last traded price.
type LTPResponse map[string]float64

// OHLCResponse maps exchange_symbol → OHLC.
type OHLCResponse map[string]OHLC

// ---------------------------------------------------------------------------
// Historical data
// ---------------------------------------------------------------------------

// HistoricalCandle is a single candle from the historical data API.
type HistoricalCandle struct {
	Timestamp int64 // epoch seconds
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    int
}

// ---------------------------------------------------------------------------
// Instrument master
// ---------------------------------------------------------------------------

// Instrument is a single row from the instrument CSV.
type Instrument struct {
	Exchange                Exchange       `csv:"exchange"`
	ExchangeToken           string         `csv:"exchange_token"`
	TradingSymbol           string         `csv:"trading_symbol"`
	GrowwSymbol             string         `csv:"groww_symbol"`
	Name                    string         `csv:"name"`
	InstrumentType          InstrumentType `csv:"instrument_type"`
	Segment                 Segment        `csv:"segment"`
	Series                  string         `csv:"series"`
	ISIN                    string         `csv:"isin"`
	UnderlyingSymbol        string         `csv:"underlying_symbol"`
	UnderlyingExchangeToken string         `csv:"underlying_exchange_token"`
	ExpiryDate              string         `csv:"expiry_date"`
	StrikePrice             int            `csv:"strike_price"`
	LotSize                 int            `csv:"lot_size"`
	TickSize                float64        `csv:"tick_size"`
	FreezeQuantity          int            `csv:"freeze_quantity"`
	IsReserved              bool           `csv:"is_reserved"`
	BuyAllowed              bool           `csv:"buy_allowed"`
	SellAllowed             bool           `csv:"sell_allowed"`
}

// ---------------------------------------------------------------------------
// WebSocket feed
// ---------------------------------------------------------------------------

// FeedInstrument identifies an instrument for WebSocket subscription.
type FeedInstrument struct {
	Exchange      Exchange `json:"exchange"`
	Segment       Segment  `json:"segment"`
	ExchangeToken string   `json:"exchange_token"`
}

// FeedMetadata is passed to feed callbacks to identify the data source.
type FeedMetadata struct {
	Exchange Exchange `json:"exchange"`
	Segment  Segment  `json:"segment"`
	FeedType string   `json:"feed_type"`
	FeedKey  string   `json:"feed_key"`
}

// LTPData is the live LTP value from the WebSocket feed.
type LTPData struct {
	TsInMillis int64   `json:"tsInMillis"`
	LTP        float64 `json:"ltp"`
}

// IndexData is the live index value from the WebSocket feed.
type IndexData struct {
	TsInMillis int64   `json:"tsInMillis"`
	Value      float64 `json:"value"`
}

// MarketDepthData is the live depth from the WebSocket feed.
type MarketDepthData struct {
	TsInMillis int64              `json:"tsInMillis"`
	BuyBook    map[int]DepthEntry `json:"buyBook,omitempty"`
	SellBook   map[int]DepthEntry `json:"sellBook,omitempty"`
}

// OrderUpdate represents a real-time order status change from the feed.
type OrderUpdate struct {
	Quantity      int         `json:"qty"`
	Price         string      `json:"price,omitempty"`
	FilledQty     int         `json:"filledQty"`
	AvgFillPrice  string      `json:"avgFillPrice"`
	GrowwOrderID  string      `json:"growwOrderId"`
	ExchangeOrdID string      `json:"exchangeOrderId"`
	OrderStatus   OrderStatus `json:"orderStatus"`
	Exchange      Exchange    `json:"exchange"`
	Segment       Segment     `json:"segment"`
	Product       Product     `json:"product,omitempty"`
	ContractID    string      `json:"contractId"`
}

// PositionUpdate represents a real-time position change from the feed.
type PositionUpdate struct {
	SymbolISIN       string                   `json:"symbolIsin"`
	ExchangePosition map[Exchange]ExchangePos `json:"exchangePosition"`
}

// ExchangePos holds credit/debit quantities for one exchange.
type ExchangePos struct {
	CreditQty   float64 `json:"creditQty"`
	CreditPrice float64 `json:"creditPrice"`
	DebitQty    float64 `json:"debitQty"`
	DebitPrice  float64 `json:"debitPrice"`
}
