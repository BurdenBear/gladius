package main

import (
	"log"
	"os"
	"sync"
	"syscall"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/strategy"
)

var logger = GetLogger()

type Strategy struct {
	*sync.Once
	*strategy.Strategy
	sent bool
}

func (stg *Strategy) OnDepth(depth *Depth) {
	symbol := depth.Contract.GetID()
	if !stg.sent {
		logger.Infof("%s: %+v", symbol, depth)
	}
	if symbol == stg.Symbols[0] {
		stg.sent = true
		stg.Do(func() {
			stg.placeAndCancelFromGateway(depth)
		})
	}
}

func (stg *Strategy) placeAndCancelFromGateway(dep *Depth) {
	leverage := 20
	depth := 10
	price := dep.BidList[depth-1].Price
	amount := 0.0
	symbol := dep.Contract.GetID()
	logger.Debugf("sending order of %s", symbol)
	orderID, err := stg.PlaceOrder(symbol, price, amount,
		OT_LIMIT, OO_BUY, leverage)
	if err != nil {
		logger.Error(err.Error())
	}
	logger.Debugf("sent order %s of %s", orderID, symbol)
	logger.Debugf("cancelling order %s", orderID)
	stg.CancelOrder(symbol, orderID)
	logger.Debugf("cancelled order %s", orderID)
}

func exit() error {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}

func (stg *Strategy) OnOrder(order *Order) {
	logger.Infof("%+v", order)
	if order.IsFinished() {
		exit()
	}
}

func NewStrategy(name string, router *strategy.Router) strategy.IStrategy {
	return &Strategy{Strategy: strategy.NewStrategy(name, router), Once: &sync.Once{}}
}

func main() {
	log.Println("call main")
}
