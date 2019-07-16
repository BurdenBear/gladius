package gateways

import (
	"time"

	. "github.com/BurdenBear/gladius"
	goex "github.com/nntaoli-project/GoEx"
)

func TimeStringToInt64(s string) (int64, error) {
	t, err := time.Parse(s, time.RFC3339)
	if err != nil {
		return 0, err
	}
	return t.UTC().UnixNano() / int64(time.Millisecond), nil
}

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
	o.UpdateTime = Int64ToTime(order.OrderTime)
	o.Status = NewOrderStatusFromGoex(order.Status)
	if o.Status == OS_UNFILLED {
		o.CreateTime = o.UpdateTime
	}
	o.Offset = NewOrderOffsetFromGoex(order.OType)
	// TODO: o.Type =
	o.LeverRate = order.LeverRate
	o.Fee = order.Fee
	return
}

func AdapterFutureKlines(contract IContract, klines []goex.FutureKline) (a []Bar) {
	bars := make([]Bar, 0, len(klines))
	bar := Bar{}
	for _, kline := range klines {
		bar.Contract = contract
		bar.Timestamp = Int64ToTime(kline.Timestamp * 1000) //kline.Timestamp is seconds
		bar.Open = kline.Open
		bar.High = kline.High
		bar.Low = kline.Low
		bar.Close = kline.Close
		bar.Vol = kline.Vol
		bar.Vol2 = kline.Vol2
		bars = append(bars, bar)
	}
	return bars
}

func AdapterDepth(contract IContract, depth *goex.Depth) (d *Depth) {
	d = new(Depth)
	d.Contract = contract
	d.Time = depth.UTime
	d.AskList = &depth.AskList
	d.BidList = &depth.BidList
	return
}
