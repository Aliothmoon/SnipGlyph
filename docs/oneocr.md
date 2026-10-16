# OneOCR 技术备忘

2026-10-17 在开发机实测（Windows 11 10.0.26300，Snipping Tool 11.2608.41.0，oneocr.dll 11.0.15，
同目录 onnxruntime.dll 1.23.2；`C:\Windows\System32\onnxruntime.dll` 是 1.17.x）。
用纯 Go（`CGO_ENABLED=0`，`golang.org/x/sys/windows` + `syscall.SyscallN`）跑通。

社区参考：b1tg/win11-oneocr `ocr.cpp`、AuroraWright/oneocr、wangfu91/oneocr-rs `src/ffi.rs`、
Coxxs/sp-oneocr、UnigramDev/Unigram `Telegram.Native/AI/TextRecognizerOne.h`、shiyori/win-oneocr-go（AGPL，勿抄代码）。

## 文件与加载

- 位置：`<Snipping Tool 安装目录>\SnippingTool\` 下的 `oneocr.dll`、`oneocr.onemodel`、`onnxruntime.dll`。
- 定位：`GetPackagesByPackageFamily("Microsoft.ScreenSketch_8wekyb3d8bbwe")` → `GetPackagePathByFullName`。
  直接列 `C:\Program Files\WindowsApps` 会被拒绝。
- **DLL 必须复制出来**：文件 ACL 只给 BUILTIN\Users 读权限，执行权限仅限带 Snipping Tool 包身份的进程，
  直接 `LoadLibraryEx` 报 Access is denied。读取（`os.Open`）没问题，模型可以原地读。
- DLL 不检查调用方：未签名、无包身份的 exe 可以调用。
- 先按全路径加载同目录的 `onnxruntime.dll`，再加载 `oneocr.dll`（都用 `LOAD_WITH_ALTERED_SEARCH_PATH`）。
  oneocr.dll 运行时动态加载 onnxruntime，旧版本曾误加载 System32 下的 1.17 而失败。
- **模型路径按 ANSI 代码页解析**：含中文的路径传 UTF-8 返回 5；`WideCharToMultiByte(CP_ACP)` 后成功。
  ACP 表示不了的字符无解，只能用纯 ASCII 路径。DLL 本身用 W 版 API 加载，路径无此限制。
- 只有 x64 DLL（ARM64 包里有单独构建）。

## C API

x64 调用约定；返回 int32 状态码，0 成功。句柄都是 int64。浮点数都通过指针输出。

```
CreateOcrInitOptions(int64* out)
OcrInitOptionsSetUseModelDelayLoad(h, char flag)
CreateOcrPipeline(const char* modelPath /*ANSI*/, const char* key, initOpts, int64* outPipeline)
CreateOcrProcessOptions(int64* out)
OcrProcessOptionsSetMaxRecognitionLineCount(opts, int)      // 默认 100
OcrProcessOptionsGet/SetResizeResolution(opts, w, h)        // 默认 1152x768，改了没观察到效果
RunOcrPipeline(pipeline, Image*, procOpts, int64* outResult)
GetImageAngle(result, float*)
GetOcrLineCount(result, int64*)
GetOcrLine(result, int64 idx, int64* line)
GetOcrLineContent(line, char**)                              // UTF-8，NUL 结尾
GetOcrLineBoundingBox(line, BBox**)                          // 4 个角点，8 个 float32
GetOcrLineStyle(line, int32* style, float* conf)             // 手写 / 印刷分类，不是行置信度
GetOcrLineWordCount(line, int64*)
GetOcrWord(line, int64 idx, int64* word)
GetOcrWordContent(word, char**) / GetOcrWordBoundingBox(word, BBox**) / GetOcrWordConfidence(word, float*)
ReleaseOcrResult / ReleaseOcrProcessOptions / ReleaseOcrPipeline / ReleaseOcrInitOptions(h)
```

- 模型 key：``kj)TGtrK>f]b[Piow.gU+nC@s""""""4``（11.2409 与 11.2608 相同）。
- 没有行置信度 API，用该行词置信度的最小值 / 平均值代替。
- 结果里的字符串和框指针在 `ReleaseOcrResult` 后失效，先拷贝再释放。
- 像素缓冲在调用期间要 pin 住（`runtime.Pinner`）。
- 另有 Document / Table 系列导出（`CreateOcrDocumentPipeline` 等），签名未知。

### 图像结构体（0x20 字节）

```go
type ocrImage struct {
	Type   int32   // 3 = BGRA，2 = BGR（step = w*3）
	Width  int32
	Height int32
	_      int32
	Step   int64   // 每行字节数
	Data   uintptr // 像素指针
}
type bbox struct{ X1, Y1, X2, Y2, X3, Y3, X4, Y4 float32 }
```

### 状态码（实测，含义系推断）

| 函数 | 码 | 含义 |
|---|---|---|
| CreateOcrPipeline | 5 | 打不开模型（不存在，或路径编码不对） |
| CreateOcrPipeline | 6 | key 错误 |
| RunOcrPipeline | 3 | 宽或高不在 [50, 10000] |
| RunOcrPipeline | 1 / 2 | Type 为 1 / 4 |
| RunOcrPipeline | 0 且无行 | 纯色图，或 Type 为 0 |

## 识别能力（实测）

- 中英日韩俄希腊阿拉伯法德泰印地越南希伯来土耳其波兰，混排一次识别，无需语言参数；繁体、竖排可识别（竖排返回竖长框）。
- 全角 `，！：` 输出为半角 `,!:`；`。`、`、` 保持原样；`元 / $` 之间的空格被去掉。
- 中日文的"词"是单字。

## 性能与内存（Ryzen 9 7845HX）

| 项 | 数值 |
|---|---|
| 冷启动 LoadLibrary | 约 142ms（之后约 9ms） |
| CreateOcrPipeline | 450–585ms |
| 900x360，5 行 | 中位数约 145ms |
| 260x50 单词 | 约 12ms |
| 3840x2160，150 行 | 约 1.2s |
| 初始化后工作集 | 约 121MB |
| 识别后 | 135–140MB，峰值 200–216MB |
| Close 后（DLL 不卸载） | 约 51MB |

`SetUseModelDelayLoad(1)` 只是把加载时间挪到第一次识别，总耗时差不多。

线程安全：官方无说明。1/2/4/8 个 goroutine 共用一个 pipeline 各跑 20 次无失败、结果一致；本项目仍串行调用。

## 备选引擎结论

Windows.Media.Ocr 在同样测试图上：中文字间插空格，繁体和小字乱码，日韩俄基本不可用，竖排失败；
速度更快（约 16–45ms）。不作为备选。Windows AI TextRecognizer 仅限 Copilot+ PC 的 NPU。
