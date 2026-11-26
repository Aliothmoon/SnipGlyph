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

	"SnipGlyph/internal/i18n"
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

func (b *Baidu) Name() string { return i18n.Current().BaiduName }

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
	return "", fmt.Errorf(i18n.Current().BaiduUnsupportedLang, l)
}

func fromBaiduCode(c string) Lang {
	for l, bc := range baiduCodes {
		if bc == c {
			return l
		}
	}
	return Lang(c)
}



func (b *Baidu) Translate(ctx context.Context, req Request, partial func(string)) (Result, error) {
	tr := i18n.Current()
	if b.AppID == "" || b.Key == "" {
		return Result{}, errors.New(tr.BaiduNotConfigured)
	}
	if len(req.Text) > baiduMaxBytes {
		return Result{}, fmt.Errorf(tr.BaiduTextTooLong, baiduMaxBytes, len(req.Text))
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
		return Result{}, fmt.Errorf(tr.BaiduRequestFailed, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Result{}, fmt.Errorf(tr.BaiduReadFailed, err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf(tr.BaiduHTTPStatus, resp.StatusCode)
	}

	var r struct {
		From      string                 `json:"from"`
		Results   []struct{ Dst string } `json:"trans_result"`
		ErrorCode any                    `json:"error_code"` // string or number
		ErrorMsg  string                 `json:"error_msg"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return Result{}, fmt.Errorf(tr.BaiduParseFailed, err)
	}
	if code := fmt.Sprint(r.ErrorCode); r.ErrorCode != nil && code != "52000" {
		msg := tr.BaiduErrors[code]
		if msg == "" {
			msg = r.ErrorMsg
		}
		return Result{}, fmt.Errorf(tr.BaiduAPIError, code, msg)
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
