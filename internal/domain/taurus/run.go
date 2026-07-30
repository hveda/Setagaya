package taurus

import "fmt"

// Outcome is how a bzt run ended, as told by its exit code. It is deliberately
// not Honryu's verdict: a verdict is a judgement about the target, rolled up
// across an execution or a campaign, and only some outcomes contribute to one.
type Outcome string

const (
	// OutcomePassed means the run completed and no pass/fail criteria tripped.
	OutcomePassed Outcome = "passed"
	// OutcomeFailed means the run completed and criteria tripped -- a statement
	// about the target.
	OutcomeFailed Outcome = "failed"
	// OutcomeAborted means the run was interrupted before it could finish.
	OutcomeAborted Outcome = "aborted"
	// OutcomeError means bzt or the engine itself failed -- a statement about
	// the generator, not the target.
	OutcomeError Outcome = "error"
)

// bzt's exit codes, from its source rather than observation:
// NormalShutdown.get_rc() is 0, ManualShutdown 2, AutomatedShutdown 3 (raised
// by bzt/modules/passfail.py when criteria trip), and bzt/cli.py falls back to
// 1 for any other exception.
const (
	exitNormal    = 0
	exitManual    = 2
	exitAutomated = 3
)

// OutcomeFromExitCode interprets a bzt exit code.
//
// An exit code alone cannot tell Honryu that a run was aborted, and callers
// must not use one for that. bzt installs its SIGINT/SIGTERM handlers inside
// cli.py's `if __name__ == "__main__"` block, which neither the `bzt` console
// script nor `python -m bzt` executes -- so ManualShutdown is never raised and
// exit 2 never occurs in practice. Verified against bzt 1.16.51:
//
//	SIGINT  -> Python's own KeyboardInterrupt; bzt shuts down gracefully and
//	           writes final stats, but exits 1, indistinguishable from a config
//	           error.
//	SIGTERM -> no handler, so the process dies immediately: exit 143 at the
//	           shell, and no final stats are written at all.
//
// Kubernetes sends SIGTERM when it deletes a pod, so that is the path every
// Honryu teardown takes. Abort is therefore established from Honryu's own
// state -- it issued the teardown and knows it -- and this mapping is only
// consulted for runs Honryu did not stop.
//
// Exit 2 is still mapped, for correctness if bzt is ever launched in a way that
// installs its handlers.
func OutcomeFromExitCode(code int) Outcome {
	switch code {
	case exitNormal:
		return OutcomePassed
	case exitManual:
		return OutcomeAborted
	case exitAutomated:
		return OutcomeFailed
	default:
		return OutcomeError
	}
}

// CountsTowardVerdict reports whether this outcome is evidence about the target.
// Only a completed run is: an abort was Honryu's own doing, and an engine error
// means the generator broke before it could measure anything.
func (o Outcome) CountsTowardVerdict() bool {
	return o == OutcomePassed || o == OutcomeFailed
}

// Command is the argv that runs a compiled config. artifactsDir may be empty,
// in which case bzt chooses its own.
//
// The config path precedes the overrides because bzt treats bare arguments as
// configuration files. Update checking is disabled so an engine pod needs no
// outbound internet access.
func Command(configPath, artifactsDir string) []string {
	argv := []string{"bzt", configPath, "-o", "settings.check-updates=false"}
	if artifactsDir != "" {
		argv = append(argv, "-o", fmt.Sprintf("settings.artifacts-dir=%s", artifactsDir))
	}
	return argv
}
