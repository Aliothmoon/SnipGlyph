# SnipGlyph

[English](#english) | [中文](#中文)

---

## English

A lightweight, pure-Go Windows tray utility for offline **OCR**, **translation**, and **clipboard history**.

### Features

| Feature | Default Hotkey |
|---|---|
| Screenshot OCR | `Win+Alt+Shift+C` |
| Silent OCR (copy directly to clipboard) | `Win+Alt+C` |
| Screenshot Translate | `Win+Alt+S` |
| Selection Translate | `Win+Alt+X` |
| Input Translate | `Win+Alt+I` |
| Clipboard History | `Alt+V` |

- **UI Language**: Follows your Windows UI language automatically (`zh-CN` / `English`), and can be switched live in **Settings → General → Language**.
- **Offline OCR**: Powered by Microsoft OneOCR (from local files or Windows 11 Snipping Tool).
- **Translation**: Supports Baidu Translate with history search, settings UI, and optional Windows startup.

### Usage

Download `snipglyph.exe` from [Releases](https://github.com/Aliothmoon/SnipGlyph/releases) and run it. It lives in the system tray; configure hotkeys, language, and translation keys via **Tray → Settings**.

#### Using Existing OneOCR Files (Without MS Store / Snipping Tool)

If you already have `oneocr.dll`, `oneocr.onemodel`, and `onnxruntime.dll` (x64), place all three files in either:

1. A `oneocr\` subfolder next to `snipglyph.exe` (recommended), or
2. Directly beside `snipglyph.exe`.

You can also point to a custom folder via `ocr.dir` in `config.json` or the `SNIPGLYPH_ONEOCR_DIR` environment variable. If no local files are present, SnipGlyph automatically falls back to the installed Windows Snipping Tool.

> **Note**: OneOCR's model loader uses the Windows ANSI code page for the model path; keep the files in an ASCII-compatible path if you encounter path encoding errors.

#### Portable Mode

To store all settings, history, logs, and caches inside the application folder (`<exeDir>\data\`) instead of `%APPDATA%` / `%LOCALAPPDATA%`, enable portable mode in any of these ways:

- Create an empty file named `portable` (or `portable.txt`) next to `snipglyph.exe`, or
- Launch with `snipglyph.exe --portable`, or
- Set environment variable `SNIPGLYPH_PORTABLE=1`.

> **Note**: Saved API keys in `config.json` are encrypted with Windows DPAPI (bound to the current Windows user/machine). If you move a portable folder to another PC or account, other settings remain intact and you only need to re-enter your API key.

### Build

```sh
CGO_ENABLED=0 go build -ldflags "-H windowsgui -s -w" -o bin/snipglyph.exe ./cmd/snipglyph
```

### Disclaimer

This project does **not** bundle or redistribute any Microsoft files. OneOCR belongs to Microsoft and is loaded at runtime from local files you provide or from your installed Windows Snipping Tool. Not affiliated with Microsoft.

---

## 中文

Windows 上的离线 **OCR** + **翻译** + **剪贴板历史**托盘工具，纯 Go 编写。

### 功能

| 功能 | 默认快捷键 |
|---|---|
| 截图 OCR | `Win+Alt+Shift+C` |
| 静默 OCR（识别后直接复制） | `Win+Alt+C` |
| 截图翻译 | `Win+Alt+S` |
| 划词翻译 | `Win+Alt+X` |
| 输入翻译 | `Win+Alt+I` |
| 剪贴板历史 | `Alt+V` |

- **双语界面**：默认自动跟随系统语言（简体中文 / English），可在 **设置 → 常规 → 界面语言** 中实时切换。
- **离线 OCR**：使用 OneOCR（支持自带本地文件或读取本机 Windows 11 截图工具）。
- **翻译与历史**：支持百度翻译、OCR/翻译历史搜索、剪贴板历史和开机自启。

### 使用

从 [Releases](https://github.com/Aliothmoon/SnipGlyph/releases) 下载 `snipglyph.exe` 直接运行，程序常驻托盘，快捷键、语言和翻译密钥可在托盘菜单的「设置」里配置。

#### 使用本地已有的 OneOCR 文件（无需安装 Microsoft Store / 截图工具）

如果你本地已有 x64 版本的 `oneocr.dll`、`oneocr.onemodel` 和 `onnxruntime.dll`，直接放到以下任一位置即可优先加载：

1. `snipglyph.exe` 同目录的 `oneocr\` 子目录（推荐）
2. `snipglyph.exe` 同目录

也可通过 `config.json` 中的 `ocr.dir` 或环境变量 `SNIPGLYPH_ONEOCR_DIR` 指定自定义目录。未找到本地文件时会自动回退到已安装的 Windows 截图工具。

#### 便携模式

通过以下任一方式开启便携模式，配置、历史记录、日志和缓存将统一保存在程序目录的 `data\` 下，不写入 `AppData`：

- 在 `snipglyph.exe` 同目录创建一个名为 `portable`（或 `portable.txt`）的空文件
- 使用命令行参数 `snipglyph.exe --portable` 启动
- 设置环境变量 `SNIPGLYPH_PORTABLE=1`

> **提示**：`config.json` 中的翻译密钥使用 Windows DPAPI 加密（与当前 Windows 账户/设备绑定）。将便携目录迁移到其他电脑后，其余配置均正常保留，只需重新填写密钥即可。

### 构建

```sh
CGO_ENABLED=0 go build -ldflags "-H windowsgui -s -w" -o bin/snipglyph.exe ./cmd/snipglyph
```

### 声明

本项目不包含也不分发任何微软文件，OneOCR 属于微软，运行时从用户本地文件或本机截图工具读取。与 Microsoft 无关。
