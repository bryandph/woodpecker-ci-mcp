package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

const (
	defaultWaitTimeout       = 2 * time.Hour
	maxWaitTimeout           = 6 * time.Hour
	defaultPollInterval      = 15 * time.Second
	minPollInterval          = 5 * time.Second
	maxPollInterval          = 60 * time.Second
	defaultFailureLogLines   = 100
	maxFailureLogLines       = 1000
	maxFailedStepsWithLogs   = 5
	maxConcurrentWaits       = 3
	statusDeclined           = "declined"
	maxCommitLookupPipelines = 50
)

// pipelineSource is the subset of the Woodpecker client used while waiting.
type pipelineSource interface {
	GetPipeline(repoID, pipelineNum int64) (*woodpecker.Pipeline, error)
	GetLastPipeline(repoID int64) (*woodpecker.Pipeline, error)
	ListPipelines(repoID int64) ([]*woodpecker.Pipeline, error)
	GetStepLogs(repoID, pipelineNum, stepID int64) ([]*woodpecker.LogEntry, error)
}

// waitSlots bounds concurrent waits so abandoned calls cannot exhaust the
// stdio worker pool that also serves every other tool.
var waitSlots = make(chan struct{}, maxConcurrentWaits)

type waitOptions struct {
	repoID         int64
	pipelineNumber int64
	latest         bool
	commit         string
	timeout        time.Duration
	pollInterval   time.Duration
	logLines       int
	waitOnBlocked  bool
}

// progressFunc reports liveness to the client while a wait is in progress.
type progressFunc func(elapsed time.Duration, status string)

type failedStep struct {
	Workflow string   `json:"workflow"`
	Name     string   `json:"name"`
	StepID   int64    `json:"step_id"`
	State    string   `json:"state"`
	ExitCode int      `json:"exit_code"`
	Error    string   `json:"error,omitempty"`
	Logs     []string `json:"logs,omitempty"`
	LogNote  string   `json:"log_note,omitempty"`
}

type waitResult struct {
	RepoID          int64             `json:"repo_id"`
	PipelineNumber  int64             `json:"pipeline_number"`
	Status          string            `json:"status"`
	Outcome         string            `json:"outcome"`
	TimedOut        bool              `json:"timed_out"`
	WaitedSeconds   int64             `json:"waited_seconds"`
	DurationSeconds int64             `json:"duration_seconds,omitempty"`
	Event           string            `json:"event,omitempty"`
	Branch          string            `json:"branch,omitempty"`
	Commit          string            `json:"commit,omitempty"`
	Message         string            `json:"message,omitempty"`
	ForgeURL        string            `json:"forge_url,omitempty"`
	PipelineErrors  []string          `json:"pipeline_errors,omitempty"`
	FailedSteps     []failedStep      `json:"failed_steps,omitempty"`
	StepStates      map[string]string `json:"step_states,omitempty"`
}

func isTerminalStatus(status string) bool {
	switch status {
	case woodpecker.StatusSuccess, woodpecker.StatusFailure, woodpecker.StatusKilled,
		woodpecker.StatusError, woodpecker.StatusSkipped, statusDeclined:
		return true
	}
	return false
}

func isFailedState(state string) bool {
	return state == woodpecker.StatusFailure || state == woodpecker.StatusError
}

func parseWaitOptions(arguments map[string]interface{}, repoID int64) (waitOptions, error) {
	opts := waitOptions{
		repoID:        repoID,
		latest:        getBool(arguments, "latest", false),
		commit:        strings.TrimSpace(getString(arguments, "commit", "")),
		timeout:       time.Duration(getNumber(arguments, "timeout_seconds", defaultWaitTimeout.Seconds())) * time.Second,
		pollInterval:  time.Duration(getNumber(arguments, "poll_interval_seconds", defaultPollInterval.Seconds())) * time.Second,
		logLines:      int(getNumber(arguments, "log_lines", defaultFailureLogLines)),
		waitOnBlocked: getBool(arguments, "wait_on_blocked", false),
	}

	if num, err := requireNumber(arguments, "pipeline_number"); err == nil {
		opts.pipelineNumber = int64(num)
	}

	selectors := 0
	if opts.pipelineNumber > 0 {
		selectors++
	}
	if opts.latest {
		selectors++
	}
	if opts.commit != "" {
		selectors++
	}
	if selectors != 1 {
		return opts, fmt.Errorf("exactly one of pipeline_number, latest=true, or commit must be provided")
	}

	if opts.timeout <= 0 {
		opts.timeout = defaultWaitTimeout
	}
	if opts.timeout > maxWaitTimeout {
		opts.timeout = maxWaitTimeout
	}
	if opts.pollInterval < minPollInterval {
		opts.pollInterval = minPollInterval
	}
	if opts.pollInterval > maxPollInterval {
		opts.pollInterval = maxPollInterval
	}
	if opts.logLines < 0 {
		opts.logLines = 0
	}
	if opts.logLines > maxFailureLogLines {
		opts.logLines = maxFailureLogLines
	}

	return opts, nil
}

// waitForPipeline polls until the selected pipeline reaches a terminal state,
// the timeout elapses, or ctx is cancelled.
func waitForPipeline(ctx context.Context, src pipelineSource, opts waitOptions, progress progressFunc) (*waitResult, error) {
	start := time.Now()
	deadline := start.Add(opts.timeout)
	interval := opts.pollInterval

	sleep := func() error {
		wait := interval
		if remaining := time.Until(deadline); remaining < wait {
			wait = remaining
		}
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		// Back off gradually; long pipelines do not need 15s resolution.
		interval = min(interval+interval/2, maxPollInterval)
		return nil
	}

	pipelineNum := opts.pipelineNumber
	var pipeline *woodpecker.Pipeline
	var err error

	// Resolve the selector to a fixed pipeline number so a newer pipeline
	// cannot replace the one being waited on.
	for pipelineNum == 0 {
		if opts.latest {
			pipeline, err = src.GetLastPipeline(opts.repoID)
			if err != nil {
				return nil, err
			}
			pipelineNum = pipeline.Number
			break
		}

		pipeline, err = findPipelineByCommit(src, opts.repoID, opts.commit)
		if err != nil {
			return nil, err
		}
		if pipeline != nil {
			pipelineNum = pipeline.Number
			break
		}
		if !time.Now().Before(deadline) {
			return &waitResult{
				RepoID:        opts.repoID,
				Status:        "not_found",
				Outcome:       "timeout",
				TimedOut:      true,
				WaitedSeconds: int64(time.Since(start).Seconds()),
				Commit:        opts.commit,
				Message:       fmt.Sprintf("no pipeline for commit %s appeared before the timeout", opts.commit),
			}, nil
		}
		if progress != nil {
			progress(time.Since(start), "waiting for pipeline to be created")
		}
		if err := sleep(); err != nil {
			return nil, err
		}
	}

	for {
		pipeline, err = src.GetPipeline(opts.repoID, pipelineNum)
		if err != nil {
			return nil, err
		}

		if isTerminalStatus(pipeline.Status) {
			return summarizePipeline(src, opts, pipeline, start, false), nil
		}
		if pipeline.Status == woodpecker.StatusBlocked && !opts.waitOnBlocked {
			result := summarizePipeline(src, opts, pipeline, start, false)
			result.Outcome = "blocked"
			return result, nil
		}
		if !time.Now().Before(deadline) {
			return summarizePipeline(src, opts, pipeline, start, true), nil
		}

		if progress != nil {
			progress(time.Since(start), pipeline.Status)
		}
		if err := sleep(); err != nil {
			return nil, err
		}
	}
}

func findPipelineByCommit(src pipelineSource, repoID int64, commit string) (*woodpecker.Pipeline, error) {
	pipelines, err := src.ListPipelines(repoID)
	if err != nil {
		return nil, err
	}
	commit = strings.ToLower(commit)
	for i, p := range pipelines {
		if i >= maxCommitLookupPipelines {
			break
		}
		if strings.HasPrefix(strings.ToLower(p.Commit), commit) {
			return p, nil
		}
	}
	return nil, nil
}

func summarizePipeline(src pipelineSource, opts waitOptions, p *woodpecker.Pipeline, start time.Time, timedOut bool) *waitResult {
	result := &waitResult{
		RepoID:         opts.repoID,
		PipelineNumber: p.Number,
		Status:         p.Status,
		TimedOut:       timedOut,
		WaitedSeconds:  int64(time.Since(start).Seconds()),
		Event:          p.Event,
		Branch:         p.Branch,
		Commit:         p.Commit,
		Message:        firstLine(p.Message),
		ForgeURL:       p.ForgeURL,
	}
	if p.Started > 0 && p.Finished >= p.Started {
		result.DurationSeconds = p.Finished - p.Started
	}
	for _, pe := range p.Errors {
		if pe != nil && !pe.IsWarning {
			result.PipelineErrors = append(result.PipelineErrors, pe.Message)
		}
	}

	switch {
	case timedOut:
		result.Outcome = "timeout"
	case p.Status == woodpecker.StatusSuccess:
		result.Outcome = "success"
	case p.Status == woodpecker.StatusSkipped:
		result.Outcome = "skipped"
	case p.Status == woodpecker.StatusKilled || p.Status == statusDeclined:
		result.Outcome = "cancelled"
	default:
		result.Outcome = "failure"
	}

	// Per-step states are only useful while something is still unresolved.
	if timedOut || p.Status == woodpecker.StatusBlocked {
		result.StepStates = map[string]string{}
		for _, wf := range p.Workflows {
			for _, step := range wf.Children {
				result.StepStates[wf.Name+"/"+step.Name] = step.State
			}
		}
	}

	if result.Outcome == "failure" {
		result.FailedSteps = collectFailedSteps(src, opts, p)
	}

	return result
}

func collectFailedSteps(src pipelineSource, opts waitOptions, p *woodpecker.Pipeline) []failedStep {
	var failed []failedStep
	for _, wf := range p.Workflows {
		stepFailed := false
		for _, step := range wf.Children {
			if !isFailedState(step.State) {
				continue
			}
			stepFailed = true
			fs := failedStep{
				Workflow: wf.Name,
				Name:     step.Name,
				StepID:   step.ID,
				State:    step.State,
				ExitCode: step.ExitCode,
				Error:    step.Error,
			}
			switch {
			case opts.logLines == 0:
			case len(failed) >= maxFailedStepsWithLogs:
				fs.LogNote = "logs omitted; use get_logs with this step_id"
			default:
				fs.Logs, fs.LogNote = tailStepLogs(src, opts.repoID, p.Number, step.ID, opts.logLines)
			}
			failed = append(failed, fs)
		}
		// A workflow can fail before any step runs (e.g. agent or config errors).
		if !stepFailed && isFailedState(wf.State) {
			failed = append(failed, failedStep{
				Workflow: wf.Name,
				State:    wf.State,
				Error:    wf.Error,
			})
		}
	}
	return failed
}

func tailStepLogs(src pipelineSource, repoID, pipelineNum, stepID int64, lines int) ([]string, string) {
	entries, err := src.GetStepLogs(repoID, pipelineNum, stepID)
	if err != nil {
		return nil, fmt.Sprintf("failed to fetch logs: %v", err)
	}
	var out []string
	for _, entry := range entries {
		if len(entry.Data) > 0 {
			out = append(out, strings.TrimRight(string(entry.Data), "\n"))
		}
	}
	if len(out) > lines {
		note := fmt.Sprintf("last %d of %d lines", lines, len(out))
		return out[len(out)-lines:], note
	}
	return out, ""
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (tm *ToolManager) handleWaitForPipeline(ctx context.Context, request mcp.CallToolRequest, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if cancelled := checkContextCancelled(ctx); cancelled != nil {
		return cancelled, nil
	}

	repoID, err := getRepoID(tm.client, arguments)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	opts, err := parseWaitOptions(arguments, repoID)
	if err != nil {
		return tm.errorResult(err.Error()), nil
	}

	select {
	case waitSlots <- struct{}{}:
		defer func() { <-waitSlots }()
	default:
		return tm.errorResult(fmt.Sprintf("too many concurrent waits (max %d); retry later", maxConcurrentWaits)), nil
	}

	result, err := waitForPipeline(ctx, tm.client, opts, tm.progressReporter(ctx, request, opts.timeout))
	if err != nil {
		return tm.errorResult(fmt.Sprintf("Failed while waiting for pipeline: %v", err)), nil
	}

	return tm.jsonResult(result)
}

// progressReporter sends MCP progress notifications when the client asked
// for them, which lets clients that reset timeouts on progress keep waiting.
func (tm *ToolManager) progressReporter(ctx context.Context, request mcp.CallToolRequest, total time.Duration) progressFunc {
	if request.Params.Meta == nil || request.Params.Meta.ProgressToken == nil {
		return nil
	}
	srv := server.ServerFromContext(ctx)
	if srv == nil {
		return nil
	}
	token := request.Params.Meta.ProgressToken
	return func(elapsed time.Duration, status string) {
		err := srv.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
			"progressToken": token,
			"progress":      elapsed.Seconds(),
			"total":         total.Seconds(),
			"message":       fmt.Sprintf("pipeline %s after %s", status, elapsed.Round(time.Second)),
		})
		if err != nil {
			tm.logger.WithError(err).Debug("Failed to send progress notification")
		}
	}
}
