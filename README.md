# SnipGlyph

Windows 11 上的 OCR + 划词翻译托盘工具。方案见 [docs/PLAN.md](docs/PLAN.md)，OneOCR 技术细节见 [docs/oneocr.md](docs/oneocr.md)。

## 构建

```sh
go generate ./cmd/snipglyph   # 由 winres/ 生成 manifest + 图标资源（.syso），改了 winres/ 才需要
CGO_ENABLED=0 go build -ldflags "-H windowsgui -s -w" -o bin/snipglyph.exe ./cmd/snipglyph
```

重画图标：`go run ./tools/genicon -o winres/icon.png`，然后重新 `go generate`。

刚编译出的 exe 第一次启动可能被 Windows Defender 扫描十几秒，之后正常。

排查卡死用调试版：`go build -tags debug ...`，以 `SNIPGLYPH_PPROF=127.0.0.1:6060` 启动，
再访问 `http://127.0.0.1:6060/debug/pprof/goroutine?debug=2` 看所有 goroutine 的栈。

## 测试

```sh
go test ./...                              # 需要本机装有截图工具，否则 OCR 测试自动跳过
SNIPGLYPH_UI_TESTS=1 go test ./internal/capture ./internal/ui ./internal/toast   # 会在屏幕上闪现遮罩层、弹窗和 toast
SNIPGLYPH_BAIDU_APPID=... SNIPGLYPH_BAIDU_KEY=... go test ./internal/translate   # 调用真实的百度翻译 API
SNIPGLYPH_CLIPBOARD_TESTS=1 go test ./internal/textgrab   # 读写真实剪贴板，结束时还原
```

## 进程

同一个 exe 有两种模式：默认是托盘程序；`snipglyph.exe --ocr-worker <缓存目录>` 是 OCR 子进程，
由托盘程序在按下截图快捷键时启动、空闲 10 分钟后结束，日志经 stderr 转进托盘程序的日志。

## 运行时文件

| 路径 | 内容 |
|---|---|
| `%APPDATA%\SnipGlyph\config.json` | 配置（首次启动写入默认值；改完在托盘菜单点「重新加载配置」） |
| `%LOCALAPPDATA%\SnipGlyph\logs\snipglyph.log` | 日志，超过 5MB 启动时轮转 |
| `%LOCALAPPDATA%\SnipGlyph\oneocr\<版本>\` | 从截图工具复制出来的 OneOCR DLL（约 56MB） |

## 翻译配置

设置页（M4）完成前，直接编辑 `config.json` 的 `translate` 部分，然后在托盘菜单点「重新加载配置」：

```json
"translate": {
  "engine": "baidu",
  "target_lang": "zh-Hans",
  "baidu": { "app_id": "你的 APP ID", "key": "你的密钥", "proxy": "" }
}
```

- `key` 可以填明文，程序读取后会立即改写为 `dpapi:...` 密文，只有当前 Windows 账户能解开。
- `proxy`：空字符串跟随系统代理；`"direct"` 直连；或 `"http://127.0.0.1:7890"`、`"socks5://127.0.0.1:1080"`。
- `target_lang`：非中文原文的目标语言（中文原文固定译成英文），可选 `zh-Hans`、`zh-Hant`、`en`、`ja`、`ko`、`fr`、`de`、`es`、`ru`。
