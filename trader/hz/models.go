package hz

type account struct {
	AccountID       string `json:"accountId"`
	Currency        string `json:"currency"`
	Balance         string `json:"balance"`
	Equity          string `json:"equity"`
	AvailableMargin string `json:"availableMargin"`
	UsedMargin      string `json:"usedMargin"`
	MarginRatio     string `json:"marginRatio"`
	UnrealizedPnL   string `json:"unrealizedPnl"`
	Tradable        bool   `json:"tradable"`
}

type position struct {
	PositionID    string  `json:"positionId"`
	Instrument    string  `json:"instrument"`
	Side          string  `json:"side"`
	MarginMode    string  `json:"marginMode"`
	Leverage      int     `json:"leverage"`
	Lots          string  `json:"lots"`
	EntryPrice    string  `json:"entryPrice"`
	CurrentPrice  string  `json:"currentPrice"`
	PositionValue string  `json:"positionValue"`
	InitialMargin string  `json:"initialMargin"`
	UnrealizedPnL string  `json:"unrealizedPnl"`
	TakeProfit    *string `json:"takeProfit"`
	StopLoss      *string `json:"stopLoss"`
	OpenedAt      string  `json:"openedAt"`
}

type order struct {
	OrderID       string  `json:"orderId"`
	ClientOrderID string  `json:"clientOrderId"`
	PositionID    *string `json:"positionId"`
	Instrument    string  `json:"instrument"`
	Side          string  `json:"side"`
	OrderType     string  `json:"orderType"`
	MarginMode    string  `json:"marginMode"`
	Leverage      int     `json:"leverage"`
	Lots          string  `json:"lots"`
	LimitPrice    *string `json:"limitPrice"`
	TriggerPrice  *string `json:"triggerPrice"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

type quote struct {
	Instrument   string `json:"instrument"`
	Bid          string `json:"bid"`
	Ask          string `json:"ask"`
	Reference    string `json:"reference"`
	SourceStatus string `json:"sourceStatus"`
	ReceivedAt   string `json:"receivedAt"`
}

type instrument struct {
	Instrument   string `json:"instrument"`
	LotPrecision int    `json:"lotPrecision"`
	LotStep      string `json:"lotStep"`
	Tradable     bool   `json:"tradable"`
}

type trade struct {
	TradeID     string `json:"tradeId"`
	OrderID     string `json:"orderId"`
	Instrument  string `json:"instrument"`
	Side        string `json:"side"`
	Lots        string `json:"lots"`
	Price       string `json:"price"`
	Fee         string `json:"fee"`
	RealizedPnL string `json:"realizedPnl"`
	ExecutedAt  string `json:"executedAt"`
}

type orderPage struct {
	Items []order `json:"items"`
}

type tradePage struct {
	Items []trade `json:"items"`
}

type createOrderRequest struct {
	ClientOrderID string  `json:"clientOrderId"`
	Instrument    string  `json:"instrument"`
	Side          string  `json:"side"`
	OrderType     string  `json:"orderType"`
	SizeMode      string  `json:"sizeMode"`
	Size          string  `json:"size"`
	Leverage      int     `json:"leverage"`
	MarginMode    string  `json:"marginMode"`
	LimitPrice    *string `json:"limitPrice,omitempty"`
	TriggerPrice  *string `json:"triggerPrice,omitempty"`
	TakeProfit    *string `json:"takeProfit,omitempty"`
	StopLoss      *string `json:"stopLoss,omitempty"`
}

type positionAction struct {
	ClosedPosition position  `json:"closedPosition"`
	OpenedPosition *position `json:"openedPosition"`
}
