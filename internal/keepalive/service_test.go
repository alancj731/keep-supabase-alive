package keepalive

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testURL = "postgresql://postgres.abc:s3cr3t-password@db.example.com:5432/postgres"

// stubConnector stands in for a database so retries, redaction and concurrency
// can be tested without one.
type stubConnector struct {
	rows  int
	err   error
	calls atomic.Int32
	// failuresBeforeSuccess makes the first N attempts fail.
	failuresBeforeSuccess int32
	before                func()
}

func (s *stubConnector) Ping(ctx context.Context, project Project) (int, error) {
	call := s.calls.Add(1)
	if s.before != nil {
		s.before()
	}
	if s.err != nil && call > s.failuresBeforeSuccess {
		return 0, s.err
	}
	if call <= s.failuresBeforeSuccess {
		return 0, errors.New("connection refused")
	}
	return s.rows, nil
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

func testService(t *testing.T, connector Connector, urls []string) *Service {
	t.Helper()
	projects, err := BuildProjects(urls, []string{"public.users"})
	if err != nil {
		t.Fatalf("BuildProjects: %v", err)
	}
	return NewService(projects, connector, Options{RetryAttempts: 3, RetryBackoff: 0}, quietLogger())
}

func TestRunAllSucceeds(t *testing.T) {
	service := testService(t, &stubConnector{rows: 1}, []string{testURL})

	summary, err := service.RunAll(context.Background(), "test")
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if summary.Succeeded != 1 || summary.Failed != 0 {
		t.Errorf("summary = %d ok / %d failed", summary.Succeeded, summary.Failed)
	}
	result := summary.Results[0]
	if !result.Success || result.Attempts != 1 {
		t.Errorf("result = %+v", result)
	}
	if result.RowsSeen == nil || *result.RowsSeen != 1 {
		t.Errorf("rowsSeen = %v, want 1", result.RowsSeen)
	}
	if result.Error != nil {
		t.Errorf("error = %v, want nil", *result.Error)
	}
	if service.LastRun() == nil || len(service.LastResults()) != 1 {
		t.Error("state should be recorded after a run")
	}
}

func TestRunAllSucceedsOnEmptyTable(t *testing.T) {
	service := testService(t, &stubConnector{rows: 0}, []string{testURL})

	summary, err := service.RunAll(context.Background(), "test")
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if got := summary.Results[0]; !got.Success || *got.RowsSeen != 0 {
		t.Errorf("an empty table is still a success, got %+v", got)
	}
}

func TestRunAllRetriesThenRecovers(t *testing.T) {
	connector := &stubConnector{rows: 1, failuresBeforeSuccess: 1}
	service := testService(t, connector, []string{testURL})

	summary, err := service.RunAll(context.Background(), "test")
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if !summary.Results[0].Success || summary.Results[0].Attempts != 2 {
		t.Errorf("result = %+v, want success on attempt 2", summary.Results[0])
	}
	if connector.calls.Load() != 2 {
		t.Errorf("connector called %d times, want 2", connector.calls.Load())
	}
}

func TestRunAllRecordsFailureAfterExhaustingRetries(t *testing.T) {
	service := testService(t,
		&stubConnector{err: errors.New("password authentication failed (SQLSTATE 28P01)")},
		[]string{testURL})

	summary, err := service.RunAll(context.Background(), "test")
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if summary.Failed != 1 {
		t.Fatalf("summary = %+v, want 1 failure", summary)
	}
	result := summary.Results[0]
	if result.Success || result.Attempts != 3 || result.RowsSeen != nil {
		t.Errorf("result = %+v", result)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "28P01") {
		t.Errorf("error = %v, want the SQLSTATE preserved", result.Error)
	}
}

func TestRunAllKeepsPasswordOutOfRecordedError(t *testing.T) {
	service := testService(t,
		&stubConnector{err: errors.New("FATAL: password s3cr3t-password rejected")},
		[]string{testURL})

	summary, _ := service.RunAll(context.Background(), "test")
	message := *summary.Results[0].Error
	if strings.Contains(message, "s3cr3t-password") {
		t.Errorf("error %q leaks the password", message)
	}
	if !strings.Contains(message, "***") {
		t.Errorf("error %q should show the redaction marker", message)
	}
}

func TestRunAllRejectsConcurrentRun(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var once atomic.Bool
	connector := &stubConnector{rows: 1, before: func() {
		if once.CompareAndSwap(false, true) {
			close(started)
		}
		<-release
	}}
	service := testService(t, connector, []string{testURL})

	go func() { _, _ = service.RunAll(context.Background(), "first") }()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first run never started")
	}
	if !service.IsRunning() {
		t.Error("IsRunning should be true during a run")
	}
	if _, err := service.RunAll(context.Background(), "second"); !errors.Is(err, ErrRunInProgress) {
		t.Errorf("second run error = %v, want ErrRunInProgress", err)
	}
	close(release)
}

// A hosted runtime can suspend an instance mid-run. The frozen run never
// releases the guard, so without a takeover every later request would 409
// forever.
func TestStaleRunIsTakenOver(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var once atomic.Bool
	connector := &stubConnector{rows: 1, before: func() {
		if once.CompareAndSwap(false, true) {
			close(started)
			<-release // only the first call blocks, standing in for a frozen run
		}
	}}
	projects, err := BuildProjects([]string{testURL}, []string{"public.users"})
	if err != nil {
		t.Fatalf("BuildProjects: %v", err)
	}
	service := NewService(projects, connector,
		Options{RetryAttempts: 1, RetryBackoff: 0, StaleRunAfter: time.Millisecond}, quietLogger())

	go func() { _, _ = service.RunAll(context.Background(), "frozen") }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first run never started")
	}
	time.Sleep(10 * time.Millisecond) // let it become stale

	summary, err := service.RunAll(context.Background(), "takeover")
	if err != nil {
		t.Fatalf("the stale run should have been taken over, got %v", err)
	}
	if summary.Succeeded != 1 {
		t.Errorf("summary = %+v", summary)
	}

	// The abandoned run finishing later must not clear the guard it lost.
	if !service.IsRunning() {
		close(release)
		return
	}
	close(release)
}

func TestSupersededRunDoesNotClearTheNewGuard(t *testing.T) {
	projects, err := BuildProjects([]string{testURL}, []string{"public.users"})
	if err != nil {
		t.Fatalf("BuildProjects: %v", err)
	}
	service := NewService(projects, &stubConnector{rows: 1},
		Options{RetryAttempts: 1, StaleRunAfter: time.Millisecond}, quietLogger())

	first, ok := service.beginRun()
	if !ok {
		t.Fatal("first run should have claimed the guard")
	}
	time.Sleep(5 * time.Millisecond)
	second, ok := service.beginRun()
	if !ok {
		t.Fatal("the stale guard should have been taken over")
	}
	if second == first {
		t.Fatal("the replacement run must get a new generation")
	}

	service.endRun(first) // the abandoned run finally finishes
	if !service.IsRunning() {
		t.Error("a superseded run cleared the guard belonging to its replacement")
	}
	service.endRun(second)
	if service.IsRunning() {
		t.Error("the owning run should have cleared the guard")
	}
}

func TestNoResultsBeforeFirstRun(t *testing.T) {
	service := testService(t, &stubConnector{rows: 1}, []string{testURL})

	if len(service.LastResults()) != 0 {
		t.Error("LastResults should be empty before the first run")
	}
	if service.LastRun() != nil {
		t.Error("LastRun should be nil before the first run")
	}
	if service.ProjectCount() != 1 {
		t.Errorf("ProjectCount = %d, want 1", service.ProjectCount())
	}
}

func TestRunAllPingsEveryProjectInOrder(t *testing.T) {
	service := testService(t, &stubConnector{rows: 1},
		[]string{testURL, "postgresql://postgres.def:pw@other.example.com:5432/postgres"})

	summary, err := service.RunAll(context.Background(), "test")
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if summary.Total != 2 || summary.Succeeded != 2 {
		t.Errorf("summary = %+v", summary)
	}
	if summary.Results[0].ProjectID != "p1" || summary.Results[1].ProjectID != "p2" {
		t.Errorf("results out of order: %q, %q", summary.Results[0].ProjectID, summary.Results[1].ProjectID)
	}
}

func TestRedactLeavesShortPasswordsAlone(t *testing.T) {
	if got := redact("password pw rejected", "pw"); got != "password pw rejected" {
		t.Errorf("redact = %q; short passwords must not mangle unrelated words", got)
	}
	if got := redact("password longsecret rejected", "longsecret"); !strings.Contains(got, "***") {
		t.Errorf("redact = %q, want redaction", got)
	}
}
