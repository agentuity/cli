package errsystem

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/cli/internal/util"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

var Version string = "dev"

const baseDocURL = "https://agentuity.dev/errors/%s"

type crashReport struct {
	ID         string         `json:"id"`
	Timestamp  string         `json:"timestamp"`
	Error      string         `json:"error"`
	ErrorType  errorType      `json:"error_type"`
	Username   string         `json:"username"`
	Message    string         `json:"message,omitempty"`
	OSName     string         `json:"os_name"`
	OSArch     string         `json:"os_arch"`
	CLIVersion string         `json:"cli_version"`
	Attributes map[string]any `json:"attributes,omitempty"`
	StackTrace string         `json:"stack_trace,omitempty"`
}

func (e *errSystem) writeCrashReportFile(stackTrace string) string {
	tmp, err := os.Create(fmt.Sprintf(".agentuity-crash-%d.json", time.Now().Unix()))
	if err != nil {
		return ""
	}
	defer tmp.Close()
	var report crashReport
	report.ID = e.id
	report.Timestamp = time.Now().Format(time.RFC3339)
	if user, err := user.Current(); err == nil {
		report.Username = user.Username
	}
	report.OSName = runtime.GOOS
	report.OSArch = runtime.GOARCH
	report.Message = e.message
	if e.err != nil {
		report.Error = e.err.Error()
	}
	report.ErrorType = e.code
	report.Attributes = e.attributes
	report.CLIVersion = Version
	report.StackTrace = stackTrace
	enc := json.NewEncoder(tmp)
	enc.SetIndent(" ", " ")
	enc.Encode(report)
	return tmp.Name()
}

func (e *errSystem) sendReport(filename string) {
	u, err := url.Parse(e.apiurl)
	if err != nil {
		return
	}
	u.Path = "/cli/error"
	of, err := os.Open(filename)
	if err != nil {
		return
	}
	defer of.Close()
	req, err := http.NewRequest("POST", u.String(), of)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			os.Remove(filename)
		}
	}
}

// ShowErrorAndExit shows an error message and exits the program.
// If the program is running in a terminal, it will wait for a key press
// and then upload the error report to the Agentuity team.
// If the program is not running in a terminal, it will just exit with a non-zero exit code.
func (e *errSystem) ShowErrorAndExit() {
	tui.CancelSpinner() // cancel in case we get an error inside a spinner action
	stackTrace := string(debug.Stack())
	var body strings.Builder
	if e.message != "" {
		body.WriteString(e.message + "\n\n")
	} else {
		body.WriteString(e.code.Message + "\n\n")
	}
	var detail []string
	if e.err != nil {
		var apiError *util.APIError
		// if this is an API error, add the error details to the attributes
		if errors.As(e.err, &apiError) && apiError != nil {
			e.attributes["api_error"] = apiError.Error()
			e.attributes["api_error_url"] = apiError.URL
			e.attributes["api_error_method"] = apiError.Method
			e.attributes["api_error_status"] = strconv.Itoa(apiError.Status)
			e.attributes["api_error_body"] = apiError.Body
		}
		errmsg := e.err.Error()
		if errmsg != "" {
			errmsg = strings.ReplaceAll(errmsg, "\n", ". ")
			color := lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}
			style := tui.BannerBodyStyle().Width(65).AlignHorizontal(lipgloss.Left).Foreground(color)
			detail = append(detail, tui.Bold(tui.PadRight("Error:", 10, " "))+style.Render(errmsg+"\n"))
		}
	}
	detail = append(detail, tui.Bold(tui.PadRight("Code:", 10, " "))+e.code.Code)
	detail = append(detail, tui.Bold(tui.PadRight("ID:", 10, " "))+e.id)
	detail = append(detail, tui.Bold(tui.PadRight("Help:", 10, " "))+tui.Link(baseDocURL, e.code.Code))
	crashReportFile := e.writeCrashReportFile(stackTrace)
	for _, d := range detail {
		body.WriteString(tui.Muted(d) + "\n")
	}
	if !tui.HasTTY {
		fmt.Println(body.String())
		for k, v := range e.attributes {
			fmt.Printf("%s: %v\n", k, v)
		}
		os.Exit(1)
	}
	tui.ShowBanner(tui.Warning("☹ Error Detected"), body.String(), false)
	if isatty.IsTerminal(os.Stdout.Fd()) {
		tui.WaitForAnyKeyMessage(" Press any key to upload the error report\n to the Agentuity team or press Ctrl+C to cancel ...")
		fmt.Println()
		action := func() {
			e.sendReport(crashReportFile)
		}
		tui.ShowSpinner("Uploading error report...", action)
		tui.ShowSuccess("We will process the report as soon as possible! 🏃")
	}
	os.Exit(1)
}
