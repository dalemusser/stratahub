package mhssteplog

import "regexp"

// crashNoticeRe matches the unhandled promise rejections the game raises
// while it keeps running: pointer-lock requests made without a user gesture
// or refused by a permissions check, fullscreen and AudioContext refusals.
// In the Oct 2026 data 94 of 100 "crash" records were these; the game went
// on and the page kept heartbeating. They are browser notices, not crashes.
var crashNoticeRe = regexp.MustCompile(`(?i)pointer lock|permissions check failed|too many pointer lock|fullscreen|audiocontext`)

// IsCrashNotice reports whether a crash entry of the given type and message
// is a browser notice rather than a failure. Only unhandled rejections can
// be notices: a runtime error or a load failure with the same words is real.
func IsCrashNotice(crashType, message string) bool {
	return crashType == "unhandled_rejection" && crashNoticeRe.MatchString(message)
}
