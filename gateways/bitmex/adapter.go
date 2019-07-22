package bitmex

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"time"

	. "github.com/nntaoli-project/GoEx"
)

// common utils, maybe should be extracted in future
func timeStringToInt64(t string) (int64, error) {
	timestamp, err := time.Parse(time.RFC3339, t)
	if err != nil {
		return 0, err
	}
	return timestamp.UnixNano() / int64(time.Millisecond), nil
}

func int64ToTime(ti int64) time.Time {
	return time.Unix(0, ti*int64(time.Millisecond)).UTC()
}

func int64ToTimeString(ti int64) string {
	t := int64ToTime(ti)
	return t.Format(time.RFC3339)
}

func getExpires() string {
	return fmt.Sprintf("%d", time.Now().UTC().Add(15*time.Second).UnixNano()/int64(time.Second))
}

func getSign(apiSecretKey, method, rawurl, expires, body string) (string, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return "", err
	}
	var path string
	if len(u.RawQuery) > 0 {
		path = u.Path + "?" + u.RawQuery
	} else {
		path = u.Path
	}
	if len(path) > 0 && path[0] != '/' {
		path = "/" + path
	}
	data := method + path + expires + body
	return GetParamHmacSHA256Sign(apiSecretKey, data)
}

func parseBitmexSymbol(symbol string) (CurrencyPair, string, error) {
	if len(symbol) == 6 {
		underlying := symbol[:3]
		if underlying == "XBT" {
			underlying = "BTC"
		}
		pair := NewCurrencyPair(NewCurrency(underlying, ""), USD)
		if symbol[3:6] == "USD" {
			return pair, SWAP_CONTRACT, nil
		} else {
			switch symbol[3] {
			case 'M':
				return pair, THIS_WEEK_CONTRACT, nil
			case 'U':
				return pair, QUARTER_CONTRACT, nil
			case 'Z':
				return pair, "bi" + QUARTER_CONTRACT, nil //半年合约
			}
		}
	}
	return UNKNOWN_PAIR, "", fmt.Errorf("unknown symbol: %s", symbol)
}

func adaptBixmexSymbol(currencyPair CurrencyPair, contractType string) (string, error) {
	underlying := currencyPair.CurrencyA.Symbol
	if underlying == "BTC" {
		underlying = "XBT"
	}
	switch contractType {
	case SWAP_CONTRACT:
		return underlying + "USD", nil
	case THIS_WEEK_CONTRACT:
		return underlying + ":weekly", nil
	case QUARTER_CONTRACT:
		return underlying + ":quarterly", nil
	case "bi" + QUARTER_CONTRACT:
		return underlying + ":biquarterly", nil
	default:
		return "", fmt.Errorf("no bitmex instrument contract type (%s) refer to", contractType)
	}
}

type BitmexAdapter struct {
}

func NewBitmexAdapter() *BitmexAdapter {
	return &BitmexAdapter{}
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _fixStatusMap = map[string]TradeStatus{
	"0":                ORDER_UNFINISH,
	"New":              ORDER_UNFINISH,
	"1":                ORDER_PART_FINISH,
	"Partially filled": ORDER_PART_FINISH,
	"2":                ORDER_FINISH,
	"Filled":           ORDER_FINISH,
	"3":                ORDER_CANCEL,
	"Done for day":     ORDER_CANCEL, // done for day, means cancelled auto after a day
	"4":                ORDER_CANCEL,
	"Canceled":         ORDER_CANCEL,
	"6":                ORDER_CANCEL_ING,
	"Pending Cancel":   ORDER_CANCEL_ING,
	"7":                ORDER_CANCEL, // stopped, TODO: may be not right
	"Stopped":          ORDER_CANCEL,
	"8":                ORDER_REJECT,
	"Rejected":         ORDER_REJECT,
	"C":                ORDER_CANCEL,
	"Expired":          ORDER_CANCEL,
}

func AdapterFixOrderStatus(orderStatus string) TradeStatus {
	if os, ok := _fixStatusMap[orderStatus]; ok {
		return os
	}
	return ORDER_UNFINISH
}

func (adapter *BitmexAdapter) AdaptDepth(depths []BitmexDepth, size int) (*Depth, error) {
	if len(depths) == 0 {
		return nil, errors.New("can't adapt empty bitmex depths")
	}
	currencyPair, contractType, err := parseBitmexSymbol(depths[0].Symbol)
	if err != nil {
		return nil, err
	}
	depth := new(Depth)
	depth.AskList = make([]DepthRecord, 0, size)
	depth.BidList = make([]DepthRecord, 0, size)
	depth.Pair = currencyPair
	depth.ContractType = contractType
	for _, d := range depths {
		record := DepthRecord{}
		record.Price = d.Price
		record.Amount = d.Size
		switch d.Side {
		case "Sell":
			depth.AskList = append(depth.AskList, record)
		case "Buy":
			depth.BidList = append(depth.BidList, record)
		default:
			return nil, fmt.Errorf("can't adapt bitmex depths: %v", depths)
		}
	}
	l := len(depth.AskList)
	if size > 0 && size < l {
		depth.AskList = depth.AskList[l-size:]
		depth.BidList = depth.BidList[:size]
	}
	return depth, nil
}

func (adapter *BitmexAdapter) adaptOType(order *BitmexOrder) (int, error) {
	switch order.Side {
	case "Buy":
		switch order.ExecInst {
		case "Close":
			return CLOSE_SELL, nil
		case "ReduceOnly":
			return CLOSE_SELL, nil
		default:
			return OPEN_BUY, nil
		}
	case "Sell":
		switch order.ExecInst {
		case "Close":
			return CLOSE_BUY, nil
		case "ReduceOnly":
			return CLOSE_BUY, nil
		default:
			return OPEN_SELL, nil
		}
	default:
		return -1, fmt.Errorf("unknown bitmex order side: %s", order.Side)
	}
}

func (adapter *BitmexAdapter) AdaptTimeInt64(s string) (int64, error) {
	t, err := time.Parse("2006-01-02T15:04:05.999Z", s)
	if err != nil {
		return 0, err
	}
	return t.UnixNano() / int64(time.Millisecond), nil
}

func (adapter *BitmexAdapter) AdaptOrder(order *BitmexOrder) (*FutureOrder, string, error) {
	currencyPair, contractType, err := parseBitmexSymbol(order.Symbol)
	if err != nil {
		return nil, "", err
	}
	futureOrder := new(FutureOrder)
	futureOrder.OrderID2 = order.OrderID
	futureOrder.OrderID, _ = strconv.ParseInt(order.OrderID, 10, 64)
	futureOrder.Price = order.Price
	futureOrder.Amount = order.OrderQty
	futureOrder.AvgPrice = order.AvgPx
	futureOrder.DealAmount = order.CumQty
	var timeStr string
	if order.Timestamp != "" {
		timeStr = order.Timestamp // update time
	} else {
		timeStr = order.TransactTime // maybe the order create time
	}
	futureOrder.OrderTime, err = adapter.AdaptTimeInt64(timeStr)
	if err != nil {
		return nil, "", err
	}
	futureOrder.Status = AdapterFixOrderStatus(order.OrdStatus)
	futureOrder.Currency = currencyPair
	futureOrder.OType, err = adapter.adaptOType(order)
	if err != nil {
		return nil, "", err
	}
	futureOrder.ContractName = order.Symbol
	return futureOrder, contractType, nil
}

func (adapter *BitmexAdapter) AdaptWsOrderBook(ob *bitmexOrderBook, size int) (*Depth, error) {
	var err error
	depth := new(Depth)
	currencyPair, contractType, err := parseBitmexSymbol(ob.Symbol)
	depth.ContractType = contractType
	depth.Pair = currencyPair
	depth.UTime, err = time.Parse(time.RFC3339, ob.Timestamp)
	if err != nil {
		return nil, err
	}
	l := len(ob.Asks)
	depth.AskList = make([]DepthRecord, l)
	depth.BidList = make([]DepthRecord, l)
	for i, v := range ob.Asks {
		depth.AskList[i].Price = v[0]
		depth.AskList[i].Amount = v[1]
	}
	for i, v := range ob.Bids {
		depth.BidList[i].Price = v[0]
		depth.BidList[i].Amount = v[1]
	}
	if size > 0 && size < l {
		depth.AskList = depth.AskList[:size]
		depth.BidList = depth.BidList[:size]
	}
	Reverse(depth.AskList)
	return depth, nil
}

func (adapter *BitmexAdapter) AdatpTradeSide(t *bitmexWsTrade) (TradeSide, error) {
	switch t.Side {
	case "Sell":
		return SELL, nil
	case "Buy":
		return BUY, nil
	default:
		return 0, fmt.Errorf("unknown TradeSide of %s", t.Side)
	}
}

func (adapter *BitmexAdapter) AdaptWsTrade(t *bitmexWsTrade) (*Trade, string, error) {
	var err error
	trade := new(Trade)
	currencyPair, contractType, err := parseBitmexSymbol(t.Symbol)
	trade.Pair = currencyPair
	trade.Date, err = timeStringToInt64(t.Timestamp)
	if err != nil {
		return nil, "", err
	}
	trade.Type, err = adapter.AdatpTradeSide(t)
	if err != nil {
		return nil, "", err
	}
	trade.Amount = t.Size
	trade.Price = t.Price
	return trade, contractType, nil
}
