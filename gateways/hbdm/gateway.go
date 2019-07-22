package hbdm

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

type HbdmGatewayConfig struct {
	*gateways.GoExGatewayConfig
}

func NewHbdmGatewayFromConfig(name string, engine *EventEngine, value interface{}) (gateways.IGateway, error) {
	var config *HbdmGatewayConfig
	if c, ok := value.(*HbdmGatewayConfig); ok {
		config = c
	} else {
		v, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		config = new(HbdmGatewayConfig)
		err = json.Unmarshal(v, config)
		if err != nil {
			return nil, err
		}
	}
	return NewHbdmGateway(name, engine, config)
}

func GetDefaultHbdmGatewayConfig() *HbdmGatewayConfig {
	apiKey := gateways.GetEnv("GOEX_HBDM_API_KEY", "")
	apiSecretKey := gateways.GetEnv("GOEX_HBDM_API_SECRET_KEY", "")
	passphrase := gateways.GetEnv("GOEX_HBDM_PASSPHRASE", "")

	config := &HbdmGatewayConfig{
		GoExGatewayConfig: &gateways.GoExGatewayConfig{
			Secret: gateways.GoExGatewaySecretConfig{
				APIKey:       apiKey,
				APISecretKey: apiSecretKey,
				Passphrase:   passphrase,
			},
		},
	}
	return config
}

func NewHbdmGateway(name string, engine *EventEngine, config interface{}) (*HbdmGateway, error) {
	var err error
	gateway := &HbdmGateway{}
	gateway.apiFactory = NewHbdmAPIFactory()
	gateway.GoExGateway, err = gateways.NewGoExGateway(name, engine, gateway.apiFactory, config)
	if err != nil {
		return nil, err
	}
	return gateway, nil
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
	c, ok := config.(*HbdmGatewayConfig)
	if !ok {
		return fmt.Errorf("invalid config type of HbdmAPIFactory: %v", config)
	}
	if c == nil {
		c = GetDefaultHbdmGatewayConfig()
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

func (factory *HbdmAPIFactory) GetFutureAPI() (gateways.ExtendedFutureRestAPI, error) {
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
	}
	factory.futureRestAPI = NewHbdmRestAPI(apiConfig)
	return factory.futureRestAPI, nil
}

func (factory *HbdmAPIFactory) GetFutureWs() (gateways.FutureWebsocket, error) {
	var fallback gateways.FutureWebsocket
	apiKey := factory.config.Secret.APIKey
	apiSecretKey := factory.config.Secret.APISecretKey
	passphrase := factory.config.Secret.Passphrase
	factory.futureWs = NewHbdmWsWrapper()
	if factory.config.Heartbeat.Websocket > 0 {
		t := time.Duration(factory.config.Heartbeat.Websocket) * time.Second
		factory.futureWs.HbdmWs.mdWs.WsBuilder.Heartbeat(nil, t)
		factory.futureWs.HbdmWs.orderWs.WsBuilder.Heartbeat(nil, t)
	}
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
