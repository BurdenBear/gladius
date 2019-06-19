package gateways

import (
	. "github.com/BurdenBear/gladius"
)

type IGateway interface {
	OnOrder(Order)
	OnTicker(Ticker)
	OnDepth(Depth)
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
