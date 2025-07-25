// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package terminal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/mitchellh/go-wordwrap"

	"github.com/hashicorp/nomad-pack/internal/pkg/errors"
	"github.com/hashicorp/nomad-pack/internal/pkg/helper"
)

type nonInteractiveUI struct {
	mu sync.Mutex
}

func NonInteractiveUI(ctx context.Context) UI {
	result := &nonInteractiveUI{}
	return result
}

func (ui *nonInteractiveUI) Input(input *Input) (string, error) {
	return "", ErrNonInteractive
}

// Interactive implements UI
func (ui *nonInteractiveUI) Interactive() bool {
	return false
}

// Output implements UI
func (ui *nonInteractiveUI) Output(msg string, raw ...any) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	msg, style, w := Interpret(msg, raw...)

	switch style {
	case DebugStyle:
		msg = colorDebug.Sprintf("debug: %s\n", msg)
	case HeaderStyle:
		msg = "\n» " + msg
	case ErrorStyle, ErrorBoldStyle:
		lines := strings.Split(msg, "\n")
		if len(lines) > 0 {
			fmt.Fprintln(w, "! "+lines[0])
			for _, line := range lines[1:] {
				fmt.Fprintln(w, "  "+line)
			}
		}

		return
	case WarningStyle, WarningBoldStyle:
		msg = colorWarning.Sprintf("warning: %s\n", msg)
	case TraceStyle:
		msg = colorTrace.Sprintf("trace: %s\n", msg)
	case SuccessStyle, SuccessBoldStyle:

	case InfoStyle:
		lines := strings.Split(msg, "\n")
		for i, line := range lines {
			lines[i] = colorInfo.Sprintf("  %s", line)
		}

		msg = strings.Join(lines, "\n")
	}

	fmt.Fprintln(w, msg)
}

// TODO: Added purely for compilation purposes. Untested
func (ui *nonInteractiveUI) AppendToRow(msg string, raw ...any) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	msg, style, w := Interpret(msg, raw...)

	switch style {
	case HeaderStyle:
		msg = "\n» " + msg
	case ErrorStyle, ErrorBoldStyle:
		lines := strings.Split(msg, "\n")
		if len(lines) > 0 {
			fmt.Fprintln(w, "! "+lines[0])
			for _, line := range lines[1:] {
				fmt.Fprintln(w, "  "+line)
			}
		}

		return

	case WarningStyle, WarningBoldStyle:
		msg = "warning: " + msg

	case SuccessStyle, SuccessBoldStyle:

	case InfoStyle:
		lines := strings.Split(msg, "\n")
		for i, line := range lines {
			lines[i] = colorInfo.Sprintf("  %s", line)
		}

		msg = strings.Join(lines, "\n")
	}

	fmt.Fprint(w, msg) // TODO does this work
}

// NamedValues implements UI
func (ui *nonInteractiveUI) NamedValues(rows []NamedValue, opts ...Option) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	cfg := &config{Writer: color.Output}
	for _, opt := range opts {
		opt(cfg)
	}

	var buf bytes.Buffer
	tr := tabwriter.NewWriter(&buf, 1, 8, 0, ' ', tabwriter.AlignRight)
	for _, row := range rows {
		switch v := row.Value.(type) {
		case int, uint, int8, uint8, int16, uint16, int32, uint32, int64, uint64:
			fmt.Fprintf(tr, "  %s: \t%d\n", row.Name, row.Value)
		case float32, float64:
			fmt.Fprintf(tr, "  %s: \t%f\n", row.Name, row.Value)
		case bool:
			fmt.Fprintf(tr, "  %s: \t%v\n", row.Name, row.Value)
		case string:
			if v == "" {
				continue
			}
			fmt.Fprintf(tr, "  %s: \t%s\n", row.Name, row.Value)
		default:
			fmt.Fprintf(tr, "  %s: \t%s\n", row.Name, row.Value)
		}
	}
	tr.Flush()

	fmt.Fprintln(cfg.Writer, buf.String())
}

// OutputWriters implements UI
func (ui *nonInteractiveUI) OutputWriters() (io.Writer, io.Writer, error) {
	return os.Stdout, os.Stderr, nil
}

// Status implements UI
func (ui *nonInteractiveUI) Status() Status {
	return &nonInteractiveStatus{mu: &ui.mu}
}

func (ui *nonInteractiveUI) StepGroup() StepGroup {
	return &nonInteractiveStepGroup{mu: &ui.mu}
}

// Table implements UI
func (ui *nonInteractiveUI) Table(tbl *Table, opts ...Option) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	// Build our config and set our options
	cfg := &config{Writer: color.Output}
	for _, opt := range opts {
		opt(cfg)
	}

	table := TableWithSettings(cfg.Writer, tbl.Headers)
	table.Bulk(tbl.Rows)
	table.Render()
}

// Debug implements UI
func (ui *nonInteractiveUI) Debug(msg string) {
	ui.Output(msg, WithDebugStyle())
}

// Error implements UI
func (ui *nonInteractiveUI) Error(msg string) {
	ui.Output(msg, WithErrorStyle())
}

// ErrorWithContext satisfies the ErrorWithContext function on the UI
// interface.
func (ui *nonInteractiveUI) ErrorWithContext(err error, sub string, ctx ...string) {
	ui.Error(helper.Title(sub))
	ui.Error("  Error: " + err.Error())

	// Selectively promote Details and Suggestion from the context.
	var extractItem = func(ctx []string, key string) ([]string, string, bool) {
		for i, v := range ctx {
			if strings.HasPrefix(v, key) {
				outStr := v
				outCtx := slices.Delete(ctx, i, i+1)
				return outCtx, outStr, true
			}
		}
		return ctx, "", false
	}
	var promote = func(key string) {
		if oc, item, found := extractItem(ctx, key); found {
			ctx = oc
			if key == "" {
				return
			}

			key, rest, found := strings.Cut(item, ": ")

			if !found {
				wrapped := wordwrap.WrapString(key, 78)
				lines := strings.Split(wrapped, "\n")
				for _, l := range lines {
					ui.Error("  " + l)
				}
				return
			}
			wrapped := wordwrap.WrapString(rest, uint(78-len(key)))
			lines := strings.Split(wrapped, "\n")
			for i, l := range lines {
				if i == 0 {
					ui.Error(fmt.Sprintf("  %s: %s", key, l))
					continue
				}

				ui.Error(fmt.Sprintf("  %s  %s", strings.Repeat(" ", len(key)), l))
			}
		}
	}

	promote(errors.UIContextErrorDetail)
	promote(errors.UIContextErrorSuggestion)

	ui.Error("  Context:")
	max := 0
	for _, entry := range ctx {
		if loc := strings.Index(entry, ":") + 1; loc > max {
			max = loc
		}
	}
	for _, entry := range ctx {
		padding := max - strings.Index(entry, ":") + 1
		ui.Error("  " + strings.Repeat(" ", padding) + entry)
	}
}

// Header implements UI
func (ui *nonInteractiveUI) Header(msg string) {
	ui.Output(msg, WithHeaderStyle())
}

// Info implements UI
func (ui *nonInteractiveUI) Info(msg string) {
	ui.Output(msg, WithInfoStyle())
}

// Success implements UI
func (ui *nonInteractiveUI) Success(msg string) {
	ui.Output(msg, WithSuccessStyle())
}

// Trace implements UI
func (ui *nonInteractiveUI) Trace(msg string) {
	ui.Output(msg, WithTraceStyle())
}

// Warning implements UI
func (ui *nonInteractiveUI) Warning(msg string) {
	ui.Output(msg, WithWarningStyle())
}

// WarningBold implements UI
func (ui *nonInteractiveUI) WarningBold(msg string) {
	ui.Output(msg, WithStyle(WarningBoldStyle))
}

type nonInteractiveStatus struct {
	mu *sync.Mutex
}

func (s *nonInteractiveStatus) Update(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintln(color.Output, msg)
}

func (s *nonInteractiveStatus) Step(status, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(color.Output, "%s: %s\n", textStatus[status], msg)
}

func (s *nonInteractiveStatus) Close() error {
	return nil
}

type nonInteractiveStepGroup struct {
	mu     *sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

// Start a step in the output
func (f *nonInteractiveStepGroup) Add(str string, args ...any) Step {
	// Build our step
	step := &nonInteractiveStep{mu: f.mu}

	// Setup initial status
	step.Update(str, args...)

	// Grab the lock now so we can update our fields
	f.mu.Lock()
	defer f.mu.Unlock()

	// If we're closed we don't add this step to our waitgroup or document.
	// We still create a step and return a non-nil step so downstreams don't
	// crash.
	if !f.closed {
		// Add since we have a step
		step.wg = &f.wg
		f.wg.Add(1)
	}

	return step
}

func (f *nonInteractiveStepGroup) Wait() {
	f.mu.Lock()
	f.closed = true
	wg := &f.wg
	f.mu.Unlock()

	wg.Wait()
}

type nonInteractiveStep struct {
	mu   *sync.Mutex
	wg   *sync.WaitGroup
	done bool
}

func (f *nonInteractiveStep) TermOutput() io.Writer {
	return &stripAnsiWriter{Next: color.Output}
}

func (f *nonInteractiveStep) Update(str string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fmt.Fprintln(color.Output, "-> "+fmt.Sprintf(str, args...))
}

func (f *nonInteractiveStep) Status(status string) {}

func (f *nonInteractiveStep) Done() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.done {
		return
	}

	// Set done
	f.done = true

	// Unset the waitgroup
	f.wg.Done()
}

func (f *nonInteractiveStep) Abort() {
	f.Done()
}

type stripAnsiWriter struct {
	Next io.Writer
}

func (w *stripAnsiWriter) Write(p []byte) (n int, err error) {
	return w.Next.Write(reAnsi.ReplaceAll(p, []byte{}))
}

var reAnsi = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
