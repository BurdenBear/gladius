package gateways

import (
	"reflect"
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

func Reverse(slice interface{}) {
	s := reflect.ValueOf(slice)
	// if s is a pointer of slice
	if s.Kind() == reflect.Ptr {
		s = s.Elem()
	}
	swp := reflect.Swapper(s.Interface())
	for i, j := 0, s.Len()-1; i < j; i, j = i+1, j-1 {
		swp(i, j)
	}
}

func ensureDepthPriceOrder(depth goex.DepthRecords, asending bool) {
	if len(depth) <= 1 {
		return
	}
	asendingInData := true
	for i := 0; i < len(depth)-1; i++ {
		if depth[i].Price > depth[i+1].Price {
			asendingInData = false
			break
		}
	}
	if asending != asendingInData {
		Reverse(depth)
	}
}

func AdapterDepth(contract IContract, depth *goex.Depth) (d *Depth) {
	d = new(Depth)
	d.Contract = contract
	d.Time = depth.UTime
	// TODO: can not modify depth.AskList or depth.BidList?
	d.AskList = depth.AskList
	ensureDepthPriceOrder(d.AskList, true)
	d.BidList = depth.BidList
	ensureDepthPriceOrder(d.BidList, false)
	return
}
