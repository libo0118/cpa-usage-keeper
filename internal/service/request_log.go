package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
)

const (
	requestLogMaxBytes = int(cpa.RequestLogPreviewMaxBytes)
)

var (
	ErrRequestLogUnavailable = errors.New("request log unavailable")
	ErrRequestLogMissingID   = errors.New("usage event request id missing")
)

type RequestLogClient interface {
	FetchRequestLogByID(ctx context.Context, requestID string) (*cpa.RequestLogResult, error)
	OpenRequestLogByID(ctx context.Context, requestID string) (*cpa.RequestLogStream, error)
}

type RequestLogProvider interface {
	GetRequestDiagnostic(ctx context.Context, requestID string) (RequestLogResponse, error)
	GetUsageEventRequestLog(ctx context.Context, eventID int64) (RequestLogResponse, error)
	DownloadUsageEventRequestLog(ctx context.Context, eventID int64) (RequestLogDownload, error)
}

// GetRequestDiagnostic also works for interrupted requests without a usage row.
func (s *requestLogService) GetRequestDiagnostic(ctx context.Context, requestID string) (RequestLogResponse, error) {
	if s == nil || s.client == nil {
		return RequestLogResponse{}, fmt.Errorf("request log client is not configured")
	}
	inflight, leader := s.beginFetch(requestID)
	if leader {
		go s.fetchRequestLog(requestID, inflight)
	}
	response, err := s.waitForRequestLogFetch(ctx, 0, requestID, inflight)
	response.Downloadable = false
	return response, err
}

type RequestLogResponse struct {
	EventID      int64
	RequestID    string
	Filename     string
	Available    bool
	Previewable  bool
	TooLarge     bool
	Downloadable bool
	Sections     []RequestLogSection
}

type RequestLogDownload struct {
	EventID       int64
	RequestID     string
	Filename      string
	ContentType   string
	ContentLength int64
	Body          io.ReadCloser
	Downloadable  bool
}

type RequestLogSection struct {
	Title   string
	Content string
}

type requestLogService struct {
	db       *gorm.DB
	client   RequestLogClient
	mu       sync.Mutex
	inflight map[string]*requestLogInflight
}

type requestLogInflight struct {
	done     chan struct{}
	fetchCtx context.Context
	cancel   context.CancelFunc
	waiters  int
	response RequestLogResponse
	err      error
}

func NewRequestLogService(db *gorm.DB, client RequestLogClient) RequestLogProvider {
	return &requestLogService{
		db:       db,
		client:   client,
		inflight: map[string]*requestLogInflight{},
	}
}

func (s *requestLogService) GetUsageEventRequestLog(ctx context.Context, eventID int64) (RequestLogResponse, error) {
	if s == nil {
		return RequestLogResponse{}, fmt.Errorf("request log service is nil")
	}
	if s.db == nil {
		return RequestLogResponse{}, fmt.Errorf("database is nil")
	}
	if s.client == nil {
		return RequestLogResponse{}, fmt.Errorf("request log client is not configured")
	}
	event, err := repository.FindUsageEventDiagnosticByID(s.db.WithContext(ctx), eventID)
	if err != nil {
		return RequestLogResponse{}, err
	}
	requestID := strings.TrimSpace(event.RequestID)
	if requestID == "" {
		return RequestLogResponse{EventID: eventID, Available: false}, ErrRequestLogMissingID
	}

	inflight, leader := s.beginFetch(requestID)
	if leader {
		go s.fetchRequestLog(requestID, inflight)
	}
	response, err := s.waitForRequestLogFetch(ctx, eventID, requestID, inflight)
	if errors.Is(err, ErrRequestLogUnavailable) {
		return missingRequestDiagnostic(event), nil
	}
	return response, err
}

func (s *requestLogService) fetchRequestLog(requestID string, inflight *requestLogInflight) {
	result, err := s.client.FetchRequestLogByID(inflight.fetchCtx, requestID)
	if err != nil {
		if result != nil && result.StatusCode == http.StatusNotFound {
			response := RequestLogResponse{RequestID: requestID, Available: false}
			s.finishFetch(requestID, inflight, response, ErrRequestLogUnavailable)
			return
		}
		s.finishFetch(requestID, inflight, RequestLogResponse{}, err)
		return
	}
	if result == nil {
		err := fmt.Errorf("request log result is nil")
		s.finishFetch(requestID, inflight, RequestLogResponse{}, err)
		return
	}
	response := RequestLogResponse{
		RequestID:    requestID,
		Filename:     "request-diagnostic-" + requestID + ".log",
		Available:    true,
		Previewable:  true,
		Downloadable: true,
		Sections:     diagnosticSections(result.Filename, result.Body, result.BodyTruncated || len(result.Body) > requestLogMaxBytes),
	}
	s.finishFetch(requestID, inflight, response, nil)
}

func (s *requestLogService) waitForRequestLogFetch(ctx context.Context, eventID int64, requestID string, inflight *requestLogInflight) (RequestLogResponse, error) {
	select {
	case <-inflight.done:
		if err := ctx.Err(); err != nil {
			return RequestLogResponse{}, err
		}
	case <-ctx.Done():
		s.releaseFetchWaiter(requestID, inflight)
		return RequestLogResponse{}, ctx.Err()
	}
	response := inflight.response
	response.EventID = eventID
	return response, inflight.err
}

func (s *requestLogService) DownloadUsageEventRequestLog(ctx context.Context, eventID int64) (RequestLogDownload, error) {
	result, err := s.GetUsageEventRequestLog(ctx, eventID)
	if err != nil {
		return RequestLogDownload{}, err
	}
	var content strings.Builder
	for _, section := range result.Sections {
		fmt.Fprintf(&content, "=== %s ===\n%s\n\n", section.Title, section.Content)
	}
	return RequestLogDownload{
		EventID:       eventID,
		RequestID:     result.RequestID,
		Filename:      result.Filename,
		ContentType:   "text/plain; charset=utf-8",
		ContentLength: int64(content.Len()),
		Body:          io.NopCloser(strings.NewReader(content.String())),
		Downloadable:  true,
	}, nil
}

func (s *requestLogService) beginFetch(requestID string) (*requestLogInflight, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight == nil {
		s.inflight = map[string]*requestLogInflight{}
	}
	if inflight, ok := s.inflight[requestID]; ok {
		inflight.waiters++
		return inflight, false
	}
	fetchCtx, cancel := context.WithCancel(context.Background())
	inflight := &requestLogInflight{
		done:     make(chan struct{}),
		fetchCtx: fetchCtx,
		cancel:   cancel,
		waiters:  1,
	}
	s.inflight[requestID] = inflight
	return inflight, true
}

func (s *requestLogService) releaseFetchWaiter(requestID string, inflight *requestLogInflight) {
	s.mu.Lock()
	if s.inflight[requestID] != inflight || inflight.waiters <= 0 {
		s.mu.Unlock()
		return
	}
	inflight.waiters--
	if inflight.waiters > 0 {
		s.mu.Unlock()
		return
	}
	delete(s.inflight, requestID)
	cancel := inflight.cancel
	s.mu.Unlock()
	cancel()
}

func (s *requestLogService) finishFetch(requestID string, inflight *requestLogInflight, response RequestLogResponse, err error) {
	s.mu.Lock()
	inflight.response = response
	inflight.err = err
	// The last waiter may have removed this entry and a replacement fetch may already exist.
	if s.inflight[requestID] == inflight {
		delete(s.inflight, requestID)
	}
	s.mu.Unlock()
	inflight.cancel()
	close(inflight.done)
}

func RequestLogPreviewMaxBytes() int {
	return requestLogMaxBytes
}

func ParseRequestLogSections(raw string) []RequestLogSection {
	lines := strings.Split(raw, "\n")
	sections := make([]RequestLogSection, 0, 8)
	currentTitle := ""
	currentLines := []string{}
	flush := func() {
		if currentTitle == "" {
			return
		}
		sections = append(sections, RequestLogSection{
			Title:   currentTitle,
			Content: strings.TrimRight(strings.Join(currentLines, "\n"), "\n"),
		})
		currentLines = []string{}
	}
	for _, line := range lines {
		title, ok := parseRequestLogSectionTitle(line)
		if ok {
			flush()
			currentTitle = title
			continue
		}
		if currentTitle != "" {
			currentLines = append(currentLines, line)
		}
	}
	flush()
	if len(sections) == 0 && strings.TrimSpace(raw) != "" {
		return []RequestLogSection{{Title: "RAW LOG", Content: raw}}
	}
	return sections
}

func parseRequestLogSectionTitle(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "===") || !strings.HasSuffix(trimmed, "===") {
		return "", false
	}
	title := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "==="), "==="))
	if title == "" {
		return "", false
	}
	return title, true
}
