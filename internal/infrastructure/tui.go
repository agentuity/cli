package infrastructure

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-shellwords"
	"github.com/muesli/reflow/wordwrap"
	"github.com/sergeymakinen/go-quote/unix"
	"golang.design/x/clipboard"
	"golang.org/x/term"
)

func init() {
	clipboard.Init()
}

var commandPrompt = lipgloss.AdaptiveColor{Light: "#FF7F50", Dark: "#FFAC1C"}
var commandPromptStyle = lipgloss.NewStyle().Foreground(commandPrompt)

var commandBody = lipgloss.AdaptiveColor{Light: "#009900", Dark: "#00FF00"}
var commandBodyStyle = lipgloss.NewStyle().Foreground(commandBody)

var textBody = lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"}

var commandBorderStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).AlignVertical(lipgloss.Top).
	AlignHorizontal(lipgloss.Left).
	BorderForeground(lipgloss.Color("63")).
	PaddingLeft(1).PaddingRight(1).
	MaxWidth(80).Width(78).Foreground(textBody).MarginBottom(1)

type actionType int

const (
	skip actionType = iota
	run
	manual
	cancelled
	edit
)

func confirmAction(canExecute bool) actionType {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	ch := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		os.Stdin.Read(buf)
		fmt.Print("\x1b[2K\x1b[2K\r") // erase current line and move cursor to beginning
		ch <- buf[0]
	}()
	if canExecute {
		fmt.Printf(" %s%s%s%s%s %s %s ", commandPromptStyle.Render("[R]un"), tui.Muted(", "), commandPromptStyle.Render("[E]dit"), tui.Muted(", "), commandPromptStyle.Render("[S]kip"), tui.Muted("or"), commandPromptStyle.Render("[M]anual"))
	} else {
		fmt.Printf("%s %s %s ", commandPromptStyle.Render("[S]kip"), tui.Muted("or"), commandPromptStyle.Render("[C]ompleted"))
	}
	select {
	case <-ctx.Done():
		fmt.Println()
		return cancelled
	case answer := <-ch:
		select {
		case <-ctx.Done():
			fmt.Println()
			return cancelled
		default:
		}
		switch answer {
		case 'R', 'r', '\n', '\r':
			if canExecute {
				return run
			}
			return manual
		case 'S', 's':
			return skip
		case 'M', 'm', 'C', 'c':
			return manual
		case 'E', 'e':
			return edit
		}
	}
	return cancelled
}

type possibleSkipFunc func(ctx context.Context) (bool, error)
type runFunc func(ctx context.Context, cmd string, args []string) error

func quoteCmdArg(arg string) string {
	if unix.SingleQuote.MustQuote(arg) {
		return unix.SingleQuote.Quote(arg)
	}
	return arg
}

func execAction(ctx context.Context, canExecute bool, instruction string, help string, cmd string, args []string, runner runFunc, success string, skipFunc possibleSkipFunc) error {
	fmt.Println(commandBorderStyle.Render(instruction + "\n\n" + tui.Muted(help)))
	f := wordwrap.NewWriter(78)
	f.Newline = []rune{'\r'}
	f.KeepNewlines = true
	f.Breakpoints = []rune{' ', '|'}
	f.Write([]byte(commandPromptStyle.Render("$ ")))
	f.Write([]byte(commandBodyStyle.Render(cmd)))
	f.Write([]byte(" "))
	for _, arg := range args {
		f.Write([]byte(commandBodyStyle.Render(arg)))
		f.Write([]byte(" "))
	}
	f.Close()
	v := f.String()
	v = strings.ReplaceAll(v, "\n", tui.Muted(" \\\n  "))
	cmdbuf := []byte(cmd + " " + strings.Join(args, " "))
	clipboard.Write(clipboard.FmtText, cmdbuf)
	fmt.Println(v)
	fmt.Println()
	switch confirmAction(canExecute) {
	case run:
		var skip bool
		var err error
		if skipFunc != nil {
			skip, err = skipFunc(ctx)
			if err != nil {
				return err
			}
		}
		if !skip {
			if err := runner(ctx, cmd, args); err != nil {
				return err
			}
		}
		tui.ShowSuccess("%s", success)
	case skip:
		tui.ShowWarning("Skipped")
	case manual:
		tui.ShowSuccess("Manually executed")
	case cancelled:
		os.Exit(1)
	case edit:
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		tf, err := os.CreateTemp("", "")
		if err != nil {
			return fmt.Errorf("error opening temporary file for editing: %w", err)
		}
		tf.Write([]byte(cmd))
		for _, arg := range args {
			tf.Write([]byte(" "))
			if strings.HasPrefix(arg, "--") {
				tf.Write([]byte(arg))
			} else {
				tf.Write([]byte(quoteCmdArg(arg)))
			}
		}
		tf.Close()
		defer func() {
			os.Remove(tf.Name())
		}()
		c := exec.Command(editor, tf.Name())
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		if err := c.Run(); err != nil {
			return fmt.Errorf("error running editor: %w", err)
		}
		newbuf, err := os.ReadFile(tf.Name())
		if err != nil {
			return fmt.Errorf("error reading edited file: %w", err)
		}
		args, err := shellwords.Parse(strings.TrimSpace(string(newbuf)))
		if err != nil {
			return fmt.Errorf("error parsing edited command: %w", err)
		}
		return execAction(ctx, canExecute, instruction, help, args[0], args[1:], runner, success, skipFunc)
	}
	fmt.Println()
	return nil
}
