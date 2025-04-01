package localeval

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"

	"github.com/dpb587/robotstxtd/internal"
	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/dpb587/robotstxtd/robotstxtdutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	log      *slog.Logger
	upstream robotstxtdpb.CrawlerClient

	cacheGetBot      *robotstxtdpb.UserAgent
	cacheGetBotMutex sync.Mutex

	cacheGetFileMutex     sync.RWMutex
	cacheGetFileByService map[string]*robotstxtdpb.File

	queueGetFileUpstream chan queueGetFileUpstreamRequest
}

var _ robotstxtdpb.CrawlerClient = &Service{}

func NewService(log *slog.Logger, upstream robotstxtdpb.CrawlerClient) *Service {
	s := &Service{
		log:                   log,
		upstream:              upstream,
		cacheGetFileByService: map[string]*robotstxtdpb.File{},
		queueGetFileUpstream:  make(chan queueGetFileUpstreamRequest, 64),
	}

	go func() {
		s.log.Info("thread (GetFileUpstream) started")
		defer s.log.Info("thread (GetFileUpstream) done")

		s.queueGetFileUpstreamWorker()
	}()

	return s
}

func (s *Service) Close() error {
	close(s.queueGetFileUpstream)

	return nil
}

func (s *Service) GetBot(ctx context.Context, req *robotstxtdpb.Crawler_GetBot_Request, opts ...grpc.CallOption) (*robotstxtdpb.UserAgent, error) {
	s.cacheGetBotMutex.Lock()
	if s.cacheGetBot == nil {
		res, err := s.upstream.GetBot(ctx, req, opts...)
		if err != nil {
			s.cacheGetBotMutex.Unlock()

			return nil, err
		}

		s.cacheGetBot = res
	}

	s.cacheGetBotMutex.Unlock()

	return s.cacheGetBot, nil
}

func (s *Service) InvalidateFileCache(ctx context.Context, req *robotstxtdpb.Crawler_InvalidateFileCache_Request, opts ...grpc.CallOption) (*robotstxtdpb.Crawler_InvalidateFileCache_Response, error) {
	delete(s.cacheGetFileByService, req.ServiceUri)

	return s.upstream.InvalidateFileCache(ctx, req, opts...)
}

func (s *Service) EvaluateUri(ctx context.Context, req *robotstxtdpb.Crawler_EvaluateUri_Request, opts ...grpc.CallOption) (*robotstxtdpb.UriEvaluation, error) {
	bot, err := s.GetBot(ctx, &robotstxtdpb.Crawler_GetBot_Request{})
	if err != nil {
		return nil, status.Error(codes.Unavailable, fmt.Sprintf("get user agent: %v", err))
	}

	parsedURL, err := url.Parse(req.Uri)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "url could not be parsed")
	}

	res, err := s.GetFile(ctx, &robotstxtdpb.Crawler_GetFile_Request{
		ServiceUri: internal.Origin(parsedURL),
	})
	if err != nil {
		return nil, err
	}

	return robotstxtdutil.EvaluateUri(res, bot.Identifier, req.Uri), nil
}
