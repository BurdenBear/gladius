package gladius

import (
	"fmt"

	goex "github.com/nntaoli-project/GoEx"
)

// OrderOffset
type OrderOffset int

const (
	OO_BUY_TO_OPEN   OrderOffset = 1 + iota //开多
	OO_SELL_TO_CLOSE                        //平多
	OO_SELL_TO_OPEN                         //开空
	OO_BUY_TO_CLOSE                         //平空
	OO_BUY           = OO_BUY_TO_OPEN
	OO_SELL          = OO_SELL_TO_CLOSE
	OO_SHORT         = OO_SELL_TO_OPEN
	OO_COVER         = OO_BUY_TO_CLOSE
)

var orderOffsetSymbol = [...]string{"BUY", "SELL", "SHORT", "COVER"}

func (oo OrderOffset) String() string {
	i := oo - 1
	if i >= 0 && int(i) < len(orderOffsetSymbol) {
		return orderOffsetSymbol[i]
	}
	return fmt.Sprintf("UNKNOWN_ORDER_OFFSET(%d)", oo)
}

type OrderType = goex.OrderType

// OrderType
const (
	OT_LIMIT     = goex.ORDER_TYPE_LIMIT
	OT_MARKET    = goex.ORDER_TYPE_MARKET
	OT_FAK       = goex.ORDER_TYPE_FAK
	OT_FOK       = goex.ORDER_TYPE_FOK
	OT_POST_ONLY = goex.ORDER_TYPE_POST_ONLY
)

// OrderStatus
type OrderStatus int

const (
	OS_SUBMITTED = iota
	OS_UNFILLED
	OS_PART_FILLED
	OS_FULLY_FILLED
	OS_CANCELED
	OS_REJECTED
	OS_CANCELLING
)

var orderStatusSymbol = [...]string{"SUBMITTED", "UNFILLED", "PART_FILLED", "FULLY_FILLED", "CANCELED", "REJECT", "CANCELLING"}

func (os OrderStatus) String() string {
	i := os
	if i >= 0 && int(i) < len(orderStatusSymbol) {
		return orderStatusSymbol[i]
	}
	return fmt.Sprintf("UNKNOWN_ORDER_STATUS(%d)", os)
}

func (status OrderStatus) IsFinished() bool {
	return status == OS_FULLY_FILLED || status == OS_REJECTED || status == OS_CANCELED
}

const (
	KLINE_PERIOD_1MIN = 1 + iota
	KLINE_PERIOD_3MIN
	KLINE_PERIOD_5MIN
	KLINE_PERIOD_15MIN
	KLINE_PERIOD_30MIN
	KLINE_PERIOD_60MIN
	KLINE_PERIOD_1H
	KLINE_PERIOD_2H
	KLINE_PERIOD_4H
	KLINE_PERIOD_6H
	KLINE_PERIOD_8H
	KLINE_PERIOD_12H
	KLINE_PERIOD_1DAY
	KLINE_PERIOD_3DAY
	KLINE_PERIOD_1WEEK
	KLINE_PERIOD_1MONTH
	KLINE_PERIOD_1YEAR
)

const (
	THIS_WEEK_CONTRACT = "this_week" //周合约
	NEXT_WEEK_CONTRACT = "next_week" //次周合约
	QUARTER_CONTRACT   = "quarter"   //季度合约
)

//exchanges const
const (
	OKCOIN_CN   = "okcoin.cn"
	OKCOIN_COM  = "okcoin.com"
	OKEX        = "okex.com"
	OKEX_FUTURE = "okex.com"
	OKEX_SWAP   = "okex.com_swap"
	HUOBI       = "huobi.com"
	HUOBI_PRO   = "huobi.pro"
	BITSTAMP    = "bitstamp.net"
	KRAKEN      = "kraken.com"
	ZB          = "zb.com"
	BITFINEX    = "bitfinex.com"
	BINANCE     = "binance.com"
	POLONIEX    = "poloniex.com"
	COINEX      = "coinex.com"
	BITHUMB     = "bithumb.com"
	GATEIO      = "gate.io"
	BITTREX     = "bittrex.com"
	GDAX        = "gdax.com"
	WEX_NZ      = "wex.nz"
	BIGONE      = "big.one"
	COIN58      = "58coin.com"
	FCOIN       = "fcoin.com"
	HITBTC      = "hitbtc.com"
	BITMEX      = "bitmex.com"
	CRYPTOPIA   = "cryptopia.co.nz"
	HBDM        = "hbdm.com"
)

var FDTRADER_SEPERATOR = ":"
