# SnipGlyph

Windows 11 上的 OCR + 划词翻译托盘工具。方案见 [docs/PLAN.md](docs/PLAN.md)，OneOCR 技术细节见 [docs/oneocr.md](docs/oneocr.md)。

## 构建

```sh
go generate ./cmd/snipglyph   # 由 winres/ 生成 manifest + 图标资源（.syso），改了 winres/ 才需要
CGO_ENABLED=0 go build -ldflags "-H windowsgui -s -w" -o bin/snipglyph.exe ./cmd/snipglyph
```

重画图标：`go run ./tools/genicon -o winres/icon.png`，然后重新 `go generate`。

刚编译出的 exe 第一次启动可能被 Windows Defender 扫描十几秒，之后正常。

## 测试

```sh
go test ./...                              # 需要本机装有截图工具，否则 OCR 测试自动跳过
SNIPGLYPH_UI_TESTS=1 go test ./internal/capture   # 会在屏幕上闪现截图遮罩层
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
