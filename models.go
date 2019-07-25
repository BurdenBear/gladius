package gladius

import (
	"time"

	goex "github.com/nntaoli-project/GoEx"
)

type Ticker struct {
	*goex.FutureTicker
	Contract IContract
}

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

func (order *Order) IsFinished() bool {
	return order.Status.IsFinished()
}

type Depth struct {
	Contract IContract
	Time     time.Time
	AskList  goex.DepthRecords // Price in ascending order
	BidList  goex.DepthRecords // Price in descending order
}
