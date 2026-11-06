package autostart

import (
	"bytes"
	"embed"
	"encoding/xml"
	"strings"
	"text/template"
)

//go:embed templates
var templateFiles embed.FS

// templates are the service definitions, each given the paths it runs.
var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"xml":   xmlText,
	"quote": systemdQuote,
}).ParseFS(templateFiles, "templates/*"))

// launchdAgent is what templates/launchd.plist is filled in with.
type launchdAgent struct {
	Label  string
	Binary string
	Log    string
}

// systemdService is what templates/systemd.service is filled in with.
type systemdService struct {
	Binary string
}

func render(name string, data any) (string, error) {
	var b strings.Builder
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		return "", err
	}

	return b.String(), nil
}

func xmlText(s string) string {
	var b bytes.Buffer

	_ = xml.EscapeText(&b, []byte(s))

	return b.String()
}

// systemdQuote makes path one word of a command line, where systemd would
// otherwise expand % specifiers and $ variables in it.
func systemdQuote(path string) string {
	return `"` + strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`%`, `%%`,
		`$`, `$$`,
	).Replace(path) + `"`
}
