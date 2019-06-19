package gladius

import (
	"fmt"
	"strings"

	goex "github.com/nntaoli-project/GoEx"
)

type IContract interface {
	GetID() string
	GetGateway() string
	GetSymbol() string
}

type CryptoCurrencyContract struct {
	ID           string
	Symbol       string
	Gateway      string
	Exchange     string
	CurrencyPair goex.CurrencyPair
	ContractType string
}

func NewCryptoCurrencyContract(gateway string, exchange string, currencyPair goex.CurrencyPair, contractType string) *CryptoCurrencyContract {
	symbol := GetCryptoCurrencyContractSymbol(currencyPair, contractType)

	id := strings.Join([]string{gateway, symbol}, FDTRADER_SEPERATOR)

	return &CryptoCurrencyContract{
		ID:           strings.ToUpper(id),
		Symbol:       symbol,
		Gateway:      gateway,
		Exchange:     exchange,
		ContractType: contractType,
		CurrencyPair: currencyPair,
	}
}

func (c *CryptoCurrencyContract) GetID() string {
	return c.ID
}

func (c *CryptoCurrencyContract) GetSymbol() string {
	return c.Symbol
}

func (c *CryptoCurrencyContract) GetGateway() string {
	return c.Gateway
}

func GetCryptoCurrencyContractSymbol(currencyPair goex.CurrencyPair, contractType string) string {
	return strings.ToUpper(strings.Join([]string{currencyPair.ToSymbol("_"), contractType}, FDTRADER_SEPERATOR))
}

func ParseCryptoCurrencyContractSymbol(symbol string) (goex.CurrencyPair, string, error) {
	parts := strings.Split(symbol, FDTRADER_SEPERATOR)
	if len(parts) == 2 {
		currencies := strings.Split(parts[0], "_")
		if len(currencies) == 2 {
			currencyA := goex.NewCurrency(currencies[0], "")
			currencyB := goex.NewCurrency(currencies[1], "")
			currencyPair := goex.NewCurrencyPair(currencyA, currencyB)
			contractType := strings.ToLower(parts[1])
			return currencyPair, contractType, nil
		}
	}
	return goex.UNKNOWN_PAIR, "", fmt.Errorf("unknown crypto currency contract: %s", symbol)
}
