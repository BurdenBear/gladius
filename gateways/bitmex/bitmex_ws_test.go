package bitmex

import (
	"fmt"
	"log"
	"sync"
	"testing"

	"github.com/nntaoli-project/GoEx"
	"github.com/stretchr/testify/assert"
)

func newBitmexWs() *BitmexWs {
	bitmexWs := NewBitmexWs()
	bitmexWs.
		WsUrl("wss://testnet.bitmex.com/realtime").
		ProxyUrl("socks5://127.0.0.1:10808").
		ErrorHandleFunc(func(err error) {
			log.Println(err)
		})
	return bitmexWs
}

var (
	bitmexWs = newBitmexWs()
)

func TestBitmexWsDepthCallback(t *testing.T) {
	n := 10
	depths := make([]goex.Depth, 0)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	bitmexWs.DepthCallback(func(depth *goex.Depth) {
		if len(depths) < n {
			t.Log(depth)
			assert.True(t, checkDepth(depth))
			depths = append(depths, *depth)
			if len(depths) == n {
				wg.Done()
			}
		}
	})
	// bitmexWs.SubscribeDepth(goex.BTC_USD, goex.QUARTER_CONTRACT, 5)
	bitmexWs.SubscribeDepth(goex.BTC_USD, goex.SWAP_CONTRACT, 5)
	wg.Wait()
}

// func TestBitmexWsTickerCallback(t *testing.T) {
// 	n := 10
// 	tickers := make([]goex.FutureTicker, 0)
// 	wg := &sync.WaitGroup{}
// 	wg.Add(1)
// 	bitmexWs.TickerCallback(func(ticker *goex.FutureTicker) {
// 		t.Log(ticker, ticker.Ticker)
// 		if len(tickers) < n {
// 			tickers = append(tickers, *ticker)
// 			if len(tickers) == n {
// 				wg.Done()
// 			}
// 		}
// 	})
// 	bitmexWs.SubscribeTicker(goex.BTC_USD, goex.QUARTER_CONTRACT)
// 	bitmexWs.SubscribeTicker(goex.BTC_USD, goex.SWAP_CONTRACT)
// 	wg.Wait()
// }

func TestBitmexWsTradeCallback(t *testing.T) {
	n := 5
	trades := make([]goex.Trade, 0)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	bitmexWs.TradeCallback(func(trade *goex.Trade, contractType string) {
		if len(trades) < n {
			t.Log(contractType, trade)
			trades = append(trades, *trade)
			if len(trades) == n {
				wg.Done()
			}
		}
	})
	bitmexWs.SubscribeTrade(goex.BTC_USD, goex.QUARTER_CONTRACT)
	bitmexWs.SubscribeTrade(goex.BTC_USD, goex.SWAP_CONTRACT)
	wg.Wait()
}

func TestBitmexWsLogin(t *testing.T) {
	if authed {
		config := GetDefaultBitmexConfig()
		bitmexWs := newBitmexWs()
		err := bitmexWs.Login("", config.ApiSecretKey, "") // fail
		assert.True(t, err != nil)
		bitmexWs = newBitmexWs()
		err = bitmexWs.Login(config.ApiKey, config.ApiSecretKey, "") //succes
		assert.Nil(t, err)
		err = bitmexWs.Login(config.ApiKey, config.ApiSecretKey, "") //duplicate login
		assert.Nil(t, err)
	} else {
		t.Log("not authed, skip test websocket login")
	}
}

func placeAndCancel(currencyPair goex.CurrencyPair, contractType string) {
	leverage := 20
	depth := 25
	dep, _ := bitmex.GetFutureDepth(currencyPair, contractType, depth)
	price := fmt.Sprintf("%f", dep.BidList[depth-1].Price)
	instrument, _ := bitmex.GetInstrument(currencyPair, contractType)
	amount := fmt.Sprintf("%f", instrument.LotSize)
	orderID, err := bitmex.PlaceFutureOrder(
		currencyPair, contractType, price, amount, goex.OPEN_BUY, 0, leverage)
	if err != nil {
		log.Println(err)
	}
	_, err = bitmex.FutureCancelOrder(currencyPair, contractType, orderID)
	if err != nil {
		log.Println(err)
	}
}

func TestBitmexWsOrderCallback(t *testing.T) {
	if authed {
		config := GetDefaultBitmexConfig()
		err := bitmexWs.Login(config.ApiKey, config.ApiSecretKey, "")
		assert.Nil(t, err)
		n := 4
		orders := make([]goex.FutureOrder, 0)
		wg := &sync.WaitGroup{}
		wg.Add(1)
		bitmexWs.OrderCallback(func(order *goex.FutureOrder, contractType string) {
			if len(orders) < n {
				t.Log(contractType, order)
				orders = append(orders, *order)
				if len(orders) == n {
					wg.Done()
				}
			}
		})
		err = bitmexWs.SubscribeOrder(goex.BTC_USD, goex.QUARTER_CONTRACT)
		assert.Nil(t, err)
		err = bitmexWs.SubscribeOrder(goex.BTC_USD, goex.SWAP_CONTRACT)
		assert.Nil(t, err)
		placeAndCancel(goex.BTC_USD, goex.QUARTER_CONTRACT)
		placeAndCancel(goex.BTC_USD, goex.SWAP_CONTRACT)
		wg.Wait()
	}
}
