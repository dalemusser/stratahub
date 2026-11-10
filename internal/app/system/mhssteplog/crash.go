package mhssteplog

import (
	"regexp"
	"strings"
)

// crashNoticeRe matches the unhandled promise rejections the game raises
// while it keeps running: pointer-lock requests made without a user gesture
// or refused by a permissions check, fullscreen and AudioContext refusals.
// In the Oct 2026 data 94 of 100 "crash" records were these; the game went
// on and the page kept heartbeating. They are browser notices, not crashes.
var crashNoticeRe = regexp.MustCompile(`(?i)pointer lock|permissions check failed|too many pointer lock|fullscreen|audiocontext`)

// IsCrashNotice reports whether a crash entry of the given type and message
// is a browser notice rather than a failure. Unhandled rejections matching
// crashNoticeRe are notices (a runtime error or a load failure with the same
// words is real), and so is a runtime error whose message is the browser's
// masked "Script error.": an error raised by a script from another origin,
// with no detail at all (Nov 2026: 20 of School O's 47 crash records, most
// likely a filter or monitoring extension on managed laptops; some runs went
// on normally after it). Nothing in it says the game failed.
func IsCrashNotice(crashType, message string) bool {
	if crashType == "runtime_error" && IsMaskedScriptError(message) {
		return true
	}
	return crashType == "unhandled_rejection" && crashNoticeRe.MatchString(message)
}

// IsMaskedScriptError reports whether an error message is the browser's
// masked cross-origin error ("Script error." in Chrome and Firefox).
func IsMaskedScriptError(message string) bool {
	m := strings.TrimSpace(message)
	return m == "Script error." || m == "Script error"
}
