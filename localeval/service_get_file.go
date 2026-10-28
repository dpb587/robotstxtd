package localeval

import (
	"context"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"google.golang.org/grpc"
)

func (s *Service) GetFile(ctx context.Context, req *robotstxtdpb.Crawler_GetFile_Request, opts ...grpc.CallOption) (*robotstxtdpb.File, error) {
	if cached, ok := s.cachedFile(req.ServiceUri); ok {
		return cached, nil
	}

	v, err, _ := s.getFileGroup.Do(req.ServiceUri, func() (any, error) {
		if cached, ok := s.cachedFile(req.ServiceUri); ok {
			return cached, nil
		}

		res, err := s.upstream.GetFile(ctx, &robotstxtdpb.Crawler_GetFile_Request{
			ServiceUri: req.ServiceUri,
		}, opts...)
		if err != nil {
			return nil, err
		}

		s.cacheGetFileMutex.Lock()
		s.cacheGetFileByService[req.ServiceUri] = res
		s.cacheGetFileMutex.Unlock()

		return res, nil
	})
	if err != nil {
		return nil, err
	}

	return v.(*robotstxtdpb.File), nil
}

func (s *Service) cachedFile(serviceURI string) (*robotstxtdpb.File, bool) {
	s.cacheGetFileMutex.RLock()
	defer s.cacheGetFileMutex.RUnlock()

	cached, ok := s.cacheGetFileByService[serviceURI]

	return cached, ok
}
