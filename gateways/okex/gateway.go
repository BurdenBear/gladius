package okex

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/gateways"
	goex "github.com/nntaoli-project/GoEx"
	"github.com/nntaoli-project/GoEx/builder"
	"github.com/nntaoli-project/GoEx/okcoin"
)

type OkexGatewayConfig struct {
	*gateways.GoExGatewayConfig
}

func NewOkexGatewayFromConfig(name string, engine *EventEngine, value interface{}) (gateways.IGateway, error) {
	var config *OkexGatewayConfig
	if c, ok := value.(*OkexGatewayConfig); ok {
		config = c
	} else {
		v, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		config = new(OkexGatewayConfig)
		err = json.Unmarshal(v, config)
		if err != nil {
			return nil, err
		}
	}
	return NewOkexGateway(name, engine, config)
}

func GetDefaultOkexGatewayConfig() *OkexGatewayConfig {
	apiKey := gateways.GetEnv("GOEX_OKEX_API_KEY", "")
	apiSecretKey := gateways.GetEnv("GOEX_OKEX_API_SECRET_KEY", "")
	passphrase := gateways.GetEnv("GOEX_OKEX_PASSPHRASE", "")
	restfulURL := gateways.GetEnv("GOEX_OKEX_RESTFUL_URL", "")
	websocketURL := gateways.GetEnv("GOEX_OKEX_WEBSOCKET_URL", "")
	config := &OkexGatewayConfig{
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

func NewOkexGateway(name string, engine *EventEngine, config interface{}) (*OkexGateway, error) {
	var err error
	gateway := &OkexGateway{}
	gateway.apiFactory = NewOkexAPIFactory()
	gateway.GoExGateway, err = gateways.NewGoExGateway(name, engine, gateway.apiFactory, config)
	if err != nil {
		return nil, err
	}
	return gateway, nil
}

type OkexGateway struct {
	*gateways.GoExGateway
	apiFactory *OkexAPIFactory
}

func (gateway *OkexGateway) GetExchange() string {
	return "OKEX"
}

type OKExV3FutureWsWrapper struct {
	*okcoin.OKExV3FutureWs
}

func (w *OKExV3FutureWsWrapper) TickerCallback(cb func(*goex.FutureTicker)) {
	w.OKExV3FutureWs.TickerCallback(cb)
}
func (w *OKExV3FutureWsWrapper) DepthCallback(cb func(*goex.Depth)) {
	w.OKExV3FutureWs.DepthCallback(cb)
}
func (w *OKExV3FutureWsWrapper) TradeCallback(cb func(*goex.Trade, string)) {
	w.OKExV3FutureWs.TradeCallback(cb)
}

// KlineCallback(func(*goex.FutureKline, int)) FutureWebsocketAPI
func (w *OKExV3FutureWsWrapper) OrderCallback(cb func(*goex.FutureOrder, string)) {
	w.OKExV3FutureWs.OrderCallback(cb)
}

func newOKExV3FutureWs(url string, provider okcoin.IContractIDProvider) *OKExV3FutureWsWrapper {
	okWs := &OKExV3FutureWsWrapper{}
	okWs.OKExV3FutureWs = okcoin.NewOKExV3FutureWs(provider)
	if url != "" {
		okWs.WsBuilder = okWs.WsBuilder.WsUrl(url)
	}
	okWs.WsBuilder = okWs.WsBuilder.Heartbeat([]byte("ping"), 15*time.Second)
	return okWs
}

func newOKExV3(apiConfig *goex.APIConfig) *okcoin.OKExV3 {
	return okcoin.NewOKExV3(
		apiConfig.HttpClient, apiConfig.ApiKey, apiConfig.ApiSecretKey,
		apiConfig.ApiPassphrase, apiConfig.Endpoint)
}

func NewOkexAPIFactory() *OkexAPIFactory {
	return &OkexAPIFactory{}
}

type OkexAPIFactory struct {
	futureRestAPI   *okcoin.OKExV3
	futureWs        *OKExV3FutureWsWrapper
	futureAuthoried bool
	config          *OkexGatewayConfig
}

func (factory *OkexAPIFactory) Config(gateway *gateways.GoExGateway, config interface{}) error {
	c, ok := config.(*OkexGatewayConfig)
	if !ok {
		return fmt.Errorf("invalid config type of OkexAPIFactory: %v", config)
	}
	if c == nil {
		c = GetDefaultOkexGatewayConfig()
	}
	factory.config = c
	if len(factory.config.Secret.APIKey) > 0 ||
		len(factory.config.Secret.APISecretKey) > 0 ||
		len(factory.config.Secret.Passphrase) > 0 {
		factory.futureAuthoried = true
	}
	gateway.SetSymbols(factory.config.Symbols)
	interval := time.Duration(2 / (20.0 / 3 * 0.8) * float64(time.Second)) // 20 query / 2 second limit
	gateway.SetOrderQueryInterval(interval)
	return nil
}

func (factory *OkexAPIFactory) GetFutureAPI() (gateways.ExtendedFutureRestAPI, error) {
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
	factory.futureRestAPI = newOKExV3(apiConfig)
	return factory.futureRestAPI, nil
}

func (factory *OkexAPIFactory) GetFutureWs() (gateways.FutureWebsocket, error) {
	var fallback gateways.FutureWebsocket
	apiKey := factory.config.Secret.APIKey
	apiSecretKey := factory.config.Secret.APISecretKey
	passphrase := factory.config.Secret.Passphrase
	factory.futureWs = newOKExV3FutureWs(factory.config.Urls.Future.Websocket, factory.futureRestAPI)
	if factory.config.Heartbeat.Websocket > 0 {
		t := time.Duration(factory.config.Heartbeat.Websocket) * time.Second
		factory.futureWs.WsBuilder.Heartbeat([]byte("ping"), t)
	}
	if factory.futureAuthoried {
		err := factory.futureWs.Login(apiKey, apiSecretKey, passphrase)
		if err != nil {
			return fallback, err
		}
	}
	return factory.futureWs, nil
}

func (factory *OkexAPIFactory) GetFutureAuthoried() bool {
	return factory.futureAuthoried
}
