/*
 * @Date: 2021-04-16 19:53:00
 * @LastEditors: KUNzfw
 * @LastEditTime: 2021-04-16 21:55:18
 * @FilePath: \go-onebot\caller\wscaller.go
 */
package caller

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/mitchellh/mapstructure"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const TIME_OUT time.Duration = time.Second * 10
const ECHO_FLAG string = "go-onebot"

// 通过websocket进行api调用的封装
type WsCaller struct {
	url          string
	access_token string
	ctx          context.Context
}

// api调用的返回数据
type responseData struct {
	Status  string
	Retcode int
	Data    interface{}
	Echo    string
}

// CreateWsCaller 创建WsCaller实例
func CreateWsCaller(url string, access_token string, ctx context.Context) *WsCaller {
	return &WsCaller{
		url:          url,
		access_token: access_token,
		ctx:          ctx,
	}
}

// Call 实现Call接口
func (wc *WsCaller) Call(action string, data map[string]interface{}, result interface{}) error {
	// 设置超时
	ctx, cancel := context.WithTimeout(wc.ctx, TIME_OUT)
	defer cancel()

	// 处理请求头
	opts := &websocket.DialOptions{}
	opts.HTTPHeader = http.Header{}
	if wc.access_token != "" {
		opts.HTTPHeader.Add("Authorization", "Bearer "+wc.access_token)
	}

	// 建立websocket连接
	c, resp, err := websocket.Dial(ctx, wc.url, opts)

	// 检查鉴权错误
	if resp.StatusCode == 401 {
		return errors.New("API服务器连接失败: 401 Unauthorized, 可能因为访问密钥未提供")
	}
	if resp.StatusCode == 403 {
		return errors.New("API服务器连接失败: 403 Forbidden, 可能因为访问密钥错误")
	}
	// 其他错误
	if err != nil {
		return errors.New("API服务器连接失败: " + err.Error())
	}

	defer c.Close(websocket.StatusInternalError, "internal error")

	// 编码参数，使用echo过滤生命周期回报
	wsdata := map[string]interface{}{
		"action": action,
		"params": data,
		"echo":   ECHO_FLAG,
	}

	// 发送数据
	err = wsjson.Write(ctx, c, wsdata)
	if err != nil {
		return errors.New("向API服务器发送数据失败: " + err.Error())
	}

	raw_result := make(map[string]interface{})
	// 接受回报
	for {
		err = wsjson.Read(ctx, c, &raw_result)
		if err != nil {
			return errors.New("从服务器读取数据失败: " + err.Error())
		}
		if raw_result["echo"] == ECHO_FLAG {
			break
		}
	}

	c.Close(websocket.StatusNormalClosure, "")

	// 将数据转换为结构体
	resp_result := responseData{
		Data: result,
	}
	if err := mapstructure.Decode(raw_result, &resp_result); err != nil {
		return errors.New("解析API调用返回失败: " + err.Error())
	}

	// 检测400和404错误
	switch resp_result.Retcode {
	case 1404:
		return errors.New("API调用失败: 404 Not Found")
	case 1400:
		return errors.New("API调用失败: 400 Bad Request")
	}

	return nil
}
