package installer

import (
	"fmt"
	"strings"
)

// Label is the launchd service label for the swarmery daemon.
const Label = "com.swarmery.daemon"

// EnvVar is one launchd EnvironmentVariables entry baked into the plist. Order
// is preserved (deterministic output for the golden tests).
type EnvVar struct{ Key, Value string }

// Plist renders the launchd property list for the daemon.
//
// binPath is the installed binary (~/.swarmery/bin/swarmery), logsDir the
// directory for stdout/stderr logs. If port > 0 a SWARMERY_PORT entry is
// emitted first, followed by any extra vars (e.g. SWARMERY_ONBOARD_ROOTS).
// launchd does NOT inherit the installing shell's environment, so anything the
// daemon needs at runtime must be baked in here. With no port and no extra
// vars the EnvironmentVariables block is omitted entirely.
func Plist(binPath, logsDir string, port int, extra ...EnvVar) string {
	entries := make([]EnvVar, 0, 1+len(extra))
	if port > 0 {
		entries = append(entries, EnvVar{Key: "SWARMERY_PORT", Value: fmt.Sprintf("%d", port)})
	}
	entries = append(entries, extra...)

	var env string
	if len(entries) > 0 {
		var sb strings.Builder
		sb.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
		for _, e := range entries {
			fmt.Fprintf(&sb, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", e.Key, xmlEscape(e.Value))
		}
		sb.WriteString("\t</dict>\n")
		env = sb.String()
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>serve</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s/swarmery.out.log</string>
	<key>StandardErrorPath</key>
	<string>%s/swarmery.err.log</string>
%s</dict>
</plist>
`, Label, binPath, logsDir, logsDir, env)
	return b.String()
}

// xmlEscape escapes the characters that would break a plist <string> value.
// Env values are paths/root lists, but an ampersand or angle bracket must not
// corrupt the XML.
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
