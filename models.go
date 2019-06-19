package gladius

import (
	"time"

	goex "github.com/nntaoli-project/GoEx"
)

type Ticker struct {
	*goex.FutureTicker
	Contract IContract
}

type Order struct {
	OrderID       string //请尽量用这个字段替代OrderID字段
	ClientOrderID string //客户端自定义ID
	Price         float64
	Amount        float64
	AvgPrice      float64
	DealAmount    float64
	CreateTime    time.Time
	Status        OrderStatus
	Contract      IContract
	Offset        OrderOffset //1：开多 2：平多 3：开空 4： 平空
	Type          OrderType   //1. 市价单 2. 限价单
	LeverRate     int         //杠杆率，现货为1
	Fee           float64     //手续费
}

func (order *Order) IsFinished() bool {
	return order.Status.IsFinished()
}

type Depth struct {
	Contract IContract
	Time     time.Time
	AskList  *goex.DepthRecords // Descending order
	BidList  *goex.DepthRecords // Descending order
}
