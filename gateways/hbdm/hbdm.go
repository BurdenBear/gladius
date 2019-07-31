package hbdm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurdenBear/gladius/gateways"

	goex "github.com/nntaoli-project/GoEx"
	"github.com/nntaoli-project/GoEx/huobi"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetDefaultHbdmConfig() *goex.APIConfig {
	apiKey := getEnv("GOEX_HBDM_API_KEY", "")
	apiSecretKey := getEnv("GOEX_HBDM_API_SECRET_KEY", "")
	passphrase := ""
	endpoint := getEnv("GOEX_HBDM_RESTFUL_URL", "https://api.hbdm.com")
	config := &goex.APIConfig{
		HttpClient:    http.DefaultClient,
		Endpoint:      endpoint,
		ApiKey:        apiKey,
		ApiSecretKey:  apiSecretKey,
		ApiPassphrase: passphrase,
	}
	return config
}

type HbdmRestAPI struct {
	*huobi.Hbdm
	config *goex.APIConfig
}

var (
	apiURL = "https://api.hbdm.com"
)

func NewHbdmRestAPI(config *goex.APIConfig) *HbdmRestAPI {
	dm := &HbdmRestAPI{}
	config.Endpoint = apiURL // apiurl必须为此值，不然goex.hdbm处理逻辑可能出错
	dm.config = config
	dm.Hbdm = huobi.NewHbdm(config)
	return dm
}

func (dm *HbdmRestAPI) adaptOrderType(orderType int) (string, error) {
	switch orderType {
	case goex.ORDER_TYPE_LIMIT:
		return "limit", nil
	case goex.ORDER_TYPE_POST_ONLY:
		return "post_only", nil
	default:
		return "", fmt.Errorf("orderType not support in hbdm: %d", orderType)
	}
}

func (dm *HbdmRestAPI) adaptOpenType(openType int) (string, string) {
	switch openType {
	case goex.OPEN_BUY:
		return "buy", "open"
	case goex.OPEN_SELL:
		return "sell", "open"
	case goex.CLOSE_SELL:
		return "sell", "close"
	case goex.CLOSE_BUY:
		return "buy", "close"
	default:
		return "", ""
	}
}

func (dm *HbdmRestAPI) GetFutureDepth(currencyPair goex.CurrencyPair, contractType string, size int) (*goex.Depth, error) {
	if contractType == goex.SWAP_CONTRACT {
		return nil, errors.New("hbdm not support swap contract")
	}
	return dm.Hbdm.GetFutureDepth(currencyPair, contractType, size)
}

func (dm *HbdmRestAPI) nomalizePrice(currencyPair goex.CurrencyPair, price string) (string, error) {
	var tickSize string
	if currencyPair == goex.BTC_USD {
		tickSize = "0.01"
	} else {
		tickSize = "0.001"
	}
	p, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return "", err
	}
	return gateways.NormalizeByIncrement(p, tickSize)
}

func (dm *HbdmRestAPI) nomalizeAmount(currencyPair goex.CurrencyPair, amount string) (string, error) {
	lotSize := "1"
	a, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return "", err
	}
	return gateways.NormalizeByIncrement(a, lotSize)
}

func (dm *HbdmRestAPI) PlaceFutureOrder(currencyPair goex.CurrencyPair, contractType, price, amount string, openType, matchPrice, leverRate int) (string, error) {
	price, err := dm.nomalizePrice(currencyPair, price)
	if err != nil {
		return "", err
	}
	amount, err = dm.nomalizeAmount(currencyPair, amount)
	if err != nil {
		return "", err
	}
	return dm.Hbdm.PlaceFutureOrder(currencyPair, contractType, price, amount, openType, matchPrice, leverRate)
}

func (dm *HbdmRestAPI) PlaceFutureOrder2(currencyPair goex.CurrencyPair, contractType, price, amount string, orderType, openType, matchPrice, leverRate int) (string, error) {
	var data struct {
		OrderId  int64 `json:"order_id"`
		COrderId int64 `json:"client_order_id"`
	}

	price, err := dm.nomalizePrice(currencyPair, price)
	if err != nil {
		return "", err
	}
	amount, err = dm.nomalizeAmount(currencyPair, amount)
	if err != nil {
		return "", err
	}

	params := &url.Values{}
	path := "/api/v1/contract_order"

	params.Add("contract_type", contractType)
	params.Add("symbol", currencyPair.CurrencyA.Symbol)
	params.Add("price", price)
	params.Add("volume", amount)
	params.Add("lever_rate", fmt.Sprint(leverRate))
	// 以对手价下单，orderType不生效
	if matchPrice == 1 {
		params.Add("order_price_type", "opponent")
	} else {
		typeStr, err := dm.adaptOrderType(orderType)
		if err != nil {
			return "", err
		}
		params.Add("order_price_type", typeStr)
	}
	params.Add("contract_code", "")

	direction, offset := dm.adaptOpenType(openType)
	params.Add("offset", offset)
	params.Add("direction", direction)

	err = dm.doRequest(path, params, &data)

	return fmt.Sprint(data.OrderId), err
}

func (dm *HbdmRestAPI) buildPostForm(reqMethod, path string, postForm *url.Values) error {
	postForm.Set("AccessKeyId", dm.config.ApiKey)
	postForm.Set("SignatureMethod", "HmacSHA256")
	postForm.Set("SignatureVersion", "2")
	postForm.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05"))
	apiURL := dm.getApiURL()
	domain := strings.Replace(apiURL, "https://", "", len(apiURL))
	payload := fmt.Sprintf("%s\n%s\n%s\n%s", reqMethod, domain, path, postForm.Encode())
	sign, _ := goex.GetParamHmacSHA256Base64Sign(dm.config.ApiSecretKey, payload)
	postForm.Set("Signature", sign)

	return nil
}

func (dm *HbdmRestAPI) getApiURL() string {
	if dm.config.Endpoint == "" {
		return apiURL
	} else {
		return dm.config.Endpoint
	}
}

func (dm *HbdmRestAPI) doRequest(path string, params *url.Values, data interface{}) error {
	dm.buildPostForm("POST", path, params)
	jsonD, _ := goex.ValuesToJson(*params)
	//log.Println(string(jsonD))

	var ret huobi.BaseResponse

	resp, err := goex.HttpPostForm3(dm.config.HttpClient, dm.getApiURL()+path+"?"+params.Encode(), string(jsonD),
		map[string]string{"Content-Type": "application/json", "Accept-Language": "zh-cn"})

	if err != nil {
		return err
	}

	//log.Println(string(resp))
	err = json.Unmarshal(resp, &ret)
	if err != nil {
		return err
	}

	if ret.Status != "ok" {
		return errors.New(fmt.Sprintf("%d:[%s]", ret.ErrCode, ret.ErrMsg))
	}

	return json.Unmarshal(ret.Data, data)
}
