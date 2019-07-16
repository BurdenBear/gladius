package strategy

import (
	"fmt"

	. "github.com/BurdenBear/gladius"
)

type IStrategy interface {
	Init()
	Start()
	Stop()
	IsRunning() bool
	GetName() string
	GetRouter() *Router
	GetSymbols() []string
	OnDepth(depth *Depth)
	OnOrder(order *Order)
	PlaceOrder(symbol string, price, size float64, orderType OrderType, offset OrderOffset, l int) (string, error)
	CancelOrder(symbol, clOrdID string)
	GetCandles(symbol string, period, size int) ([]Bar, error)
}

type Strategy struct {
	isRunning bool
	name      string
	router    *Router
	Symbols   []string
}

type IStrategyConstructor = func(string, *Router) IStrategy

func NewStrategy(name string, router *Router) *Strategy {
	stg := new(Strategy)
	stg.router = router
	stg.name = name
	return stg
}

func (stg *Strategy) Init() {
}

func (stg *Strategy) Start() {
	stg.isRunning = true
}

func (stg *Strategy) Stop() {
	stg.isRunning = false
}

func (stg *Strategy) OnDepth(depth *Depth) {
}

func (stg *Strategy) OnOrder(order *Order) {
}

func (stg *Strategy) IsRunning() bool {
	return stg.isRunning
}

func (stg *Strategy) GetName() string {
	return stg.name
}

func (stg *Strategy) GetRouter() *Router {
	return stg.router
}

func (stg *Strategy) GetSymbols() []string {
	return stg.Symbols
}

func (stg *Strategy) PlaceOrder(symbol string, price, size float64, orderType OrderType, offset OrderOffset, l int) (string, error) {
	if stg.IsRunning() {
		return stg.router.PlaceOrder(symbol, price, size, orderType, offset, l)
	} else {
		err := fmt.Errorf("strategy(%s) is not running, skip placeOrder of {%v %v %v %v %v %v}", stg.GetName(), symbol, price, size, orderType, offset, l)
		logger.Warning(err.Error())
		return "", err
	}
}

func (stg *Strategy) CancelOrder(symbol string, clOrdID string) {
	stg.router.CancelOrder(symbol, clOrdID)
}

func (stg *Strategy) GetCandles(symbol string, period, size int) ([]Bar, error) {
	return stg.router.GetCandles(symbol, period, size)
}
