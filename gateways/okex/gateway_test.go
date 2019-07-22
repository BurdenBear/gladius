package okex

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/BurdenBear/gladius"
	goex "github.com/nntaoli-project/GoEx"
	"github.com/stretchr/testify/assert"
)

var logger = GetLogger()

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

func TestOKExGatewayDepth(t *testing.T) {
	engine := NewEventEngine(100000)

	symbols := []string{
		GetCryptoCurrencyContractSymbol(goex.EOS_USD, goex.SWAP_CONTRACT),
		GetCryptoCurrencyContractSymbol(goex.EOS_USD, goex.QUARTER_CONTRACT),
	}

	config := *GetDefaultOkexGatewayConfig() // copy
	config.Secret.APIKey = ""
	config.Secret.APISecretKey = ""
	config.Secret.Passphrase = ""
	config.Symbols = symbols
	config.Urls.Future.Restful = "https://www.okex.me"
	config.Urls.Future.Websocket = "wss://okexcomreal.bafang.com:10442/ws/v3"
	okex, err := NewOkexGateway("OKEX", engine, &config)
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
	err = okex.Connect()
	assert.Nil(t, err)
	for i := 0; i < wait; i++ {
		<-ch
	}
	// time.Sleep(3 * time.Second)
}

func placeAndCancel(t *testing.T, gateway *OkexGateway, dep *Depth) {
	leverage := 20
	depth := 5
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

func TestOKExGatewayOrder(t *testing.T) {
	engine := NewEventEngine(100000)

	symbols := []string{
		GetCryptoCurrencyContractSymbol(goex.EOS_USD, goex.SWAP_CONTRACT),
		GetCryptoCurrencyContractSymbol(goex.EOS_USD, goex.QUARTER_CONTRACT),
	}
	onces := map[string]*sync.Once{
		symbols[0]: &sync.Once{},
		symbols[1]: &sync.Once{},
	}
	config := *GetDefaultOkexGatewayConfig() // copy
	config.Symbols = symbols
	okex, err := NewOkexGateway("OKEX", engine, &config)
	assert.Nil(t, err)
	wait := 6 // (SUBMITTED, UNFINISHED, CANCELLED) X 2 contract
	ch := make(chan interface{}, wait)
	engine.Register(EVENT_DEPTH, NewDepthEventHandler(
		func(depth *Depth) {
			symbol := depth.Contract.GetSymbol()
			onces[symbol].Do(
				func() {
					placeAndCancel(t, okex, depth)
				})
		}))
	engine.Register(EVENT_ORDER, NewOrderEventHandler(
		func(order *Order) {
			ch <- nil
			logger.Debugf("%v %v", order.Contract, order)
		}))
	engine.Start()
	err = okex.Connect()
	assert.Nil(t, err)
	for i := 0; i < wait; i++ {
		<-ch
	}
}
