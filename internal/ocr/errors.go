package ocr

import (
	"errors"
	"fmt"
)

// ErrCode classifies user-facing OCR failures so the UI can localize them
// without coupling the worker process to a UI language.
type ErrCode string

const (
	ErrCodeNotInstalled      ErrCode = "not_installed"
	ErrCodeCustomDirMissing  ErrCode = "custom_dir_missing"
	ErrCodeStageFailed       ErrCode = "stage_failed"
	ErrCodeLoadDLL           ErrCode = "load_dll"
	ErrCodeBadArch           ErrCode = "bad_arch"
	ErrCodeMissingProc       ErrCode = "missing_proc"
	ErrCodeModelPathEncoding ErrCode = "model_path_encoding"
	ErrCodeModelOpen         ErrCode = "model_open"
	ErrCodeModelKey          ErrCode = "model_key"
	ErrCodePipelineInit      ErrCode = "pipeline_init"
	ErrCodeSelfTest          ErrCode = "self_test"
	ErrCodeWorkerStart       ErrCode = "worker_start"
	ErrCodeWorkerTimeout     ErrCode = "worker_timeout"
	ErrCodeWorkerIO          ErrCode = "worker_io"
)

// Error is a structured OCR failure that survives serialization across the
// worker process boundary.
type Error struct {
	Code   ErrCode
	Arg    string
	Detail string
	Err    error
}

// ErrNotInstalled means neither local OneOCR files nor Snipping Tool were found.
var ErrNotInstalled error = &Error{Code: ErrCodeNotInstalled}

func (e *Error) Error() string {
	switch e.Code {
	case ErrCodeNotInstalled:
		return "OneOCR files not found; place oneocr.dll, oneocr.onemodel, and onnxruntime.dll beside SnipGlyph (or in a oneocr subfolder), or install Windows Snipping Tool from Microsoft Store"
	case ErrCodeCustomDirMissing:
		return fmt.Sprintf("configured OneOCR folder %s is missing: %s", e.Arg, e.Detail)
	case ErrCodeStageFailed:
		return fmt.Sprintf("failed to copy OneOCR files: %s", e.Detail)
	case ErrCodeLoadDLL:
		return fmt.Sprintf("failed to load %s: %s", e.Arg, e.Detail)
	case ErrCodeBadArch:
		return fmt.Sprintf("failed to load %s (x64 DLL required): %s", e.Arg, e.Detail)
	case ErrCodeMissingProc:
		return fmt.Sprintf("OneOCR is missing %s (incompatible version): %s", e.Arg, e.Detail)
	case ErrCodeModelPathEncoding:
		return fmt.Sprintf("OneOCR model path %q is not representable in the Windows ANSI code page; move the files to an ASCII path", e.Arg)
	case ErrCodeModelOpen:
		return fmt.Sprintf("OneOCR could not open model file %s", e.Arg)
	case ErrCodeModelKey:
		return "OneOCR model key mismatch (incompatible OneOCR version)"
	case ErrCodePipelineInit:
		return fmt.Sprintf("%s returned %s", e.Arg, e.Detail)
	case ErrCodeSelfTest:
		return fmt.Sprintf("OneOCR self-test failed: %s", e.Detail)
	case ErrCodeWorkerStart:
		return fmt.Sprintf("failed to start OCR worker: %s", e.Detail)
	case ErrCodeWorkerTimeout:
		return "OCR worker timed out while starting"
	case ErrCodeWorkerIO:
		return fmt.Sprintf("OCR worker communication failed: %s", e.Detail)
	}
	if e.Detail != "" {
		return e.Detail
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool {
	var o *Error
	if errors.As(target, &o) {
		return e.Code == o.Code && (o.Arg == "" || e.Arg == o.Arg)
	}
	return false
}

func wrapErr(code ErrCode, arg string, err error) *Error {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return &Error{Code: code, Arg: arg, Detail: detail, Err: err}
}
