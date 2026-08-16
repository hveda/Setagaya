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

// CombineOutcomes rolls up several shards' exit codes into one outcome for a
// sharded run.
//
// No codes at all -- every shard torn down before it could report one -- is
// not evidence the target passed; it is no evidence at all, and is treated as
// an error rather than assumed clean.
func CombineOutcomes(codes []int) Outcome {
	if len(codes) == 0 {
		return OutcomeError
	}
	outcomes := make([]Outcome, len(codes))
	for i, code := range codes {
		outcomes[i] = OutcomeFromExitCode(code)
	}
	return WorstOutcome(outcomes)
}

// WorstOutcome returns the most severe of several outcomes, for a caller that
// has some of its own outside any exit code -- e.g. metricsapp.Finalize folds
// in Honryu's own certainty that it stopped a run alongside whatever a shard
// that had already finished naturally reported.
//
// Most severe wins. A shard whose engine errored means the run says nothing
// trustworthy about the target, no matter what the other shards measured; a
// shard whose criteria failed means the target failed, no matter that other
// shards happened to pass. Only when every shard passed does the run pass.
func WorstOutcome(outcomes []Outcome) Outcome {
	seen := make(map[Outcome]bool, len(outcomes))
	for _, o := range outcomes {
		seen[o] = true
	}
	switch {
	case seen[OutcomeError]:
		return OutcomeError
	case seen[OutcomeFailed]:
		return OutcomeFailed
	case seen[OutcomeAborted]:
		return OutcomeAborted
	default:
		return OutcomePassed
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
