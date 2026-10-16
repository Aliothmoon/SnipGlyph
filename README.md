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
SNIPGLYPH_UI_TESTS=1 go test ./internal/capture ./internal/ui ./internal/toast   # 会在屏幕上闪现遮罩层、弹窗、设置 / 历史 / 剪贴板窗口、toast
SNIPGLYPH_BAIDU_APPID=... SNIPGLYPH_BAIDU_KEY=... go test ./internal/translate   # 调用真实的百度翻译 API
SNIPGLYPH_CLIPBOARD_TESTS=1 go test ./internal/winclip ./internal/textgrab   # 读写真实剪贴板，结束时还原
```

## 进程

同一个 exe 有两种模式：默认是托盘程序；`snipglyph.exe --ocr-worker <缓存目录>` 是 OCR 子进程，
由托盘程序在按下截图快捷键时启动、空闲 10 分钟后结束，日志经 stderr 转进托盘程序的日志。

## 运行时文件

| 路径 | 内容 |
|---|---|
| `%APPDATA%\SnipGlyph\config.json` | 配置（首次启动写入默认值；手动改完在托盘菜单点「重新加载配置」） |
| `%LOCALAPPDATA%\SnipGlyph\history.db` | 翻译历史（SQLite，只存文字，最近 5000 条） |
| `%LOCALAPPDATA%\SnipGlyph\clipboard.db` | 剪贴板历史（文字、HTML、RTF、图片 PNG、文件路径） |
| `%LOCALAPPDATA%\SnipGlyph\logs\snipglyph.log` | 日志，超过 5MB 启动时轮转 |
| `%LOCALAPPDATA%\SnipGlyph\oneocr\<版本>\` | 从截图工具复制出来的 OneOCR DLL（约 56MB） |

开机自启写在 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 的 `SnipGlyph` 值里，指向开启时的 exe 路径；
exe 换了位置要在设置里重新保存一次。任务管理器「启动应用」里禁用后，设置页显示为关闭。

## 设置

托盘菜单「设置」：开机自启、历史记录开关、快捷键（点输入框后直接按组合键，Backspace 清除）、目标语言、
百度翻译的 APP ID / 密钥 / 代理（可先点「测试」）。保存后立即生效，不用重启。

## 历史记录

托盘菜单「历史记录」：按时间倒序列出 OCR 和翻译结果，搜索框按空格分隔多个关键字（都要命中），
三个字以上走全文索引（trigram），更短的逐条匹配。双击或 Enter 在弹窗里重新打开，可改原文再翻译，结果写回同一条；
Delete 删除选中的一条，「清空全部」删除后会压缩数据库文件，不留残余文字。

## 剪贴板历史

后台记录每次复制（取代 CopyQ），`Alt+V` 在鼠标所在屏幕中央打开面板，再按一次关闭：

| 按键 | 作用 |
|---|---|
| 直接打字 | 搜索当前标签（空格分隔多个关键字） |
| ↑ ↓ / PgUp PgDn / Ctrl+Home End | 选择 |
| Enter / 双击 | 粘贴到打开面板前的窗口 |
| Shift+Enter | 以纯文本粘贴（去掉 HTML / RTF） |
| Ctrl+C | 只放进剪贴板 |
| Ctrl+P | 置顶 / 取消置顶 |
| F2 | 编辑文本（Ctrl+S 保存，保存后去掉原格式） |
| Del | 删除（搜索框有字时用 Shift+Del） |
| Ctrl+Tab | 切换标签 |
| 右键 | 以上操作，以及移动 / 复制到其他标签；标签栏右键可新建、重命名、删除标签 |

- 默认标签「剪贴板」自动收录，保留 5000 条（置顶的不计）；再次复制相同内容时把原条目移到最前。自建标签不自动收录、不淘汰。
- 不记录密码管理器标记为私密的内容（`ExcludeClipboardContentFromMonitorProcessing` 等），也不记录划词模拟的 Ctrl+C。
- 文字复制只存文字和格式，不存 Office 附带的图片；单张图片超过 6400 万像素不记录。
- 粘贴靠切回原窗口后模拟 Ctrl+V；以管理员身份运行的程序收不到（UIPI 限制）。
- 设置里可以关闭记录或改快捷键。`Alt+V` 在记事本等程序里原本是「查看」菜单。

## 配置文件

设置页覆盖了所有选项；也可以直接编辑 `config.json`，然后在托盘菜单点「重新加载配置」：

```json
"translate": {
  "engine": "baidu",
  "target_lang": "zh-Hans",
  "baidu": { "app_id": "你的 APP ID", "key": "你的密钥", "proxy": "" }
},
"history": { "enabled": true },
"clipboard": { "enabled": true, "limit": 5000 }
```

- `key` 可以填明文，程序读取后会立即改写为 `dpapi:...` 密文，只有当前 Windows 账户能解开。
- `proxy`：空字符串跟随系统代理；`"direct"` 直连；或 `"http://127.0.0.1:7890"`、`"socks5://127.0.0.1:1080"`。
- `target_lang`：非中文原文的目标语言（中文原文固定译成英文），可选 `zh-Hans`、`zh-Hant`、`en`、`ja`、`ko`、`fr`、`de`、`es`、`ru`。
