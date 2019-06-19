package gateways

import (
	"time"

	. "github.com/BurdenBear/gladius"
	goex "github.com/nntaoli-project/GoEx"
)

func Int64ToTime(ti int64) time.Time {
	return time.Unix(0, ti*int64(time.Millisecond)).UTC()
}

func AdapterFutureOrder(contract IContract, order *goex.FutureOrder) (o *Order) {
	o = new(Order)
	o.Contract = contract
	o.OrderID = order.OrderID2
	o.Price = order.Price
	o.Amount = order.Amount
	o.AvgPrice = order.AvgPrice
	o.DealAmount = order.DealAmount
	o.CreateTime = Int64ToTime(order.OrderTime)
	o.Status = NewTradeStatusFromGoex(order.Status)
	o.Offset = NewOrderOffsetFromGoex(order.OType)
	// TODO: o.Type =
	o.LeverRate = order.LeverRate
	o.Fee = order.Fee
	return
}

func AdapterDepth(contract IContract, depth *goex.Depth) (d *Depth) {
	d = new(Depth)
	d.Contract = contract
	d.Time = depth.UTime
	d.AskList = &depth.AskList
	d.BidList = &depth.BidList
	return
}
