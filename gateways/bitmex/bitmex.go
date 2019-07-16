package bitmex

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurdenBear/gladius"
	"github.com/deckarep/golang-set"
	. "github.com/nntaoli-project/GoEx"
)

const (
	API_BASE_URL           = "https://www.bitmex.com/api/v1"
	ORDER_PATH             = "order"
	ACTIVE_INSTRUMENT_PATH = "instrument/active"
	ORDER_BOOK_L2_PATH     = "orderBook/L2"
	POSITION_PATH          = "position"
	OHLCV_PATH             = "trade/bucketed"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetDefaultBitmexConfig() *APIConfig {
	apiKey := getEnv("GOEX_BITMEX_API_KEY", "")
	apiSecretKey := getEnv("GOEX_BITMEX_API_SECRET_KEY", "")
	passphrase := ""
	endpoint := getEnv("GOEX_BITMEX_RESTFUL_URL", "")
	config := &APIConfig{
		HttpClient:    http.DefaultClient,
		Endpoint:      endpoint,
		ApiKey:        apiKey,
		ApiSecretKey:  apiSecretKey,
		ApiPassphrase: passphrase,
	}
	return config
}

type BitmexDepth struct {
	ID     int64
	Symbol string
	Side   string
	Size   float64
	Price  float64
}

type BitmexOrder struct {
	OrderID      string
	ClOrdID      string
	Symbol       string
	Side         string
	OrderQty     float64
	Price        float64
	DisplayQty   float64
	OrdType      string
	TimeInForce  string
	ExecInst     string
	OrdStatus    string
	OrdRejReason string
	LeavesQty    float64
	CumQty       float64
	AvgPx        float64
	TransactTime string
	Timestamp    string
}

type BitmexPosition struct {
	Symbol     string
	Leverage   float64
	ExecBuyQty float64
	//TODO:
}

/*
{
	"timestamp": "2016-02-13T00:00:00.000Z",
	"symbol": "A50G16",
	"open": 8762.5,
	"high": 8737.5,
	"low": 8737.5,
	"close": 8737.5,
	"trades": 1,
	"volume": 3,
	"vwap": 8737.5,
	"lastSize": 3,
	"turnover": 262125000,
	"homeNotional": 2.62125,
	"foreignNotional": 22903.171875
}
*/
type BitmexOHLCVData struct {
	Timestamp       string
	Symbol          string
	Open            float64
	High            float64
	Low             float64
	Close           float64
	Trades          float64
	Volume          float64
	Vwap            float64
	LastSize        float64
	Turnover        float64
	HomeNotional    float64
	ForeignNotional float64
}

// contract information
type BitmexInstrument struct {
	Symbol        string
	Underlying    string
	QuoteCurrency string
	TickSize      float64
	LotSize       float64
}

var tail_zero_re = regexp.MustCompile("0+$")

func normalizeByIncrement(num float64, increment string) (string, error) {
	precision := 0
	i := strings.Index(increment, ".")
	// increment is decimal
	if i > -1 {
		decimal := increment[i+1:]
		trimTailZero := tail_zero_re.ReplaceAllString(decimal, "")
		precision = len(trimTailZero)
		return fmt.Sprintf("%."+fmt.Sprintf("%df", precision), num), nil
	}
	// increment is int
	incrementInt, err := strconv.ParseInt(increment, 10, 64)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(int(num) / int(incrementInt)), nil
}

func (bi BitmexInstrument) NormalizePrice(price float64) (string, error) {
	tickSize := fmt.Sprintf("%f", bi.TickSize)
	if len(tickSize) == 0 {
		return "", fmt.Errorf("no tick size info in instrument %v", bi)
	}
	return normalizeByIncrement(price, tickSize)
}

func (bi BitmexInstrument) NormalizePriceString(price string) (string, error) {
	p, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return "", err
	}
	return bi.NormalizePrice(p)
}

func (bi BitmexInstrument) NormalizeAmount(amount float64) (string, error) {
	increment := fmt.Sprintf("%f", bi.LotSize)
	if len(increment) == 0 {
		return "", fmt.Errorf("no lot size info in instrument %v", bi)
	}
	return normalizeByIncrement(amount, increment)
}

func (bi BitmexInstrument) NormalizeAmountString(amount string) (string, error) {
	a, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return "", err
	}
	return bi.NormalizeAmount(a)
}

type instrumentsMapKey struct {
	CurrencyPair CurrencyPair
	ContractType string
}

type instrumentsMap map[instrumentsMapKey]*BitmexInstrument

func newInstrumentsMap(instruments []BitmexInstrument) instrumentsMap {
	instrumentsMap := instrumentsMap{}
	for _, v := range instruments {
		func(v BitmexInstrument) {
			currencyPair, contractType, err := parseBitmexSymbol(v.Symbol)
			if err != nil {
				return
			}
			key := instrumentsMapKey{
				CurrencyPair: currencyPair,
				ContractType: contractType,
			}
			instrumentsMap[key] = &v
		}(v)
	}
	return instrumentsMap
}

// NOTE:
// instruments信息默认五分钟更新一次。
type Bitmex struct {
	config         *APIConfig
	adapter        *BitmexAdapter
	instrumentsMap instrumentsMap
	instrumentsRW  *sync.RWMutex
}

func NewBitmex(config *APIConfig) *Bitmex {
	bitmex := new(Bitmex)
	bitmex.config = config
	bitmex.instrumentsRW = &sync.RWMutex{}
	bitmex.adapter = NewBitmexAdapter()
	instruments, err := bitmex.getAllInstruments()
	if err != nil {
		panic(err)
	}
	bitmex.setInstruments(instruments)
	return bitmex
}

func (bitmex *Bitmex) GetUrlRoot() (url string) {
	if bitmex.config.Endpoint != "" {
		url = bitmex.config.Endpoint
	} else {
		url = API_BASE_URL
	}
	if url[len(url)-1] != '/' {
		url = url + "/"
	}
	return
}

func (bitmex *Bitmex) setInstruments(instruments []BitmexInstrument) {
	instrumentsMap := newInstrumentsMap(instruments)
	bitmex.instrumentsRW.Lock()
	defer bitmex.instrumentsRW.Unlock()
	bitmex.instrumentsMap = instrumentsMap
}

func (bitmex *Bitmex) getAllInstruments() ([]BitmexInstrument, error) {
	bitmexInstrument, err := bitmex.getFutureInstruments()
	if err != nil {
		return nil, err
	}
	return bitmexInstrument, nil
}

func (bitmex *Bitmex) getInstrumentByKey(key instrumentsMapKey) (*BitmexInstrument, error) {
	bitmex.instrumentsRW.RLock()
	defer bitmex.instrumentsRW.RUnlock()
	data, ok := bitmex.instrumentsMap[key]
	if !ok {
		msg := fmt.Sprintf("not found in bitmex instruments map for %v", key)
		return nil, errors.New(msg)
	}
	return data, nil
}

func (bitmex *Bitmex) GetInstrument(currencyPair CurrencyPair, contractType string) (*BitmexInstrument, error) {
	key := instrumentsMapKey{
		CurrencyPair: currencyPair,
		ContractType: contractType,
	}
	return bitmex.getInstrumentByKey(key)
}

// func (bitmex *Bitmex) GetContractID(currencyPair CurrencyPair, contractType string) (string, error) {
// 	fallback := ""
// 	contract, err := bitmex.GetContract(currencyPair, contractType)
// 	if err != nil {
// 		return fallback, err
// 	}
// 	return contract.InstrumentID, nil
// }

// func (bitmex *Bitmex) ParseContractID(contractID string) (CurrencyPair, string, error) {
// 	contract, err := bitmex.getContractByID(contractID)
// 	if err != nil {
// 		return UNKNOWN_PAIR, "", err
// 	}
// 	currencyA := NewCurrency(contract.UnderlyingIndex, "")
// 	currencyB := NewCurrency(contract.QuoteCurrency, "")
// 	return NewCurrencyPair(currencyA, currencyB), contract.Alias, nil
// }

// func (bitmex *Bitmex) getContractByID(instrumentID string) (*BitmexInstrument, error) {
// 	bitmex.instrumentsRW.RLock()
// 	defer bitmex.instrumentsRW.RUnlock()
// 	data, ok := bitmex.contractsIDMap[instrumentID]
// 	if !ok {
// 		msg := fmt.Sprintf("no contract in okex instruments with id %s", instrumentID)
// 		return nil, errors.New(msg)
// 	}
// 	return data, nil
// }

func (bitmex *Bitmex) startUpdateInstrumentsLoop() {
	interval := 5 * time.Minute
	go func() {
		for {
			time.Sleep(interval)
			instruments, err := bitmex.getAllInstruments()
			if err == nil {
				bitmex.setInstruments(instruments)
			}
		}
	}()
}

func (bitmex *Bitmex) GetExchangeName() string {
	return BITMEX
}

func (bitmex *Bitmex) getSign(method, rawurl, expires, body string) (string, error) {
	return getSign(bitmex.config.ApiSecretKey, method, rawurl, expires, body)
}

func (bitmex *Bitmex) getSignedHTTPHeader(method, rawurl string) (map[string]string, error) {
	expires := getExpires()
	sign, err := bitmex.getSign(method, rawurl, expires, "")
	if err != nil {
		return nil, err
	}

	header := map[string]string{
		"Content-Type": "application/json",
	}
	if bitmex.config.ApiKey != "" && bitmex.config.ApiSecretKey != "" {
		header["API-KEY"] = bitmex.config.ApiKey
		header["API-SIGNATURE"] = sign
		header["API-EXPIRES"] = expires
	}
	return header, nil
}

func (bitmex *Bitmex) getSignedHTTPHeader2(method, rawurl, body string) (map[string]string, error) {
	expires := getExpires()
	sign, err := bitmex.getSign(method, rawurl, expires, body)
	if err != nil {
		return nil, err
	}

	header := map[string]string{
		"Content-Type": "application/json",
	}
	if bitmex.config.ApiKey != "" && bitmex.config.ApiSecretKey != "" {
		header["API-KEY"] = bitmex.config.ApiKey
		header["API-SIGNATURE"] = sign
		header["API-EXPIRES"] = expires
	}
	return header, nil
}

func (bitmex *Bitmex) getSignedHTTPHeader3(method, rawurl string, postData map[string]string) (map[string]string, error) {
	body, _ := json.Marshal(postData)
	return bitmex.getSignedHTTPHeader2(method, rawurl, string(body))
}

func (bitmex *Bitmex) getFutureInstruments() ([]BitmexInstrument, error) {
	url := bitmex.GetUrlRoot() + ACTIVE_INSTRUMENT_PATH
	headers, err := bitmex.getSignedHTTPHeader("GET", url)
	if err != nil {
		return nil, err
	}

	body, err := HttpGet5(bitmex.config.HttpClient, url, headers)
	if err != nil {
		return nil, err
	}
	instruments := []BitmexInstrument{}
	err = json.Unmarshal(body, &instruments)
	if err != nil {
		return nil, err
	}
	return instruments, nil
}

func (bitmex *Bitmex) getTimeFromRespHeader(header http.Header) (time.Time, error) {
	date := header.Get("date")
	return time.Parse("Mon, 02 Jan 2006 15:04:05 GMT", date)
}

func (bitmex *Bitmex) GetFutureDepth(currencyPair CurrencyPair, contractType string, size int) (*Depth, error) {
	var fallback *Depth
	rawurl := bitmex.GetUrlRoot() + ORDER_BOOK_L2_PATH
	symbol, err := adaptBixmexSymbol(currencyPair, contractType)
	if err != nil {
		return fallback, err
	}
	values := url.Values{}
	values.Add("symbol", symbol)
	values.Add("depth", strconv.Itoa(size))
	rawurl = rawurl + "?" + values.Encode()
	headers, err := bitmex.getSignedHTTPHeader("GET", rawurl)
	if err != nil {
		return fallback, err
	}

	body, respHeader, err := gladius.HttpGet5(bitmex.config.HttpClient, rawurl, headers)
	if err != nil {
		return nil, err
	}

	var depths []BitmexDepth
	err = json.Unmarshal(body, &depths)
	if err != nil {
		return nil, err
	}
	depth, err := bitmex.adapter.AdaptDepth(depths, size)
	if err != nil {
		return nil, err
	}
	date, err := bitmex.getTimeFromRespHeader(respHeader)
	if err != nil {
		return nil, err
	}
	depth.UTime = date
	return depth, nil
}

func (bitmex *Bitmex) GetFutureTicker(currencyPair CurrencyPair, contractType string) (*Ticker, error) {
	return nil, errors.New("not supported")
}

func (bitmex *Bitmex) PlaceFutureOrder(currencyPair CurrencyPair, contractType, price, amount string, openType, matchPrice, leverRate int) (string, error) {
	var requestURL string
	requestURL = bitmex.GetUrlRoot() + ORDER_PATH

	instrument, err := bitmex.GetInstrument(currencyPair, contractType)
	if err != nil {
		return "", err
	}
	symbol := instrument.Symbol

	price, err = instrument.NormalizePriceString(price)
	if err != nil {
		return "", err
	}

	amount, err = instrument.NormalizeAmountString(amount)
	if err != nil {
		return "", err
	}

	postData := make(map[string]string)
	postData["symbol"] = symbol
	postData["price"] = price
	postData["orderQty"] = amount
	switch openType {
	case OPEN_BUY:
		postData["side"] = "Buy"
	case OPEN_SELL:
		postData["side"] = "Sell"
	case CLOSE_BUY:
		postData["side"] = "Sell"
	case CLOSE_SELL:
		postData["side"] = "Buy"
	}
	if openType == CLOSE_BUY || openType == CLOSE_SELL {
		postData["execInst"] = "ReduceOnly"
	}

	headers, err := bitmex.getSignedHTTPHeader3("POST", requestURL, postData)
	if err != nil {
		return "", err
	}

	body, err := HttpPostForm4(bitmex.config.HttpClient, requestURL, postData, headers)

	if err != nil {
		return "", err
	}

	order := new(BitmexOrder)
	err = json.Unmarshal(body, &order)
	if err != nil {
		return "", err
	}
	return order.OrderID, nil
}

func (bitmex *Bitmex) FutureCancelOrder(currencyPair CurrencyPair, contractType, orderID string) (bool, error) {
	var requestURL string

	requestURL = bitmex.GetUrlRoot() + ORDER_PATH

	instrument, err := bitmex.GetInstrument(currencyPair, contractType)
	if err != nil {
		return false, err
	}
	symbol := instrument.Symbol

	postData := url.Values{}
	postData.Add("orderID", orderID)
	postData.Add("symbol", symbol)

	headers, err := bitmex.getSignedHTTPHeader2("DELETE", requestURL, postData.Encode())
	if err != nil {
		return false, err
	}

	body, err := HttpDeleteForm(bitmex.config.HttpClient, requestURL, postData, headers)

	if err != nil {
		return false, err
	}

	orders := make([]BitmexOrder, 0)
	err = json.Unmarshal(body, &orders)
	if err != nil {
		return false, err
	}

	for _, order := range orders {
		if order.OrderID == orderID {
			return true, nil
		}
	}
	return false, fmt.Errorf("can't found order %s", orderID)
}

func (bitmex *Bitmex) GetFutureOrder(orderID string, currencyPair CurrencyPair, contractType string) (*FutureOrder, error) {
	var requestURL string
	requestURL = bitmex.GetUrlRoot() + ORDER_PATH

	instrument, err := bitmex.GetInstrument(currencyPair, contractType)
	if err != nil {
		return nil, err
	}
	symbol := instrument.Symbol

	postData := url.Values{}
	filter, _ := json.Marshal(map[string]string{"orderID": orderID})
	postData.Add("filter", string(filter))
	postData.Add("symbol", symbol)

	requestURL = requestURL + "?" + postData.Encode()

	headers, err := bitmex.getSignedHTTPHeader("GET", requestURL)
	if err != nil {
		return nil, err
	}

	body, err := HttpGet5(bitmex.config.HttpClient, requestURL, headers)

	if err != nil {
		return nil, err
	}

	orders := make([]BitmexOrder, 0)
	err = json.Unmarshal(body, &orders)
	if err != nil {
		return nil, err
	}

	for _, order := range orders {
		if order.OrderID == orderID {
			o, _, err := bitmex.adapter.AdaptOrder(&order)
			return o, err
		}
	}
	return nil, fmt.Errorf("no order found for orderID: %s", orderID)
}

func (bitmex *Bitmex) GetFutureOrders(orderIDs []string, currencyPair CurrencyPair, contractType string) ([]FutureOrder, error) {
	var requestURL string
	requestURL = bitmex.GetUrlRoot() + ORDER_PATH

	instrument, err := bitmex.GetInstrument(currencyPair, contractType)
	if err != nil {
		return nil, err
	}
	symbol := instrument.Symbol

	postData := url.Values{}
	// filter, _ := json.Marshal(map[string]string{"orderID": strings.Join(orderIDs, ",")})
	// postData.Add("filter", string(filter))
	postData.Add("symbol", symbol)
	postData.Add("count", "500")
	postData.Add("reverse", "true")

	requestURL = requestURL + "?" + postData.Encode()

	headers, err := bitmex.getSignedHTTPHeader("GET", requestURL)
	if err != nil {
		return nil, err
	}

	body, err := HttpGet5(bitmex.config.HttpClient, requestURL, headers)

	if err != nil {
		return nil, err
	}

	bitmexOrders := make([]BitmexOrder, 0)
	err = json.Unmarshal(body, &bitmexOrders)
	if err != nil {
		return nil, err
	}

	orderSet := mapset.NewThreadUnsafeSet()
	for _, oid := range orderIDs {
		orderSet.Add(oid)
	}

	orders := make([]FutureOrder, 0, len(bitmexOrders))
	for _, order := range bitmexOrders {
		if orderSet.Contains(order.OrderID) {
			o, _, err := bitmex.adapter.AdaptOrder(&order)
			if err != nil {
				log.Println(err)
				continue
			}
			orders = append(orders, *o)
		}
	}
	return orders, nil
}

func (bitmex *Bitmex) GetUnfinishFutureOrders(currencyPair CurrencyPair, contractType string) ([]FutureOrder, error) {
	var requestURL string
	requestURL = bitmex.GetUrlRoot() + ORDER_PATH

	instrument, err := bitmex.GetInstrument(currencyPair, contractType)
	if err != nil {
		return nil, err
	}
	symbol := instrument.Symbol

	postData := url.Values{}
	filter, _ := json.Marshal(map[string]interface{}{"open": true})
	postData.Add("filter", string(filter))
	postData.Add("symbol", symbol)
	postData.Add("reverse", "true")
	postData.Add("count", "500")

	requestURL = requestURL + "?" + postData.Encode()

	headers, err := bitmex.getSignedHTTPHeader("GET", requestURL)
	if err != nil {
		return nil, err
	}

	body, err := HttpGet5(bitmex.config.HttpClient, requestURL, headers)

	if err != nil {
		return nil, err
	}

	bitmexOrders := make([]BitmexOrder, 0)
	err = json.Unmarshal(body, &bitmexOrders)
	if err != nil {
		return nil, err
	}

	orders := make([]FutureOrder, 0, len(bitmexOrders))
	for _, order := range bitmexOrders {
		o, _, err := bitmex.adapter.AdaptOrder(&order)
		if err != nil {
			log.Println(err)
			continue
		}
		orders = append(orders, *o)
	}
	return orders, nil
}

func (bitmex *Bitmex) PlaceFutureOrder2(currencyPair CurrencyPair, contractType, price, amount string, orderType, openType, matchPrice, leverRate int) (string, error) {
	var requestURL string
	requestURL = bitmex.GetUrlRoot() + ORDER_PATH

	instrument, err := bitmex.GetInstrument(currencyPair, contractType)
	if err != nil {
		return "", err
	}
	symbol := instrument.Symbol

	price, err = instrument.NormalizePriceString(price)
	if err != nil {
		return "", err
	}

	amount, err = instrument.NormalizeAmountString(amount)
	if err != nil {
		return "", err
	}

	postData := make(map[string]string)
	postData["symbol"] = symbol
	postData["price"] = price
	postData["orderQty"] = amount
	switch openType {
	case OPEN_BUY:
		postData["side"] = "Buy"
	case OPEN_SELL:
		postData["side"] = "Sell"
	case CLOSE_BUY:
		postData["side"] = "Sell"
	case CLOSE_SELL:
		postData["side"] = "Buy"
	}
	switch orderType {
	case ORDER_TYPE_LIMIT:
		postData["ordType"] = "Limit"
	case ORDER_TYPE_MARKET:
		postData["ordType"] = "Market"
	case ORDER_TYPE_FAK:
		postData["timeInForce"] = "ImmediateOrCancel"
	case ORDER_TYPE_FOK:
		postData["timeInForce"] = "FillOrKill"
	case ORDER_TYPE_POST_ONLY:
		return "", errors.New("post only order type is not support in bitmex")
	}

	if openType == CLOSE_BUY || openType == CLOSE_SELL {
		postData["execInst"] = "ReduceOnly"
	}

	headers, err := bitmex.getSignedHTTPHeader3("POST", requestURL, postData)
	if err != nil {
		return "", err
	}

	body, err := HttpPostForm4(bitmex.config.HttpClient, requestURL, postData, headers)

	if err != nil {
		return "", err
	}

	order := new(BitmexOrder)
	err = json.Unmarshal(body, &order)
	if err != nil {
		return "", err
	}
	return order.OrderID, nil
}

func (bitmex *Bitmex) GetDeliveryTime() (int, int, int, int) {
	return 4, 20, 0, 0 // 周五8点
}

func (bitmex *Bitmex) GetContractValue(currencyPair CurrencyPair) (float64, error) {
	// seems contractType should be one of the function parameters.
	return 1, nil //TODO: 与品种相关
}

func (bitmex *Bitmex) GetFee() (float64, error) {
	return 0.025, nil //大部分taker费率
}

func (bitmex *Bitmex) GetFutureEstimatedPrice(currencyPair CurrencyPair) (float64, error) {
	return 0, errors.New("not supported yet")
}

func (bitmex *Bitmex) GetFuturePosition(currencyPair CurrencyPair, contractType string) ([]FuturePosition, error) {
	return nil, errors.New("not supported yet")
}

var KlineTypeBinSizeMap = map[int]string{
	KLINE_PERIOD_1MIN: "1m",
	KLINE_PERIOD_5MIN: "5m",
	KLINE_PERIOD_1H:   "1h",
	KLINE_PERIOD_1DAY: "1d",
}

func (bitmex *Bitmex) mergeKlineRecords(klines [][]*FutureKline) []FutureKline {
	ret := make([]FutureKline, 0)
	for _, kline := range klines {
		for _, k := range kline {
			ret = append(ret, *k)
		}
	}
	return ret
}

func _reverse(s interface{}) {
	n := reflect.ValueOf(s).Len()
	swap := reflect.Swapper(s)
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		swap(i, j)
	}
}

// Okex等接口返回的都是kline的开始时间，此处需要统一转换，此函数求k线的时间戳偏移(seconds)
func (bitmex *Bitmex) getKlineTimestampOffset(binSize string) int64 {
	switch binSize {
	case "1m":
		return 60
	case "5m":
		return 5 * 60
	case "1h":
		return 60 * 60
	case "1d":
		return 24 * 60 * 60
	}
	return 0
}

func (bitmex *Bitmex) getKlineRecords(contractType string, currencyPair CurrencyPair, binSize string, count, start int, partial, reverse bool, startTime, endTime *time.Time) ([]*FutureKline, error) {
	var fallback []*FutureKline

	requestURL := bitmex.GetUrlRoot() + OHLCV_PATH

	params := url.Values{}
	params.Set("binSize", binSize)
	params.Set("count", strconv.Itoa(count))
	params.Set("start", strconv.Itoa(start))
	if partial {
		params.Set("partial", "true") //和其他交易所接口保持一致，取得不完整的当前K线
	}
	if reverse {
		params.Set("reverse", "true")
	}
	if startTime != nil {
		params.Set("startTime", startTime.Format(time.RFC3339))
	}
	if endTime != nil {
		params.Set("endTime", endTime.Format(time.RFC3339))
	}

	instrument, err := bitmex.GetInstrument(currencyPair, contractType)
	if err != nil {
		return fallback, err
	}
	params.Set("symbol", instrument.Symbol)

	requestURL = requestURL + "?" + params.Encode()
	// log.Println(requestURL)

	headers, err := bitmex.getSignedHTTPHeader("GET", requestURL)
	if err != nil {
		return fallback, err
	}

	body, err := HttpGet5(bitmex.config.HttpClient, requestURL, headers)

	if err != nil {
		return fallback, err
	}

	var klines []BitmexOHLCVData
	err = json.Unmarshal(body, &klines)
	if err != nil {
		return fallback, err
	}

	var klineRecords []*FutureKline
	for _, v := range klines {
		r := new(FutureKline)
		r.Kline = new(Kline)
		r.Pair = currencyPair
		r.Timestamp, _ = timeStringToInt64(v.Timestamp)
		r.Timestamp = r.Timestamp / 1000 // miliseconds to seconds
		r.Timestamp = r.Timestamp - bitmex.getKlineTimestampOffset(binSize)
		r.Open = v.Open
		r.High = v.High
		r.Low = v.Low
		r.Close = v.Close
		r.Vol = v.Volume
		klineRecords = append(klineRecords, r)
	}
	if reverse {
		_reverse(klineRecords)
	}
	return klineRecords, nil
}

type getKlineRequestData struct {
	binSize   string
	count     int
	start     int
	partial   bool
	reverse   bool
	startTime *time.Time
	endTime   *time.Time
}

// TODO: 取750根以上数据的时候可能有偶发的数据重复bug,问题可能在于交易所，请慎用。
func (bitmex *Bitmex) GetKlineRecords(contractType string, currencyPair CurrencyPair, period, size, since int) ([]FutureKline, error) {
	var fallback []FutureKline

	maxSize := 750

	binSize, ok := KlineTypeBinSizeMap[period]
	if !ok {
		return nil, fmt.Errorf("invalid kline period for bitmex %d", period)
	}

	params := make([]*getKlineRequestData, 0)

	if size <= 0 {
		return nil, errors.New("size of kline must > 0")
	}

	var unfinishes []*FutureKline

	if since > 0 {
		startTime := int64ToTime(int64(since))
		for start, left := 0, size; left > 0; {
			var count int
			if left > maxSize {
				count = maxSize
			} else {
				count = left
			}
			params = append(params, &getKlineRequestData{
				binSize,
				count,
				start,
				true,
				false,
				&startTime,
				nil,
			})
			start += count
			left -= count
		}
	} else {
		if size <= maxSize {
			params = append(params, &getKlineRequestData{
				binSize,
				size,
				0,
				true,
				true,
				nil,
				nil,
			})
		} else {
			// bitmex的kline接口在partial=true&reverse=true且endTime!=nil&start!=0并发请求的时候会有bug
			// 返回的数据第一条可能是最新的未完成bar
			// 通过多加一次额外请求解决该问题
			// 单独拿未完成bar, 顺便拿到交易所时间
			unfinishes, err := bitmex.getKlineRecords(
				contractType, currencyPair, binSize,
				1, 0, true, true, nil, nil)
			if err != nil {
				return fallback, err
			}
			if len(unfinishes) != 1 {
				return fallback, errors.New("length of unfinished bar should be 1")
			}
			endTime := int64ToTime(unfinishes[0].Timestamp * 1000) // 当前bar的开始时间，也就是前一根bar结束时间。
			// 取size个已完成bar
			for start, left := 0, size; left > 0; {
				var count int
				if left > maxSize {
					count = maxSize
				} else {
					count = left
				}
				params = append(params, &getKlineRequestData{
					binSize,
					count,
					start,
					false,
					true,
					nil,
					&endTime,
				})
				start += count
				left -= count
			}
		}
		_reverse(params)
	}
	lock := &sync.Mutex{}
	klinesSlice := make([][]*FutureKline, len(params))
	var err error
	wg := &sync.WaitGroup{}
	wg.Add(len(params))
	for i := 0; i < len(params); i++ {
		go func(index int) {
			defer wg.Done()
			v := params[index]
			klines, err2 := bitmex.getKlineRecords(
				contractType, currencyPair, v.binSize, v.count, v.start, v.partial, v.reverse, v.startTime, v.endTime)
			lock.Lock()
			defer lock.Unlock()
			if err2 != nil {
				if err == nil {
					err = err2
				}
				log.Println("error when get kline: ", err2)
			}
			klinesSlice[index] = klines
		}(i)
	}
	wg.Wait()
	if err != nil {
		return fallback, err
	}
	if unfinishes != nil {
		klinesSlice = append(klinesSlice, unfinishes)
	}
	klines := bitmex.mergeKlineRecords(klinesSlice)
	l := len(klines)
	if l == size+1 {
		//未完成k线和最新的已完成k线是同一根
		if klines[l-1].Timestamp == klines[l-2].Timestamp {
			return klines[:l-1], nil
		} else {
			//未完成k线和最新的已完成k线不是同一根，之前多取了一根已完成k线
			return klines[1:], nil
		}
	}
	return klines, nil
}

func (bitmex *Bitmex) GetTrades(contractType string, currencyPair CurrencyPair, since int64) ([]Trade, error) {
	return nil, errors.New("not supported yet")
}

func (bitmex *Bitmex) GetFutureIndex(currencyPair CurrencyPair) (float64, error) {
	return 0, errors.New("not supported yet")
}

func (bitmex *Bitmex) getAllCurrencies() []string {
	bitmex.instrumentsRW.RLock()
	defer bitmex.instrumentsRW.RUnlock()

	set := make(map[string]string)
	for _, v := range bitmex.instrumentsMap {
		currency := strings.ToUpper(v.Underlying)
		set[currency] = currency
	}
	currencies := make([]string, 0, len(set))
	for _, v := range set {
		currencies = append(currencies, v)
	}
	return currencies
}

func (bitmex *Bitmex) GetFutureUserinfo() (*FutureAccount, error) {
	return nil, errors.New("not supported yet")
}
