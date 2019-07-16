package gateways

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	. "github.com/BurdenBear/gladius"
	"github.com/deckarep/golang-set"
	"github.com/google/uuid"
	"github.com/imdario/mergo"
	goex "github.com/nntaoli-project/GoEx"
)

var logger = GetLogger()

type contractsMapKey struct {
	CurrencyPair goex.CurrencyPair
	ContractType string
}

func NewGoExGateway(name string, engine *EventEngine, apiFactory GoExAPIFactory, config interface{}) (*GoExGateway, error) {
	gateway := &GoExGateway{}
	gateway.BaseGateway = NewBaseGateway(name).Engine(engine)
	gateway.apiFactory = apiFactory
	gateway.unfinishedClOrders = make(map[string]*Order)
	gateway.uncertainedOrdersCache = make(map[string][]*OrderCacheData)
	gateway.ordCancelReqsCache = make(map[string]*OrdCancelCacheData)
	gateway.finishedClOrders = mapset.NewSet()
	gateway.finishedOrders = mapset.NewSet()
	gateway.orderID2ClOrdID = make(map[string]string)
	gateway.clOrdID2OrderID = make(map[string]string)
	num := 10000
	gateway.orderCh = make(chan *Order, num)
	gateway.ordersCh = make(chan []*Order, num)
	gateway.cancelCh = make(chan *GoExOrdCancelReq, num)
	gateway.queryCh = make(chan *CryptoCurrencyContract, num)
	gateway.cleanCh = make(chan bool, num)
	go gateway.startHandle()
	err := gateway.apiFactory.Config(gateway, config)
	if err != nil {
		logger.Error(err.Error())
		return nil, err
	}
	return gateway, nil
}

type GoExAPIFactory interface {
	Config(*GoExGateway, interface{}) error
	// GetSpotAPI() goex.API
	// GetSpotWs()
	GetFutureAPI() (ExtendedFutureRestAPI, error)
	GetFutureWs() (FutureWebsocket, error)
	GetFutureAuthoried() bool
}

type GoExGatewayUrlConfig struct {
	Restful   string
	Websocket string
}

type GoExGatewayUrlsConfig struct {
	Future GoExGatewayUrlConfig
	Spot   GoExGatewayUrlConfig
}

type GoExGatewaySecretConfig struct {
	APIKey       string `json:"apiKey"`
	APISecretKey string `json:"apiSecretKey"`
	Passphrase   string `json:"passphrase"`
}

type GoExGatewayHTTPConfig struct {
	Timeout int    `json:"timeout"`
	Proxy   string `json:"proxy"`
}

type GoExGatewayConfig struct {
	HTTP    GoExGatewayHTTPConfig `json:"http"`
	Secret  GoExGatewaySecretConfig
	Urls    GoExGatewayUrlsConfig
	Symbols []string
}

type GoExOrdCancelReq struct {
	Symbol  string
	ClOrdID string
}

type OrderCacheData struct {
	Order      *Order
	UpdateTime time.Time
}

type OrdCancelCacheData struct {
	Req        *GoExOrdCancelReq
	UpdateTime time.Time
}

type GoExGateway struct {
	//basic
	*BaseGateway
	apiFactory    GoExAPIFactory
	spotRestAPI   goex.API
	futureRestAPI ExtendedFutureRestAPI
	futureWs      FutureWebsocket
	connected     bool

	//orders
	orderQueryInterval     time.Duration
	finishedClOrders       mapset.Set                     // key is ClOrdID
	finishedOrders         mapset.Set                     // key is OrderID
	unfinishedClOrders     map[string]*Order              // key is ClOrdID
	uncertainedOrdersCache map[string][]*OrderCacheData   // key is OrderID
	ordCancelReqsCache     map[string]*OrdCancelCacheData // key is ClOrdID
	orderID2ClOrdID        map[string]string              // key: OrderID, value: ClOrdID
	clOrdID2OrderID        map[string]string              // key: ClOrdID, value: OrderID
	orderCh                chan *Order
	ordersCh               chan []*Order
	queryCh                chan *CryptoCurrencyContract
	cancelCh               chan *GoExOrdCancelReq
	cleanCh                chan bool

	//symbols
	symbols      []string
	contracts    []*CryptoCurrencyContract
	contractsMap map[contractsMapKey]*CryptoCurrencyContract
}

func (gateway *GoExGateway) GetExchange() string {
	return "GoEx"
}

func (gateway *GoExGateway) parseContract(symbol string) (*CryptoCurrencyContract, error) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		return nil, err
	}
	contract := NewCryptoCurrencyContract(
		gateway.GetName(),
		gateway.GetExchange(),
		currencyPair,
		contractType,
	)
	return contract, nil
}

func (gateway *GoExGateway) SetSymbols(symbols []string) *GoExGateway {
	if symbols == nil {
		symbols = make([]string, 0)
	}
	gateway.symbols = symbols
	gateway.contracts = make([]*CryptoCurrencyContract, 0, len(symbols))
	gateway.contractsMap = make(map[contractsMapKey]*CryptoCurrencyContract)
	for _, s := range symbols {
		contract, err := gateway.parseContract(s)
		if err != nil {
			logger.Error(err.Error())
			continue
		}
		gateway.contracts = append(gateway.contracts, contract)
		key := contractsMapKey{contract.CurrencyPair, contract.ContractType}
		gateway.contractsMap[key] = contract
	}
	return gateway
}

func (gateway *GoExGateway) SetOrderQueryInterval(t time.Duration) *GoExGateway {
	gateway.orderQueryInterval = t
	return gateway
}

func (gateway *GoExGateway) getUnfinishedOrderIDs(contract *CryptoCurrencyContract) []string {
	orderIDs := make([]string, 0, len(gateway.unfinishedClOrders))
	for _, o := range gateway.unfinishedClOrders {
		if o.OrderID != "" && o.Contract.GetID() == contract.GetID() {
			orderIDs = append(orderIDs, o.OrderID)
		}
	}
	return orderIDs
}

func (gateway *GoExGateway) queryContractOrders(contract *CryptoCurrencyContract, orderIDs []string) {
	futureOrders, err := gateway.futureRestAPI.GetFutureOrders(orderIDs, contract.CurrencyPair, contract.ContractType)
	if err != nil {
		logger.Warningf("error when query order: %s", err)
		return
	}
	orders := make([]*Order, 0, len(futureOrders))
	for _, o := range futureOrders {
		order := AdapterFutureOrder(contract, &o)
		orders = append(orders, order)
	}
	gateway.ordersCh <- orders
}

func (gateway *GoExGateway) startQueryOrders() {
	for {
		for _, c := range gateway.contracts {
			gateway.queryCh <- c
			time.Sleep(gateway.orderQueryInterval)
		}
	}
}

func (gateway *GoExGateway) startClean() {
	for {
		gateway.cleanCh <- true
		time.Sleep(3 * time.Minute)
	}
}

func (gateway *GoExGateway) cleanUncertainedOrdersCache() {
	now := time.Now().UTC()
	for k, ordersCache := range gateway.uncertainedOrdersCache {
		if ordersCache == nil || now.Sub(ordersCache[len(ordersCache)-1].UpdateTime).Seconds() > 5*60 { // 5min
			delete(gateway.uncertainedOrdersCache, k)
		}
	}
}

func (gateway *GoExGateway) cleanOrdCancelReqsCache() {
	now := time.Now().UTC()
	for k, cache := range gateway.ordCancelReqsCache {
		if now.Sub(cache.UpdateTime).Seconds() > 5*60 { // 5min
			delete(gateway.ordCancelReqsCache, k)
		}
	}
}

var orderStatusStageMap = map[OrderStatus]int{
	OS_SUBMITTED:    0,
	OS_UNFILLED:     1,
	OS_PART_FILLED:  2,
	OS_REJECTED:     3,
	OS_CANCELLING:   4,
	OS_CANCELED:     5,
	OS_FULLY_FILLED: 6,
}

var eps = 1e-8

func getStage(os OrderStatus) int {
	if v, ok := orderStatusStageMap[os]; ok {
		return v
	}
	return -1
}

func mergeOrder(dst *Order, src *Order, opts ...func(*mergo.Config)) {
	mergo.Merge(dst, *src, opts...)
	if dst.CreateTime.IsZero() {
		dst.CreateTime = src.CreateTime
	}
}

func (gateway *GoExGateway) handleOrder(order *Order) *Order {
	// with ClOrdID in raw data.
	if order.ClOrdID != "" {
		if gateway.finishedClOrders.Contains(order.ClOrdID) {
			return nil // already finished order
		}
		if o, ok := gateway.unfinishedClOrders[order.ClOrdID]; ok {
			if order.DealAmount < o.DealAmount {
				return nil // misordered order event
			} else if math.Abs(order.DealAmount-o.DealAmount) <= eps {
				// do not skip OS_SUBMITTED which generated locally and ordered properly
				addOrderID := order.OrderID != "" && o.OrderID == ""
				if !addOrderID && getStage(order.Status) <= getStage(o.Status) {
					return nil // misordered order event
				}
			}
			mergeOrder(order, o) // do not override
		} else {
			if order.Status != OS_SUBMITTED {
				return nil // order sent by other client
			}
		}
		if order.OrderID != "" {
			if _, ok := gateway.orderID2ClOrdID[order.OrderID]; !ok {
				// first map orderID to clOrdID
				gateway.orderID2ClOrdID[order.OrderID] = order.ClOrdID
				gateway.clOrdID2OrderID[order.ClOrdID] = order.OrderID
				if data, ok := gateway.ordCancelReqsCache[order.ClOrdID]; ok {
					go gateway.doCancelOrder(data.Req.Symbol, order.OrderID, data.Req.ClOrdID)
					delete(gateway.ordCancelReqsCache, order.ClOrdID)
				}
				// handle cached uncertained orders
				// replay them with certained ClOrdID now.
				old := gateway.unfinishedClOrders[order.ClOrdID]
				if prevOrders, ok := gateway.uncertainedOrdersCache[order.OrderID]; ok {
					os := make([]*Order, 0, len(prevOrders))
					for _, orderCache := range prevOrders {
						o := orderCache.Order
						mergeOrder(o, old)
						os = append(os, o)
					}
					delete(gateway.uncertainedOrdersCache, order.OrderID)
					go func() {
						gateway.ordersCh <- os
					}() // do in other thread to prevent dead lock.
				}
			}
		}
		if order.IsFinished() {
			if _, ok := gateway.unfinishedClOrders[order.ClOrdID]; ok {
				delete(gateway.unfinishedClOrders, order.ClOrdID)
			}
			if _, ok := gateway.clOrdID2OrderID[order.ClOrdID]; ok {
				delete(gateway.clOrdID2OrderID, order.ClOrdID)
			} // delete ClOrdID to OrderID map
			gateway.finishedClOrders.Add(order.ClOrdID)
			go func(clOrdID string) {
				time.Sleep(5 * time.Minute) // keep 5 minute to prevent readd misordered order to unfinishedClOrders
				gateway.finishedClOrders.Remove(clOrdID)
			}(order.ClOrdID)
			if order.OrderID != "" {
				if _, ok := gateway.orderID2ClOrdID[order.OrderID]; ok {
					delete(gateway.orderID2ClOrdID, order.OrderID)
				} // delete OrderID to ClOrdID map
				gateway.finishedOrders.Add(order.OrderID)
				go func(orderID string) {
					time.Sleep(5 * time.Minute) // keep 5 minute to prevent readd misordered order to uncertainedOrdersCache
					gateway.finishedOrders.Remove(orderID)
				}(order.OrderID)
			}
		} else {
			gateway.unfinishedClOrders[order.ClOrdID] = order
		}
		if order.OrderID != "" {
			return order
		}
		return nil
	}
	// without ClOrdID in raw data.
	if gateway.finishedOrders.Contains(order.OrderID) {
		return nil // already finished order
	}
	if clOrdID, ok := gateway.orderID2ClOrdID[order.OrderID]; ok {
		order.ClOrdID = clOrdID
		return gateway.handleOrder(order) // recursively
	}
	orderCache := new(OrderCacheData)
	orderCache.Order = order
	orderCache.UpdateTime = time.Now().UTC()
	var ordersCache []*OrderCacheData
	if c, ok := gateway.uncertainedOrdersCache[order.OrderID]; ok {
		ordersCache = c
	} else {
		ordersCache = make([]*OrderCacheData, 0)
	}
	ordersCache = append(ordersCache, orderCache)
	gateway.uncertainedOrdersCache[order.OrderID] = ordersCache
	return nil
}

func (gateway *GoExGateway) startHandle() {
	for {
		select {
		case order := <-gateway.orderCh:
			if o := gateway.handleOrder(order); o != nil {
				gateway.BaseGateway.OnOrder(order)
			}
		case orders := <-gateway.ordersCh:
			for _, order := range orders {
				if o := gateway.handleOrder(order); o != nil {
					gateway.BaseGateway.OnOrder(order)
				}
			}
		case contract := <-gateway.queryCh:
			orderIDs := gateway.getUnfinishedOrderIDs(contract)
			if len(orderIDs) > 0 {
				go gateway.queryContractOrders(contract, orderIDs)
			}
		case req := <-gateway.cancelCh:
			if orderID, ok := gateway.clOrdID2OrderID[req.ClOrdID]; ok {
				go gateway.doCancelOrder(req.Symbol, orderID, req.ClOrdID)
			} else {
				logger.Warningf("Order{Symbol: %s, ClOrdID: %s} not found, cache it for cancel laterly", req.Symbol, req.ClOrdID)
				data := &OrdCancelCacheData{Req: req, UpdateTime: time.Now().UTC()}
				gateway.ordCancelReqsCache[req.ClOrdID] = data
			}
		case _ = <-gateway.cleanCh:
			gateway.cleanUncertainedOrdersCache()
			gateway.cleanOrdCancelReqsCache()
		}
	}
}

func (gateway *GoExGateway) onWsOrder(order *goex.FutureOrder, contractType string) {
	contract, err := gateway.GetContract(order.Currency, contractType)
	if err != nil {
		logger.Errorf("%s", err)
		return // unsubscribed depth
	}
	o := AdapterFutureOrder(contract, order)
	gateway.orderCh <- o
}

func (gateway *GoExGateway) GetContract(currencyPair goex.CurrencyPair, contractType string) (*CryptoCurrencyContract, error) {
	key := contractsMapKey{currencyPair, contractType}
	if c, ok := gateway.contractsMap[key]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("no contract found of %s %s", currencyPair.ToSymbol("_"), contractType)
}

func (gateway *GoExGateway) onWsDepth(depth *goex.Depth) {
	contract, err := gateway.GetContract(depth.Pair, depth.ContractType)
	if err != nil {
		return // unsubscribed depth
	}
	d := AdapterDepth(contract, depth)
	gateway.BaseGateway.OnDepth(d)
}

func (gateway *GoExGateway) subscribe() {
	gateway.futureWs.DepthCallback(gateway.onWsDepth)
	gateway.futureWs.OrderCallback(gateway.onWsOrder)
	for _, c := range gateway.contracts {
		gateway.futureWs.SubscribeDepth(c.CurrencyPair, c.ContractType, 5)
	}
	if gateway.apiFactory.GetFutureAuthoried() {
		for _, c := range gateway.contracts {
			gateway.futureWs.SubscribeOrder(c.CurrencyPair, c.ContractType)
		}
	}
}

func (gateway *GoExGateway) GetClOrdID() string {
	return strings.Replace(uuid.New().String(), "-", "", 32)
}

func (gateway *GoExGateway) doPlaceOrder(currencyPair goex.CurrencyPair, contractType string, order *Order) {
	var orderID string
	var err error
	price := fmt.Sprintf("%f", order.Price)
	amount := fmt.Sprintf("%f", order.Amount)
	openType := NewGoexOpenType(order.Offset)
	orderType := order.Type

	if orderType == OT_LIMIT {
		orderID, err = gateway.futureRestAPI.PlaceFutureOrder(currencyPair, contractType,
			price, amount, openType, 0, order.LeverRate)
	} else {
		orderID, err = gateway.futureRestAPI.PlaceFutureOrder2(currencyPair, contractType,
			price, amount, int(orderType), openType, 0, order.LeverRate)
	}
	newOrder := new(Order)
	mergeOrder(newOrder, order)
	newOrder.OrderID = orderID
	newOrder.UpdateTime = time.Now().UTC()
	if err != nil {
		newOrder.Status = OS_REJECTED
		newOrder.OrdRejReason = err.Error()
		logger.Errorf("Order(%s) rejected, reason: %s", newOrder.ClOrdID, err)
		gateway.orderCh <- newOrder
		return
	}
	logger.Infof("Order(%s) submitted, exchange orderID: %s", newOrder.ClOrdID, newOrder.OrderID)
	gateway.orderCh <- newOrder
}

func (gateway *GoExGateway) PlaceOrder(symbol string, price, amount string, orderType OrderType, offset OrderOffset, leverRate int) (string, error) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		logger.Errorf("%s", err)
		return "", err
	}
	contract, err := gateway.GetContract(currencyPair, contractType)
	if err != nil {
		logger.Errorf("%s", err)
		return "", err
	}
	order := new(Order)
	order.ClOrdID = gateway.GetClOrdID()
	order.Price, _ = strconv.ParseFloat(price, 64)
	order.Amount, _ = strconv.ParseFloat(amount, 64)
	order.CreateTime = time.Now().UTC()
	order.UpdateTime = order.CreateTime
	order.Status = OS_SUBMITTED
	order.Contract = contract
	order.Offset = offset
	order.Type = orderType
	order.LeverRate = leverRate
	gateway.orderCh <- order
	go gateway.doPlaceOrder(currencyPair, contractType, order)
	return order.ClOrdID, nil
}

func (gateway *GoExGateway) doCancelOrder(symbol string, orderID string, clOrdID string) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		logger.Errorf("cancel Order{Symbol: %s, OrderID: %s, ClOrdID: %s} failed: %s", symbol, orderID, clOrdID, err)
	}
	_, err = gateway.futureRestAPI.FutureCancelOrder(currencyPair, contractType, orderID)
	if err != nil {
		logger.Errorf("cancel Order{Symbol: %s, OrderID: %s, ClOrdID: %s} failed: %s", symbol, orderID, clOrdID, err)
	}
	logger.Infof("cancel Order{Symbol: %s, OrderID: %s, ClOrdID: %s} sucessed", symbol, orderID, clOrdID)
}

func (gateway *GoExGateway) CancelOrder(symbol string, orderID string) {
	req := new(GoExOrdCancelReq)
	req.Symbol = symbol
	req.ClOrdID = orderID
	gateway.cancelCh <- req
}

func (gateway *GoExGateway) GetCandles(symbol string, period, size int) ([]Bar, error) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		logger.Error(err.Error())
	}
	contract, err := gateway.GetContract(currencyPair, contractType)
	if err != nil {
		logger.Error(err.Error())
	}
	kline, err := gateway.futureRestAPI.GetKlineRecords(contractType, currencyPair, period, size, 0)
	if err != nil {
		return nil, err
	}
	return AdapterFutureKlines(contract, kline), nil
}

func (gateway *GoExGateway) Connect() error {
	// TODO: Once
	if gateway.connected {
		logger.Warningf("gateway (%s) has been connected, skip", gateway.GetName())
		return nil
	}

	var err error
	gateway.futureRestAPI, err = gateway.apiFactory.GetFutureAPI()
	if err != nil {
		return err
	}
	gateway.futureWs, err = gateway.apiFactory.GetFutureWs()
	if err != nil {
		return err
	}
	if gateway.apiFactory.GetFutureAuthoried() &&
		gateway.orderQueryInterval > 0 {
		go gateway.startQueryOrders()
	}
	go gateway.startClean()
	gateway.subscribe()
	gateway.connected = true
	return nil
}
