package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// fakeSource serves a scripted sequence of pipeline states.
type fakeSource struct {
	states   []*woodpecker.Pipeline
	calls    int
	list     []*woodpecker.Pipeline
	logs     map[int64][]*woodpecker.LogEntry
	logCalls int
}

func (f *fakeSource) GetPipeline(_, num int64) (*woodpecker.Pipeline, error) {
	i := min(f.calls, len(f.states)-1)
	f.calls++
	p := *f.states[i]
	p.Number = num
	return &p, nil
}

func (f *fakeSource) GetLastPipeline(int64) (*woodpecker.Pipeline, error) {
	return &woodpecker.Pipeline{Number: 42, Status: woodpecker.StatusRunning}, nil
}

func (f *fakeSource) ListPipelines(int64) ([]*woodpecker.Pipeline, error) {
	return f.list, nil
}

func (f *fakeSource) GetStepLogs(_, _, stepID int64) ([]*woodpecker.LogEntry, error) {
	f.logCalls++
	entries, ok := f.logs[stepID]
	if !ok {
		return nil, errors.New("no logs")
	}
	return entries, nil
}

func logLines(n int) []*woodpecker.LogEntry {
	var out []*woodpecker.LogEntry
	for i := 1; i <= n; i++ {
		out = append(out, &woodpecker.LogEntry{Line: i, Data: []byte(fmt.Sprintf("line %d", i))})
	}
	return out
}

func fastOpts() waitOptions {
	return waitOptions{repoID: 1, pipelineNumber: 7, timeout: time.Second, pollInterval: time.Millisecond, logLines: 3}
}

func TestWaitForPipeline_SuccessOmitsLogs(t *testing.T) {
	src := &fakeSource{states: []*woodpecker.Pipeline{
		{Status: woodpecker.StatusPending},
		{Status: woodpecker.StatusRunning},
		{Status: woodpecker.StatusSuccess, Started: 100, Finished: 160, Workflows: []*woodpecker.Workflow{
			{Name: "build", State: woodpecker.StatusSuccess, Children: []*woodpecker.Step{{ID: 1, Name: "test", State: woodpecker.StatusSuccess}}},
		}},
	}}
	var progressCalls int
	result, err := waitForPipeline(context.Background(), src, fastOpts(), func(time.Duration, string) { progressCalls++ })
	require.NoError(t, err)
	require.Equal(t, "success", result.Outcome)
	require.False(t, result.TimedOut)
	require.EqualValues(t, 7, result.PipelineNumber)
	require.EqualValues(t, 60, result.DurationSeconds)
	require.Empty(t, result.FailedSteps)
	require.Nil(t, result.StepStates)
	require.Zero(t, src.logCalls)
	require.Equal(t, 2, progressCalls)
}

func TestWaitForPipeline_FailureTailsFailedStepLogs(t *testing.T) {
	src := &fakeSource{
		states: []*woodpecker.Pipeline{{Status: woodpecker.StatusFailure, Workflows: []*woodpecker.Workflow{
			{Name: "ci", State: woodpecker.StatusFailure, Children: []*woodpecker.Step{
				{ID: 1, Name: "lint", State: woodpecker.StatusSuccess},
				{ID: 2, Name: "test", State: woodpecker.StatusFailure, ExitCode: 1},
				{ID: 3, Name: "deploy", State: woodpecker.StatusSkipped},
			}},
			{Name: "arm", State: woodpecker.StatusError, Error: "no agent"},
		}}},
		logs: map[int64][]*woodpecker.LogEntry{2: logLines(10)},
	}
	result, err := waitForPipeline(context.Background(), src, fastOpts(), nil)
	require.NoError(t, err)
	require.Equal(t, "failure", result.Outcome)
	require.Len(t, result.FailedSteps, 2)

	step := result.FailedSteps[0]
	require.Equal(t, "test", step.Name)
	require.EqualValues(t, 2, step.StepID)
	require.Equal(t, 1, step.ExitCode)
	require.Equal(t, []string{"line 8", "line 9", "line 10"}, step.Logs)
	require.Equal(t, "last 3 of 10 lines", step.LogNote)

	require.Equal(t, "arm", result.FailedSteps[1].Workflow)
	require.Equal(t, "no agent", result.FailedSteps[1].Error)
	require.Equal(t, 1, src.logCalls)
}

func TestWaitForPipeline_LogFetchErrorIsReportedNotFatal(t *testing.T) {
	src := &fakeSource{states: []*woodpecker.Pipeline{{Status: woodpecker.StatusFailure, Workflows: []*woodpecker.Workflow{
		{Name: "ci", Children: []*woodpecker.Step{{ID: 9, Name: "test", State: woodpecker.StatusFailure}}},
	}}}}
	result, err := waitForPipeline(context.Background(), src, fastOpts(), nil)
	require.NoError(t, err)
	require.Contains(t, result.FailedSteps[0].LogNote, "failed to fetch logs")
}

func TestWaitForPipeline_KilledIsCancelled(t *testing.T) {
	src := &fakeSource{states: []*woodpecker.Pipeline{{Status: woodpecker.StatusKilled}}}
	result, err := waitForPipeline(context.Background(), src, fastOpts(), nil)
	require.NoError(t, err)
	require.Equal(t, "cancelled", result.Outcome)
	require.Empty(t, result.FailedSteps)
}

func TestWaitForPipeline_BlockedReturnsEarly(t *testing.T) {
	src := &fakeSource{states: []*woodpecker.Pipeline{{Status: woodpecker.StatusBlocked}}}
	result, err := waitForPipeline(context.Background(), src, fastOpts(), nil)
	require.NoError(t, err)
	require.Equal(t, "blocked", result.Outcome)
	require.Equal(t, 1, src.calls)
}

func TestWaitForPipeline_WaitOnBlocked(t *testing.T) {
	src := &fakeSource{states: []*woodpecker.Pipeline{
		{Status: woodpecker.StatusBlocked},
		{Status: woodpecker.StatusSuccess},
	}}
	opts := fastOpts()
	opts.waitOnBlocked = true
	result, err := waitForPipeline(context.Background(), src, opts, nil)
	require.NoError(t, err)
	require.Equal(t, "success", result.Outcome)
}

func TestWaitForPipeline_TimeoutIsNotAnError(t *testing.T) {
	src := &fakeSource{states: []*woodpecker.Pipeline{{Status: woodpecker.StatusRunning, Workflows: []*woodpecker.Workflow{
		{Name: "ci", Children: []*woodpecker.Step{{Name: "test", State: woodpecker.StatusRunning}}},
	}}}}
	opts := fastOpts()
	opts.timeout = 20 * time.Millisecond
	opts.pollInterval = 5 * time.Millisecond
	result, err := waitForPipeline(context.Background(), src, opts, nil)
	require.NoError(t, err)
	require.True(t, result.TimedOut)
	require.Equal(t, "timeout", result.Outcome)
	require.Equal(t, map[string]string{"ci/test": woodpecker.StatusRunning}, result.StepStates)
}

func TestWaitForPipeline_ContextCancelled(t *testing.T) {
	src := &fakeSource{states: []*woodpecker.Pipeline{{Status: woodpecker.StatusRunning}}}
	ctx, cancel := context.WithCancel(context.Background())
	opts := fastOpts()
	opts.timeout = time.Minute
	opts.pollInterval = time.Minute
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()
	_, err := waitForPipeline(ctx, src, opts, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWaitForPipeline_LatestPinsNumber(t *testing.T) {
	src := &fakeSource{states: []*woodpecker.Pipeline{{Status: woodpecker.StatusSuccess}}}
	opts := fastOpts()
	opts.pipelineNumber = 0
	opts.latest = true
	result, err := waitForPipeline(context.Background(), src, opts, nil)
	require.NoError(t, err)
	require.EqualValues(t, 42, result.PipelineNumber)
}

func TestWaitForPipeline_CommitWaitsForPipelineToAppear(t *testing.T) {
	src := &fakeSource{states: []*woodpecker.Pipeline{{Status: woodpecker.StatusSuccess}}}
	opts := fastOpts()
	opts.pipelineNumber = 0
	opts.commit = "ABC123"
	var progressCalls int
	result, err := waitForPipeline(context.Background(), src, opts, func(time.Duration, string) {
		progressCalls++
		src.list = []*woodpecker.Pipeline{{Number: 5, Commit: "def"}, {Number: 6, Commit: "abc1234567"}}
	})
	require.NoError(t, err)
	require.EqualValues(t, 6, result.PipelineNumber)
	require.Equal(t, 1, progressCalls)
}

func TestWaitForPipeline_CommitNeverAppears(t *testing.T) {
	src := &fakeSource{}
	opts := fastOpts()
	opts.pipelineNumber = 0
	opts.commit = "abc"
	opts.timeout = 10 * time.Millisecond
	result, err := waitForPipeline(context.Background(), src, opts, nil)
	require.NoError(t, err)
	require.True(t, result.TimedOut)
	require.Equal(t, "not_found", result.Status)
}

func TestParseWaitOptions(t *testing.T) {
	opts, err := parseWaitOptions(map[string]interface{}{"pipeline_number": float64(3)}, 1)
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, opts.timeout)
	require.Equal(t, 15*time.Second, opts.pollInterval)
	require.Equal(t, 100, opts.logLines)

	opts, err = parseWaitOptions(map[string]interface{}{
		"latest": true, "timeout_seconds": float64(1e9), "poll_interval_seconds": float64(1), "log_lines": float64(1e6),
	}, 1)
	require.NoError(t, err)
	require.Equal(t, 6*time.Hour, opts.timeout)
	require.Equal(t, 5*time.Second, opts.pollInterval)
	require.Equal(t, 1000, opts.logLines)

	_, err = parseWaitOptions(map[string]interface{}{}, 1)
	require.Error(t, err)
	_, err = parseWaitOptions(map[string]interface{}{"latest": true, "commit": "abc"}, 1)
	require.Error(t, err)
}
