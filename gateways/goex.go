package gateways

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	. "github.com/BurdenBear/gladius"
	"github.com/deckarep/golang-set"
	goex "github.com/nntaoli-project/GoEx"
	"github.com/nntaoli-project/GoEx/okcoin"
)

var log = GetLogger()

type contractsMapKey struct {
	CurrencyPair goex.CurrencyPair
	ContractType string
}

func NewGoExGateway(name string, engine *EventEngine, apiFactory GoExAPIFactory) *GoExGateway {
	gateway := &GoExGateway{}
	gateway.BaseGateway = NewBaseGateway(name).Engine(engine)
	gateway.apiFactory = apiFactory
	gateway.ordersRw = &sync.RWMutex{}
	gateway.unfinishedOrders = make(map[string]*Order)
	gateway.finishedOrders = mapset.NewSet()
	gateway.orderCh = make(chan *Order, 1000)
	gateway.ordersCh = make(chan []*Order, 1000)
	go gateway.startHandleOrders()
	return gateway
}

type GoExAPIFactory interface {
	Config(*GoExGateway, interface{}) error
	// GetSpotAPI() goex.API
	// GetSpotWs()
	GetFutureAPI() (ExtendedFutureRestAPI, error)
	GetFutureWs() (FutureWebsocket, error)
	GetFutureAuthoried() bool
}

type GoExGatewayConfig struct {
	HTTPTimeout   time.Duration
	HTTPProxy     string
	APIKey        string
	APISecretKey  string
	APIPassphrase string
	Symbols       []string
}

type GoExGateway struct {
	//basic
	*BaseGateway
	apiFactory    GoExAPIFactory
	spotRestAPI   goex.API
	futureRestAPI ExtendedFutureRestAPI
	futureWs      FutureWebsocket

	//orders
	orderQueryInterval time.Duration
	ordersRw           *sync.RWMutex
	finishedOrders     mapset.Set
	unfinishedOrders   map[string]*Order
	orderCh            chan *Order
	ordersCh           chan []*Order

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
		gateway.BaseGateway.GetName(),
		gateway.GetExchange(),
		currencyPair,
		contractType,
	)
	return contract, nil
}

func (gateway *GoExGateway) SetSymbols(symbols []string) *GoExGateway {
	gateway.symbols = symbols
	gateway.contracts = make([]*CryptoCurrencyContract, 0, len(symbols))
	gateway.contractsMap = make(map[contractsMapKey]*CryptoCurrencyContract)
	for _, s := range symbols {
		contract, err := gateway.parseContract(s)
		if err != nil {
			log.Error(err.Error())
			continue
		}
		gateway.contracts = append(gateway.contracts, contract)
		key := contractsMapKey{contract.CurrencyPair, contract.ContractType}
		gateway.contractsMap[key] = contract
	}
	return gateway
}

func (gateway *GoExGateway) SetConfig(config interface{}) error {
	err := gateway.apiFactory.Config(gateway, config)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	return nil
}

func (gateway *GoExGateway) SetOrderQueryInterval(t time.Duration) *GoExGateway {
	gateway.orderQueryInterval = t
	return gateway
}

func (gateway *GoExGateway) getUnfinishedOrderIDs(contract *CryptoCurrencyContract) []string {
	gateway.ordersRw.RLock()
	defer gateway.ordersRw.RUnlock()
	orderIDs := make([]string, 0, len(gateway.unfinishedOrders))
	for oid, o := range gateway.unfinishedOrders {
		if o.Contract.GetID() == contract.GetID() {
			orderIDs = append(orderIDs, oid)
		}
	}
	return orderIDs
}

func (gateway *GoExGateway) queryContractOrders(contract *CryptoCurrencyContract) {
	orderIDs := gateway.getUnfinishedOrderIDs(contract)
	if len(orderIDs) == 0 {
		return // no unfinished orders to query
	}
	futureOrders, err := gateway.futureRestAPI.GetFutureOrders(orderIDs, contract.CurrencyPair, contract.ContractType)
	if err != nil {
		log.Warningf("error when query order: %s", err)
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
			go func(contract *CryptoCurrencyContract) {
				gateway.queryContractOrders(contract)
			}(c)
			time.Sleep(gateway.orderQueryInterval)
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

func (gateway *GoExGateway) handleOrder(order *Order) *Order {
	gateway.ordersRw.Lock()
	defer gateway.ordersRw.Unlock()
	if gateway.finishedOrders.Contains(order.OrderID) {
		return nil // already finished order
	}
	if o, ok := gateway.unfinishedOrders[order.OrderID]; ok {
		if order.DealAmount < o.DealAmount {
			return nil // misordered order event
		} else if math.Abs(order.DealAmount-o.DealAmount) <= eps {
			if getStage(order.Status) <= getStage(o.Status) {
				return nil
			}
		}
		order.Type = o.Type
	}
	if order.IsFinished() {
		if _, ok := gateway.unfinishedOrders[order.OrderID]; ok {
			delete(gateway.unfinishedOrders, order.OrderID)
		}
		gateway.finishedOrders.Add(order.OrderID)
		go func(orderID string) {
			time.Sleep(5 * time.Minute) // keep 5 minute to prevent readd to unfinishedOrders
			gateway.finishedOrders.Remove(orderID)
		}(order.OrderID)
	} else {
		gateway.unfinishedOrders[order.OrderID] = order
	}
	return order
}

func (gateway *GoExGateway) startHandleOrders() {
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
		}
	}
}

func (gateway *GoExGateway) onWsOrder(order *goex.FutureOrder, contractType string) {
	contract, err := gateway.GetContract(order.Currency, contractType)
	if err != nil {
		log.Errorf("%s", err)
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

func (gateway *GoExGateway) PlaceOrder(symbol string, price, amount string, orderType OrderType, offset OrderOffset, leverRate int) (string, error) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		log.Errorf("%s", err)
		return "", err
	}
	contract, err := gateway.GetContract(currencyPair, contractType)
	if err != nil {
		log.Errorf("%s", err)
		return "", err
	}
	var orderID string
	if orderType == OT_LIMIT {
		orderID, err = gateway.futureRestAPI.PlaceFutureOrder(currencyPair, contractType,
			price, amount, NewGoexOpenType(offset), 0, leverRate)
	} else {
		orderID, err = gateway.futureRestAPI.(*okcoin.OKExV3).PlaceFutureOrder2(currencyPair, contractType,
			price, amount, int(orderType), NewGoexOpenType(offset), 0, leverRate)
	}
	if err != nil {
		log.Errorf("%s", err)
		return "", err
	}
	order := new(Order)
	order.OrderID = orderID
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
	return orderID, nil
}

func (gateway *GoExGateway) CancelOrder(symbol string, orderID string) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		log.Errorf("%s", err)
	}
	go func() {
		_, err := gateway.futureRestAPI.FutureCancelOrder(currencyPair, contractType, orderID)
		if err != nil {
			log.Errorf("%s", err)
		}
	}()
}

func (gateway *GoExGateway) GetCandles(symbol string, period, size int) ([]Bar, error) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		log.Error(err.Error())
	}
	contract, err := gateway.GetContract(currencyPair, contractType)
	if err != nil {
		log.Error(err.Error())
	}
	kline, err := gateway.futureRestAPI.GetKlineRecords(contractType, currencyPair, period, size, 0)
	if err != nil {
		return nil, err
	}
	return AdapterFutureKlines(contract, kline), nil
}

func (gateway *GoExGateway) Connect() error {
	// TODO: Once

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
	gateway.subscribe()
	return nil
}
