package bitmex

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/BurdenBear/gladius"
	goex "github.com/nntaoli-project/GoEx"
	"github.com/stretchr/testify/assert"
)

func checkGatewayDepth(dep *Depth) bool {
	for i := 0; i < len(dep.AskList)-1; i++ {
		if dep.AskList[i+1].Price < dep.AskList[i].Price {
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

func TestBitmexGatewayDepth(t *testing.T) {
	engine := NewEventEngine(100000)

	symbols := []string{
		GetCryptoCurrencyContractSymbol(goex.BTC_USD, goex.SWAP_CONTRACT),
		GetCryptoCurrencyContractSymbol(goex.BTC_USD, goex.QUARTER_CONTRACT),
	}

	config := *GetDefaultBitmexGatewayConfig() // copy
	config.Secret.APIKey = ""
	config.Secret.APISecretKey = ""
	config.Secret.Passphrase = ""
	config.Symbols = symbols
	config.HTTP.Timeout = 10
	config.HTTP.Proxy = "socks5://localhost:10808"
	Bitmex, err := NewBitmexGateway("Bitmex", engine, &config)
	assert.Nil(t, err)
	wait := 10
	ch := make(chan interface{}, wait)
	engine.Register(EVENT_DEPTH, NewDepthEventHandler(
		func(depth *Depth) {
			t.Log(depth.Contract, depth.Time, depth.AskList, depth.BidList)
			assert.True(t, checkGatewayDepth(depth))
			ch <- nil
		}))
	engine.Start()
	err = Bitmex.Connect()
	assert.Nil(t, err)
	for i := 0; i < wait; i++ {
		<-ch
	}
	// time.Sleep(3 * time.Second)
}

func placeAndCancelFromGateway(t *testing.T, gateway *BitmexGateway, dep *Depth) {
	leverage := 20
	depth := 10
	price := fmt.Sprintf("%f", dep.BidList[depth-1].Price)
	amount := "1"
	symbol := dep.Contract.GetSymbol()
	logger.Debugf("sending order of %s", symbol)
	orderID, err := gateway.PlaceOrder(symbol, price, amount,
		OT_LIMIT, OO_BUY, leverage)
	assert.Nil(t, err)
	logger.Debugf("sent order %s of %s", orderID, symbol)
	logger.Debugf("cancelling order %s", orderID)
	gateway.CancelOrder(symbol, orderID)
	logger.Debugf("cancelled order %s", orderID)
}

func TestBitmexGatewayOrder(t *testing.T) {
	engine := NewEventEngine(100000)

	symbols := []string{
		GetCryptoCurrencyContractSymbol(goex.BTC_USD, goex.SWAP_CONTRACT),
		GetCryptoCurrencyContractSymbol(goex.BTC_USD, goex.QUARTER_CONTRACT),
	}
	onces := map[string]*sync.Once{
		symbols[0]: &sync.Once{},
		symbols[1]: &sync.Once{},
	}
	config := *GetDefaultBitmexGatewayConfig() // copy
	config.Symbols = symbols
	config.HTTP.Timeout = 10
	config.HTTP.Proxy = "socks5://localhost:10808"
	Bitmex, err := NewBitmexGateway("BITMEX", engine, &config)
	assert.Nil(t, err)
	wait := 1
	ch := make(chan interface{}, wait)
	engine.Register(EVENT_DEPTH, NewDepthEventHandler(
		func(depth *Depth) {
			symbol := depth.Contract.GetSymbol()
			onces[symbol].Do(
				func() {
					placeAndCancelFromGateway(t, Bitmex, depth)
				})
		}))
	engine.Register(EVENT_ORDER, NewOrderEventHandler(
		func(order *Order) {
			logger.Debugf("%+v", order)
			if order.IsFinished() {
				ch <- nil
			}
		}))
	engine.Start()
	err = Bitmex.Connect()
	assert.Nil(t, err)
	for i := 0; i < wait; i++ {
		<-ch
	}
}
