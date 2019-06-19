package okex

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/gateways"
	"github.com/deckarep/golang-set"
	goex "github.com/nntaoli-project/GoEx"
	"github.com/nntaoli-project/GoEx/builder"
	"github.com/nntaoli-project/GoEx/okcoin"
)

var log = GetLogger()

type BaseHTTPConfig struct {
	HTTPTimeout time.Duration
	HTTPProxy   string
}

type OkexGatewayConfig struct {
	*BaseHTTPConfig
	RestfulURL    string
	WebSocketURL  string
	APIKey        string
	APISecretKey  string
	APIPassphrase string
	Symbols       []string
}

func GetDefaultOkexGatewayConfig() *OkexGatewayConfig {
	apiKey := gateways.GetEnv("GOEX_OKEX_API_KEY", "")
	apiSecretKey := gateways.GetEnv("GOEX_OKEX_API_SECRET_KEY", "")
	passphrase := gateways.GetEnv("GOEX_OKEX_PASSPHRASE", "")
	config := &OkexGatewayConfig{
		WebSocketURL:  "wss://okexcomreal.bafang.com:10442/ws/v3",
		APIKey:        apiKey,
		APISecretKey:  apiSecretKey,
		APIPassphrase: passphrase,
	}
	return config
}

type okexGatewayContractsMapKey struct {
	CurrencyPair goex.CurrencyPair
	ContractType string
}

type OkexGateway struct {
	*gateways.BaseGateway
	config           *OkexGatewayConfig
	spotRestAPI      goex.API
	futureRestAPI    goex.FutureRestAPI
	futureWs         *okcoin.OKExV3FutureWs
	ordersRw         *sync.RWMutex
	finishedOrders   mapset.Set
	unfinishedOrders map[string]*Order
	authorized       bool
	orderCh          chan *Order
	ordersCh         chan []*Order
	contracts        []*CryptoCurrencyContract
	contractsMap     map[okexGatewayContractsMapKey]*CryptoCurrencyContract
}

func NewOkexGateway(name string, engine *EventEngine) *OkexGateway {
	gateway := &OkexGateway{}
	gateway.BaseGateway = gateways.NewBaseGateway(name).Engine(engine)
	gateway.ordersRw = &sync.RWMutex{}
	gateway.unfinishedOrders = make(map[string]*Order)
	gateway.finishedOrders = mapset.NewSet()
	gateway.orderCh = make(chan *Order, 1000)
	gateway.ordersCh = make(chan []*Order, 1000)
	go gateway.startHandleOrders()
	return gateway
}

func newOKExV3FutureWs(url string, provider okcoin.IContractIDProvider) *okcoin.OKExV3FutureWs {
	okWs := okcoin.NewOKExV3FutureWs(provider)
	okWs.WsBuilder = okWs.WsBuilder.
		WsUrl(url).Heartbeat([]byte("ping"), 15*time.Second)
	return okWs
}

func newOKExV3(apiConfig *goex.APIConfig) *okcoin.OKExV3 {
	return okcoin.NewOKExV3(
		apiConfig.HttpClient, apiConfig.ApiKey, apiConfig.ApiSecretKey,
		apiConfig.ApiPassphrase, apiConfig.Endpoint)
}

func (gateway *OkexGateway) parseContract(symbol string) (*CryptoCurrencyContract, error) {
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

func (gateway *OkexGateway) GetExchange() string {
	return "OKEX"
}

func (gateway *OkexGateway) Config(config *OkexGatewayConfig) *OkexGateway {
	gateway.config = config
	gateway.contracts = make([]*CryptoCurrencyContract, 0, len(config.Symbols))
	gateway.contractsMap = make(map[okexGatewayContractsMapKey]*CryptoCurrencyContract)
	for _, s := range config.Symbols {
		contract, err := gateway.parseContract(s)
		if err != nil {
			log.Errorf("%s", err)
			continue
		}
		gateway.contracts = append(gateway.contracts, contract)
		key := okexGatewayContractsMapKey{contract.CurrencyPair, contract.ContractType}
		gateway.contractsMap[key] = contract
	}
	if len(gateway.config.APIKey) > 0 ||
		len(gateway.config.APISecretKey) > 0 ||
		len(gateway.config.APIPassphrase) > 0 {
		gateway.authorized = true
	}
	return gateway
}

func (gateway *OkexGateway) getUnfinishedOrderIDs(contract *CryptoCurrencyContract) []string {
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

func (gateway *OkexGateway) queryContractOrders(contract *CryptoCurrencyContract) {
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
		order := gateways.AdapterFutureOrder(contract, &o)
		orders = append(orders, order)
	}
	gateway.ordersCh <- orders
}

func (gateway *OkexGateway) startQueryOrders(interval time.Duration) {
	for {
		for _, c := range gateway.contracts {
			go func(contract *CryptoCurrencyContract) {
				gateway.queryContractOrders(contract)
			}(c)
			time.Sleep(interval)
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

func (gateway *OkexGateway) handleOrder(order *Order) *Order {
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

func (gateway *OkexGateway) startHandleOrders() {
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

func (gateway *OkexGateway) onWsOrder(order *goex.FutureOrder, contractType string) {
	contract, err := gateway.getContract(order.Currency, contractType)
	if err != nil {
		log.Errorf("%s", err)
		return // unsubscribed depth
	}
	o := gateways.AdapterFutureOrder(contract, order)
	gateway.orderCh <- o
}

func (gateway *OkexGateway) getContract(currencyPair goex.CurrencyPair, contractType string) (*CryptoCurrencyContract, error) {
	key := okexGatewayContractsMapKey{currencyPair, contractType}
	if c, ok := gateway.contractsMap[key]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("no contract found of %s %s", currencyPair.ToSymbol("_"), contractType)
}

func (gateway *OkexGateway) onWsDepth(depth *goex.Depth) {
	contract, err := gateway.getContract(depth.Pair, depth.ContractType)
	if err != nil {
		return // unsubscribed depth
	}
	d := gateways.AdapterDepth(contract, depth)
	gateway.BaseGateway.OnDepth(d)
}

func (gateway *OkexGateway) subscribe() {
	gateway.futureWs.DepthCallback(gateway.onWsDepth)
	gateway.futureWs.OrderCallback(gateway.onWsOrder)
	for _, c := range gateway.contracts {
		gateway.futureWs.SubscribeDepth(c.CurrencyPair, c.ContractType, 5)
		if gateway.authorized {
			gateway.futureWs.SubscribeOrder(c.CurrencyPair, c.ContractType)
		}
	}
}

func (gateway *OkexGateway) PlaceOrder(symbol string, price, amount string, orderType OrderType, offset OrderOffset, leverRate int) (string, error) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		log.Errorf("%s", err)
		return "", err
	}
	contract, err := gateway.getContract(currencyPair, contractType)
	if err != nil {
		log.Errorf("%s", err)
		return "", err
	}
	orderID, err := gateway.futureRestAPI.PlaceFutureOrder(currencyPair, contractType,
		price, amount, NewGoexOpenType(offset), 0, leverRate)
	if err != nil {
		log.Errorf("%s", err)
		return "", err
	}
	order := new(Order)
	order.OrderID = orderID
	order.Price, _ = strconv.ParseFloat(price, 64)
	order.Amount, _ = strconv.ParseFloat(amount, 64)
	order.CreateTime = time.Now().UTC()
	order.Status = OS_SUBMITTED
	order.Contract = contract
	order.Offset = offset
	order.Type = orderType
	order.LeverRate = leverRate
	gateway.orderCh <- order
	return orderID, nil
}

func (gateway *OkexGateway) CancelOrder(symbol string, orderID string) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		log.Errorf("%s", err)
	}
	go gateway.futureRestAPI.FutureCancelOrder(currencyPair, contractType, orderID)
}

func (gateway *OkexGateway) GetCandles(symbol string, period, size int) ([]goex.FutureKline, error) {
	currencyPair, contractType, err := ParseCryptoCurrencyContractSymbol(symbol)
	if err != nil {
		log.Errorf("%s", err)
	}
	return gateway.futureRestAPI.GetKlineRecords(contractType, currencyPair, period, size, 0)
}

func (gateway *OkexGateway) Connect() error {
	// TODO: Once
	apiBuilder := builder.NewAPIBuilder()
	if gateway.config.BaseHTTPConfig != nil {
		apiBuilder.HttpTimeout(gateway.config.HTTPTimeout).HttpProxy(gateway.config.HTTPProxy)
	}
	apiKey := gateway.config.APIKey
	apiSecretKey := gateway.config.APISecretKey
	passphrase := gateway.config.APIPassphrase
	client := apiBuilder.GetHttpClient()
	apiConfig := &goex.APIConfig{
		HttpClient:    client,
		ApiKey:        apiKey,
		ApiSecretKey:  apiSecretKey,
		ApiPassphrase: passphrase,
		Endpoint:      gateway.config.RestfulURL,
	}
	restAPI := newOKExV3(apiConfig)
	gateway.futureRestAPI = restAPI
	gateway.futureWs = newOKExV3FutureWs(gateway.config.WebSocketURL, restAPI)
	if gateway.authorized {
		err := gateway.futureWs.Login(apiKey, apiSecretKey, passphrase)
		if err != nil {
			return err
		}
		interval := time.Duration(2 / (20.0 / 3 * 0.8) * float64(time.Second)) // 20 query / 2 second limit
		go gateway.startQueryOrders(interval)
	}
	gateway.subscribe()
	return nil
}
