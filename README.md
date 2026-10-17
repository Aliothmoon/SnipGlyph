# SnipGlyph

Windows 11 上的 OCR + 翻译 + 剪贴板历史托盘工具，纯 Go 编写。

## 功能

| 功能 | 默认快捷键 |
|---|---|
| 截图 OCR | `Win+Alt+Shift+C` |
| 静默 OCR（识别后直接复制） | `Win+Alt+C` |
| 截图翻译 | `Win+Alt+S` |
| 划词翻译 | `Win+Alt+X` |
| 输入翻译 | `Win+Alt+I` |
| 剪贴板历史 | `Alt+V` |

OCR 使用 Windows 11 截图工具自带的离线引擎 OneOCR，翻译使用百度翻译。另有翻译历史、设置页和开机自启。

## 使用

从 [Releases](https://github.com/Aliothmoon/SnipGlyph/releases) 下载 exe 直接运行，程序常驻托盘，快捷键和翻译密钥在托盘菜单的「设置」里配置。

需要 Windows 11 x64，并装有新版「截图工具」。

## 构建

```sh
CGO_ENABLED=0 go build -ldflags "-H windowsgui -s -w" -o bin/snipglyph.exe ./cmd/snipglyph
```

## 声明

本项目不包含任何微软文件，OneOCR 属于微软，运行时从本机截图工具读取。与 Microsoft 无关。
