package strategy

import (
	"fmt"
	"time"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/gateways"
	"github.com/deckarep/golang-set"
)

var logger = GetLogger()

type orderCacheData struct {
	Order     *Order
	Timestamp time.Time
}

type orderStrategyInfo struct {
	ClOrdID  string
	Strategy string
}

type Router struct {
	gateways          map[string]gateways.IGateway
	strategies        map[string]IStrategy
	orderStrategyMap  map[string]IStrategy
	orderCache        map[string][]*orderCacheData
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
	router.orderCache = make(map[string][]*orderCacheData)
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
	go router.startCleanOrderCache()
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
		case *orderStrategyInfo:
			router.handleOrderStrategyInfo(data)
		case time.Time:
			router.cleanOrderCache(data)
		}
	}
}

func (router *Router) cleanOrderCache(t time.Time) {
	for k, v := range router.orderCache {
		if v == nil || len(v) == 0 {
			delete(router.orderCache, k)
			continue
		}
		if t.Sub(v[len(v)-1].Timestamp).Seconds() > 5*60 { // clear cached order after 5min
			delete(router.orderCache, k)
		}
	}
}

func (router *Router) startCleanOrderCache() {
	for router.IsRunning() {
		router.ch <- time.Now().UTC()
		time.Sleep(2 * time.Minute)
	}
}

func (router *Router) handleOrderStrategyInfo(info *orderStrategyInfo) {
	if strategy, ok := router.strategies[info.Strategy]; !ok {
		logger.Infof("strategy %s send order %s but not found in router now", strategy, info.ClOrdID)
	} else {
		router.orderStrategyMap[info.ClOrdID] = strategy
		//replay the cached order
		if os, ok := router.orderCache[info.ClOrdID]; ok {
			go func() {
				for _, o := range os {
					router.ch <- o // replay and keep the order
				}
			}() // do in other thread to prevent deadlock
			delete(router.orderCache, info.ClOrdID)
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
	if strategy, ok := router.orderStrategyMap[order.ClOrdID]; ok {
		strategy.OnOrder(order)
		if order.IsFinished() {
			delete(router.orderStrategyMap, order.ClOrdID)
		}
	} else {
		cacheData := &orderCacheData{
			Order:     order,
			Timestamp: time.Now().UTC(),
		}
		router.orderCache[order.ClOrdID] = append(router.orderCache[order.ClOrdID], cacheData)
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

func (router *Router) PlaceOrder(strategy string, symbolID string, price, size float64, orderType OrderType, offset OrderOffset, l int) (string, error) {
	fallback := ""
	gatewayName, symbol, err := ParseContractID(symbolID)
	if err != nil {
		return fallback, err
	}
	if gateway, ok := router.gateways[gatewayName]; ok {
		priceS := fmt.Sprintf("%f", price)
		sizeS := fmt.Sprintf("%f", size)
		clOrdID, err := gateway.PlaceOrder(symbol, priceS, sizeS, orderType, offset, l)
		if err != nil {
			logger.Errorf("Error when place order: %s", err)
		} else {
			info := &orderStrategyInfo{ClOrdID: clOrdID, Strategy: strategy}
			go func() {
				router.ch <- info
			}() // do in other thread to prevent deadlock
		}
		return clOrdID, err
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
