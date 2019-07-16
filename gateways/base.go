package gateways

import (
	. "github.com/BurdenBear/gladius"
)

type IGateway interface {
	GetName() string
	GetEngine() *EventEngine
	Connect() error
	CancelOrder(symbol string, orderID string)
	GetCandles(symbol string, period, size int) ([]Bar, error)
	PlaceOrder(symbol string, price, amount string, orderType OrderType, offset OrderOffset, leverRate int) (string, error)
}

type BaseGateway struct {
	name   string
	engine *EventEngine
}

func NewBaseGateway(name string) *BaseGateway {
	return (&BaseGateway{}).Name(name)
}

func (gateway *BaseGateway) Name(name string) *BaseGateway {
	gateway.name = name
	return gateway
}

func (gateway *BaseGateway) Engine(engine *EventEngine) *BaseGateway {
	gateway.engine = engine
	return gateway
}

func (gateway *BaseGateway) GetName() string {
	return gateway.name
}

func (gateway *BaseGateway) GetEngine() *EventEngine {
	return gateway.engine
}

func (gateway *BaseGateway) OnOrder(order *Order) {
	gateway.engine.ProcessData(EVENT_ORDER, order)
}

func (gateway *BaseGateway) OnTicker(ticker *Ticker) {
	gateway.engine.ProcessData(EVENT_TICKER, ticker)
}

func (gateway *BaseGateway) OnDepth(depth *Depth) {
	gateway.engine.ProcessData(EVENT_DEPTH, depth)
}
