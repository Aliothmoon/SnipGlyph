<div align="center">

<img src="winres/icon.png" width="76" alt="SnipGlyph" />

# SnipGlyph

**轻量、纯 Go 编写的 Windows 托盘工具：离线 OCR + 翻译 + 剪贴板历史。**

[![Release](https://img.shields.io/github/v/release/Aliothmoon/SnipGlyph?style=flat-square&color=2563eb)](https://github.com/Aliothmoon/SnipGlyph/releases/latest)
[![Platform](https://img.shields.io/badge/platform-Windows%20x64-0078d4?style=flat-square)](https://github.com/Aliothmoon/SnipGlyph/releases/latest)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00add8?style=flat-square)](go.mod)

[English](README.md) · **简体中文**

</div>

---

## 特性

- **单文件、零依赖、低占用** — 纯 Go（`CGO_ENABLED=0`）原生 Win32 界面，常驻托盘。
- **独立进程运行离线 OneOCR** — 识别引擎在独立子进程中按需启动、空闲 10 分钟自动退出释放内存，崩溃不影响主程序。
- **极速剪贴板历史** — SQLite 全文索引，支持自定义标签页、置顶、文本编辑、图片缩略图与文件路径；自动忽略密码管理器标记的私密内容。
- **双语界面与便携模式** — 默认跟随 Windows 系统语言（简体中文 / English），支持设置页实时切换，也支持完全不写 `AppData` 的便携模式。

## 功能与默认快捷键

| 功能 | 说明 | 默认快捷键 |
|---|---|---|
| **截图 OCR** | 框选区域或单击窗口识别文字，弹窗支持编辑、复制与翻译 | `Win+Alt+Shift+C` |
| **静默 OCR** | 识别后直接复制到剪贴板，屏幕下方轻提示反馈 | `Win+Alt+C` |
| **截图翻译** | 截图识别后立即翻译 | `Win+Alt+S` |
| **划词翻译** | 翻译前台应用选中文字（优先 UI Automation，不可读时回退 Ctrl+C） | `Win+Alt+X` |
| **输入翻译** | 弹出快捷翻译窗，自动预填当前剪贴板文本 | `Win+Alt+I` |
| **剪贴板历史** | 分标签、可搜索的文字 / 富文本 / 图片 / 文件剪贴板面板 | `Alt+V` |

所有快捷键、界面语言、百度翻译密钥、代理模式（跟随系统 / 直连 / 自定义 HTTP 或 SOCKS5）、历史开关和开机自启均可在 **托盘菜单 → 设置** 中调整，保存后立即生效，无需重启。

## 快速开始

1. 从 [GitHub Releases](https://github.com/Aliothmoon/SnipGlyph/releases/latest) 下载最新版本：
   - **标准版**：`snipglyph-v*-windows-amd64.exe`（配置存 `%APPDATA%\SnipGlyph`，数据存 `%LOCALAPPDATA%\SnipGlyph`）
   - **便携版**：`snipglyph-v*-windows-amd64-portable.zip`（解压即用，全部文件保存在同目录 `data\` 下）
2. 双击运行 `snipglyph.exe`。由于可执行文件未签名，首次运行可能触发 Windows SmartScreen 提示，点击 **更多信息 → 仍要运行** 即可。
3. 点击系统托盘图标进入 **设置**，配置快捷键、界面语言或翻译密钥。

## OneOCR 引擎配置

SnipGlyph 使用微软离线 **OneOCR** 引擎（x64），按以下优先级自动查找：

1. **自定义目录** *(可选)* — 在 `config.json` 中填写 `ocr.dir`（相对路径以程序目录为基准），或设置环境变量 `SNIPGLYPH_ONEOCR_DIR`。
2. **本地已有文件** *(无需安装 Microsoft Store 或截图工具)* — 将 x64 版本的 `oneocr.dll`、`oneocr.onemodel`、`onnxruntime.dll` 放到 `snipglyph.exe` 同目录的 `oneocr\` 子目录（推荐），或直接与 `snipglyph.exe` 放在一起：
   ```text
   SnipGlyph/
   ├── snipglyph.exe
   └── oneocr/
       ├── oneocr.dll
       ├── oneocr.onemodel
       └── onnxruntime.dll
   ```
3. **Windows 11 截图工具** *(自动回退)* — 未检测到本地文件时自动读取本机已安装的截图工具。

> [!NOTE]
> OneOCR 加载模型时按系统 ANSI 代码页解析路径。若路径包含当前代码页无法表示的字符，SnipGlyph 会自动尝试使用 Windows 8.3 短路径回退；如所在磁盘关闭了短文件名，建议将文件放在纯 ASCII 路径下。

## 便携模式

任意 `snipglyph.exe` 均可通过以下方式开启便携模式（按优先级从高到低）：

1. 带参数启动：`snipglyph.exe --portable`
2. 设置环境变量：`SNIPGLYPH_PORTABLE=1`
3. 在 `snipglyph.exe` 同目录新建名为 `portable`（或 `portable.txt`）的空文件

开启后所有运行数据均存放在 `<exeDir>\data\`，与用户手动放置的 `<exeDir>\oneocr\` 互不干扰：

```text
SnipGlyph/
├── snipglyph.exe
├── portable
├── oneocr/                 # 可选：自带本地 OneOCR 文件
└── data/
    ├── config.json         # 配置文件（翻译密钥由 Windows DPAPI 加密）
    ├── history.db          # OCR 与翻译历史（SQLite FTS5）
    ├── clipboard.db        # 剪贴板历史（SQLite FTS5）
    ├── logs/snipglyph.log  # 按大小轮转的运行日志
    └── cache/oneocr/       # 临时缓存（仅加载 MSIX 截图工具时使用）
```

> [!TIP]
> `config.json` 中的 API 密钥采用 Windows DPAPI 加密（与当前 Windows 用户和设备绑定）。将便携目录迁移到其他电脑或账户时，其余设置会完整保留，只需在设置页重新填写密钥即可。

## 剪贴板面板快捷键

<details>
<summary>查看剪贴板历史面板（<code>Alt+V</code>）内的常用操作</summary>

| 按键 | 功能 |
|---|---|
| 直接打字 | 搜索当前标签（空格分隔多个关键字） |
| `↑` `↓` / `PgUp` `PgDn` / `Ctrl+Home` `End` | 移动选中项 |
| `Enter` / 双击 | 粘贴回打开面板前的窗口 |
| `Shift+Enter` | 以纯文本粘贴（去除 HTML / RTF 格式） |
| `Ctrl+C` | 仅复制到剪贴板，不触发粘贴 |
| `Ctrl+P` | 置顶 / 取消置顶 |
| `F2` | 编辑文本条目（`Ctrl+S` 保存，`Esc` 取消） |
| `Del`（搜索框有字时用 `Shift+Del`） | 删除条目 |
| `Ctrl+Tab` / `Ctrl+Shift+Tab` | 切换标签页 |
| 右键菜单 | 移动/复制到其他标签，或在标签栏新建/重命名/删除标签 |

</details>

## 从源码构建

需要 Windows 环境与 Go 1.26+：

```sh
# 编译发布版
CGO_ENABLED=0 go build -ldflags "-H windowsgui -s -w" -o bin/snipglyph.exe ./cmd/snipglyph

# 运行单元测试
go test ./...
```

如果修改了 `winres/winres.json` 或 `winres/icon.png`，需先重新生成内嵌资源文件：

```sh
go generate ./cmd/snipglyph
```

## 声明

本仓库及发布的二进制文件**不包含也不分发任何微软文件**。OneOCR 属于微软，程序仅在运行时从用户本地提供的文件或本机已安装的 Windows 截图工具中加载。本项目与 Microsoft 无关，也未经其认可。
