package gladius

import (
	"testing"

	"github.com/nntaoli-project/GoEx"
	"github.com/stretchr/testify/assert"
)

var received = make(map[EventType]int)

func onStop() {
	received[EVENT_STOP]++
}

func onOrder(order *Order) {
	received[EVENT_ORDER]++
}

func onTrade(trade *goex.Trade) {
	received[EVENT_TRADE]++
}

func onAccount(account *goex.Account) {
	received[EVENT_ACCOUNT]++
}

func TestEventEngine(t *testing.T) {
	assert := assert.New(t)
	engine := NewEventEngine(100000)
	engine.Register(EVENT_STOP, NewStopEventHandler(onStop))
	engine.Register(EVENT_ORDER, NewOrderEventHandler(onOrder))
	engine.Register(EVENT_TRADE, NewTradeEventHandler(onTrade))
	engine.Register(EVENT_ACCOUNT, NewAccountEventHandler(onAccount))
	engine.Start()
	engine.ProcessData(EVENT_ORDER, &Order{})
	engine.ProcessData(EVENT_TRADE, &goex.Trade{})
	engine.ProcessData(EVENT_ACCOUNT, &goex.Account{})
	engine.Stop()
	assert.Equal(received[EVENT_STOP], 1, "Didn't receive Stop event")
	assert.Equal(received[EVENT_ORDER], 1, "Didn't receive Order event")
	assert.Equal(received[EVENT_TRADE], 1, "Didn't receive Trade event")
	assert.Equal(received[EVENT_ACCOUNT], 1, "Didn't receive Account event")
}
