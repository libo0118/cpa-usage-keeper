package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
	"gorm.io/gorm"
)

type requestLogClientStub struct {
	mu     sync.Mutex
	calls  int
	result *cpa.RequestLogResult
	err    error

	downloadCalls  int
	downloadResult *cpa.RequestLogStream
	downloadErr    error
	started        chan struct{}
	block          chan struct{}
	fetchCanceled  chan struct{}
}

func (s *requestLogClientStub) FetchRequestLogByID(ctx context.Context, _ string) (*cpa.RequestLogResult, error) {
	s.mu.Lock()
	s.calls++
	result := s.result
	err := s.err
	started := s.started
	block := s.block
	fetchCanceled := s.fetchCanceled
	s.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			if fetchCanceled != nil {
				select {
				case fetchCanceled <- struct{}{}:
				default:
				}
			}
			return nil, ctx.Err()
		}
	}
	return result, err
}

func (s *requestLogClientStub) fetchCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *requestLogClientStub) OpenRequestLogByID(context.Context, string) (*cpa.RequestLogStream, error) {
	s.mu.Lock()
	s.downloadCalls++
	result := s.result
	err := s.err
	downloadResult := s.downloadResult
	downloadErr := s.downloadErr
	s.mu.Unlock()
	if downloadResult != nil || downloadErr != nil {
		return downloadResult, downloadErr
	}
	if result == nil {
		return nil, err
	}
	return &cpa.RequestLogStream{
		StatusCode:    result.StatusCode,
		Filename:      result.Filename,
		ContentType:   result.ContentType,
		ContentLength: int64(len(result.Body)),
		Body:          io.NopCloser(bytes.NewReader(result.Body)),
	}, err
}

func TestRequestLogServiceLoadsEventLogWithoutCachingByRequestID(t *testing.T) {
	db := requestLogTestDatabase(t, "req-log-1")

	client := &requestLogClientStub{result: &cpa.RequestLogResult{
		StatusCode: http.StatusOK,
		Filename:   "v1-responses-req-log-1.log",
		Body:       []byte("=== REQUEST INFO ===\nURL: /v1/responses\n=== API RESPONSE ===\n{\"ok\":true}\n"),
	}}
	provider := service.NewRequestLogService(db, client)

	first, err := provider.GetUsageEventRequestLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUsageEventRequestLog returned error: %v", err)
	}
	if first.RequestID != "req-log-1" || first.Filename != "request-diagnostic-req-log-1.log" {
		t.Fatalf("unexpected first response: %+v", first)
	}
	if len(first.Sections) != 2 || first.Sections[0].Title != "PRIVACY" || first.Sections[1].Title != "REQUEST DIAGNOSTICS" {
		t.Fatalf("unexpected sections: %+v", first.Sections)
	}

	_, err = provider.GetUsageEventRequestLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("second GetUsageEventRequestLog returned error: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("expected repeated CPA calls without cache, got %d", client.calls)
	}
}

func TestRequestLogServiceMapsCPANotFoundToUsageSummary(t *testing.T) {
	db := requestLogTestDatabase(t, "req-missing")

	client := &requestLogClientStub{
		result: &cpa.RequestLogResult{StatusCode: http.StatusNotFound, Body: []byte(`{"error":"missing"}`)},
		err:    errors.New("management request log request returned status 404"),
	}
	provider := service.NewRequestLogService(db, client)

	response, err := provider.GetUsageEventRequestLog(context.Background(), 1)
	if err != nil || !response.Available || response.Sections[0].Title != "CAPTURE UNAVAILABLE" || !strings.Contains(response.Sections[0].Content, `"stream_status": "unknown"`) {
		t.Fatalf("expected an explicitly limited usage summary, got %+v, %v", response, err)
	}

	_, err = provider.GetUsageEventRequestLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected second limited usage summary, got %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("expected repeated 404 responses to refetch without cache, got %d calls", client.calls)
	}
}

func TestRequestLogServiceCoalescesConcurrentPreviewMisses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := requestLogTestDatabase(t, "req-singleflight", "req-singleflight")

		client := &requestLogClientStub{
			result:  &cpa.RequestLogResult{StatusCode: http.StatusOK, Body: []byte("=== RAW LOG ===\ncoalesced\n")},
			started: make(chan struct{}, 2),
			block:   make(chan struct{}),
		}
		provider := service.NewRequestLogService(db, client)
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, eventID := range []int64{1, 2} {
			wg.Add(1)
			go func(eventID int64) {
				defer wg.Done()
				response, err := provider.GetUsageEventRequestLog(context.Background(), eventID)
				if err == nil && !strings.Contains(response.Sections[0].Content, "bodies are hidden") {
					err = errors.New("unexpected response content")
				}
				if err == nil && response.EventID != eventID {
					err = fmt.Errorf("expected event id %d, got %d", eventID, response.EventID)
				}
				errs <- err
			}(eventID)
		}
		<-client.started
		synctest.Wait()
		close(client.block)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent GetUsageEventRequestLog returned error: %v", err)
			}
		}
		if client.fetchCalls() != 1 {
			t.Fatalf("expected concurrent miss to fetch once, got %d calls", client.fetchCalls())
		}
	})
}

func TestRequestLogServiceCancelsPreviewFetchWhenOnlyWaiterCancels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := requestLogTestDatabase(t, "req-singleflight-only-waiter-cancel")

		client := &requestLogClientStub{
			result:        &cpa.RequestLogResult{StatusCode: http.StatusOK, Body: []byte("=== RAW LOG ===\nunused\n")},
			started:       make(chan struct{}, 1),
			block:         make(chan struct{}),
			fetchCanceled: make(chan struct{}, 1),
		}
		provider := service.NewRequestLogService(db, client)
		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			_, err := provider.GetUsageEventRequestLog(ctx, 1)
			errCh <- err
		}()
		<-client.started
		cancel()

		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected canceled waiter to return context.Canceled, got %v", err)
			}
		case <-time.After(time.Second):
			close(client.block)
			<-errCh
			t.Fatal("expected canceled waiter to return promptly")
		}

		select {
		case <-client.fetchCanceled:
		case <-time.After(time.Second):
			close(client.block)
			t.Fatal("expected the CPA fetch to be canceled after the last waiter left")
		}

		client.mu.Lock()
		client.block = nil
		client.mu.Unlock()
		response, err := provider.GetUsageEventRequestLog(context.Background(), 1)
		if err != nil {
			t.Fatalf("expected a fresh fetch after cancellation, got %v", err)
		}
		if len(response.Sections) != 2 || !strings.Contains(response.Sections[0].Content, "bodies are hidden") {
			t.Fatalf("unexpected retry response: %+v", response)
		}
		if client.fetchCalls() != 2 {
			t.Fatalf("expected cancellation cleanup to allow a new fetch, got %d calls", client.fetchCalls())
		}
	})
}

func TestRequestLogServiceHidesRawPreviewAtSixMiBLimit(t *testing.T) {
	const sixMiB = 6 * 1024 * 1024
	db := requestLogTestDatabase(t, "req-six-mib")

	client := &requestLogClientStub{result: &cpa.RequestLogResult{
		StatusCode: http.StatusOK,
		Body:       bytes.Repeat([]byte("x"), sixMiB),
	}}
	provider := service.NewRequestLogService(db, client)

	response, err := provider.GetUsageEventRequestLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUsageEventRequestLog returned error: %v", err)
	}
	if got := service.RequestLogPreviewMaxBytes(); got != sixMiB {
		t.Fatalf("expected preview limit %d, got %d", sixMiB, got)
	}
	if response.TooLarge || !response.Previewable || len(response.Sections) != 2 {
		t.Fatalf("expected six MiB response to remain previewable, got %+v", response)
	}
	if !strings.Contains(response.Sections[0].Content, "bodies are hidden") {
		t.Fatal("expected a privacy notice instead of raw preview content")
	}
}

func TestRequestLogServiceHidesOversizedBodyWithoutTruncatedFlag(t *testing.T) {
	const sixMiB = 6 * 1024 * 1024
	db := requestLogTestDatabase(t, "req-six-mib-plus-one")

	client := &requestLogClientStub{result: &cpa.RequestLogResult{
		StatusCode:    http.StatusOK,
		Body:          bytes.Repeat([]byte("x"), sixMiB+1),
		BodyTruncated: false,
	}}
	provider := service.NewRequestLogService(db, client)

	response, err := provider.GetUsageEventRequestLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUsageEventRequestLog returned error: %v", err)
	}
	if response.TooLarge || !response.Previewable || !response.Downloadable || len(response.Sections) != 2 {
		t.Fatalf("expected oversized raw body to be hidden, got %+v", response)
	}
}

func TestRequestLogServiceCoalescedPreviewReturnsLeaderCancellationWithoutAbortingFollower(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := requestLogTestDatabase(t, "req-singleflight-leader-cancel")

		client := &requestLogClientStub{
			result:  &cpa.RequestLogResult{StatusCode: http.StatusOK, Body: []byte("=== RAW LOG ===\nleader survived\n")},
			started: make(chan struct{}, 1),
			block:   make(chan struct{}),
		}
		provider := service.NewRequestLogService(db, client)
		leaderCtx, cancelLeader := context.WithCancel(context.Background())
		leaderErr := make(chan error, 1)
		go func() {
			response, err := provider.GetUsageEventRequestLog(leaderCtx, 1)
			if err == nil && !strings.Contains(response.Sections[0].Content, "bodies are hidden") {
				err = errors.New("unexpected leader response content")
			}
			leaderErr <- err
		}()
		<-client.started

		followerErr := make(chan error, 1)
		go func() {
			response, err := provider.GetUsageEventRequestLog(context.Background(), 1)
			if err == nil && !strings.Contains(response.Sections[0].Content, "bodies are hidden") {
				err = errors.New("unexpected follower response content")
			}
			followerErr <- err
		}()
		synctest.Wait()
		cancelLeader()
		select {
		case err := <-leaderErr:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("expected canceled leader to return context.Canceled, got %v", err)
			}
		case <-time.After(time.Second):
			t.Error("expected canceled leader to return promptly while the shared fetch continues")
		}
		close(client.block)

		if err := <-followerErr; err != nil {
			t.Fatalf("expected follower to receive shared fetch result, got %v", err)
		}
		if client.fetchCalls() != 1 {
			t.Fatalf("expected shared fetch to run once, got %d calls", client.fetchCalls())
		}
	})
}

func TestRequestLogServiceCancelsSharedPreviewAfterLastConcurrentWaiterLeaves(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := requestLogTestDatabase(t, "req-singleflight-all-cancel", "req-singleflight-all-cancel")

		client := &requestLogClientStub{
			result:        &cpa.RequestLogResult{StatusCode: http.StatusOK, Body: []byte("=== RAW LOG ===\nunused\n")},
			started:       make(chan struct{}, 1),
			block:         make(chan struct{}),
			fetchCanceled: make(chan struct{}, 1),
		}
		provider := service.NewRequestLogService(db, client)
		leaderCtx, cancelLeader := context.WithCancel(context.Background())
		followerCtx, cancelFollower := context.WithCancel(context.Background())
		leaderErr := make(chan error, 1)
		followerErr := make(chan error, 1)
		go func() {
			_, err := provider.GetUsageEventRequestLog(leaderCtx, 1)
			leaderErr <- err
		}()
		<-client.started
		go func() {
			_, err := provider.GetUsageEventRequestLog(followerCtx, 2)
			followerErr <- err
		}()
		synctest.Wait()

		cancelLeader()
		if err := <-leaderErr; !errors.Is(err, context.Canceled) {
			t.Fatalf("expected leader cancellation, got %v", err)
		}
		synctest.Wait()
		select {
		case <-client.fetchCanceled:
			t.Fatal("expected shared fetch to remain active while the follower is waiting")
		default:
		}

		cancelFollower()
		if err := <-followerErr; !errors.Is(err, context.Canceled) {
			t.Fatalf("expected follower cancellation, got %v", err)
		}
		select {
		case <-client.fetchCanceled:
		case <-time.After(time.Second):
			close(client.block)
			t.Fatal("expected the last waiter to cancel the shared fetch")
		}
		if client.fetchCalls() != 1 {
			t.Fatalf("expected concurrent waiters to share one fetch, got %d calls", client.fetchCalls())
		}
	})
}

func TestRequestLogServiceCoalescedPreviewFollowerCanCancelIndependently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := requestLogTestDatabase(t, "req-singleflight-follower-cancel")

		client := &requestLogClientStub{
			result:  &cpa.RequestLogResult{StatusCode: http.StatusOK, Body: []byte("=== RAW LOG ===\nfollower survived\n")},
			started: make(chan struct{}, 1),
			block:   make(chan struct{}),
		}
		provider := service.NewRequestLogService(db, client)
		leaderErr := make(chan error, 1)
		go func() {
			_, err := provider.GetUsageEventRequestLog(context.Background(), 1)
			leaderErr <- err
		}()
		<-client.started

		followerCtx, cancelFollower := context.WithCancel(context.Background())
		followerErr := make(chan error, 1)
		go func() {
			_, err := provider.GetUsageEventRequestLog(followerCtx, 1)
			followerErr <- err
		}()
		cancelFollower()
		if err := <-followerErr; !errors.Is(err, context.Canceled) {
			t.Fatalf("expected follower to stop waiting on context cancellation, got %v", err)
		}

		close(client.block)
		if err := <-leaderErr; err != nil {
			t.Fatalf("expected leader fetch to complete after follower cancellation, got %v", err)
		}
		if client.fetchCalls() != 1 {
			t.Fatalf("expected shared fetch to run once, got %d calls", client.fetchCalls())
		}
	})
}

func TestRequestLogServiceHandlesLargePreviewAsDownloadable(t *testing.T) {
	db := requestLogTestDatabase(t, "req-large")

	client := &requestLogClientStub{result: &cpa.RequestLogResult{
		StatusCode:    http.StatusOK,
		Filename:      "large-request.log",
		Body:          make([]byte, service.RequestLogPreviewMaxBytes()+1),
		BodyTruncated: true,
		ContentType:   "text/plain",
		ContentLength: int64(service.RequestLogPreviewMaxBytes() + 1),
	}}
	provider := service.NewRequestLogService(db, client)

	response, err := provider.GetUsageEventRequestLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUsageEventRequestLog returned error: %v", err)
	}
	if response.TooLarge || !response.Downloadable || !response.Previewable || len(response.Sections) != 2 {
		t.Fatalf("unexpected large preview response: %+v", response)
	}
	if response.Filename != "request-diagnostic-req-large.log" {
		t.Fatalf("unexpected filename %q", response.Filename)
	}
}

func TestRequestLogServiceTooLargePreviewRefetchesWithoutCache(t *testing.T) {
	db := requestLogTestDatabase(t, "req-large-expire")

	client := &requestLogClientStub{result: &cpa.RequestLogResult{
		StatusCode:    http.StatusOK,
		Filename:      "large-request.log",
		BodyTruncated: true,
		ContentType:   "text/plain",
		ContentLength: int64(service.RequestLogPreviewMaxBytes() + 1),
	}}
	provider := service.NewRequestLogService(db, client)

	response, err := provider.GetUsageEventRequestLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUsageEventRequestLog returned error: %v", err)
	}
	if response.TooLarge || !response.Previewable || len(response.Sections) != 2 {
		t.Fatalf("expected redacted metadata notice, got %+v", response)
	}

	client.mu.Lock()
	client.result = &cpa.RequestLogResult{StatusCode: http.StatusOK, Body: []byte("=== RAW LOG ===\nsmall again\n")}
	client.mu.Unlock()
	response, err = provider.GetUsageEventRequestLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected too-large response to refetch successfully, got %v", err)
	}
	if response.TooLarge || len(response.Sections) != 2 || !strings.Contains(response.Sections[0].Content, "bodies are hidden") {
		t.Fatalf("expected refetched preview response, got %+v", response)
	}
	if client.fetchCalls() != 2 {
		t.Fatalf("expected too-large response to refetch without cache, got %d calls", client.fetchCalls())
	}
}

func TestRequestLogServiceDownloadUsesRedactedPreview(t *testing.T) {
	db := requestLogTestDatabase(t, "req-download")

	client := &requestLogClientStub{result: &cpa.RequestLogResult{
		StatusCode:    http.StatusOK,
		Filename:      "download.log",
		ContentType:   "text/plain; charset=utf-8",
		ContentLength: 7,
		Body:          []byte("raw log with secret-prompt"),
	}}
	provider := service.NewRequestLogService(db, client)
	downloader, ok := provider.(interface {
		DownloadUsageEventRequestLog(context.Context, int64) (service.RequestLogDownload, error)
	})
	if !ok {
		t.Fatalf("request log provider does not support downloads")
	}

	download, err := downloader.DownloadUsageEventRequestLog(context.Background(), 1)
	if err != nil {
		t.Fatalf("DownloadUsageEventRequestLog returned error: %v", err)
	}
	body, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatalf("read download body: %v", err)
	}
	_ = download.Body.Close()
	if strings.Contains(string(body), "secret-prompt") || !strings.Contains(string(body), "bodies are hidden") || download.Filename != "request-diagnostic-req-download.log" || download.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected download response: %+v", download)
	}
	if client.downloadCalls != 0 || client.fetchCalls() != 1 {
		t.Fatalf("download must use the redacted preview path, raw calls=%d", client.downloadCalls)
	}
}

func TestRequestLogServiceDirectLookupDoesNotRequireUsageRow(t *testing.T) {
	client := &requestLogClientStub{result: &cpa.RequestLogResult{
		StatusCode: http.StatusOK, Filename: "diagnostic-responses-dd5e5a0f.log",
		Body: []byte("=== REQUEST DIAGNOSTICS ===\n" + `{"diagnostic_version":1,"body_capture":"omitted","status":"incomplete","status_code":200}`),
	}}
	provider := service.NewRequestLogService(nil, client)
	response, err := provider.GetRequestDiagnostic(context.Background(), "dd5e5a0f")
	if err != nil {
		t.Fatal(err)
	}
	if response.EventID != 0 || response.Downloadable || !response.Previewable || !strings.Contains(response.Sections[0].Content, "incomplete") {
		t.Fatalf("unexpected direct diagnostic: %+v", response)
	}
}

func requestLogTestDatabase(t *testing.T, requestIDs ...string) *gorm.DB {
	t.Helper()
	db := openUsageServiceTestDatabase(t)
	var events []entities.UsageEvent
	for index, requestID := range requestIDs {
		events = append(events, entities.UsageEvent{EventKey: fmt.Sprintf("event-%d", index), RequestID: requestID})
	}
	if _, _, err := repository.InsertUsageEvents(db, events); err != nil {
		t.Fatalf("insert usage events: %v", err)
	}
	return db
}
