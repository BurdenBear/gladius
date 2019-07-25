# gladius

Gladius, event driven trading engine building with golang, powered by GoEx.

## 1.背景

对大多数非计算机背景的策略师来说，一般难于在多线程环境下编写交易代码。
而单纯使用单个线程及同步的底层接口编写出来的交易程序网络io性能并不高，
用来做一些按固定时间驱动(分钟以上级别)的交易策略可能足够，
但不适合于需要处理大量档口数据以及要求快速频繁挂撤单等需要与交易所进行频繁网络交互的交易逻辑。

本项目基于GoEx实现的底层交易接口的调用进行了进一步的封装，
隐藏了与交易所之间在多线程环境下的进行异步网络交互的实现细节，
暴露给用户了一个单线程Event Driving的交易策略编写框架，
使策略师能通过编写和单线程环境下一样简单的策略逻辑，完成一个具有比单线程程序更高执行效率的交易程序。

## 2. API接口

策略文件中需要实现一个`IStrategyConstructor`类型的函数，
之后将会在配置文件中指定该函数

```golang
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

type IStrategyConstructor = func(string, *Router) IStrategy
```

### 2.1 合约

gladius所完成的一项关键工作就是将来自不同交易所的数据推筛选推送给策略，
以及将策略的挂撤单请求路由给相应的交易所。
为了实现对应路由逻辑，gladius中定义了一层gateway交易所网关。
gateway负责交易所下的单个交易账号和交易所之间的信息交互，
可以理解为一个gateway对应一个交易账号，而合约信息中就包含gateway名,
以此确定数据的来源和去向。

合约接口:

```golang
type IContract interface {
    GetID() string      //合约全局ID,如: "GATEWAYA:BTC_USD:SWAP"
    GetGateway() string //合约gateway名,如: "GATEWAYA"
    GetSymbol() string  //合约symbol,不包含gateway信息,如: "BTC_USD:SWAP"
}
```

### 2.2 行情相关

在策略中获取行情有两种途径:

#### 2.2.1 通过回调函数接收订阅的数据

- 例:

```golang
// Strategy implements IStrategy
func (stg *Strategy) OnDepth(depth *Depth) {
    log.Println(depth.AskList[0].Price) //打印卖一价格
    log.Println(depth.AskList[0].Amount) //打印卖一数量
    log.Println(depth.BidList[0].Price) //打印买一价格
    log.Println(depth.BidList[0].Amount) //打印买一数量
}
```

- 数据结构体:

```golang
type Depth struct {
    Contract IContract
    Time     time.Time
    AskList  goex.DepthRecords // Price in ascending order
    BidList  goex.DepthRecords // Price in descending order
}
```

#### 2.2.2 主动获取k线(同步调用，阻塞直到数据返回)

- 例:

```golang
// 计算两个合约的价差均值
// stg implements IStrategy
barsA, err := stg.gateway.GetCandles(stg.SymbolA, KLINE_PERIOD_1MIN, stg.Period)
barsB, err := stg.gateway.GetCandles(stg.SymbolB, KLINE_PERIOD_1MIN, stg.Period)
spreads := make([]float64, len(barsA))
for i := 0; i < len(barsA); i++ {
    spreads[i] = 2.0 * (barsA[i].Close - barsB[i].Close) / (barsA[i].Close + barsB[i].Close)
}
mean, variance := stat.MeanVariance(spreads, nil)
```

- 数据结构体:

```golang
type Bar struct {
    Contract  IContract
    Timestamp time.Time
    Open      float64
    High      float64
    Low       float64
    Close     float64
    Vol       float64 //交易量(美元本位)
    Vol2      float64 //交易量(币本位)
}
```

### 2.3 交易相关

### 2.3.1 下单

- 例子:

```golang
//Strategy implements IStrategy
//封装一个买开的方法
func (stg *Strategy) Buy(symbol string, lots, price float64, orderType OrderType) string {
    orderID, err := stg.PlaceOrder(symbol, realPrice, amount, orderType, OO_BUY, stg.Leverage)
    if err != nil {
        log.Error(err.Error())
    }
    return orderID
}
```

### 2.3.2 取消订单

- 例子:

```golang
//stg implements IStrategy
//下单后马上撤掉
orderID, err := stg.PlaceOrder(symbol, realPrice, amount, orderType, OO_BUY, stg.Leverage)
stg.CancelOrder(symbol, orderID)
```

### 2.3.3 订单状态变化回调函数

- 例子:

```golang
//Strategy implements IStrategy
func (stg *Strategy) OnOrder(order *Order) {
    log.Debugf("%v %v", order.Contract, order)
    // 记录所有的订单对象,订单完成时删除
    if order.IsFinished() {
        if _, ok := stg.orderDict[order.ClOrdID]; ok {
            delete(stg.orderDict, order.ClOrdID)
        }
    } else {
        stg.orderDict[order.ClOrdID] = order
    }
}
```

- 数据结构体:

```golang
type Order struct {
    OrderID      string      //订单交易所ID
    ClOrdID      string      //客户端自定义ID
    Price        float64     //订单价格
    Amount       float64     //订单数量
    AvgPrice     float64     //成交均价
    DealAmount   float64     //成交数量
    CreateTime   time.Time   //订单创建时间
    UpdateTime   time.Time   //订单状态更新时间
    Status       OrderStatus //订单状态
    OrdRejReason string      //拒单原因
    Contract     IContract   //订单合约
    Offset       OrderOffset //1：开多 2：平多 3：开空 4： 平空
    Type         OrderType   //1. 市价单 2. 限价单
    LeverRate    int         //杠杆率，现货为1
    Fee          float64     //手续费
}
```

## 3. 开始使用

- gladius只支持在**linux系统**下运行，windows上建议使用wsl或者docker。

- gladius通过配置文件来配置运行，通过go plugin机制来加载策略编译后的.so文件，可以在不暴露策略源码的情况下运行策略。

### 3.1 安装gladius

安装完成后`$GOBIN`中会加入`gladius`命令

#### 3.1.1 go get 安装

```bash
go get -u github.com/BurdenBear/gladius/cmd/gladius
```

#### 3.1.2 下载源码安装

适合需要对项目源码进行本地开发修改的安装方式

```bash
git clone https://github.com/BurdenBear/gladius.git
cd gladius
git checkout master #切换到特定版本
bash install.sh
```

### 3.2 配置文件说明

[配置文件样例](https://github.com/BurdenBear/gladius/tree/master/entrypoint/gladius.yaml):

```yaml
gateways: # Gateway设置
  gatewayA: # GatewayName,会被统一大写处理
    exchange: bitmex # exchange类型,如果不指定会尝试从GatewayName的前缀自动推断，比如GatewayName为okex，那么exchange就会被推断为okex
    http: # http相关设定，可以设置超时及代理
      timeout: 10
      proxy: socks5://localhost:10808
    secret: # 密钥相关设定
      apiKey: iTmaFY90X6Bi8LoSTQipcVHD
      apiSecretKey: _nFmKbmaLp1QXWkWmsNGjxLOOf8nIc1GZnNhyiGGxdSDwEDY
      # passphrase: okexshabi # okex等交易所需要
    urls: # 连接地址设定
      future: # 期货
        restful: https://testnet.bitmex.com/api/v1 # restful接口地址
        websocket: wss://testnet.bitmex.com/realtime # websocket接口地址
    symbols: # Gateway订阅的品种列表
    - BTC_USD:SWAP # 填写合约的Symbol，Gateway会自动订阅对应合约的ticker数据
  # gatewayB:
  #   exchange: okex
  #   http:
  #     timeout: 10
  #   secret:
  #     apiKey:
  #     apiSecretKey:
  #     passphrase:
  #   symbols:
  #   - BTC_USD:THIS_WEEK
strategies: # 策略设置(当前版本所有策略都跑在一个线程中，建议一个进程只运行一个策略)
  crossOkBitmex: # 策略名字，需唯一，会被统一小写处理
    constructor: strategy.so:NewStrategy # 指定策略.so文件，以及策略的构造函数
    symbols: # Strategy订阅的品种列表
    - GATEWAYA:BTC_USD:SWAP # 填写合约的ID (需要加GatewayName)
    # - GATEWAYB:BTC_USD:THIS_WEEK
    vars: # 变量注入(可以用来设置策略的一些参数)，symbols的设置会自动注入strategy.Symbols中
    - name: #field名，确保首字母大写(只有公有属性才能从package外访问)
      values: #注入值，确保和field的类型一致，否则注入可能失败
log: # 日志设定
  console:
    # enabled: true # 是否开启控制台日志，默认开启
    level: DEBUG # 控制台日志级别
  # file:
  #   enabled: true # 是否开启文件日志，默认关闭
  #   path: gladius.log # 开启文件日志时必须指定，日志文件的位置
  #   level: DEBUG # 文件日志级别
```

### 3.3 编译策略文件

[策略文件样例](https://github.com/BurdenBear/gladius/tree/master/entrypoint/strategy/main.go):

```golang
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
```

比如您的策略文件为`main.go`,可以用以下命令将其编译为`strategy.so`

```bash
go build --buildmode=plugin -o ./strategy.so main.go
```

### 3.4 运行

在当前工作目录创建配置文件`gladius.yaml`，编辑好相关配置。

将编译好的策略文件(如`strategy.so`)放到配置文件中指定的位置(如`$PWD/strategy.so`)。

在当前工作目录打开命令行，输入:

```bash
gladius
```

> 注: 请确保3.3中编译策略文件使用的gladius项目代码和gladius命令对应的gladius项目代码是同一个版本的代码。
