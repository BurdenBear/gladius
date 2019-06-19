package okex

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/BurdenBear/gladius"
	goex "github.com/nntaoli-project/GoEx"
	"github.com/stretchr/testify/assert"
)

func TestOKExGatewayDepth(t *testing.T) {
	engine := NewEventEngine(100000)

	symbols := []string{
		GetCryptoCurrencyContractSymbol(goex.EOS_USD, goex.SWAP_CONTRACT),
		GetCryptoCurrencyContractSymbol(goex.EOS_USD, goex.QUARTER_CONTRACT),
	}

	config := *GetDefaultOkexGatewayConfig() // copy
	config.APIKey = ""
	config.APISecretKey = ""
	config.APIPassphrase = ""
	config.Symbols = symbols
	okex := NewOkexGateway("OKEX", engine).Config(&config)
	wg := &sync.WaitGroup{}
	wg.Add(10)
	engine.Register(EVENT_DEPTH, NewDepthEventHandler(
		func(depth *Depth) {
			wg.Done()
			t.Log(depth.Contract, depth.Time, depth.AskList, depth.BidList)
		}))
	engine.Start()
	okex.Connect()
	wg.Wait()
}

func placeAndCancel(t *testing.T, gateway *OkexGateway, dep *Depth) {
	leverage := 20
	depth := 5
	price := fmt.Sprintf("%f", (*dep.BidList)[depth-1].Price)
	amount := "1"
	symbol := dep.Contract.GetSymbol()
	log.Debugf("sending order of %s", symbol)
	orderID, err := gateway.PlaceOrder(symbol, price, amount,
		OT_LIMIT, OO_BUY, leverage)
	assert.Nil(t, err)
	log.Debugf("sent order %s of %s", orderID, symbol)
	log.Debugf("cancelling order %s", orderID)
	gateway.CancelOrder(symbol, orderID)
	log.Debugf("cancelled order %s", orderID)
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
	okex := NewOkexGateway("OKEX", engine).Config(&config)
	wg := &sync.WaitGroup{}
	wg.Add(6) // (SUBMITTED, UNFINISHED, CANCELLED) X 2 contract
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
			wg.Done()
			log.Debugf("%v %v", order.Contract, order)
		}))
	engine.Start()
	okex.Connect()
	wg.Wait()
}