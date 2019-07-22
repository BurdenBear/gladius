package bitmex

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/gateways"
	goex "github.com/nntaoli-project/GoEx"
	"github.com/nntaoli-project/GoEx/builder"
)

var logger = GetLogger()

type BitmexGatewayConfig struct {
	*gateways.GoExGatewayConfig
}

func NewBitmexGatewayFromConfig(name string, engine *EventEngine, value interface{}) (gateways.IGateway, error) {
	var config *BitmexGatewayConfig
	if c, ok := value.(*BitmexGatewayConfig); ok {
		config = c
	} else {
		v, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		config = new(BitmexGatewayConfig)
		err = json.Unmarshal(v, config)
		if err != nil {
			return nil, err
		}
	}
	return NewBitmexGateway(name, engine, config)
}

func GetDefaultBitmexGatewayConfig() *BitmexGatewayConfig {
	apiKey := gateways.GetEnv("GOEX_BITMEX_API_KEY", "")
	apiSecretKey := gateways.GetEnv("GOEX_BITMEX_API_SECRET_KEY", "")
	passphrase := gateways.GetEnv("GOEX_BITMEX_PASSPHRASE", "")
	restfulURL := gateways.GetEnv("GOEX_BITMEX_RESTFUL_URL", "")
	websocketURL := gateways.GetEnv("GOEX_BITMEX_WEBSOCKET_URL", "")

	config := &BitmexGatewayConfig{
		GoExGatewayConfig: &gateways.GoExGatewayConfig{
			Secret: gateways.GoExGatewaySecretConfig{
				APIKey:       apiKey,
				APISecretKey: apiSecretKey,
				Passphrase:   passphrase,
			},
			Urls: gateways.GoExGatewayUrlsConfig{
				Future: gateways.GoExGatewayUrlConfig{
					Restful:   restfulURL,
					Websocket: websocketURL,
				},
			},
		},
	}
	return config
}

func NewBitmexGateway(name string, engine *EventEngine, config interface{}) (*BitmexGateway, error) {
	var err error
	gateway := &BitmexGateway{}
	gateway.apiFactory = NewBitmexAPIFactory()
	gateway.GoExGateway, err = gateways.NewGoExGateway(name, engine, gateway.apiFactory, config)
	if err != nil {
		return nil, err
	}
	return gateway, nil
}

type BitmexGateway struct {
	*gateways.GoExGateway
	apiFactory *BitmexAPIFactory
}

func (gateway *BitmexGateway) GetExchange() string {
	return "BITMEX"
}

func NewBitmexWsWrapper() *BitmexWsWrapper {
	return &BitmexWsWrapper{BitmexWs: NewBitmexWs()}
}

type BitmexWsWrapper struct {
	*BitmexWs
}

func (w *BitmexWsWrapper) TickerCallback(cb func(*goex.FutureTicker)) {
	w.BitmexWs.TickerCallback(cb)
}
func (w *BitmexWsWrapper) DepthCallback(cb func(*goex.Depth)) {
	w.BitmexWs.DepthCallback(cb)
}
func (w *BitmexWsWrapper) TradeCallback(cb func(*goex.Trade, string)) {
	w.BitmexWs.TradeCallback(cb)
}

// KlineCallback(func(*goex.FutureKline, int)) FutureWebsocketAPI
func (w *BitmexWsWrapper) OrderCallback(cb func(*goex.FutureOrder, string)) {
	w.BitmexWs.OrderCallback(cb)
}

func NewBitmexAPIFactory() *BitmexAPIFactory {
	return &BitmexAPIFactory{}
}

type BitmexAPIFactory struct {
	futureRestAPI   *Bitmex
	futureWs        *BitmexWsWrapper
	futureAuthoried bool
	config          *BitmexGatewayConfig
}

func (factory *BitmexAPIFactory) Config(gateway *gateways.GoExGateway, config interface{}) error {
	c, ok := config.(*BitmexGatewayConfig)
	if !ok {
		return fmt.Errorf("invalid config type of BitmexAPIFactory: %v", config)
	}
	if c == nil {
		c = GetDefaultBitmexGatewayConfig()
	}
	factory.config = c
	if len(factory.config.Secret.APIKey) > 0 ||
		len(factory.config.Secret.APISecretKey) > 0 ||
		len(factory.config.Secret.Passphrase) > 0 {
		factory.futureAuthoried = true
	}
	gateway.SetSymbols(factory.config.Symbols)
	interval := time.Duration(20 * time.Second) // 60 query / 1 minutes limit for all request.
	gateway.SetOrderQueryInterval(interval)
	return nil
}

func (factory *BitmexAPIFactory) GetFutureAPI() (gateways.ExtendedFutureRestAPI, error) {
	apiBuilder := builder.NewAPIBuilder()
	if factory.config.HTTP.Timeout != 0 {
		apiBuilder.HttpTimeout(time.Duration(factory.config.HTTP.Timeout) * time.Second)
	}
	if factory.config.HTTP.Proxy != "" {
		apiBuilder.HttpProxy(factory.config.HTTP.Proxy)
	}
	apiKey := factory.config.Secret.APIKey
	apiSecretKey := factory.config.Secret.APISecretKey
	passphrase := factory.config.Secret.Passphrase
	client := apiBuilder.GetHttpClient()
	apiConfig := &goex.APIConfig{
		HttpClient:    client,
		ApiKey:        apiKey,
		ApiSecretKey:  apiSecretKey,
		ApiPassphrase: passphrase,
		Endpoint:      factory.config.Urls.Future.Restful,
	}
	factory.futureRestAPI = NewBitmex(apiConfig)
	return factory.futureRestAPI, nil
}

func (factory *BitmexAPIFactory) GetFutureWs() (gateways.FutureWebsocket, error) {
	var fallback gateways.FutureWebsocket
	apiKey := factory.config.Secret.APIKey
	apiSecretKey := factory.config.Secret.APISecretKey
	passphrase := factory.config.Secret.Passphrase
	factory.futureWs = NewBitmexWsWrapper()
	if factory.config.Heartbeat.Websocket > 0 {
		t := time.Duration(factory.config.Heartbeat.Websocket) * time.Second
		factory.futureWs.WsBuilder.Heartbeat([]byte("ping"), t)
	}
	if factory.config.Urls.Future.Websocket != "" {
		factory.futureWs.WsUrl(factory.config.Urls.Future.Websocket)
	}
	if factory.config.HTTP.Proxy != "" {
		factory.futureWs.ProxyUrl(factory.config.HTTP.Proxy)
	}
	if factory.futureAuthoried {
		err := factory.futureWs.Login(apiKey, apiSecretKey, passphrase)
		if err != nil {
			return fallback, err
		}
	}
	return factory.futureWs, nil
}

func (factory *BitmexAPIFactory) GetFutureAuthoried() bool {
	return factory.futureAuthoried
}
