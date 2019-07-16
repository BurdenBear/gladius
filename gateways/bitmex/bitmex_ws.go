package bitmex

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/imdario/mergo"
	. "github.com/nntaoli-project/GoEx"
)

type bitmexWsRequest struct {
	Op   string
	Args []interface{}
}

type bitmexWsConnectLimit struct {
	Remaining int
}

type bitmexWsWelcomeMsg struct {
	Info    string
	Version string
	Limit   bitmexWsConnectLimit
}

type bitmexWsResp struct {
	Request   bitmexWsRequest
	Success   bool
	Status    int
	Error     string
	Subscribe string
}

type bitmexWsData struct {
	Table  string
	Action string
	Data   json.RawMessage
}

type bitmexOrderBook struct {
	Symbol    string
	Asks      [][]float64
	Bids      [][]float64
	Timestamp string
}

/*
{
   "timestamp":"2019-07-08T03:50:37.022Z",
   "symbol":"XBTU19",
   "side":"Sell",
   "size":4,
   "price":11824,
   "tickDirection":"ZeroMinusTick",
   "trdMatchID":"1b2dbf51-3ea6-3c5d-c3f3-b92b98ac7933",
   "grossValue":33828,
   "homeNotional":0.00033828,
   "foreignNotional":4
}
*/
type bitmexWsTrade struct {
	Symbol    string
	Timestamp string
	Side      string
	Size      float64
	Price     float64
	Tid       string
}

/*
{
	"orderID":"5c162f7f-e367-47d9-148f-dd55c84a3602",
	"clOrdID":"",
	"clOrdLinkID":"",
	"account":132795,
	"symbol":"XBTUSD",
	"side":"Buy",
	"simpleOrderQty":null,
	"orderQty":1,
	"price":11596.5,
	"displayQty":null,
	"stopPx":null,
	"pegOffsetValue":null,
	"pegPriceType":"",
	"currency":"USD",
	"settlCurrency":"XBt",
	"ordType":"Limit",
	"timeInForce":"GoodTillCancel",
	"execInst":"",
	"contingencyType":"",
	"exDestination":"XBME",
	"ordStatus":"New",
	"triggered":"",
	"workingIndicator":false,
	"ordRejReason":"",
	"simpleLeavesQty":null,
	"leavesQty":1,
	"simpleCumQty":null,
	"cumQty":0,
	"avgPx":null,
	"multiLegReportingType":"SingleSecurity",
	"text":"Submitted via API.",
	"transactTime":"2019-07-08T06:53:42.896Z",
	"timestamp":"2019-07-08T06:53:42.896Z"
}
*/
type bitmexWsOrder = BitmexOrder

func (order bitmexWsOrder) isFinished() bool {
	os := AdapterFixOrderStatus(order.OrdStatus)
	return os == ORDER_REJECT || os == ORDER_CANCEL || os == ORDER_FINISH
}

type BitmexWs struct {
	*WsBuilder
	sync.Once
	wsConn        *WsConn
	loginCh       chan interface{}
	logined       bool
	loginLock     *sync.Mutex
	adapter       *BitmexAdapter
	apiKey        string
	apiSecretKey  string
	passphrase    string
	authoriedSubs []map[string]interface{}
	ordersMap     map[string]*bitmexWsOrder

	tickerCallback func(*FutureTicker)
	depthCallback  func(*Depth)
	tradeCallback  func(*Trade, string)
	klineCallback  func(*FutureKline, int)
	orderCallback  func(*FutureOrder, string)
}

func NewBitmexWs() *BitmexWs {
	bitmexWs := &BitmexWs{}
	bitmexWs.adapter = NewBitmexAdapter()
	bitmexWs.loginCh = make(chan interface{})
	bitmexWs.logined = false
	bitmexWs.loginLock = &sync.Mutex{}
	bitmexWs.authoriedSubs = make([]map[string]interface{}, 0)
	bitmexWs.ordersMap = make(map[string]*bitmexWsOrder)
	bitmexWs.WsBuilder = NewWsBuilder().
		WsUrl("wss://www.bitmex.com/realtime").
		Heartbeat([]byte("ping"), 10*time.Second).
		ReconnectIntervalTime(24 * time.Hour).
		UnCompressFunc(FlateUnCompress).
		ProtoHandleFunc(bitmexWs.handle)

	return bitmexWs
}

func (bitmexWs *BitmexWs) TickerCallback(tickerCallback func(*FutureTicker)) *BitmexWs {
	bitmexWs.tickerCallback = tickerCallback
	return bitmexWs
}

func (bitmexWs *BitmexWs) DepthCallback(depthCallback func(*Depth)) *BitmexWs {
	bitmexWs.depthCallback = depthCallback
	return bitmexWs
}

func (bitmexWs *BitmexWs) TradeCallback(tradeCallback func(*Trade, string)) *BitmexWs {
	bitmexWs.tradeCallback = tradeCallback
	return bitmexWs
}

func (bitmexWs *BitmexWs) OrderCallback(orderCallback func(*FutureOrder, string)) *BitmexWs {
	bitmexWs.orderCallback = orderCallback
	return bitmexWs
}

func (bitmexWs *BitmexWs) SetCallbacks(tickerCallback func(*FutureTicker),
	depthCallback func(*Depth),
	tradeCallback func(*Trade, string),
	klineCallback func(*FutureKline, int),
	orderCallback func(*FutureOrder, string)) {
	bitmexWs.tickerCallback = tickerCallback
	bitmexWs.depthCallback = depthCallback
	bitmexWs.tradeCallback = tradeCallback
	bitmexWs.klineCallback = klineCallback
	bitmexWs.orderCallback = orderCallback
}

func (bitmexWs *BitmexWs) Login(apiKey string, apiSecretKey string, passphrase string) error {
	// already logined
	if bitmexWs.logined {
		return nil
	}
	bitmexWs.connectWs()
	bitmexWs.apiKey = apiKey
	bitmexWs.apiSecretKey = apiSecretKey
	bitmexWs.passphrase = passphrase
	err := bitmexWs.login()
	if err == nil {
		bitmexWs.logined = true
	}
	return err
}

func (bitmexWs *BitmexWs) getTimestamp() string {
	seconds := float64(time.Now().UTC().UnixNano()) / float64(time.Second)
	return fmt.Sprintf("%.3f", seconds)
}

func (bitmexWs *BitmexWs) clearChan(c chan interface{}) {
	for {
		if len(c) > 0 {
			<-c
		} else {
			break
		}
	}
}

func (bitmexWs *BitmexWs) login() error {
	bitmexWs.loginLock.Lock()
	defer bitmexWs.loginLock.Unlock()
	bitmexWs.clearChan(bitmexWs.loginCh)
	apiKey := bitmexWs.apiKey
	apiSecretKey := bitmexWs.apiSecretKey
	//clear last login result
	expires := getExpires()
	expiresInt, _ := strconv.ParseInt(expires, 10, 64)
	method := "GET"
	url := "/realtime"
	sign, _ := getSign(apiSecretKey, method, url, expires, "")
	op := map[string]interface{}{
		"op":   "authKeyExpires",
		"args": []interface{}{apiKey, expiresInt, sign}}

	err := bitmexWs.wsConn.SendJsonMessage(op)
	if err != nil {
		return err
	}
	select {
	case event := <-bitmexWs.loginCh:
		if v, ok := event.(bitmexWsResp); ok {
			if v.Error == "" {
				log.Println("login success:", event)
				return nil
			}
		}
		log.Println("login failed:", event)
		return fmt.Errorf("login failed: %v", event)
	case <-time.After(time.Second * 5):
		log.Println("login timeout after 5 seconds")
		return errors.New("login timeout after 5 seconds")
	}
}

func (bitmexWs *BitmexWs) subscribe(sub map[string]interface{}) error {
	bitmexWs.connectWs()
	return bitmexWs.wsConn.Subscribe(sub)
}

func (bitmexWs *BitmexWs) SubscribeDepth(currencyPair CurrencyPair, contractType string, size int) error {
	if bitmexWs.depthCallback == nil {
		return errors.New("please set depth callback func")
	}

	symbol, err := adaptBixmexSymbol(currencyPair, contractType)
	if err != nil {
		return err
	}

	var chName string
	if size <= 10 {
		chName = fmt.Sprintf("orderBook10:%s", symbol)
	} else {
		chName = fmt.Sprintf("orderBookL2:%s", symbol)
	}

	return bitmexWs.subscribe(map[string]interface{}{
		"op":   "subscribe",
		"args": []string{chName}})
}

func (bitmexWs *BitmexWs) SubscribeTicker(currencyPair CurrencyPair, contractType string) error {
	// if bitmexWs.tickerCallback == nil {
	// 	return errors.New("please set ticker callback func")
	// }

	// symbol, err := adaptBixmexSymbol(currencyPair, contractType)
	// if err != nil {
	// 	return err
	// }

	// chName := fmt.Sprintf("tradeBin1d:%s", symbol)

	// return bitmexWs.subscribe(map[string]interface{}{
	// 	"op":   "subscribe",
	// 	"args": []string{chName}})
	return errors.New("unsupported ticker subscription in bitmex")
}

func (bitmexWs *BitmexWs) SubscribeTrade(currencyPair CurrencyPair, contractType string) error {
	if bitmexWs.tradeCallback == nil {
		return errors.New("please set trade callback func")
	}

	symbol, err := adaptBixmexSymbol(currencyPair, contractType)
	if err != nil {
		return err
	}

	chName := fmt.Sprintf("trade:%s", symbol)

	return bitmexWs.subscribe(map[string]interface{}{
		"op":   "subscribe",
		"args": []string{chName}})
}

func (bitmexWs *BitmexWs) SubscribeKline(currencyPair CurrencyPair, contractType string, period int) error {
	if bitmexWs.klineCallback == nil {
		return errors.New("place set kline callback func")
	}

	symbol, err := adaptBixmexSymbol(currencyPair, contractType)
	if err != nil {
		return err
	}

	binSize, ok := KlineTypeBinSizeMap[period]
	if !ok {
		return fmt.Errorf("invalid kline period for bitmex %d", period)
	}

	chName := fmt.Sprintf("tradeBin%s:%s", binSize, symbol)

	return bitmexWs.subscribe(map[string]interface{}{
		"op":   "subscribe",
		"args": []string{chName}})
}

func (bitmexWs *BitmexWs) SubscribeOrder(currencyPair CurrencyPair, contractType string) error {
	if bitmexWs.orderCallback == nil {
		return errors.New("place set order callback func")
	}

	symbol, err := adaptBixmexSymbol(currencyPair, contractType)
	if err != nil {
		return err
	}

	chName := fmt.Sprintf("order:%s", symbol)

	return bitmexWs.authoriedSubscribe(map[string]interface{}{
		"op":   "subscribe",
		"args": []string{chName}})
}

func (bitmexWs *BitmexWs) authoriedSubscribe(data map[string]interface{}) error {
	bitmexWs.authoriedSubs = append(bitmexWs.authoriedSubs, data)
	return bitmexWs.subscribe(data)
}

func (bitmexWs *BitmexWs) reSubscribeAuthoriedChannel() {
	for _, d := range bitmexWs.authoriedSubs {
		bitmexWs.wsConn.SendJsonMessage(d)
	}
}

func (bitmexWs *BitmexWs) connectWs() {
	bitmexWs.Do(func() {
		bitmexWs.wsConn = bitmexWs.WsBuilder.Build()
		bitmexWs.wsConn.ReceiveMessage()
	})
}

func (bitmexWs *BitmexWs) reSubscribe() {
	for _, d := range bitmexWs.authoriedSubs {
		bitmexWs.wsConn.SendJsonMessage(d)
	}
}

func (bitmexWs *BitmexWs) insertOrder(order *bitmexWsOrder) *bitmexWsOrder {
	bitmexWs.ordersMap[order.OrderID] = order
	return order
}

func (bitmexWs *BitmexWs) updateOrder(order *bitmexWsOrder) *bitmexWsOrder {
	if old, ok := bitmexWs.ordersMap[order.OrderID]; ok {
		if err := mergo.Merge(old, *order, mergo.WithOverride); err != nil {
			log.Println(err)
			return nil
		}
		return old
	}
	log.Println(fmt.Sprintf("can't find source of order update: %+v", *order))
	return nil
}

func (bitmexWs *BitmexWs) deleteOrder(orderID string) {
	if _, ok := bitmexWs.ordersMap[orderID]; ok {
		delete(bitmexWs.ordersMap, orderID)
	}
}

func (bitmexWs *BitmexWs) handle(msg []byte) error {
	msgStr := string(msg)
	if msgStr == "pong" {
		log.Println(msgStr)
		bitmexWs.wsConn.UpdateActiveTime()
		return nil
	}

	msgMap := make(map[string]interface{})
	err := json.Unmarshal(msg, &msgMap)
	if err != nil {
		return err
	}

	if msgMap["table"] != nil {
		var data bitmexWsData
		err := json.Unmarshal(msg, &data)
		if err != nil {
			return err
		}
		switch data.Table {
		case "orderBook10":
			orderBooks := make([]bitmexOrderBook, 0)
			err := json.Unmarshal(data.Data, &orderBooks)
			if err != nil {
				return err
			}
			for _, orderBook := range orderBooks {
				depth, err := bitmexWs.adapter.AdaptWsOrderBook(&orderBook, 0)
				if err != nil {
					log.Println(err)
					continue
				}
				bitmexWs.depthCallback(depth)
			}

		case "orderBookL2":
			orderBooks := make([]bitmexOrderBook, 0)
			err := json.Unmarshal(data.Data, &orderBooks)
			if err != nil {
				return err
			}
			for _, orderBook := range orderBooks {
				depth, err := bitmexWs.adapter.AdaptWsOrderBook(&orderBook, 0)
				if err != nil {
					log.Println(err)
					continue
				}
				bitmexWs.depthCallback(depth)
			}
		case "trade":
			trades := make([]bitmexWsTrade, 0)
			err := json.Unmarshal(data.Data, &trades)
			if err != nil {
				return err
			}
			for _, trade := range trades {
				trade, contractType, err := bitmexWs.adapter.AdaptWsTrade(&trade)
				if err != nil {
					continue
				}
				bitmexWs.tradeCallback(trade, contractType)
			}
		case "order":
			orders := make([]*bitmexWsOrder, 0)
			err = json.Unmarshal(data.Data, &orders)
			if err != nil {
				log.Println(err)
				return err
			}
			if data.Action == "insert" {
				for _, order := range orders {
					order = bitmexWs.insertOrder(order)
					if order.isFinished() {
						bitmexWs.deleteOrder(order.OrderID)
					}
					o, ct, err := bitmexWs.adapter.AdaptOrder(order)
					if err != nil {
						log.Println(err)
						continue
					}
					bitmexWs.orderCallback(o, ct)
				}
				return nil
			} else if data.Action == "update" {
				for _, order := range orders {
					// {"orderID":"5c162f7f-e367-47d9-148f-dd55c84a3602","workingIndicator":true,"clOrdID":"","account":132795,"symbol":"XBTUSD","timestamp":"2019-07-08T06:53:42.896Z"}
					if order.OrdStatus != "" {
						order = bitmexWs.updateOrder(order)
						if order.isFinished() {
							bitmexWs.deleteOrder(order.OrderID)
						}
						o, ct, err := bitmexWs.adapter.AdaptOrder(order)
						if err != nil {
							log.Println(err)
							continue
						}
						bitmexWs.orderCallback(o, ct)
					}
				}
				return nil
			}
			return fmt.Errorf("unhandled order data: %v", data)
		default:
			log.Println(data.Table, string(data.Data))
		}
		return nil
	}

	if msgMap["info"] != nil {
		var welcome bitmexWsWelcomeMsg
		err := json.Unmarshal(msg, &welcome)
		if err != nil {
			return err
		}
		log.Println(fmt.Sprintf("connected: %s verison: %s limit: %d", welcome.Info, welcome.Version, welcome.Limit.Remaining))
		return nil
	}

	var resp bitmexWsResp
	err = json.Unmarshal(msg, &resp)
	if err != nil {
		log.Println(fmt.Sprintf("unknown data: %s", msgStr))
		return err
	}
	switch resp.Request.Op {
	case "subscribe":
		if resp.Error != "" {
			log.Println(fmt.Sprintf("op: %s, args: %v, error: \"%s\", status: %d", resp.Request.Op, resp.Request.Args, resp.Error, resp.Status))
			if resp.Status == 401 {
				if bitmexWs.logined { // have logined successfully
					go func() {
						bitmexWs.login()
						bitmexWs.reSubscribe()
					}()
				}
			}
		} else {
			log.Println(fmt.Sprintf("subscribed: %s", resp.Subscribe))
		}
	case "authKeyExpires":
		select {
		case bitmexWs.loginCh <- resp:
			return nil
		default:
			return nil
		}
	default:
		log.Println(fmt.Sprintf("%+v", resp))
	}
	return nil
}

func (bitmexWs *BitmexWs) getKlinePeriodFormChannel(channel string) int {
	metas := strings.Split(channel, ":")
	if len(metas) != 2 {
		return 0
	}
	i, _ := strconv.ParseInt(metas[1], 10, 64)
	return int(i)
}
