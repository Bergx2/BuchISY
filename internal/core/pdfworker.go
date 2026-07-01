package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"time"
)

// The ledongthuc/pdf parser that powers the highlight-rectangle functions can,
// on certain PDFs, enter an unbounded-allocation loop (~500 MB/s, no end). A Go
// goroutine cannot be killed, so an in-process timeout could only abandon it,
// leaving the runaway goroutine to exhaust memory and freeze the whole machine.
//
// To make highlighting safe we run the parse in a SHORT-LIVED CHILD PROCESS
// (the same executable, re-entered via the BUCHISY_PDFRECTS env guard below).
// A pathological PDF is then contained: the child is killed on timeout and the
// OS reclaims all of its memory. For every normal PDF the child finishes in
// well under a second and behaviour is identical to calling the impl directly.

const pdfRectsEnv = "BUCHISY_PDFRECTS"

// rectsRequest / rectsResponse are the JSON exchanged with the worker over
// stdin/stdout.
type rectsRequest struct {
	Mode   string   `json:"mode"` // "highlight" | "block" | "row"
	Path   string   `json:"path"`
	Values []string `json:"values"`
	DPI    float64  `json:"dpi"`
}

type rectsResponse struct {
	Rects [][]Rect `json:"rects"`
	Err   string   `json:"err"`
}

// init makes any process that imports this package double as the PDF-rects
// worker when BUCHISY_PDFRECTS=1 is set in its environment. This runs before
// testing.Main / ui.New, so the child never starts the GUI (or re-runs tests).
func init() {
	if os.Getenv(pdfRectsEnv) != "1" {
		return
	}
	runPDFRectsWorker()
	os.Exit(0)
}

// runPDFRectsWorker reads one rectsRequest from stdin, computes the rectangles
// in-process (this IS the disposable child), and writes a rectsResponse to
// stdout. Any panic in the parser is reported as an error rather than crashing.
func runPDFRectsWorker() {
	// Hard-cap this disposable child's committed memory (Windows) so a runaway
	// parse aborts the process instead of exhausting the machine's RAM.
	limitWorkerMemory(1 << 30) // 1 GiB

	var resp rectsResponse
	defer func() {
		if r := recover(); r != nil {
			resp = rectsResponse{Err: "panic in pdf worker"}
		}
		_ = json.NewEncoder(os.Stdout).Encode(resp)
	}()

	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		resp.Err = err.Error()
		return
	}
	var req rectsRequest
	if err := json.Unmarshal(in, &req); err != nil {
		resp.Err = err.Error()
		return
	}

	var rects [][]Rect
	var cerr error
	switch req.Mode {
	case "block":
		rects, cerr = statementBlockRectsImpl(req.Path, req.Values, req.DPI)
	case "row":
		rects, cerr = statementRowRectsImpl(req.Path, req.Values, req.DPI)
	default:
		rects, cerr = highlightRectsImpl(req.Path, req.Values, req.DPI)
	}
	resp.Rects = rects
	if cerr != nil {
		resp.Err = cerr.Error()
	}
}

// rectsWorkerTimeout bounds a single highlight parse. Normal PDFs finish in
// well under a second; a pathological one is killed at this point and the
// preview simply renders without highlight boxes.
const rectsWorkerTimeout = 5 * time.Second

// isolatedRects runs the given highlight mode in a killable child process. On
// timeout, spawn failure, or any error it returns (nil, nil): highlighting is
// best-effort and its callers already treat a nil result as "no boxes".
func isolatedRects(mode, path string, values []string, dpi float64) ([][]Rect, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil
	}
	reqBytes, err := json.Marshal(rectsRequest{Mode: mode, Path: path, Values: values, DPI: dpi})
	if err != nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), rectsWorkerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe)
	// The child hard-caps its own committed memory (see limitWorkerMemory) and is
	// killed if it outlives the context — two independent bounds on a runaway PDF.
	cmd.Env = append(os.Environ(), pdfRectsEnv+"=1")
	hideChildConsole(cmd) // no console-window flash on Windows
	cmd.Stdin = bytes.NewReader(reqBytes)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		// Timeout (killed), non-zero exit, or spawn failure → no highlights.
		return nil, nil
	}
	var resp rectsResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return nil, nil
	}
	return resp.Rects, nil
}

// HighlightRects returns per-page yellow highlight rectangles (image pixels) for
// each verbatim occurrence of values, computed in a killable child process so a
// hostile PDF can never hang the app. See highlightRectsImpl for the details.
func HighlightRects(path string, values []string, dpi float64) ([][]Rect, error) {
	return isolatedRects("highlight", path, values, dpi)
}

// StatementBlockRects frames each matched booking's whole block (dated row plus
// its detail rows). Runs out-of-process; see statementBlockRectsImpl.
func StatementBlockRects(path string, values []string, dpi float64) ([][]Rect, error) {
	return isolatedRects("block", path, values, dpi)
}

// StatementRowRects frames just the matched statement row(s). Runs
// out-of-process; see statementRowRectsImpl.
func StatementRowRects(path string, values []string, dpi float64) ([][]Rect, error) {
	return isolatedRects("row", path, values, dpi)
}
