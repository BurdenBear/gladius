package gladius

import (
	"errors"
	"fmt"

	goex "github.com/nntaoli-project/GoEx"
)

var OrderOffsetMapTradeSide = map[goex.TradeSide]OrderOffset{
	goex.BUY:         OO_BUY,
	goex.SELL:        OO_SELL,
	goex.BUY_MARKET:  OO_BUY,
	goex.SELL_MARKET: OO_SELL,
}

var OrderOffsetMapOType = map[int]OrderOffset{
	goex.OPEN_BUY:   OO_BUY,
	goex.OPEN_SELL:  OO_SHORT,
	goex.CLOSE_BUY:  OO_SELL,
	goex.CLOSE_SELL: OO_COVER,
}

var OTypeMapOrderOffset = map[OrderOffset]int{
	OO_BUY:   goex.OPEN_BUY,
	OO_SHORT: goex.OPEN_SELL,
	OO_SELL:  goex.CLOSE_BUY,
	OO_COVER: goex.CLOSE_SELL,
}

func NewGoexOpenType(offset OrderOffset) int {
	return OTypeMapOrderOffset[offset]
}

func NewOrderOffsetFromGoex(obj interface{}) OrderOffset {
	switch v := obj.(type) {
	case goex.TradeSide:
		return OrderOffsetMapTradeSide[v]
	case int:
		return OrderOffsetMapOType[v]
	default:
		msg := fmt.Sprintf("can't transform %v to TradeSide", v)
		panic(errors.New(msg))
	}
}

var orderTypeMapTradeSide = map[goex.TradeSide]OrderType{
	goex.BUY:         OT_LIMIT,
	goex.SELL:        OT_LIMIT,
	goex.BUY_MARKET:  OT_MARKET,
	goex.SELL_MARKET: OT_MARKET,
}

func NewOrderTypeFromGoex(obj interface{}) OrderType {
	switch v := obj.(type) {
	case goex.TradeSide:
		return orderTypeMapTradeSide[v]
	default:
		msg := fmt.Sprintf("can't transform %v to OrderType", v)
		panic(errors.New(msg))
	}
}

func NewOrderStatusFromGoex(ts goex.TradeStatus) OrderStatus {
	return OrderStatus(int(ts) + 1)
}
