package hbdm

import (
	"fmt"
	"log"
	"sync"
	"testing"

	"github.com/nntaoli-project/GoEx"
	"github.com/stretchr/testify/assert"
)

var (
	hbdmWs = NewHbdmWs()
)

func TestHbdmWsTickerCallback(t *testing.T) {
	n := 10
	tickers := make([]goex.FutureTicker, 0)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	hbdmWs.TickerCallback(func(ticker *goex.FutureTicker) {
		t.Log(ticker, ticker.Ticker)
		if len(tickers) <= n {
			tickers = append(tickers, *ticker)
		}
		if len(tickers) == n {
			wg.Done()
		}
	})
	hbdmWs.SubscribeTicker(goex.EOS_USD, goex.QUARTER_CONTRACT)
	hbdmWs.SubscribeTicker(goex.EOS_USD, goex.THIS_WEEK_CONTRACT)
	wg.Wait()
}

func TestHbdmWsDepthCallback(t *testing.T) {
	n := 10
	depths := make([]goex.Depth, 0)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	hbdmWs.DepthCallback(func(depth *goex.Depth) {
		if len(depths) <= n {
			t.Log(len(depth.AskList), depth)
			depths = append(depths, *depth)
		}
		if len(depths) == n {
			wg.Done()
		}
	})
	hbdmWs.SubscribeDepth(goex.EOS_USD, goex.QUARTER_CONTRACT, 5)
	hbdmWs.SubscribeDepth(goex.EOS_USD, goex.THIS_WEEK_CONTRACT, 5)
	wg.Wait()
}

func TestHbdmWsTradeCallback(t *testing.T) {
	n := 10
	trades := make([]goex.Trade, 0)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	hbdmWs.TradeCallback(func(trade *goex.Trade, contractType string) {
		if len(trades) <= n {
			t.Log(contractType, trade)
			trades = append(trades, *trade)
		}
		if len(trades) == n {
			wg.Done()
		}
	})
	hbdmWs.SubscribeTrade(goex.EOS_USD, goex.QUARTER_CONTRACT)
	hbdmWs.SubscribeTrade(goex.EOS_USD, goex.THIS_WEEK_CONTRACT)
	wg.Wait()
}

func TestHbdmWsLogin(t *testing.T) {
	if authed {
		config := GetDefaultHbdmConfig()
		hbdmWs := NewHbdmWs()
		err := hbdmWs.Login("", config.ApiSecretKey, config.ApiPassphrase) // fail
		assert.True(t, err != nil)
		hbdmWs = NewHbdmWs()
		err = hbdmWs.Login(config.ApiKey, config.ApiSecretKey, config.ApiPassphrase) //succes
		assert.Nil(t, err)
		err = hbdmWs.Login(config.ApiKey, config.ApiSecretKey, config.ApiPassphrase) //duplicate login
		assert.Nil(t, err)
	} else {
		t.Log("not authed, skip test websocket login")
	}
}

func placeAndCancel(currencyPair goex.CurrencyPair, contractType string) {
	leverage := 20
	depth := 100
	dep, _ := hbdm.GetFutureDepth(currencyPair, contractType, depth)
	price := fmt.Sprintf("%f", dep.BidList[depth-1].Price)
	amount := "1"
	orderID, err := hbdm.PlaceFutureOrder(
		currencyPair, contractType, price, amount, goex.OPEN_BUY, 0, leverage)
	if err != nil {
		log.Println(err)
	}
	_, err = hbdm.FutureCancelOrder(currencyPair, contractType, orderID)
	if err != nil {
		log.Println(err)
	}
}

func TestHbdmWsOrderCallback(t *testing.T) {
	if authed {
		config := GetDefaultHbdmConfig()
		err := hbdmWs.Login(config.ApiKey, config.ApiSecretKey, config.ApiPassphrase)
		assert.Nil(t, err)
		n := 4
		orders := make([]goex.FutureOrder, 0)
		wg := &sync.WaitGroup{}
		wg.Add(1)
		hbdmWs.OrderCallback(func(order *goex.FutureOrder, contractType string) {
			if len(orders) <= n {
				t.Log(contractType, order)
				orders = append(orders, *order)
			}
			if len(orders) == n {
				wg.Done()
			}
		})
		err = hbdmWs.SubscribeOrder(goex.EOS_USD, goex.QUARTER_CONTRACT)
		assert.Nil(t, err)
		err = hbdmWs.SubscribeOrder(goex.EOS_USD, goex.THIS_WEEK_CONTRACT)
		assert.Nil(t, err)
		placeAndCancel(goex.EOS_USD, goex.QUARTER_CONTRACT)
		placeAndCancel(goex.EOS_USD, goex.THIS_WEEK_CONTRACT)
		wg.Wait()
	}
}
