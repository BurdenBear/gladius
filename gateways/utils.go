package gateways

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

var tail_zero_re = regexp.MustCompile("0+$")

func NormalizeByIncrement(num float64, increment string) (string, error) {
	precision := 0
	i := strings.Index(increment, ".")
	// increment is decimal
	if i > -1 {
		decimal := increment[i+1:]
		trimTailZero := tail_zero_re.ReplaceAllString(decimal, "")
		precision = len(trimTailZero)
		return fmt.Sprintf("%."+fmt.Sprintf("%df", precision), num), nil
	}
	// increment is int
	incrementInt, err := strconv.ParseInt(increment, 10, 64)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(int(num) / int(incrementInt)), nil
}
