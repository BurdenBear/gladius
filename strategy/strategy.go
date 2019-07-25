package strategy

import (
	"fmt"

	. "github.com/BurdenBear/gladius"
)

type IStrategy interface {
	Start()               // 开启策略
	Stop()                // 停止策略运行
	IsRunning() bool      // 获取策略运行状态
	GetName() string      // 获取策略名字
	GetRouter() *Router   // 获取策略绑定的下单路由
	Init()                // 初始化策略
	GetSymbols() []string // 获取策略订阅的交易合约
	OnDepth(depth *Depth) // 市场深度数据回调
	OnOrder(order *Order) // 订单状态数据回调
	// 下单并返回order.ClOrdID
	PlaceOrder(symbol string, price, size float64, orderType OrderType, offset OrderOffset, l int) (string, error)
	// 根据ClOrdID取消订单
	CancelOrder(symbol, clOrdID string)
	// 获取Bar数据
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
		return stg.router.PlaceOrder(stg.GetName(), symbol, price, size, orderType, offset, l)
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
