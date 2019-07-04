package gateways

import (
	goex "github.com/nntaoli-project/GoEx"
)

type ExtendedFutureRestAPI interface {
	goex.FutureRestAPI
	PlaceFutureOrder2(currencyPair goex.CurrencyPair, contractType, price, amount string, orderType, openType, matchPrice, leverRate int) (string, error)
}

type FutureWebsocket interface {
	TickerCallback(func(*goex.FutureTicker))
	DepthCallback(func(*goex.Depth))
	TradeCallback(func(*goex.Trade, string))
	// KlineCallback(func(*goex.FutureKline, int)) FutureWebsocketAPI
	OrderCallback(func(*goex.FutureOrder, string))
	Login(apiKey string, apiSecretKey string, passphrase string) error
	SubscribeTicker(currencyPair goex.CurrencyPair, contractType string) error
	SubscribeDepth(currencyPair goex.CurrencyPair, contractType string, size int) error
	SubscribeTrade(currencyPair goex.CurrencyPair, contractType string) error
	SubscribeOrder(currencyPair goex.CurrencyPair, contractType string) error
}
