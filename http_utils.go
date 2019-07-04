package gladius

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	_log "log"
	"net/http"
	"net/url"
	"strings"
)

func NewHttpRequest(client *http.Client, reqType string, reqUrl string, postData string, requstHeaders map[string]string) ([]byte, http.Header, error) {
	req, _ := http.NewRequest(reqType, reqUrl, strings.NewReader(postData))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 5.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/31.0.1650.63 Safari/537.36")

	if requstHeaders != nil {
		for k, v := range requstHeaders {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	defer resp.Body.Close()

	bodyData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != 200 {
		return nil, nil, errors.New(fmt.Sprintf("HttpStatusCode:%d ,Desc:%s", resp.StatusCode, string(bodyData)))
	}

	return bodyData, resp.Header, nil
}

func HttpGet(client *http.Client, reqUrl string) (map[string]interface{}, http.Header, error) {
	respData, respHeader, err := NewHttpRequest(client, "GET", reqUrl, "", nil)
	if err != nil {
		return nil, nil, err
	}

	var bodyDataMap map[string]interface{}
	err = json.Unmarshal(respData, &bodyDataMap)
	if err != nil {
		_log.Println(string(respData))
		return nil, nil, err
	}
	return bodyDataMap, respHeader, nil
}

func HttpGet2(client *http.Client, reqUrl string, headers map[string]string) (map[string]interface{}, http.Header, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	respData, respHeader, err := NewHttpRequest(client, "GET", reqUrl, "", headers)
	if err != nil {
		return nil, nil, err
	}

	var bodyDataMap map[string]interface{}
	err = json.Unmarshal(respData, &bodyDataMap)
	if err != nil {
		_log.Println("respData", string(respData))
		return nil, nil, err
	}
	return bodyDataMap, respHeader, nil
}

func HttpGet3(client *http.Client, reqUrl string, headers map[string]string) ([]interface{}, http.Header, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	respData, respHeader, err := NewHttpRequest(client, "GET", reqUrl, "", headers)
	if err != nil {
		return nil, nil, err
	}

	var bodyDataMap []interface{}
	err = json.Unmarshal(respData, &bodyDataMap)
	if err != nil {
		_log.Println("respData", string(respData))
		return nil, nil, err
	}
	return bodyDataMap, respHeader, nil
}

func HttpGet4(client *http.Client, reqUrl string, headers map[string]string, result interface{}) (http.Header, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	respData, respHeader, err := NewHttpRequest(client, "GET", reqUrl, "", headers)
	if err != nil {
		return respHeader, err
	}

	err = json.Unmarshal(respData, result)
	if err != nil {
		_log.Printf("HttpGet4 - json.Unmarshal failed : %v, resp %s", err, string(respData))
		return respHeader, err
	}

	return respHeader, nil
}

func HttpGet5(client *http.Client, reqUrl string, headers map[string]string) ([]byte, http.Header, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	respData, respHeader, err := NewHttpRequest(client, "GET", reqUrl, "", headers)
	if err != nil {
		return nil, nil, err
	}

	return respData, respHeader, nil
}

func HttpPostForm(client *http.Client, reqUrl string, postData url.Values) ([]byte, http.Header, error) {
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded"}
	return NewHttpRequest(client, "POST", reqUrl, postData.Encode(), headers)
}

func HttpPostForm2(client *http.Client, reqUrl string, postData url.Values, headers map[string]string) ([]byte, http.Header, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	return NewHttpRequest(client, "POST", reqUrl, postData.Encode(), headers)
}

func HttpPostForm3(client *http.Client, reqUrl string, postData string, headers map[string]string) ([]byte, http.Header, error) {
	return NewHttpRequest(client, "POST", reqUrl, postData, headers)
}

func HttpPostForm4(client *http.Client, reqUrl string, postData map[string]string, headers map[string]string) ([]byte, http.Header, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/json"
	data, _ := json.Marshal(postData)
	return NewHttpRequest(client, "POST", reqUrl, string(data), headers)
}

func HttpDeleteForm(client *http.Client, reqUrl string, postData url.Values, headers map[string]string) ([]byte, http.Header, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	return NewHttpRequest(client, "DELETE", reqUrl, postData.Encode(), headers)
}
