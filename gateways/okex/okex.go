package okex

import (
	"fmt"
	"time"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/gateways"
	goex "github.com/nntaoli-project/GoEx"
	"github.com/nntaoli-project/GoEx/builder"
	"github.com/nntaoli-project/GoEx/okcoin"
)

var log = GetLogger()

type BaseHTTPConfig struct {
	HTTPTimeout time.Duration
	HTTPProxy   string
}

type OkexGatewayConfig struct {
	*BaseHTTPConfig
	RestfulURL    string
	WebSocketURL  string
	APIKey        string
	APISecretKey  string
	APIPassphrase string
	Symbols       []string
}

func GetDefaultOkexGatewayConfig() *OkexGatewayConfig {
	apiKey := gateways.GetEnv("GOEX_OKEX_API_KEY", "")
	apiSecretKey := gateways.GetEnv("GOEX_OKEX_API_SECRET_KEY", "")
	passphrase := gateways.GetEnv("GOEX_OKEX_PASSPHRASE", "")
	restfulURL := gateways.GetEnv("GOEX_OKEX_RESTFUL_URL", "")
	websocketURL := gateways.GetEnv("GOEX_OKEX_WEBSOCKET_URL", "")
	config := &OkexGatewayConfig{
		RestfulURL:    restfulURL,
		WebSocketURL:  websocketURL,
		APIKey:        apiKey,
		APISecretKey:  apiSecretKey,
		APIPassphrase: passphrase,
	}
	return config
}

type OkexGateway struct {
	*gateways.GoExGateway
	apiFactory *OkexAPIFactory
}

type OkexAPIFactory struct {
	futureRestAPI   *okcoin.OKExV3
	futureWs        *OKExV3FutureWsWrapper
	futureAuthoried bool
	config          *OkexGatewayConfig
}

func NewOkexAPIFactory() *OkexAPIFactory {
	return &OkexAPIFactory{}
}

func NewOkexGateway(name string, engine *EventEngine) *OkexGateway {
	gateway := &OkexGateway{}
	gateway.apiFactory = NewOkexAPIFactory()
	gateway.GoExGateway = gateways.NewGoExGateway(name, engine, gateway.apiFactory)
	return gateway
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

func (factory *OkexAPIFactory) Config(gateway *gateways.GoExGateway, config interface{}) error {
	if config == nil {
		config = GetDefaultOkexGatewayConfig()
	}
	c, ok := config.(*OkexGatewayConfig)
	if !ok {
		return fmt.Errorf("invalid config type of OkexAPIFactory: %v", config)
	}
	factory.config = c
	if len(factory.config.APIKey) > 0 ||
		len(factory.config.APISecretKey) > 0 ||
		len(factory.config.APIPassphrase) > 0 {
		factory.futureAuthoried = true
	}
	gateway.SetSymbols(factory.config.Symbols)
	interval := time.Duration(2 / (20.0 / 3 * 0.8) * float64(time.Second)) // 20 query / 2 second limit
	gateway.SetOrderQueryInterval(interval)
	return nil
}

func (factory *OkexAPIFactory) GetFutureAPI() (gateways.ExtendedFutureRestAPI, error) {
	apiBuilder := builder.NewAPIBuilder()
	if factory.config.BaseHTTPConfig != nil {
		apiBuilder.HttpTimeout(factory.config.HTTPTimeout).HttpProxy(factory.config.HTTPProxy)
	}
	apiKey := factory.config.APIKey
	apiSecretKey := factory.config.APISecretKey
	passphrase := factory.config.APIPassphrase
	client := apiBuilder.GetHttpClient()
	apiConfig := &goex.APIConfig{
		HttpClient:    client,
		ApiKey:        apiKey,
		ApiSecretKey:  apiSecretKey,
		ApiPassphrase: passphrase,
		Endpoint:      factory.config.RestfulURL,
	}
	factory.futureRestAPI = newOKExV3(apiConfig)
	return factory.futureRestAPI, nil
}

func (factory *OkexAPIFactory) GetFutureWs() (gateways.FutureWebsocket, error) {
	var fallback gateways.FutureWebsocket
	apiKey := factory.config.APIKey
	apiSecretKey := factory.config.APISecretKey
	passphrase := factory.config.APIPassphrase
	factory.futureWs = newOKExV3FutureWs(factory.config.WebSocketURL, factory.futureRestAPI)
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
