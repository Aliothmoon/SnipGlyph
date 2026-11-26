package ocr

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// modelKey decrypts oneocr.onemodel. Unchanged from Snipping Tool 11.2409
// through 11.2608; see docs/oneocr.md.
const modelKey = `kj)TGtrK>f]b[Piow.gU+nC@s""""""4`

const maxLines = 1000 // OneOCR's default of 100 truncates dense screenshots

// ocrImage is RunOcrPipeline's image argument (0x20 bytes).
type ocrImage struct {
	Type   int32 // 3 = BGRA
	Width  int32
	Height int32
	_      int32
	Step   int64 // bytes per row
	Data   uintptr
}

type procs struct {
	CreateOcrInitOptions,
	OcrInitOptionsSetUseModelDelayLoad,
	CreateOcrPipeline,
	CreateOcrProcessOptions,
	OcrProcessOptionsSetMaxRecognitionLineCount,
	RunOcrPipeline,
	GetImageAngle,
	GetOcrLineCount,
	GetOcrLine,
	GetOcrLineContent,
	GetOcrLineBoundingBox,
	GetOcrLineWordCount,
	GetOcrWord,
	GetOcrWordContent,
	GetOcrWordBoundingBox,
	GetOcrWordConfidence,
	ReleaseOcrResult,
	ReleaseOcrInitOptions,
	ReleaseOcrPipeline,
	ReleaseOcrProcessOptions uintptr
}

// Engine is a loaded OneOCR pipeline. It is not safe for concurrent use.
type Engine struct {
	p        procs
	initOpts uintptr
	pipeline uintptr
	procOpts uintptr
}

// OpenEngine loads inst's DLLs (staging them under cacheDir when needed) and
// builds the recognition pipeline (about half a second). The DLLs stay loaded
// for the life of the process.
func OpenEngine(inst Install, cacheDir string) (*Engine, error) {
	e := &Engine{}
	var stagedDir string
	if !inst.NeedsStage {
		if err := e.load(inst.Dir); err != nil {
			// If a user copied DLLs out of WindowsApps keeping their restrictive
			// execute ACL, fall back to staging them into cacheDir.
			if !errors.Is(err, windows.ERROR_ACCESS_DENIED) || cacheDir == "" {
				return nil, err
			}
			dllDir, serr := stage(inst, cacheDir)
			if serr != nil {
				return nil, wrapErr(ErrCodeStageFailed, "", serr)
			}
			if lerr := e.load(dllDir); lerr != nil {
				return nil, lerr
			}
			stagedDir = dllDir
		}
	} else {
		dllDir, err := stage(inst, cacheDir)
		if err != nil {
			return nil, wrapErr(ErrCodeStageFailed, "", err)
		}
		if err := e.load(dllDir); err != nil {
			return nil, err
		}
		stagedDir = dllDir
	}
	if err := e.init(filepath.Join(inst.Dir, modelName)); err != nil {
		e.Close()
		return nil, err
	}
	if stagedDir != "" {
		pruneStaged(cacheDir, stagedDir)
	}
	return e, nil
}

func (e *Engine) load(dir string) error {
	// oneocr.dll loads onnxruntime.dll by name at runtime; loading ours first
	// by full path keeps it from picking up System32's older copy.
	if _, err := windows.LoadLibraryEx(filepath.Join(dir, ortName), 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH); err != nil {
		return dllLoadError(ortName, err)
	}
	dll, err := windows.LoadLibraryEx(filepath.Join(dir, dllName), 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	if err != nil {
		return dllLoadError(dllName, err)
	}
	for _, f := range []struct {
		name string
		dst  *uintptr
	}{
		{"CreateOcrInitOptions", &e.p.CreateOcrInitOptions},
		{"OcrInitOptionsSetUseModelDelayLoad", &e.p.OcrInitOptionsSetUseModelDelayLoad},
		{"CreateOcrPipeline", &e.p.CreateOcrPipeline},
		{"CreateOcrProcessOptions", &e.p.CreateOcrProcessOptions},
		{"OcrProcessOptionsSetMaxRecognitionLineCount", &e.p.OcrProcessOptionsSetMaxRecognitionLineCount},
		{"RunOcrPipeline", &e.p.RunOcrPipeline},
		{"GetImageAngle", &e.p.GetImageAngle},
		{"GetOcrLineCount", &e.p.GetOcrLineCount},
		{"GetOcrLine", &e.p.GetOcrLine},
		{"GetOcrLineContent", &e.p.GetOcrLineContent},
		{"GetOcrLineBoundingBox", &e.p.GetOcrLineBoundingBox},
		{"GetOcrLineWordCount", &e.p.GetOcrLineWordCount},
		{"GetOcrWord", &e.p.GetOcrWord},
		{"GetOcrWordContent", &e.p.GetOcrWordContent},
		{"GetOcrWordBoundingBox", &e.p.GetOcrWordBoundingBox},
		{"GetOcrWordConfidence", &e.p.GetOcrWordConfidence},
		{"ReleaseOcrResult", &e.p.ReleaseOcrResult},
		{"ReleaseOcrInitOptions", &e.p.ReleaseOcrInitOptions},
		{"ReleaseOcrPipeline", &e.p.ReleaseOcrPipeline},
		{"ReleaseOcrProcessOptions", &e.p.ReleaseOcrProcessOptions},
	} {
		if *f.dst, err = windows.GetProcAddress(dll, f.name); err != nil {
			return wrapErr(ErrCodeMissingProc, f.name, err)
		}
	}
	return nil
}

func dllLoadError(name string, err error) error {
	if errors.Is(err, windows.ERROR_BAD_EXE_FORMAT) {
		return wrapErr(ErrCodeBadArch, name, err)
	}
	return wrapErr(ErrCodeLoadDLL, name, err)
}

func (e *Engine) init(modelPath string) error {
	if rc := call(e.p.CreateOcrInitOptions, ptr(&e.initOpts)); rc != 0 {
		return &Error{Code: ErrCodePipelineInit, Arg: "CreateOcrInitOptions", Detail: fmt.Sprintf("%d", rc)}
	}
	if rc := call(e.p.OcrInitOptionsSetUseModelDelayLoad, e.initOpts, 0); rc != 0 {
		return &Error{Code: ErrCodePipelineInit, Arg: "OcrInitOptionsSetUseModelDelayLoad", Detail: fmt.Sprintf("%d", rc)}
	}
	mp, err := ansiPath(modelPath)
	if err != nil {
		return err
	}
	key := append([]byte(modelKey), 0)
	rc := call(e.p.CreateOcrPipeline, uintptr(unsafe.Pointer(&mp[0])), uintptr(unsafe.Pointer(&key[0])), e.initOpts, ptr(&e.pipeline))
	runtime.KeepAlive(mp)
	runtime.KeepAlive(key)
	switch rc {
	case 0:
	case 5:
		return &Error{Code: ErrCodeModelOpen, Arg: modelPath}
	case 6:
		return &Error{Code: ErrCodeModelKey}
	default:
		return &Error{Code: ErrCodePipelineInit, Arg: "CreateOcrPipeline", Detail: fmt.Sprintf("%d", rc)}
	}
	if rc := call(e.p.CreateOcrProcessOptions, ptr(&e.procOpts)); rc != 0 {
		return &Error{Code: ErrCodePipelineInit, Arg: "CreateOcrProcessOptions", Detail: fmt.Sprintf("%d", rc)}
	}
	if rc := call(e.p.OcrProcessOptionsSetMaxRecognitionLineCount, e.procOpts, maxLines); rc != 0 {
		return &Error{Code: ErrCodePipelineInit, Arg: "OcrProcessOptionsSetMaxRecognitionLineCount", Detail: fmt.Sprintf("%d", rc)}
	}
	return nil
}

// Recognize runs OCR on img. Box coordinates in the result refer to img.
func (e *Engine) Recognize(img Image) (Result, error) {
	if img.Width <= 0 || img.Height <= 0 || len(img.Pix) != img.Width*img.Height*4 {
		return Result{}, fmt.Errorf("invalid image %dx%d with %d bytes", img.Width, img.Height, len(img.Pix))
	}
	fitted, back := fit(img)
	res, err := e.run(fitted)
	if err != nil {
		return Result{}, err
	}
	res.mapPoints(back)
	postprocess(&res)
	return res, nil
}

func (e *Engine) run(img Image) (Result, error) {
	var pinner runtime.Pinner
	pinner.Pin(&img.Pix[0])
	defer pinner.Unpin()
	oi := &ocrImage{
		Type:   3,
		Width:  int32(img.Width),
		Height: int32(img.Height),
		Step:   int64(img.Width * 4),
		Data:   uintptr(unsafe.Pointer(&img.Pix[0])),
	}
	var res uintptr
	rc := call(e.p.RunOcrPipeline, e.pipeline, uintptr(unsafe.Pointer(oi)), e.procOpts, ptr(&res))
	runtime.KeepAlive(oi)
	if rc != 0 {
		return Result{}, &Error{Code: ErrCodePipelineInit, Arg: "RunOcrPipeline", Detail: fmt.Sprintf("%d", rc)}
	}
	defer syscall.SyscallN(e.p.ReleaseOcrResult, res)

	// Strings and boxes point into res; copy everything out before release.
	var out Result
	call(e.p.GetImageAngle, res, ptr(&out.Angle))
	var n int64
	if rc := call(e.p.GetOcrLineCount, res, ptr(&n)); rc != 0 {
		return Result{}, &Error{Code: ErrCodePipelineInit, Arg: "GetOcrLineCount", Detail: fmt.Sprintf("%d", rc)}
	}
	for i := int64(0); i < n; i++ {
		var line uintptr
		if call(e.p.GetOcrLine, res, uintptr(i), ptr(&line)) != 0 || line == 0 {
			continue
		}
		var l Line
		var sp *byte
		var bp *Quad
		if call(e.p.GetOcrLineContent, line, ptr(&sp)) == 0 {
			l.Text = windows.BytePtrToString(sp)
		}
		if call(e.p.GetOcrLineBoundingBox, line, ptr(&bp)) == 0 && bp != nil {
			l.Box = *bp
		}
		var wc int64
		call(e.p.GetOcrLineWordCount, line, ptr(&wc))
		for j := int64(0); j < wc; j++ {
			var w uintptr
			if call(e.p.GetOcrWord, line, uintptr(j), ptr(&w)) != 0 || w == 0 {
				continue
			}
			var wd Word
			var wsp *byte
			var wbp *Quad
			if call(e.p.GetOcrWordContent, w, ptr(&wsp)) == 0 {
				wd.Text = windows.BytePtrToString(wsp)
			}
			if call(e.p.GetOcrWordBoundingBox, w, ptr(&wbp)) == 0 && wbp != nil {
				wd.Box = *wbp
			}
			call(e.p.GetOcrWordConfidence, w, ptr(&wd.Confidence))
			l.Words = append(l.Words, wd)
		}
		out.Lines = append(out.Lines, l)
	}
	return out, nil
}

// Close releases the pipeline. The DLLs stay loaded.
func (e *Engine) Close() {
	if e.procOpts != 0 {
		syscall.SyscallN(e.p.ReleaseOcrProcessOptions, e.procOpts)
		e.procOpts = 0
	}
	if e.pipeline != 0 {
		syscall.SyscallN(e.p.ReleaseOcrPipeline, e.pipeline)
		e.pipeline = 0
	}
	if e.initOpts != 0 {
		syscall.SyscallN(e.p.ReleaseOcrInitOptions, e.initOpts)
		e.initOpts = 0
	}
}

// call invokes a OneOCR function and returns its int32 status (0 = success).
func call(fn uintptr, args ...uintptr) int32 {
	r, _, _ := syscall.SyscallN(fn, args...)
	return int32(r)
}

func ptr[T any](p *T) uintptr { return uintptr(unsafe.Pointer(p)) }
