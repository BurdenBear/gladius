package hbdm

import (
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/nntaoli-project/GoEx"
	"github.com/stretchr/testify/assert"
)

var (
	hbdm   = NewHbdmRestAPI(GetDefaultHbdmConfig())
	authed = len(hbdm.config.ApiKey) > 0 && len(hbdm.config.ApiSecretKey) > 0
)

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

func TestHbdm_GetFutureDepth(t *testing.T) {
	size := 10
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		dep, err := hbdm.GetFutureDepth(BTC_USD, QUARTER_CONTRACT, size)
		assert.Nil(t, err)
		t.Log(dep)
		assert.True(t, checkDepth(dep))
	}()
	go func() {
		defer wg.Done()
		dep, err := hbdm.GetFutureDepth(BTC_USD, THIS_WEEK_CONTRACT, size)
		assert.Nil(t, err)
		t.Log(dep)
		assert.True(t, checkDepth(dep))
	}()
	wg.Wait()
}

func TestHbdm_GetFutureTicker(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		ticker, err := hbdm.GetFutureTicker(BTC_USD, QUARTER_CONTRACT)
		assert.Nil(t, err)
		t.Log(ticker)
	}()
	go func() {
		defer wg.Done()
		ticker, err := hbdm.GetFutureTicker(BTC_USD, THIS_WEEK_CONTRACT)
		assert.Nil(t, err)
		t.Log(ticker)
	}()
	wg.Wait()
}

func testPlaceAndCancel(t *testing.T, currencyPair CurrencyPair, contractType string) {
	// 以100档对手价下买单然后马上撤掉
	leverage := 20
	depth := 100
	dep, err := hbdm.GetFutureDepth(currencyPair, contractType, depth)
	assert.Nil(t, err)
	price := fmt.Sprintf("%f", dep.BidList[depth-1].Price)
	amount := "1"
	orderID, err := hbdm.PlaceFutureOrder(
		currencyPair, contractType, price, amount, OPEN_BUY, 0, leverage)
	assert.Nil(t, err)
	t.Log(orderID)
	order, err := hbdm.GetFutureOrder(orderID, currencyPair, contractType)
	assert.Nil(t, err)
	t.Log(order)
	cancelled, err := hbdm.FutureCancelOrder(currencyPair, contractType, orderID)
	assert.Nil(t, err)
	assert.True(t, cancelled)
	order, err = hbdm.GetFutureOrder(orderID, currencyPair, contractType)
	assert.Nil(t, err)
	t.Log(order)
}

func TestHbdm_PlaceAndCancelFutureOrder(t *testing.T) {
	if authed {
		testPlaceAndCancel(t, EOS_USD, QUARTER_CONTRACT)
		testPlaceAndCancel(t, EOS_USD, THIS_WEEK_CONTRACT)
	} else {
		t.Log("not authed, skip test place and cancel future order")
	}
}

func testPlaceAndGetInfo(t *testing.T, currencyPair CurrencyPair, contractType string) {
	leverage := 20
	depth := 100
	dep, err := hbdm.GetFutureDepth(currencyPair, contractType, depth)
	assert.Nil(t, err)
	price := fmt.Sprintf("%f", dep.BidList[depth-1].Price)
	amount := "1"
	orderID1, err := hbdm.PlaceFutureOrder(
		currencyPair, contractType, price, amount, OPEN_BUY, 0, leverage)
	assert.Nil(t, err)
	t.Log(orderID1)
	orderID2, err := hbdm.PlaceFutureOrder(
		currencyPair, contractType, price, amount, OPEN_BUY, 0, leverage)
	assert.Nil(t, err)
	t.Log(orderID2)
	// get info of order1
	order, err := hbdm.GetFutureOrder(orderID1, currencyPair, contractType)
	assert.Nil(t, err)
	t.Log(order)
	order, err = hbdm.GetFutureOrder(orderID2, currencyPair, contractType)
	assert.Nil(t, err)
	t.Log(order)
	// sleep for a while when place order,
	// time.Sleep(1 * time.Second)
	// get infos of order1 and order2
	orders, err := hbdm.GetFutureOrders([]string{orderID1, orderID2}, currencyPair, contractType)
	assert.Nil(t, err)
	assert.True(t, len(orders) == 2)
	t.Log(orders)
	//cancel order1 and order2
	cancelled, err := hbdm.FutureCancelOrder(currencyPair, contractType, orderID1)
	assert.Nil(t, err)
	assert.True(t, cancelled)
	cancelled, err = hbdm.FutureCancelOrder(currencyPair, contractType, orderID2)
	assert.Nil(t, err)
	assert.True(t, cancelled)
	// sleep for a while when cancel order,
	// the order info will be missing in orders api for all states for a short time.
	// time.Sleep(2 * time.Second)
	// get infos of order1 and order2 after cancelling
	orders, err = hbdm.GetFutureOrders([]string{orderID1, orderID2}, currencyPair, contractType)
	assert.Nil(t, err)
	assert.True(t, len(orders) == 2)
	t.Log(orders)
}

func TestHbdm_PlaceAndGetOrdersInfo(t *testing.T) {
	if authed {
		testPlaceAndGetInfo(t, EOS_USD, QUARTER_CONTRACT)
		testPlaceAndGetInfo(t, EOS_USD, THIS_WEEK_CONTRACT)
	} else {
		t.Log("not authed, skip test place future order and get order info")
	}
}

func TestHbdm_GetFutureEstimatedPrice(t *testing.T) {
	f, err := hbdm.GetFutureEstimatedPrice(EOS_USD)
	assert.Nil(t, err)
	t.Log(f)
}

func TestHbdm_GetFee(t *testing.T) {
	f, err := hbdm.GetFee()
	assert.Nil(t, err)
	assert.True(t, f > 0)
	t.Log(f)
}

func testGetContractValue(t *testing.T, currencyPair CurrencyPair) {
	f, err := hbdm.GetContractValue(currencyPair)
	assert.Nil(t, err)
	assert.True(t, f > 0)
	t.Log(f)
}

func TestHbdm_GetContractValue(t *testing.T) {
	testGetContractValue(t, BTC_USD)
	testGetContractValue(t, EOS_USD)
}

func isEqualDiff(klines []FutureKline, seconds int64) bool {
	for i := 0; i < len(klines)-1; i++ {
		diff := klines[i+1].Timestamp - klines[i].Timestamp
		if diff != seconds {
			return false
		}
	}
	return true
}

func testGetKlineRecords(t *testing.T, contractType string, currency CurrencyPair, maxSize int) {
	now := time.Now().UTC()
	timestamp := (now.UnixNano() - 20*int64(time.Hour)) / int64(time.Millisecond)
	size := 10
	period := KLINE_PERIOD_1MIN
	seconds := int64(60)
	kline, err := hbdm.GetKlineRecords(contractType, currency, period, size, 0)
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	kline, err = hbdm.GetKlineRecords(contractType, currency, period, size, int(timestamp))
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	size = maxSize
	kline, err = hbdm.GetKlineRecords(contractType, currency, period, size, 0)
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	kline, err = hbdm.GetKlineRecords(contractType, currency, period, size, int(timestamp))
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	size = 3 * maxSize
	kline, err = hbdm.GetKlineRecords(contractType, currency, period, size, 0)
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	assert.True(t, isEqualDiff(kline, seconds))
	kline, err = hbdm.GetKlineRecords(contractType, currency, period, size, int(timestamp))
	assert.Nil(t, err)
	assert.True(t, len(kline) == size)
	t.Log(len(kline))
	assert.True(t, isEqualDiff(kline, seconds))
}

func TestHbdm_GetKlineRecords(t *testing.T) {
	testGetKlineRecords(t, QUARTER_CONTRACT, EOS_USD, 300)
	testGetKlineRecords(t, THIS_WEEK_CONTRACT, EOS_USD, 200)
}
