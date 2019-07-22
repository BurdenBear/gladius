package hbdm

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	goex "github.com/nntaoli-project/GoEx"
	"github.com/nntaoli-project/GoEx/huobi"
)

type TradeData struct {
	TradeID       int64   `json:"trade_id"`       //撮合结果id
	TradeVolume   float64 `json:"trade_volume"`   //成交量
	TradePrice    float64 `json:"trade_price"`    //撮合价格
	TradeFee      float64 `json:"trade_fee"`      //成交手续费
	TradeTurnover float64 `json:"trade_turnover"` //成交金额
	CreatedAt     int64   `json:"created_at"`     //成交创建时间
	Role          string
}

/*
{
“op”: “notify”, // 操作名称
“topic”: “orders.btc”, // 主题
"ts": 1489474082831, // 时间戳
"symbol": "BTC", //品种
"contract_type": "this_week", //合约类型
"contract_code": "BTC180914", //合约代码
"volume": 111, //委托数量
"price": 1111, //委托价格
"order_price_type": "limit", //订单报价类型 "limit":限价 "opponent":对手价 "post_only":只做maker单,post only下单只受用户持仓数量限制
"direction": "buy", //"buy":买 "sell":卖
"offset": "open", //"open":开 "close":平
"status": 6 //订单状态( 3已提交 4部分成交 5部分成交已撤单 6全部成交 7已撤单)
"lever_rate": 10, //杠杆倍数
"order_id": 106837, //订单ID
"client_order_id": 10683, //客户订单ID
"order_source": "web", //订单来源   （system:系统 web:用户网页 api:用户API m:用户M站 risk:风控系统）
"order_type": 1, //订单类型  (1:报单 、2:撤单 、3:强平、4:交割)
"created_at": 1408076414000, //订单创建时间
"trade_volume": 1, //成交数量
"trade_turnover": 1200, //成交总金额
"fee": 0, //手续费
"trade_avg_price": 10, //成交均价
"margin_frozen": 10, //冻结保证金
"profit": 2, //收益
"trade":[{
    "trade_id":112, //撮合结果id
    "trade_volume":1, //成交量
    "trade_price":123.4555, //撮合价格
    "trade_fee":0.234, //成交手续费
    "trade_turnover":34.123, //成交金额
    "created_at": 1490759594752, //成交创建时间
    "role": "maker" //taker或maker
  }]
}
*/
type OrderNotification struct {
	Op             string
	Topic          string
	Ts             int64
	Symbol         string
	ContractType   string `json:"contract_type"`
	ContractCode   string `json:"contract_code"`
	Volume         float64
	Price          float64
	OrderPriceType string `json:"order_price_type"`
	Direction      string
	Offset         string
	Status         int
	LeverRate      int     `json:"lever_rate"`
	OrderID        int64   `json:"order_id"`
	ClientOrderID  int64   `json:"client_order_id"`
	OrderSource    string  `json:"order_source"`
	OrderType      int     `json:"order_type"`
	CreatedAt      int64   `json:"created_at"`
	TradeVolume    float64 `json:"trade_volume"`
	TradeTurnover  float64 `json:"trade_turnover"`
	Fee            float64
	TradeAvgPrice  float64 `json:"trade_avg_price"`
	MarginFrozen   float64 `json:"margin_frozen"`
	Profit         float64
	Trade          []TradeData
}

type OpResponse struct {
	Op      string
	Type    string
	Ts      int64
	ErrCode int    `json:"err_code"`
	ErrMsg  string `json:"err_msg"`
	Data    json.RawMessage
	Topic   string
}

type HbdmOrderWs struct {
	*goex.WsBuilder
	sync.Once
	wsConn *goex.WsConn

	subs         []interface{}
	loginLock    *sync.Mutex
	loginCh      chan interface{}
	logined      bool
	apiKey       string
	apiSecretKey string

	orderCallback func(*goex.FutureOrder, string)
}

func NewHbdmOrderWs() *HbdmOrderWs {
	hbdmWs := &HbdmOrderWs{WsBuilder: goex.NewWsBuilder()}
	hbdmWs.WsBuilder = hbdmWs.WsBuilder.
		WsUrl("wss://api.hbdm.com/notification").
		Heartbeat(nil, 10*time.Second).
		ErrorHandleFunc(func(err error) {
			log.Printf("hbdm order ws internal error: %s\n", err.Error())
		}).
		ReconnectIntervalTime(24 * time.Hour).
		UnCompressFunc(goex.GzipUnCompress).
		ProtoHandleFunc(hbdmWs.handle)
	hbdmWs.subs = make([]interface{}, 0)
	hbdmWs.loginLock = &sync.Mutex{}
	hbdmWs.loginCh = make(chan interface{})
	return hbdmWs
}

func (hbdmWs *HbdmOrderWs) SetOrderCallback(orderCallback func(*goex.FutureOrder, string)) {
	hbdmWs.orderCallback = orderCallback
}

func (hbdmWs *HbdmOrderWs) handle(msg []byte) error {
	//心跳
	msgStr := string(msg)
	if strings.Contains(msgStr, "ping") {
		var ping struct {
			Op string `json:"op"`
			Ts string `json:"ts"`
		}
		json.Unmarshal(msg, &ping)
		pong := struct {
			Op string `json:"op"`
			Ts string `json:"ts"`
		}{Op: "pong", Ts: ping.Ts}
		// log.Println("hbdm orderWs:", pong)
		hbdmWs.wsConn.SendJsonMessage(pong)
		hbdmWs.wsConn.UpdateActiveTime()
		return nil
	}

	// check if msg is OrderNotification
	if strings.Contains(msgStr, "order_id") {
		var orderNotification OrderNotification
		err := json.Unmarshal(msg, &orderNotification)
		if err != nil {
			return err
		}
		order, contractType, err := hbdmWs.adaptOrder(orderNotification)
		if err != nil {
			return err
		}
		hbdmWs.orderCallback(order, contractType)
		return nil
	}

	// check if msg is OpResponse
	var resp OpResponse
	err := json.Unmarshal(msg, &resp)
	if err != nil {
		return err
	}
	// 2002: Authentication required.
	if resp.ErrCode == 2002 {
		if hbdmWs.logined { // have logined successfully
			go func() {
				hbdmWs.login()
				hbdmWs.reSubscribe()
			}()
		}
		return nil
	}
	// for auth
	switch resp.Op {
	case "auth":
		select {
		case hbdmWs.loginCh <- resp:
			return nil
		default:
			break
		}
	case "sub":
		if resp.ErrCode == 0 {
			log.Println("subscribed:", resp)
		}
	}

	// else
	if resp.ErrCode != 0 {
		return fmt.Errorf("error in websocket: %v", resp)
	}
	// TODO: Futher more message
	return nil
}

func (hbdmWs *HbdmOrderWs) SubscribeOrder(currencyPair goex.CurrencyPair, contractType string) error {
	if hbdmWs.orderCallback == nil {
		return errors.New("please set ticker callback func")
	}
	return hbdmWs.subscribe(map[string]interface{}{
		"op":    "sub",
		"topic": fmt.Sprintf("orders.%s", currencyPair.CurrencyA.Symbol)})
}

func (hbdmWs *HbdmOrderWs) clearChan(c chan interface{}) {
	for {
		if len(c) > 0 {
			<-c
		} else {
			break
		}
	}
}

func (hbdmWs *HbdmOrderWs) login() error {
	hbdmWs.loginLock.Lock()
	defer hbdmWs.loginLock.Unlock()
	hbdmWs.connectWs()
	hbdmWs.clearChan(hbdmWs.loginCh)
	apiKey := hbdmWs.apiKey
	apiSecretKey := hbdmWs.apiSecretKey
	opReq := map[string]string{
		"op":   "auth",
		"type": "api",
	}
	err := hbdmWs.getSignedData(opReq, apiKey, apiSecretKey)
	if err != nil {
		return err
	}
	err = hbdmWs.wsConn.SendJsonMessage(opReq)
	if err != nil {
		return err
	}
	select {
	case opResp := <-hbdmWs.loginCh:
		if r, ok := opResp.(OpResponse); ok {
			if r.ErrCode == 0 {
				log.Println("login success:", r, string(r.Data))
				return nil
			}
		}
		log.Println("login failed:", opResp)
		return fmt.Errorf("login failed: %v", opResp)
	case <-time.After(time.Second * 5):
		log.Println("login timeout after 5 seconds")
		return errors.New("login timeout after 5 seconds")
	}
}

func (hbdmWs *HbdmOrderWs) getSignedData(data map[string]string, apiKey, apiSecretKey string) error {
	data["AccessKeyId"] = apiKey
	data["SignatureMethod"] = "HmacSHA256"
	data["SignatureVersion"] = "2"
	data["Timestamp"] = time.Now().UTC().Format("2006-01-02T15:04:05")
	postForm := url.Values{}
	// 当type为api时，参数op，type，cid，Signature不参加签名计算
	isApi := data["type"] == "api"
	for k, v := range data {
		if isApi && (k == "op" || k == "cid" || k == "type") {
			continue
		}
		postForm.Set(k, v)
	}
	u, err := url.Parse(hbdmWs.wsConn.WsUrl)
	if err != nil {
		return err
	}
	payload := fmt.Sprintf("%s\n%s\n%s\n%s", "GET", u.Host, u.Path, postForm.Encode())
	sign, _ := goex.GetParamHmacSHA256Base64Sign(apiSecretKey, payload)
	data["Signature"] = sign
	return nil
}

func (hbdmWs *HbdmOrderWs) Login(apiKey, apiSecretKey, passphrase string) error {
	if hbdmWs.logined {
		return nil
	}
	hbdmWs.apiKey = apiKey
	hbdmWs.apiSecretKey = apiSecretKey
	err := hbdmWs.login()
	if err == nil {
		hbdmWs.logined = true
	}
	return err
}

func (hbdmWs *HbdmOrderWs) subscribe(sub map[string]interface{}) error {
	log.Println(sub)
	hbdmWs.connectWs()
	err := hbdmWs.wsConn.SendJsonMessage(sub)
	if err != nil {
		return err
	}
	hbdmWs.subs = append(hbdmWs.subs, sub)
	return nil
}

func (hdbmWs *HbdmOrderWs) reSubscribe() {
	for _, d := range hdbmWs.subs {
		hdbmWs.wsConn.SendJsonMessage(d)
	}
}

func (hbdmWs *HbdmOrderWs) connectWs() {
	hbdmWs.Do(func() {
		hbdmWs.wsConn = hbdmWs.WsBuilder.Build()
		hbdmWs.wsConn.ReceiveMessage()
	})
}

func (hdbmWs *HbdmOrderWs) adaptOrderOType(direction, offset string) (int, error) {
	fallback := -1
	switch direction {
	case "buy":
		switch offset {
		case "open":
			return goex.OPEN_BUY, nil
		case "close":
			return goex.CLOSE_BUY, nil
		}
	case "sell":
		switch offset {
		case "open":
			return goex.OPEN_SELL, nil
		case "close":
			return goex.CLOSE_SELL, nil
		}
	}
	return fallback, fmt.Errorf("unknown OType for direction %s, offset %s", direction, offset)
}

func (hdbmWs *HbdmOrderWs) adapterOrderStatus(status int) goex.TradeStatus {
	switch status {
	case 3:
		return goex.ORDER_UNFINISH
	case 4:
		return goex.ORDER_PART_FINISH
	case 5:
		return goex.ORDER_CANCEL
	case 6:
		return goex.ORDER_FINISH
	case 7:
		return goex.ORDER_CANCEL
	case 11:
		return goex.ORDER_CANCEL_ING
	default:
		return goex.ORDER_UNFINISH
	}
}

func (hdbmWs *HbdmOrderWs) adaptOrder(orderNotification OrderNotification) (*goex.FutureOrder, string, error) {
	var fallback *goex.FutureOrder
	var err error
	order := new(goex.FutureOrder)
	order.OrderID = orderNotification.OrderID
	order.OrderID2 = strconv.FormatInt(orderNotification.OrderID, 10)
	order.Price = orderNotification.Price
	order.Amount = orderNotification.Volume
	order.AvgPrice = orderNotification.TradeAvgPrice
	order.DealAmount = orderNotification.TradeVolume
	order.Status = hdbmWs.adapterOrderStatus(orderNotification.Status)
	order.OType, err = hdbmWs.adaptOrderOType(orderNotification.Direction, orderNotification.Offset)
	if err != nil {
		return fallback, "", err
	}
	order.OrderTime = orderNotification.Ts
	order.Fee = orderNotification.Fee
	order.ContractName = orderNotification.ContractCode
	order.Currency = goex.NewCurrencyPair2(orderNotification.Symbol + "_USD")
	order.LeverRate = orderNotification.LeverRate
	contractType := orderNotification.ContractType
	return order, contractType, nil
}

type HbdmWs struct {
	mdWs           *huobi.HbdmWs // market data websocket
	orderWs        *HbdmOrderWs
	tickerCallback func(*goex.FutureTicker)
	depthCallback  func(*goex.Depth)
	tradeCallback  func(*goex.Trade, string)
	orderCallback  func(*goex.FutureOrder, string)
}

func NewHbdmWs() *HbdmWs {
	hbdmWs := &HbdmWs{}
	hbdmWs.mdWs = huobi.NewHbdmWs()
	hbdmWs.mdWs.WsBuilder = hbdmWs.mdWs.WsBuilder.Heartbeat(nil, 10*time.Second)
	hbdmWs.orderWs = NewHbdmOrderWs()
	return hbdmWs
}

func (hbdmWs *HbdmWs) TickerCallback(tickerCallback func(*goex.FutureTicker)) *HbdmWs {
	hbdmWs.tickerCallback = tickerCallback
	hbdmWs.setCallbacks()
	return hbdmWs
}

func (hbdmWs *HbdmWs) DepthCallback(depthCallback func(*goex.Depth)) *HbdmWs {
	hbdmWs.depthCallback = depthCallback
	hbdmWs.setCallbacks()
	return hbdmWs
}

func (hbdmWs *HbdmWs) TradeCallback(tradeCallback func(*goex.Trade, string)) *HbdmWs {
	hbdmWs.tradeCallback = tradeCallback
	hbdmWs.setCallbacks()
	return hbdmWs
}

func (hbdmWs *HbdmWs) OrderCallback(orderCallback func(*goex.FutureOrder, string)) *HbdmWs {
	hbdmWs.orderCallback = orderCallback
	hbdmWs.setCallbacks()
	return hbdmWs
}

func (hbdmWs *HbdmWs) setCallbacks() {
	hbdmWs.mdWs.SetCallbacks(hbdmWs.tickerCallback, hbdmWs.depthCallback, hbdmWs.tradeCallback)
	hbdmWs.orderWs.SetOrderCallback(hbdmWs.orderCallback)
}

func (hbdmWs *HbdmWs) SetCallbacks(tickerCallback func(*goex.FutureTicker),
	depthCallback func(*goex.Depth),
	tradeCallback func(*goex.Trade, string),
	orderCallback func(*goex.FutureOrder, string)) {
	hbdmWs.tickerCallback = tickerCallback
	hbdmWs.depthCallback = depthCallback
	hbdmWs.tradeCallback = tradeCallback
	hbdmWs.orderCallback = orderCallback
	hbdmWs.setCallbacks()
}

func (hbdmWs *HbdmWs) SubscribeTicker(pair goex.CurrencyPair, contract string) error {
	return hbdmWs.mdWs.SubscribeTicker(pair, contract)
}

func (hbdmWs *HbdmWs) SubscribeDepth(pair goex.CurrencyPair, contract string, size int) error {
	return hbdmWs.mdWs.SubscribeDepth(pair, contract, size)
}

func (hbdmWs *HbdmWs) SubscribeTrade(pair goex.CurrencyPair, contract string) error {
	return hbdmWs.mdWs.SubscribeTrade(pair, contract)
}

func (hbdmWs *HbdmWs) SubscribeOrder(pair goex.CurrencyPair, contract string) error {
	return hbdmWs.orderWs.SubscribeOrder(pair, contract)
}

func (hbdmWs *HbdmWs) Login(apiKey, apiSecret, passphrase string) error {
	return hbdmWs.orderWs.Login(apiKey, apiSecret, passphrase)
}

func NewHbdmWsWrapper() *HbdmWsWrapper {
	return &HbdmWsWrapper{HbdmWs: NewHbdmWs()}
}

type HbdmWsWrapper struct {
	*HbdmWs
}

func (w *HbdmWsWrapper) TickerCallback(cb func(*goex.FutureTicker)) {
	w.HbdmWs.TickerCallback(cb)
}
func (w *HbdmWsWrapper) DepthCallback(cb func(*goex.Depth)) {
	w.HbdmWs.DepthCallback(cb)
}
func (w *HbdmWsWrapper) TradeCallback(cb func(*goex.Trade, string)) {
	w.HbdmWs.TradeCallback(cb)
}

// KlineCallback(func(*goex.FutureKline, int)) FutureWebsocketAPI
func (w *HbdmWsWrapper) OrderCallback(cb func(*goex.FutureOrder, string)) {
	w.HbdmWs.OrderCallback(cb)
}
