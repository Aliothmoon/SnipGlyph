package i18n

// Catalog holds every user-facing string in a single language.
// Struct fields give compile-time checking and let tests verify parity
// between catalogs.
type Catalog struct {
	Lang     Lang
	FontFace string

	// Startup / main
	AlreadyRunning      string
	FatalSingleInstance string
	FatalDataDir        string
	FatalConfigDir      string
	FatalLogFile        string
	FatalExePath        string
	FatalStart          string

	// Actions / History kinds
	ActionScreenshotOCR       string
	ActionSilentOCR           string
	ActionScreenshotTranslate string
	ActionSelectionTranslate  string
	ActionInputTranslate      string
	ActionClipboardHistory    string

	// Tray menu & notifications
	TrayHistory              string
	TraySettings             string
	TrayReload               string
	TrayOpenConfigDir        string
	TrayOpenLogDir           string
	TrayExit                 string
	TrayStartupProblemsTitle string
	TrayConfigError          string
	TrayConfigErrorTitle     string
	TraySecretUnreadable     string
	TrayConfigProblemsTitle  string
	TrayConfigReloaded       string
	TrayFeatureWIP           string
	TrayOCRUnavailable       string
	TrayHotkeyBadFormat      string
	TrayHotkeyTaken          string
	TrayHotkeyRegFailed      string
	TrayHotkeyRegErr         string

	// App runtime feedback
	CaptureFailed         string
	OpenPopupFailed       string
	OpenWindowFailed      string
	OpenSettingsFailed    string
	OpenHistoryFailed     string
	OpenClipPanelFailed   string
	OCRRecognizing        string
	OCRFailed             string
	OCRNoText             string
	CopyToClipFailed      string
	PutToClipFailed       string
	CopiedLinesOne        string
	CopiedLinesMany       string
	GrabReleaseKeys       string
	GrabNoSelection       string
	GrabClipboardBusy     string
	GrabTimeout           string
	GrabFailed            string
	NoTranslateEngine     string
	BaiduProxyBad         string
	TranslateTimeout      string
	TranslateFailed       string
	AutoDetect            string
	InputHintPrefilled    string
	InputHintEmpty        string
	HistoryUnavailable    string
	HistoryUnavailableLog string
	ClipUnavailable       string
	ClipUnavailableLog    string
	ClipWatchFailed       string
	SaveConfigFailed      string
	AutostartFailed       string
	HotkeyFormatErr       string
	HotkeyDupErr          string

	// Settings window
	SettingsTitle             string
	SettingsGeneral           string
	SettingsLanguage          string
	LangAuto                  string
	LangZhCN                  string
	LangEn                    string
	SettingsAutostart         string
	SettingsSaveHistory       string
	SettingsSaveClipboard     string
	SettingsHotkeys           string
	SettingsKeyHint           string
	HotkeyUnset               string
	HotkeyReserved            string
	HotkeyNeedsMod            string
	HotkeyUnsupported         string
	SettingsTranslate         string
	SettingsTargetLang        string
	SettingsTargetNote        string
	BaiduName                 string
	BaiduApplyKey             string
	BaiduAppID                string
	BaiduKey                  string
	SettingsProxy             string
	ProxySystem               string
	ProxyDirect               string
	ProxyCustom               string
	SettingsTest              string
	SettingsSave              string
	SettingsCancel            string
	SettingsNeedProxy         string
	SettingsNeedBothCreds     string
	SettingsNeedCredsFirst    string
	SettingsNotSaved          string
	SettingsSavedWithProblems string
	SettingsTesting           string
	SettingsTestFailed        string
	SettingsTestOK            string

	// Result popup
	PopupHint        string
	PopupPinTip      string
	PopupUnpinTip    string
	PopupCloseTip    string
	PopupSource      string
	PopupResult      string
	PopupCopy        string
	PopupCopied      string
	PopupCopyRetry   string
	PopupEngineTip   string
	PopupTranslate   string
	PopupTranslating string

	// History window
	HistoryTitle         string
	HistorySearchCue     string
	HistoryClearAll      string
	HistoryColTime       string
	HistoryColKind       string
	HistoryColSource     string
	HistoryColResult     string
	HistoryDelete        string
	HistoryOpenPopup     string
	HistoryOpenPopupTip  string
	HistoryReadFailed    string
	HistoryFoundMany     string
	HistoryFoundCount    string
	HistoryTotalPartial  string
	HistoryTotalCount    string
	HistoryPausedNote    string
	HistoryNoTranslation string
	HistoryClearTitle    string
	HistoryClearConfirm  string
	OpFailed             string

	// Clipboard panel & list
	ClipTitle            string
	ClipHeader           string
	ClipDefaultTab       string
	ClipSearchCue        string
	ClipHint             string
	ClipEditHint         string
	ClipPromptHint       string
	ClipReadFailed       string
	ClipEmptySearch      string
	ClipEmptyTab         string
	ClipEmptyPaused      string
	ClipEmptyDefault     string
	ClipFoundCount       string
	ClipTotalCount       string
	ClipTabExists        string
	ClipNewTabLabel      string
	ClipRenameTabLabel   string
	ClipDeleteTabTitle   string
	ClipDeleteTabConfirm string
	ClipMenuPaste        string
	ClipMenuPastePlain   string
	ClipMenuCopy         string
	ClipMenuPin          string
	ClipMenuUnpin        string
	ClipMenuEdit         string
	ClipMenuMoveTo       string
	ClipMenuCopyTo       string
	ClipMenuNoOtherTabs  string
	ClipMenuDelete       string
	ClipTabNew           string
	ClipTabRename        string
	ClipTabDelete        string
	ClipImageMeta        string
	ClipFilesMore        string
	AgoJustNow           string
	AgoMinutes           string
	AgoYesterday         string

	// Translation languages & Baidu errors
	LangNames            map[string]string
	BaiduErrors          map[string]string
	BaiduUnsupportedLang string
	BaiduNotConfigured   string
	BaiduTextTooLong     string
	BaiduRequestFailed   string
	BaiduReadFailed      string
	BaiduHTTPStatus      string
	BaiduParseFailed     string
	BaiduAPIError        string

	// Proxy errors
	ProxyInvalidURL        string
	ProxyUnsupportedScheme string

	// OCR errors
	OCRNotInstalled      string
	OCRCustomDirMissing  string
	OCRStageFailed       string
	OCRLoadDLL           string
	OCRBadArch           string
	OCRMissingProc       string
	OCRModelPathEncoding string
	OCRModelOpen         string
	OCRModelKey          string
	OCRPipelineInit      string
	OCRSelfTest          string
	OCRWorkerStart       string
	OCRWorkerTimeout     string
	OCRWorkerIO          string
}

var zhCN = Catalog{
	Lang:     ZhCN,
	FontFace: "Microsoft YaHei UI",

	AlreadyRunning:      "SnipGlyph 已经在运行了，可以在任务栏托盘里找到它。",
	FatalSingleInstance: "单实例检查失败",
	FatalDataDir:        "无法创建数据目录",
	FatalConfigDir:      "无法创建配置目录",
	FatalLogFile:        "无法打开日志文件",
	FatalExePath:        "无法确定程序路径",
	FatalStart:          "启动失败",

	ActionScreenshotOCR:       "截图 OCR",
	ActionSilentOCR:           "静默 OCR",
	ActionScreenshotTranslate: "截图翻译",
	ActionSelectionTranslate:  "划词翻译",
	ActionInputTranslate:      "输入翻译",
	ActionClipboardHistory:    "剪贴板历史",

	TrayHistory:              "历史记录",
	TraySettings:             "设置",
	TrayReload:               "重新加载配置",
	TrayOpenConfigDir:        "打开配置目录",
	TrayOpenLogDir:           "打开日志目录",
	TrayExit:                 "退出",
	TrayStartupProblemsTitle: "SnipGlyph 启动时发现问题",
	TrayConfigError:          "配置文件有误，已使用默认设置：%s",
	TrayConfigErrorTitle:     "配置文件有误，已使用默认设置",
	TraySecretUnreadable:     "已保存的加密密钥无法解密（可能来自其他电脑或账户），请在设置中重新输入",
	TrayConfigProblemsTitle:  "配置中有问题",
	TrayConfigReloaded:       "配置已重新加载",
	TrayFeatureWIP:           "功能开发中",
	TrayOCRUnavailable:       "OCR 不可用：%s",
	TrayHotkeyBadFormat:      "%s（%s）：格式错误",
	TrayHotkeyTaken:          "已被其他程序占用",
	TrayHotkeyRegFailed:      "注册失败",
	TrayHotkeyRegErr:         "%s（%s）：%s",

	CaptureFailed:         "截图失败：%s",
	OpenPopupFailed:       "无法打开结果窗口：%s",
	OpenWindowFailed:      "无法打开窗口：%s",
	OpenSettingsFailed:    "无法打开设置窗口：%s",
	OpenHistoryFailed:     "无法打开历史记录窗口：%s",
	OpenClipPanelFailed:   "无法打开面板：%s",
	OCRRecognizing:        "正在识别…",
	OCRFailed:             "识别失败：%s",
	OCRNoText:             "没有识别到文字",
	CopyToClipFailed:      "复制到剪贴板失败：%s",
	PutToClipFailed:       "无法放入剪贴板：%s",
	CopiedLinesOne:        "已复制 1 行文字",
	CopiedLinesMany:       "已复制 %d 行文字",
	GrabReleaseKeys:       "请先松开快捷键再取词",
	GrabNoSelection:       "没有获取到选中的文字",
	GrabClipboardBusy:     "剪贴板正被其他程序占用，请稍后再试",
	GrabTimeout:           "取词超时，请再试一次",
	GrabFailed:            "取词失败：%s",
	NoTranslateEngine:     "还没有配置翻译引擎。\n请在托盘菜单的「设置」里填写百度翻译的 APP ID 和密钥。",
	BaiduProxyBad:         "百度翻译的代理设置有误：%s",
	TranslateTimeout:      "翻译超时，请检查网络或代理设置",
	TranslateFailed:       "翻译失败：%s",
	AutoDetect:            "自动检测",
	InputHintPrefilled:    "已填入剪贴板内容，按 Ctrl+Enter 翻译",
	InputHintEmpty:        "输入或粘贴文字，按 Ctrl+Enter 翻译",
	HistoryUnavailable:    "历史记录不可用：%s",
	HistoryUnavailableLog: "历史记录不可用，原因见日志",
	ClipUnavailable:       "剪贴板历史不可用：%s",
	ClipUnavailableLog:    "剪贴板历史不可用，原因见日志",
	ClipWatchFailed:       "无法监听剪贴板：%s",
	SaveConfigFailed:      "写入配置文件失败：%s",
	AutostartFailed:       "开机自启设置失败：%s",
	HotkeyFormatErr:       "「%s」的快捷键 %s 格式错误",
	HotkeyDupErr:          "「%s」和「%s」用了同一个快捷键 %s",

	SettingsTitle:             "SnipGlyph 设置",
	SettingsGeneral:           "常规",
	SettingsLanguage:          "界面语言",
	LangAuto:                  "跟随系统",
	LangZhCN:                  "简体中文",
	LangEn:                    "English",
	SettingsAutostart:         "开机时自动启动",
	SettingsSaveHistory:       "保存历史记录（只存文字，最近 %d 条）",
	SettingsSaveClipboard:     "记录剪贴板历史（文字、图片、文件路径；最近 %d 条，置顶不计）",
	SettingsHotkeys:           "快捷键",
	SettingsKeyHint:           "点击输入框后按下新的组合键；Backspace 清除",
	HotkeyUnset:               "未设置",
	HotkeyReserved:            "%s 是系统和各程序通用的组合键，不能用作快捷键",
	HotkeyNeedsMod:            "快捷键要包含 Win、Ctrl 或 Alt（F1–F24、PrintScreen、Pause 除外）",
	HotkeyUnsupported:         "不支持这个按键",
	SettingsTranslate:         "翻译",
	SettingsTargetLang:        "目标语言",
	SettingsTargetNote:        "中文原文总是译成英文",
	BaiduName:                 "百度翻译",
	BaiduApplyKey:             "申请密钥",
	BaiduAppID:                "APP ID",
	BaiduKey:                  "密钥",
	SettingsProxy:             "代理",
	ProxySystem:               "跟随系统",
	ProxyDirect:               "直连",
	ProxyCustom:               "自定义",
	SettingsTest:              "测试",
	SettingsSave:              "保存",
	SettingsCancel:            "取消",
	SettingsNeedProxy:         "请填写百度翻译的代理地址，或选择「跟随系统」",
	SettingsNeedBothCreds:     "百度翻译的 APP ID 和密钥要一起填写",
	SettingsNeedCredsFirst:    "请先填写 APP ID 和密钥",
	SettingsNotSaved:          "没有保存：%s",
	SettingsSavedWithProblems: "已保存，但有问题：\n%s",
	SettingsTesting:           "正在测试…",
	SettingsTestFailed:        "测试失败：%s",
	SettingsTestOK:            "可以使用：hello → %s",

	PopupHint:        "Ctrl+Enter 翻译 · Esc 关闭",
	PopupPinTip:      "钉住：点别处不关闭",
	PopupUnpinTip:    "取消钉住",
	PopupCloseTip:    "关闭（Esc）",
	PopupSource:      "原文",
	PopupResult:      "译文",
	PopupCopy:        "复制",
	PopupCopied:      "已复制",
	PopupCopyRetry:   "复制失败，重试",
	PopupEngineTip:   "翻译引擎",
	PopupTranslate:   "翻译",
	PopupTranslating: "翻译中…",

	HistoryTitle:         "SnipGlyph 历史记录",
	HistorySearchCue:     "搜索原文或译文，空格分隔多个关键字（Ctrl+F）",
	HistoryClearAll:      "清空全部…",
	HistoryColTime:       "时间",
	HistoryColKind:       "类型",
	HistoryColSource:     "原文",
	HistoryColResult:     "译文",
	HistoryDelete:        "删除",
	HistoryOpenPopup:     "在弹窗中打开",
	HistoryOpenPopupTip:  "可以修改原文、重新翻译（双击或 Enter）",
	HistoryReadFailed:    "读取历史记录失败：%s",
	HistoryFoundMany:     "找到很多条，只列出最近 %d 条",
	HistoryFoundCount:    "找到 %d 条",
	HistoryTotalPartial:  "共 %d 条，列出最近 %d 条",
	HistoryTotalCount:    "共 %d 条",
	HistoryPausedNote:    " · 已暂停记录，可在设置中开启",
	HistoryNoTranslation: "（没有翻译）",
	HistoryClearTitle:    "清空历史记录",
	HistoryClearConfirm:  "删除全部历史记录？删除后无法恢复。",
	OpFailed:             "操作失败：%s",

	ClipTitle:            "SnipGlyph · 剪贴板",
	ClipHeader:           "剪贴板历史",
	ClipDefaultTab:       "剪贴板",
	ClipSearchCue:        "搜索当前标签，空格分隔多个关键字",
	ClipHint:             "Enter 粘贴 · Shift+Enter 纯文本 · Ctrl+P 置顶 · F2 编辑 · Del 删除 · 右键更多",
	ClipEditHint:         "Ctrl+S 保存 · Esc 取消（保存后去掉原有格式）",
	ClipPromptHint:       "Enter 确定 · Esc 取消",
	ClipReadFailed:       "读取剪贴板记录失败：%s",
	ClipEmptySearch:      "没有找到",
	ClipEmptyTab:         "这个标签还是空的。\n在条目上右键，选「移动到」或「复制到」。",
	ClipEmptyPaused:      "剪贴板记录已关闭，可在设置中开启。",
	ClipEmptyDefault:     "还没有记录。\n复制的文字、图片和文件会出现在这里。",
	ClipFoundCount:       "找到 %d 条",
	ClipTotalCount:       "共 %d 条",
	ClipTabExists:        "已经有同名的标签",
	ClipNewTabLabel:      "新标签名称",
	ClipRenameTabLabel:   "标签名称",
	ClipDeleteTabTitle:   "删除标签",
	ClipDeleteTabConfirm: "删除标签「%s」和其中的全部条目？",
	ClipMenuPaste:        "粘贴\tEnter",
	ClipMenuPastePlain:   "以纯文本粘贴\tShift+Enter",
	ClipMenuCopy:         "复制\tCtrl+C",
	ClipMenuPin:          "置顶\tCtrl+P",
	ClipMenuUnpin:        "取消置顶\tCtrl+P",
	ClipMenuEdit:         "编辑\tF2",
	ClipMenuMoveTo:       "移动到",
	ClipMenuCopyTo:       "复制到",
	ClipMenuNoOtherTabs:  "（没有其他标签，点标签栏的 + 新建）",
	ClipMenuDelete:       "删除\tDel",
	ClipTabNew:           "新建标签…",
	ClipTabRename:        "重命名…",
	ClipTabDelete:        "删除标签…",
	ClipImageMeta:        "图片 %d×%d",
	ClipFilesMore:        "%s 等 %d 项",
	AgoJustNow:           "刚刚",
	AgoMinutes:           "%d 分钟前",
	AgoYesterday:         "昨天 %s",

	LangNames: map[string]string{
		"auto":    "自动",
		"zh-Hans": "简体中文",
		"zh-Hant": "繁体中文",
		"en":      "英语",
		"ja":      "日语",
		"ko":      "韩语",
		"fr":      "法语",
		"de":      "德语",
		"es":      "西班牙语",
		"ru":      "俄语",
	},
	BaiduErrors: map[string]string{
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
	},
	BaiduUnsupportedLang: "百度翻译不支持语言 %s",
	BaiduNotConfigured:   "百度翻译未配置 APP ID 或密钥",
	BaiduTextTooLong:     "文本过长：百度翻译单次最多 %d 字节，当前 %d 字节",
	BaiduRequestFailed:   "百度翻译请求失败：%w",
	BaiduReadFailed:      "百度翻译响应读取失败：%w",
	BaiduHTTPStatus:      "百度翻译返回 HTTP %d",
	BaiduParseFailed:     "百度翻译响应无法解析：%w",
	BaiduAPIError:        "百度翻译错误 %s：%s",

	ProxyInvalidURL:        "代理地址 %q 无效，应形如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080",
	ProxyUnsupportedScheme: "不支持的代理协议 %q",

	OCRNotInstalled:      "未找到 OneOCR 文件：请将 oneocr.dll、oneocr.onemodel、onnxruntime.dll 放到程序目录（或 oneocr 子目录），或从 Microsoft Store 安装/更新「截图工具」",
	OCRCustomDirMissing:  "配置的 OneOCR 目录 %s 缺少文件：%s",
	OCRStageFailed:       "复制 OneOCR 文件失败：%s",
	OCRLoadDLL:           "加载 %s 失败：%s",
	OCRBadArch:           "加载 %s 失败（需要 64 位 x64 版本）：%s",
	OCRMissingProc:       "OneOCR 缺少函数 %s（版本可能不兼容）：%s",
	OCRModelPathEncoding: "模型路径 %q 包含当前系统代码页无法表示的字符，请将 OneOCR 文件移至纯 ASCII 路径",
	OCRModelOpen:         "OneOCR 无法打开模型文件 %s",
	OCRModelKey:          "OneOCR 模型密钥不匹配，OneOCR 版本可能不兼容",
	OCRPipelineInit:      "%s 返回 %s",
	OCRSelfTest:          "OneOCR 自检失败：%s",
	OCRWorkerStart:       "启动 OCR 进程失败：%s",
	OCRWorkerTimeout:     "OCR 进程启动超时",
	OCRWorkerIO:          "OCR 进程通信失败：%s",
}

var enUS = Catalog{
	Lang:     En,
	FontFace: "Segoe UI",

	AlreadyRunning:      "SnipGlyph is already running. You can find it in the system tray.",
	FatalSingleInstance: "Single-instance check failed",
	FatalDataDir:        "Cannot create data directory",
	FatalConfigDir:      "Cannot create config directory",
	FatalLogFile:        "Cannot open log file",
	FatalExePath:        "Cannot determine executable path",
	FatalStart:          "Failed to start",

	ActionScreenshotOCR:       "Screenshot OCR",
	ActionSilentOCR:           "Silent OCR",
	ActionScreenshotTranslate: "Screenshot Translate",
	ActionSelectionTranslate:  "Selection Translate",
	ActionInputTranslate:      "Input Translate",
	ActionClipboardHistory:    "Clipboard History",

	TrayHistory:              "History",
	TraySettings:             "Settings",
	TrayReload:               "Reload Config",
	TrayOpenConfigDir:        "Open Config Folder",
	TrayOpenLogDir:           "Open Log Folder",
	TrayExit:                 "Exit",
	TrayStartupProblemsTitle: "SnipGlyph started with issues",
	TrayConfigError:          "Invalid config file; using defaults: %s",
	TrayConfigErrorTitle:     "Invalid config file; using defaults",
	TraySecretUnreadable:     "Saved encrypted credentials cannot be decrypted (e.g. from another PC or account); please re-enter them in Settings.",
	TrayConfigProblemsTitle:  "Configuration issues",
	TrayConfigReloaded:       "Configuration reloaded",
	TrayFeatureWIP:           "Feature in progress",
	TrayOCRUnavailable:       "OCR unavailable: %s",
	TrayHotkeyBadFormat:      "%s (%s): invalid format",
	TrayHotkeyTaken:          "already in use by another app",
	TrayHotkeyRegFailed:      "registration failed",
	TrayHotkeyRegErr:         "%s (%s): %s",

	CaptureFailed:         "Screenshot failed: %s",
	OpenPopupFailed:       "Cannot open result window: %s",
	OpenWindowFailed:      "Cannot open window: %s",
	OpenSettingsFailed:    "Cannot open Settings window: %s",
	OpenHistoryFailed:     "Cannot open History window: %s",
	OpenClipPanelFailed:   "Cannot open clipboard panel: %s",
	OCRRecognizing:        "Recognizing…",
	OCRFailed:             "OCR failed: %s",
	OCRNoText:             "No text recognized",
	CopyToClipFailed:      "Failed to copy to clipboard: %s",
	PutToClipFailed:       "Failed to write to clipboard: %s",
	CopiedLinesOne:        "Copied 1 line",
	CopiedLinesMany:       "Copied %d lines",
	GrabReleaseKeys:       "Release the hotkey to capture selection",
	GrabNoSelection:       "No selected text found",
	GrabClipboardBusy:     "Clipboard is busy; please try again",
	GrabTimeout:           "Timed out reading selection; please try again",
	GrabFailed:            "Failed to grab selection: %s",
	NoTranslateEngine:     "No translation engine configured.\nOpen Settings from the tray menu to enter your Baidu Translate APP ID and Secret Key.",
	BaiduProxyBad:         "Invalid Baidu Translate proxy setting: %s",
	TranslateTimeout:      "Translation timed out; check your network or proxy settings",
	TranslateFailed:       "Translation failed: %s",
	AutoDetect:            "Auto-detect",
	InputHintPrefilled:    "Prefilled from clipboard · Ctrl+Enter to translate",
	InputHintEmpty:        "Type or paste text · Ctrl+Enter to translate",
	HistoryUnavailable:    "History unavailable: %s",
	HistoryUnavailableLog: "History unavailable; see logs for details",
	ClipUnavailable:       "Clipboard history unavailable: %s",
	ClipUnavailableLog:    "Clipboard history unavailable; see logs for details",
	ClipWatchFailed:       "Cannot monitor clipboard: %s",
	SaveConfigFailed:      "Failed to save config: %s",
	AutostartFailed:       "Failed to update startup setting: %s",
	HotkeyFormatErr:       "\"%s\": invalid hotkey %s",
	HotkeyDupErr:          "\"%s\" and \"%s\" use the same hotkey %s",

	SettingsTitle:             "SnipGlyph Settings",
	SettingsGeneral:           "General",
	SettingsLanguage:          "Language",
	LangAuto:                  "Follow System",
	LangZhCN:                  "简体中文",
	LangEn:                    "English",
	SettingsAutostart:         "Start with Windows",
	SettingsSaveHistory:       "Save history (text only, last %d entries)",
	SettingsSaveClipboard:     "Save clipboard history (text, images, files; last %d, excl. pinned)",
	SettingsHotkeys:           "Hotkeys",
	SettingsKeyHint:           "Click a box and press a key combo; Backspace to clear",
	HotkeyUnset:               "Not set",
	HotkeyReserved:            "%s is reserved by the system and cannot be used",
	HotkeyNeedsMod:            "Include Win, Ctrl, or Alt (except F1–F24, PrintScreen, Pause)",
	HotkeyUnsupported:         "This key is not supported",
	SettingsTranslate:         "Translation",
	SettingsTargetLang:        "Target language",
	SettingsTargetNote:        "Chinese text always translates to English",
	BaiduName:                 "Baidu Translate",
	BaiduApplyKey:             "Get API Key",
	BaiduAppID:                "APP ID",
	BaiduKey:                  "Secret Key",
	SettingsProxy:             "Proxy",
	ProxySystem:               "System",
	ProxyDirect:               "Direct",
	ProxyCustom:               "Custom",
	SettingsTest:              "Test",
	SettingsSave:              "Save",
	SettingsCancel:            "Cancel",
	SettingsNeedProxy:         "Enter a Baidu Translate proxy URL or choose System",
	SettingsNeedBothCreds:     "Both Baidu Translate APP ID and Secret Key are required",
	SettingsNeedCredsFirst:    "Enter APP ID and Secret Key first",
	SettingsNotSaved:          "Not saved: %s",
	SettingsSavedWithProblems: "Saved with issues:\n%s",
	SettingsTesting:           "Testing…",
	SettingsTestFailed:        "Test failed: %s",
	SettingsTestOK:            "Working: hello → %s",

	PopupHint:        "Ctrl+Enter Translate · Esc Close",
	PopupPinTip:      "Pin: keep open when clicking away",
	PopupUnpinTip:    "Unpin",
	PopupCloseTip:    "Close (Esc)",
	PopupSource:      "Source",
	PopupResult:      "Translation",
	PopupCopy:        "Copy",
	PopupCopied:      "Copied",
	PopupCopyRetry:   "Copy failed, retry",
	PopupEngineTip:   "Translation engine",
	PopupTranslate:   "Translate",
	PopupTranslating: "Translating…",

	HistoryTitle:         "SnipGlyph History",
	HistorySearchCue:     "Search source or translation, space-separated terms (Ctrl+F)",
	HistoryClearAll:      "Clear All…",
	HistoryColTime:       "Time",
	HistoryColKind:       "Type",
	HistoryColSource:     "Source",
	HistoryColResult:     "Translation",
	HistoryDelete:        "Delete",
	HistoryOpenPopup:     "Open in Popup",
	HistoryOpenPopupTip:  "Edit source or re-translate (double-click or Enter)",
	HistoryReadFailed:    "Failed to load history: %s",
	HistoryFoundMany:     "Many matches; showing latest %d",
	HistoryFoundCount:    "%d found",
	HistoryTotalPartial:  "%d total, showing latest %d",
	HistoryTotalCount:    "%d total",
	HistoryPausedNote:    " · Recording paused (enable in Settings)",
	HistoryNoTranslation: "(No translation)",
	HistoryClearTitle:    "Clear History",
	HistoryClearConfirm:  "Delete all history entries? This cannot be undone.",
	OpFailed:             "Operation failed: %s",

	ClipTitle:            "SnipGlyph · Clipboard",
	ClipHeader:           "Clipboard History",
	ClipDefaultTab:       "Clipboard",
	ClipSearchCue:        "Search current tab, space-separated terms",
	ClipHint:             "Enter Paste · Shift+Enter Plain · Ctrl+P Pin · F2 Edit · Del Delete · Right-click More",
	ClipEditHint:         "Ctrl+S Save · Esc Cancel (clears rich formatting)",
	ClipPromptHint:       "Enter Confirm · Esc Cancel",
	ClipReadFailed:       "Failed to load clipboard items: %s",
	ClipEmptySearch:      "No matches",
	ClipEmptyTab:         "This tab is empty.\nRight-click an item to Move or Copy it here.",
	ClipEmptyPaused:      "Clipboard recording is off. Enable it in Settings.",
	ClipEmptyDefault:     "No items yet.\nCopied text, images, and files will appear here.",
	ClipFoundCount:       "%d found",
	ClipTotalCount:       "%d items",
	ClipTabExists:        "A tab with that name already exists",
	ClipNewTabLabel:      "New tab name",
	ClipRenameTabLabel:   "Tab name",
	ClipDeleteTabTitle:   "Delete Tab",
	ClipDeleteTabConfirm: "Delete tab \"%s\" and all its items?",
	ClipMenuPaste:        "Paste\tEnter",
	ClipMenuPastePlain:   "Paste as Plain Text\tShift+Enter",
	ClipMenuCopy:         "Copy\tCtrl+C",
	ClipMenuPin:          "Pin\tCtrl+P",
	ClipMenuUnpin:        "Unpin\tCtrl+P",
	ClipMenuEdit:         "Edit\tF2",
	ClipMenuMoveTo:       "Move to",
	ClipMenuCopyTo:       "Copy to",
	ClipMenuNoOtherTabs:  "(No other tabs; click + on the tab bar)",
	ClipMenuDelete:       "Delete\tDel",
	ClipTabNew:           "New Tab…",
	ClipTabRename:        "Rename…",
	ClipTabDelete:        "Delete Tab…",
	ClipImageMeta:        "Image %d×%d",
	ClipFilesMore:        "%s (%d items)",
	AgoJustNow:           "Just now",
	AgoMinutes:           "%dm ago",
	AgoYesterday:         "Yest. %s",

	LangNames: map[string]string{
		"auto":    "Auto",
		"zh-Hans": "Simplified Chinese",
		"zh-Hant": "Traditional Chinese",
		"en":      "English",
		"ja":      "Japanese",
		"ko":      "Korean",
		"fr":      "French",
		"de":      "German",
		"es":      "Spanish",
		"ru":      "Russian",
	},
	BaiduErrors: map[string]string{
		"52001": "request timed out, please retry",
		"52002": "Baidu Translate system error, please retry",
		"52003": "invalid APP ID or General Translate service not enabled",
		"54000": "missing required parameter",
		"54001": "invalid signature, please check your Secret Key",
		"54003": "rate limit exceeded (1 req/s on standard plan)",
		"54004": "insufficient account balance",
		"54005": "long-text requests too frequent, please retry later",
		"58000": "client IP is not in Baidu's IP allowlist",
		"58001": "unsupported language direction",
		"58002": "service disabled; enable it in the Baidu Translate console",
		"90107": "verification failed or not yet active",
	},
	BaiduUnsupportedLang: "Baidu Translate does not support language %s",
	BaiduNotConfigured:   "Baidu Translate APP ID or Secret Key is not configured",
	BaiduTextTooLong:     "text too long: Baidu Translate allows at most %d bytes per request (got %d bytes)",
	BaiduRequestFailed:   "Baidu Translate request failed: %w",
	BaiduReadFailed:      "failed to read Baidu Translate response: %w",
	BaiduHTTPStatus:      "Baidu Translate returned HTTP %d",
	BaiduParseFailed:     "failed to parse Baidu Translate response: %w",
	BaiduAPIError:        "Baidu Translate error %s: %s",

	ProxyInvalidURL:        "invalid proxy URL %q; expected e.g. http://127.0.0.1:7890 or socks5://127.0.0.1:1080",
	ProxyUnsupportedScheme: "unsupported proxy scheme %q",

	OCRNotInstalled:      "OneOCR files not found. Place oneocr.dll, oneocr.onemodel, and onnxruntime.dll beside SnipGlyph (or in a oneocr subfolder), or install/update Windows Snipping Tool from Microsoft Store.",
	OCRCustomDirMissing:  "Configured OneOCR folder %s is missing: %s",
	OCRStageFailed:       "Failed to copy OneOCR files: %s",
	OCRLoadDLL:           "Failed to load %s: %s",
	OCRBadArch:           "Failed to load %s (64-bit x64 DLL required): %s",
	OCRMissingProc:       "OneOCR is missing %s (version may be incompatible): %s",
	OCRModelPathEncoding: "Model path %q cannot be represented in the Windows ANSI code page; move the OneOCR files to an ASCII path",
	OCRModelOpen:         "OneOCR could not open model file %s",
	OCRModelKey:          "OneOCR model key mismatch; this OneOCR version may be incompatible",
	OCRPipelineInit:      "%s returned %s",
	OCRSelfTest:          "OneOCR self-test failed: %s",
	OCRWorkerStart:       "Failed to start OCR worker: %s",
	OCRWorkerTimeout:     "OCR worker timed out while starting",
	OCRWorkerIO:          "OCR worker communication failed: %s",
}
