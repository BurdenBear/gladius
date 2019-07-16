package strategy

import (
	"fmt"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/gateways"
	"github.com/deckarep/golang-set"
)

var logger = GetLogger()

type Router struct {
	gateways          map[string]gateways.IGateway
	strategies        map[string]IStrategy
	orderStrategyMap  map[string]IStrategy
	symbolStrategyMap map[string]mapset.Set
	isRunning         bool
	ch                chan interface{}
}

type OrderReq struct {
	Symbol    string
	Price     float64
	Size      float64
	OrderType OrderType
	Offset    OrderOffset
	Leverage  int
}

func NewRouter(engine *EventEngine) *Router {
	router := new(Router)
	router.gateways = make(map[string]gateways.IGateway)
	router.strategies = make(map[string]IStrategy)
	router.orderStrategyMap = make(map[string]IStrategy)
	router.symbolStrategyMap = make(map[string]mapset.Set)
	router.ch = make(chan interface{}, engine.GetQueueSize())
	router.Start()
	router.Register(engine)
	return router
}

func (router *Router) Register(engine *EventEngine) {
	engine.Register(EVENT_ORDER, NewOrderEventHandler(router.OnOrder))
	engine.Register(EVENT_DEPTH, NewDepthEventHandler(router.OnDepth))
}

func (router *Router) AddGateway(gws ...gateways.IGateway) {
	router.ch <- gws
}

func (router *Router) AddStrategy(strategies ...IStrategy) {
	router.ch <- strategies
}

func (router *Router) OnDepth(depth *Depth) {
	router.ch <- depth
}

func (router *Router) OnOrder(order *Order) {
	router.ch <- order
}

func (router *Router) Start() {
	router.isRunning = true
	go router.handle()
}

func (router *Router) Stop() {
	router.isRunning = false
}

func (router *Router) IsRunning() bool {
	return router.isRunning
}

func (router *Router) handle() {
	for router.IsRunning() {
		t := <-router.ch
		switch data := t.(type) {
		case []gateways.IGateway:
			router.handleGateway(data)
		case []IStrategy:
			router.handleStrategy(data)
		case *Depth:
			router.handleDepth(data)
		case *Order:
			router.handleOrder(data)
		}
	}
}

func (router *Router) handleStrategy(strategies []IStrategy) {
	for _, strategy := range strategies {
		name := strategy.GetName()
		router.strategies[name] = strategy
		logger.Debugf("Strategy(%s) binded to router with symbols: %+v", name, strategy.GetSymbols())
		for _, symbolID := range strategy.GetSymbols() {
			if _, ok := router.symbolStrategyMap[symbolID]; !ok {
				router.symbolStrategyMap[symbolID] = mapset.NewThreadUnsafeSet()
			}
			router.symbolStrategyMap[symbolID].Add(strategy)
		}
	}
}

func (router *Router) handleGateway(gws []gateways.IGateway) {
	for _, gw := range gws {
		name := gw.GetName()
		router.gateways[name] = gw
		logger.Debugf("Strategy(%s) binded to router", name)
	}
}

func (router *Router) handleOrder(order *Order) {
	if strategy, ok := router.orderStrategyMap[order.OrderID]; ok {
		strategy.OnOrder(order)
		if order.IsFinished() {
			delete(router.orderStrategyMap, order.OrderID)
		}
	}
}

func (router *Router) handleDepth(depth *Depth) {
	symbolID := depth.Contract.GetID()
	if strategies, ok := router.symbolStrategyMap[symbolID]; ok {
		if strategies != nil {
			it := strategies.Iterator()
			for v := range it.C {
				stg := v.(IStrategy)
				if stg.IsRunning() {
					stg.OnDepth(depth)
				}
			}
		}
	}
}

func (router *Router) PlaceOrder(symbolID string, price, size float64, orderType OrderType, offset OrderOffset, l int) (string, error) {
	fallback := ""
	gatewayName, symbol, err := ParseContractID(symbolID)
	if err != nil {
		return fallback, err
	}
	if gateway, ok := router.gateways[gatewayName]; ok {
		priceS := fmt.Sprintf("%f", price)
		sizeS := fmt.Sprintf("%f", size)
		return gateway.PlaceOrder(symbol, priceS, sizeS, orderType, offset, l)
	}
	err = fmt.Errorf("Gateway(%s) in symbol(%s) is not found, place order failed", gatewayName, symbolID)
	logger.Warningf(err.Error())
	return fallback, err
}

// TODO: return error
func (router *Router) CancelOrder(symbolID string, clOrdID string) {
	gatewayName, symbol, err := ParseContractID(symbolID)
	if err != nil {
		logger.Errorf("%s is not valid symbol id, cancel order(%s) failed: %s", symbolID, clOrdID, err)
		return
	}
	if gateway, ok := router.gateways[gatewayName]; ok {
		gateway.CancelOrder(symbol, clOrdID)
		return
	}
	err = fmt.Errorf("Gateway(%s) in symbol(%s) is not found, cancel order(%s) failed", gatewayName, symbolID, clOrdID)
	logger.Warningf(err.Error())
}

func (router *Router) GetCandles(symbolID string, period, size int) ([]Bar, error) {
	var fallback []Bar
	gatewayName, symbol, err := ParseContractID(symbolID)
	if err != nil {
		return fallback, err
	}
	if gateway, ok := router.gateways[gatewayName]; ok {
		return gateway.GetCandles(symbol, period, size)
	}
	err = fmt.Errorf("Gateway(%s) in symbol(%s) is not found, get candles failed", gatewayName, symbolID)
	logger.Warningf(err.Error())
	return fallback, err
}
