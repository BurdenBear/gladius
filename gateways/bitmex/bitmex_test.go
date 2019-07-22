package bitmex

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"testing"
	"time"

	. "github.com/nntaoli-project/GoEx"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/proxy"
)

var (
	bitmex = NewBitmex(getBitmexConfig())
	authed = len(bitmex.config.ApiKey) > 0 && len(bitmex.config.ApiSecretKey) > 0
)

func getBitmexConfig() *APIConfig {
	config := GetDefaultBitmexConfig()
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:10808", nil, proxy.Direct)
	if err != nil {
		panic(fmt.Errorf("can't connect to the proxy:%s", err))
	}
	// setup a http client
	httpTransport := &http.Transport{}
	httpClient := &http.Client{Transport: httpTransport}
	// set our socks5 as the dialer
	httpTransport.Dial = dialer.Dial
	config.HttpClient = httpClient
	return config
}

func checkDepth(dep *Depth) bool {
	for i := 0; i < len(dep.AskList)-1; i++ {
		if dep.AskList[i+1].Price > dep.AskList[i].Price {
			return false
		}
	}
	for i := 0; i < len(dep.BidList)-1; i++ {
		if dep.BidList[i+1].Price > dep.BidList[i].Price {
			return false
		}
	}
	return true
}

func TestBitmex_GetFutureDepth(t *testing.T) {
	size := 10
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		dep, err := bitmex.GetFutureDepth(BTC_USD, QUARTER_CONTRACT, size)
		assert.Nil(t, err)
		t.Log(dep)
		assert.True(t, checkDepth(dep))
	}()
	go func() {
		defer wg.Done()
		dep, err := bitmex.GetFutureDepth(BTC_USD, SWAP_CONTRACT, size)
		assert.Nil(t, err)
		assert.True(t, checkDepth(dep))
		t.Log(dep)
	}()
	wg.Wait()
}

func testPlaceAndCancel(t *testing.T, currencyPair CurrencyPair, contractType string) {
	// 以100档对手价下买单然后马上撤掉
	leverage := 20
	depth := 100
	dep, err := bitmex.GetFutureDepth(currencyPair, contractType, depth)
	assert.Nil(t, err)
	price := fmt.Sprintf("%f", dep.BidList[depth-1].Price)
	symbol, err := bitmex.GetInstrument(currencyPair, contractType)
	assert.Nil(t, err)
	amount := fmt.Sprintf("%f", symbol.LotSize)
	orderID, err := bitmex.PlaceFutureOrder(
		currencyPair, contractType, price, amount, OPEN_BUY, 0, leverage)
	assert.Nil(t, err)
	t.Log(orderID)
	order, err := bitmex.GetFutureOrder(orderID, currencyPair, contractType)
	assert.Nil(t, err)
	t.Log(order)
	cancelled, err := bitmex.FutureCancelOrder(currencyPair, contractType, orderID)
	assert.Nil(t, err)
	assert.True(t, cancelled)
	order, err = bitmex.GetFutureOrder(orderID, currencyPair, contractType)
	assert.Nil(t, err)
	t.Log(order)
}

func TestBitmex_PlaceAndCancelFutureOrder(t *testing.T) {
	if authed {
		// testPlaceAndCancel(t, BTC_USD, QUARTER_CONTRACT)
		testPlaceAndCancel(t, BTC_USD, SWAP_CONTRACT)
	} else {
		t.Log("not authed, skip test place and cancel future order")
	}
}

func testPlaceAndGetInfo(t *testing.T, currencyPair CurrencyPair, contractType string) {
	leverage := 20
	depth := 100
	dep, err := bitmex.GetFutureDepth(currencyPair, contractType, depth)
	assert.Nil(t, err)
	price := fmt.Sprintf("%f", dep.BidList[depth-1].Price)
	symbol, err := bitmex.GetInstrument(currencyPair, contractType)
	assert.Nil(t, err)
	amount := fmt.Sprintf("%f", symbol.LotSize)
	orderID1, err := bitmex.PlaceFutureOrder(
		currencyPair, contractType, price, amount, OPEN_BUY, 0, leverage)
	assert.Nil(t, err)
	t.Log(orderID1)
	orderID2, err := bitmex.PlaceFutureOrder(
		currencyPair, contractType, price, amount, OPEN_BUY, 0, leverage)
	assert.Nil(t, err)
	t.Log(orderID2)
	// get info of order1
	order, err := bitmex.GetFutureOrder(orderID1, currencyPair, contractType)
	assert.Nil(t, err)
	t.Log(order)
	order, err = bitmex.GetFutureOrder(orderID2, currencyPair, contractType)
	assert.Nil(t, err)
	t.Log(order)
	// sleep for a while when place order,
	time.Sleep(1 * time.Second)
	// get infos of order1 and order2
	orders, err := bitmex.GetFutureOrders([]string{orderID1, orderID2}, currencyPair, contractType)
	assert.Nil(t, err)
	assert.True(t, len(orders) == 2)
	t.Log(orders)
	//cancel order1 and order2
	cancelled, err := bitmex.FutureCancelOrder(currencyPair, contractType, orderID1)
	assert.Nil(t, err)
	assert.True(t, cancelled)
	cancelled, err = bitmex.FutureCancelOrder(currencyPair, contractType, orderID2)
	assert.Nil(t, err)
	assert.True(t, cancelled)
	// sleep for a while when cancel order,
	// the order info will be missing in orders api for all states for a short time.
	time.Sleep(2 * time.Second)
	// get infos of order1 and order2 after cancelling
	orders, err = bitmex.GetFutureOrders([]string{orderID1, orderID2}, currencyPair, contractType)
	assert.Nil(t, err)
	assert.True(t, len(orders) == 2)
	t.Log(orders)
}

func TestBitmex_PlaceAndGetOrdersInfo(t *testing.T) {
	if authed {
		testPlaceAndGetInfo(t, BTC_USD, QUARTER_CONTRACT)
		testPlaceAndGetInfo(t, BTC_USD, SWAP_CONTRACT)
	} else {
		t.Log("not authed, skip test place future order and get order info")
	}
}

func isEqualDiff(klines []FutureKline, seconds int64) bool {
	for i := 0; i < len(klines)-1; i++ {
		diff := klines[i+1].Timestamp - klines[i].Timestamp
		if diff != seconds {
			log.Println(i, *klines[i+1].Kline, *klines[i].Kline, *klines[len(klines)-1].Kline)
			return false
		}
	}
	return true
}

func getTimeRange(kline []FutureKline) (time.Time, time.Time) {
	start := int64ToTime(kline[0].Timestamp * 1000)
	end := int64ToTime(kline[len(kline)-1].Timestamp * 1000)
	return start, end
}

func testGetKlineRecords(t *testing.T, contractType string, currency CurrencyPair, maxSize int) {
	now := time.Now().UTC()
	timestamp := (now.UnixNano() - 48*int64(time.Hour)) / int64(time.Millisecond)
	size := 10
	period := KLINE_PERIOD_1MIN
	seconds := int64(60)
	kline, err := bitmex.GetKlineRecords(contractType, currency, period, size, 0)
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	t.Log(getTimeRange(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	kline, err = bitmex.GetKlineRecords(contractType, currency, period, size, int(timestamp))
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	t.Log(getTimeRange(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	size = maxSize
	kline, err = bitmex.GetKlineRecords(contractType, currency, period, size, 0)
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	t.Log(getTimeRange(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	kline, err = bitmex.GetKlineRecords(contractType, currency, period, size, int(timestamp))
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	t.Log(getTimeRange(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	size = 3 * maxSize
	kline, err = bitmex.GetKlineRecords(contractType, currency, period, size, 0)
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	t.Log(getTimeRange(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	kline, err = bitmex.GetKlineRecords(contractType, currency, period, size, int(timestamp))
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	t.Log(getTimeRange(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	size = 3*maxSize + 1
	kline, err = bitmex.GetKlineRecords(contractType, currency, period, size, 0)
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	t.Log(getTimeRange(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	kline, err = bitmex.GetKlineRecords(contractType, currency, period, size, int(timestamp))
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	t.Log(getTimeRange(kline))
	assert.True(t, isEqualDiff(kline, seconds))
}

func TestBitmex_GetKlineRecords(t *testing.T) {
	testGetKlineRecords(t, QUARTER_CONTRACT, BTC_USD, 750)
	testGetKlineRecords(t, SWAP_CONTRACT, BTC_USD, 750)
}
