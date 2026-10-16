package translate

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const baiduEndpoint = "https://fanyi-api.baidu.com/api/trans/vip/translate"

// baiduMaxBytes is the documented per-request limit on q.
const baiduMaxBytes = 6000

// Baidu is the Baidu general translation API (fanyi-api.baidu.com).
type Baidu struct {
	AppID string
	Key   string
	HTTP  *http.Client
	// Endpoint overrides the API URL (tests).
	Endpoint string
}

func (b *Baidu) Name() string { return "百度翻译" }

var baiduCodes = map[Lang]string{
	Auto:               "auto",
	ChineseSimplified:  "zh",
	ChineseTraditional: "cht",
	English:            "en",
	Japanese:           "jp",
	Korean:             "kor",
	French:             "fra",
	German:             "de",
	Spanish:            "spa",
	Russian:            "ru",
}

func baiduCode(l Lang) (string, error) {
	if c, ok := baiduCodes[l]; ok {
		return c, nil
	}
	return "", fmt.Errorf("百度翻译不支持语言 %s", l)
}

func fromBaiduCode(c string) Lang {
	for l, bc := range baiduCodes {
		if bc == c {
			return l
		}
	}
	return Lang(c)
}

// baiduErrors explains the error codes users actually run into.
var baiduErrors = map[string]string{
	"52001": "请求超时，请重试",
	"52002": "百度翻译系统错误，请重试",
	"52003": "APP ID 无效，或未开通通用翻译服务",
	"54000": "缺少必填参数",
	"54001": "签名错误，请检查密钥",
	"54003": "访问频率受限（标准版每秒 1 次）",
	"54004": "账户余额不足",
	"54005": "长文本请求过于频繁，请稍后再试",
	"58000": "客户端 IP 不在百度后台的 IP 白名单中",
	"58001": "不支持该语言方向",
	"58002": "服务已关闭，请到百度翻译开放平台开启",
	"90107": "认证未通过或未生效",
}

func (b *Baidu) Translate(ctx context.Context, req Request, partial func(string)) (Result, error) {
	if b.AppID == "" || b.Key == "" {
		return Result{}, errors.New("百度翻译未配置 APP ID 或密钥")
	}
	if len(req.Text) > baiduMaxBytes {
		return Result{}, fmt.Errorf("文本过长：百度翻译单次最多 %d 字节，当前 %d 字节", baiduMaxBytes, len(req.Text))
	}
	from, err := baiduCode(req.From)
	if err != nil {
		return Result{}, err
	}
	to, err := baiduCode(req.To)
	if err != nil {
		return Result{}, err
	}

	salt := strconv.Itoa(rand.IntN(1 << 30))
	form := url.Values{
		"q":     {req.Text},
		"from":  {from},
		"to":    {to},
		"appid": {b.AppID},
		"salt":  {salt},
		"sign":  {baiduSign(b.AppID, req.Text, salt, b.Key)},
	}
	endpoint := b.Endpoint
	if endpoint == "" {
		endpoint = baiduEndpoint
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Result{}, err
	}
	hreq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := b.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(hreq)
	if err != nil {
		return Result{}, fmt.Errorf("百度翻译请求失败：%w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Result{}, fmt.Errorf("百度翻译响应读取失败：%w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("百度翻译返回 HTTP %d", resp.StatusCode)
	}

	var r struct {
		From      string                 `json:"from"`
		Results   []struct{ Dst string } `json:"trans_result"`
		ErrorCode any                    `json:"error_code"` // string or number
		ErrorMsg  string                 `json:"error_msg"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return Result{}, fmt.Errorf("百度翻译响应无法解析：%w", err)
	}
	if code := fmt.Sprint(r.ErrorCode); r.ErrorCode != nil && code != "52000" {
		msg := baiduErrors[code]
		if msg == "" {
			msg = r.ErrorMsg
		}
		return Result{}, fmt.Errorf("百度翻译错误 %s：%s", code, msg)
	}
	// Baidu translates each input line separately.
	lines := make([]string, len(r.Results))
	for i, t := range r.Results {
		lines[i] = t.Dst
	}
	out := Result{Text: strings.Join(lines, "\n"), From: fromBaiduCode(r.From)}
	if partial != nil {
		partial(out.Text)
	}
	return out, nil
}

func baiduSign(appID, q, salt, key string) string {
	sum := md5.Sum([]byte(appID + q + salt + key))
	return hex.EncodeToString(sum[:])
}
