package entrypoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"plugin"
	"reflect"
	"strings"
	"syscall"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/gateways"
	"github.com/BurdenBear/gladius/gateways/bitmex"
	"github.com/BurdenBear/gladius/gateways/hbdm"
	"github.com/BurdenBear/gladius/gateways/okex"
	"github.com/BurdenBear/gladius/strategy"
	"github.com/spf13/viper"
)

var logger = GetLogger()

type IRegisterable interface {
	Register(engine *EventEngine)
}

func GetConfig() {
	viper.SetConfigName("gladius")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.gladius")
	viper.AddConfigPath("/etc/gladius/")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
}

var exchangeConstructors = map[string]func(string, *EventEngine, interface{}) (gateways.IGateway, error){
	"okex":   okex.NewOkexGatewayFromConfig,
	"hbdm":   hbdm.NewHbdmGatewayFromConfig,
	"bitmex": bitmex.NewBitmexGatewayFromConfig,
}

func GetGateways(engine *EventEngine) []gateways.IGateway {
	gws := make(map[string]gateways.IGateway)
	if viper.IsSet("gateways") {
		gwViper := viper.Sub("gateways")
		gwConfigs := gwViper.AllSettings()
		for gwName, gwConfig := range gwConfigs {
			gwName = strings.ToUpper(gwName)
			exchange := gwViper.Sub(gwName).GetString("exchange")
			if exchange == "" {
				for exchangeName, _ := range exchangeConstructors {
					if strings.HasPrefix(strings.ToLower(gwName), exchangeName) {
						exchange = exchangeName
						break
					}
				}
			}
			if c, ok := exchangeConstructors[exchange]; ok {
				gateway, err := c(gwName, engine, gwConfig)
				if err == nil {
					if _, ok := gws[gwName]; ok {
						logger.Warningf("Gateway(%s) already exist, skip it! Please check your config file", gwName)
					} else {
						gws[gwName] = gateway
						logger.Infof("Gateway(%s) created  success", gwName)
					}
				} else {
					logger.Errorf("Gateway(%s) created faild: %s", gwName, err)
				}
			} else {
				logger.Errorf("Unknow exchange(%s) for gateway(%s)", exchange, gwName)
			}
		}
	}
	gwList := make([]gateways.IGateway, 0, len(gws))
	for _, v := range gws {
		gwList = append(gwList, v)
	}
	return gwList
}

func loadStrategyConstructor(file string, cls string) (strategy.IStrategyConstructor, error) {
	var fallback strategy.IStrategyConstructor = nil
	p, err := plugin.Open(file)
	if err != nil {
		return fallback, err
	}
	v, err := p.Lookup(cls)
	if err != nil {
		return fallback, err
	}
	if cons, ok := v.(strategy.IStrategyConstructor); ok {
		return cons, nil
	}
	return fallback, fmt.Errorf("%+v does not implement interface \"strategy.IStrategy\"", v)
}

type InjectData struct {
	Name  string      `mapstructure:"name"`
	Value interface{} `mapstructure:"name"`
}

func injectData(dst interface{}, src []*InjectData) error {
	vd := reflect.ValueOf(dst)
	if vd.Kind() != reflect.Ptr {
		return errors.New("dst is not a pointer")
	}
	// vb is a pointer, indirect it to get the
	// underlying value, and make sure it is a struct.
	vd = vd.Elem()
	if vd.Kind() != reflect.Struct {
		return errors.New("*dst is not a struct")
	}

	for _, s := range src {
		field := vd.FieldByName(s.Name)
		vs := reflect.ValueOf(s.Value)
		if field.CanSet() {
			tt := reflect.TypeOf(s.Value)
			tf := field.Type()
			if tt == tf {
				field.Set(vs)
				return nil
			} else {
				return fmt.Errorf("Type to inject is %s, but type of field(%s) is %s", tt, s.Name, tf)
			}
		} else {
			return fmt.Errorf("Field(%s) is not settable", s.Name)
		}
	}
	return nil
}

func GetStrategy(router *strategy.Router) []strategy.IStrategy {
	stgs := make([]strategy.IStrategy, 0)
	if viper.IsSet("strategies") {
		stgViper := viper.Sub("strategies")
		stgConfigs := stgViper.AllSettings()
		for stgName, _ := range stgConfigs {
			vi := stgViper.Sub(stgName)
			cons := vi.GetString("constructor")
			arr := strings.Split(cons, ":")
			if len(arr) != 2 {
				logger.Errorf("Error constructor format of strategy(%s): %s, should be $pluginPath:$funcName", stgName, cons)
			}
			stgCons, err := loadStrategyConstructor(arr[0], arr[1])
			if err != nil {
				logger.Errorf("Error when load constructor of strategy(%s): %s", stgName, err)
				continue
			}
			stg := stgCons(stgName, router)
			logger.Infof("Strategy(%s) created", stgName)
			toInjects := []*InjectData{}
			vi.Unmarshal(injectData)
			symbols := vi.GetStringSlice("symbols")
			toInjects = append(toInjects, &InjectData{Name: "Symbols", Value: symbols})
			if err := injectData(stg, toInjects); err == nil {
				logger.Infof("Strategy(%s) inject success", stgName)
				stgs = append(stgs, stg)
			} else {
				logger.Errorf("Strategy(%s) inject failed and will not be start: %s", stgName, err)
			}
		}
	}
	return stgs
}

func GetLogConfig() {
	c := NewLogConfig()
	if viper.IsSet("log") {
		logConfig := viper.Sub("log").AllSettings()
		v, err := json.Marshal(logConfig)
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(v, c)
		if err != nil {
			panic(err)
		}
	}
	InitLog(c)
}

func Run() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM)
	GetConfig()
	GetLogConfig()
	engine := NewEventEngine(100000)
	gws := GetGateways(engine)
	router := strategy.NewRouter(engine)
	router.AddGateway(gws...)
	stgs := GetStrategy(router)
	router.AddStrategy(stgs...)
	engine.Start()
	for _, v := range gws {
		err := v.Connect()
		name := v.GetName()
		if err != nil {
			logger.Errorf("Gateway (%s) connect failed: %s", name, err)
		} else {
			logger.Infof("Gateway (%s) connect successed", name)
		}
	}
	for _, stg := range stgs {
		name := stg.GetName()
		logger.Info("Strategy(%s) initialing...", name)
		stg.Init()
		logger.Info("Strategy(%s) starting...", name)
		stg.Start()
		logger.Info("Strategy(%s) started", name)
	}
loop:
	for {
		select {
		case <-sigs:
			logger.Info("Got shutdown, exiting")
			// Break out of the outer for statement and end the program
			engine.Stop()
			break loop
		}
	}
}
