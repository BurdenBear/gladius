package hbdm

import (
	"fmt"
	"time"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/gateways"
	goex "github.com/nntaoli-project/GoEx"
	"github.com/nntaoli-project/GoEx/builder"
)

var log = GetLogger()

type HbdmGatewayConfig struct {
	*gateways.GoExGatewayConfig
}

func GetDefaultHbdmGatewayConfig() *HbdmGatewayConfig {
	apiKey := gateways.GetEnv("GOEX_Hbdm_API_KEY", "")
	apiSecretKey := gateways.GetEnv("GOEX_Hbdm_API_SECRET_KEY", "")
	passphrase := gateways.GetEnv("GOEX_Hbdm_PASSPHRASE", "")

	config := &HbdmGatewayConfig{
		GoExGatewayConfig: &gateways.GoExGatewayConfig{
			APIKey:        apiKey,
			APISecretKey:  apiSecretKey,
			APIPassphrase: passphrase,
		},
	}
	return config
}

func NewHbdmGateway(name string, engine *EventEngine) *HbdmGateway {
	gateway := &HbdmGateway{}
	gateway.apiFactory = NewHbdmAPIFactory()
	gateway.GoExGateway = gateways.NewGoExGateway(name, engine, gateway.apiFactory)
	return gateway
}

type HbdmGateway struct {
	*gateways.GoExGateway
	apiFactory *HbdmAPIFactory
}

func (gateway *HbdmGateway) GetExchange() string {
	return "HBDM"
}

func NewHbdmAPIFactory() *HbdmAPIFactory {
	return &HbdmAPIFactory{}
}

type HbdmAPIFactory struct {
	futureRestAPI   *HbdmRestAPI
	futureWs        *HbdmWsWrapper
	futureAuthoried bool
	config          *HbdmGatewayConfig
}

func (factory *HbdmAPIFactory) Config(gateway *gateways.GoExGateway, config interface{}) error {
	if config == nil {
		config = GetDefaultHbdmGatewayConfig()
	}
	c, ok := config.(*HbdmGatewayConfig)
	if !ok {
		return fmt.Errorf("invalid config type of HbdmAPIFactory: %v", config)
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

func (factory *HbdmAPIFactory) GetFutureAPI() (gateways.ExtendedFutureRestAPI, error) {
	apiBuilder := builder.NewAPIBuilder()
	if factory.config.HTTPTimeout != 0 {
		apiBuilder.HttpTimeout(factory.config.HTTPTimeout)
	}
	if factory.config.HTTPProxy != "" {
		apiBuilder.HttpProxy(factory.config.HTTPProxy)
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
	}
	factory.futureRestAPI = NewHbdmRestAPI(apiConfig)
	return factory.futureRestAPI, nil
}

func (factory *HbdmAPIFactory) GetFutureWs() (gateways.FutureWebsocket, error) {
	var fallback gateways.FutureWebsocket
	apiKey := factory.config.APIKey
	apiSecretKey := factory.config.APISecretKey
	passphrase := factory.config.APIPassphrase
	factory.futureWs = NewHbdmWsWrapper()
	if factory.futureAuthoried {
		err := factory.futureWs.Login(apiKey, apiSecretKey, passphrase)
		if err != nil {
			return fallback, err
		}
	}
	return factory.futureWs, nil
}

func (factory *HbdmAPIFactory) GetFutureAuthoried() bool {
	return factory.futureAuthoried
}
