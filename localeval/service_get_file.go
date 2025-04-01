package localeval

import (
	"context"
	"time"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"google.golang.org/grpc"
)

func (s *Service) GetFile(ctx context.Context, req *robotstxtdpb.Crawler_GetFile_Request, opts ...grpc.CallOption) (*robotstxtdpb.File, error) {
	s.cacheGetFileMutex.RLock()
	cached, known := s.cacheGetFileByService[req.ServiceUri]
	s.cacheGetFileMutex.RUnlock()

	if known {
		return cached, nil
	}

	rc := make(chan queueGetFileUpstreamResult, 1)

	s.queueGetFileUpstream <- queueGetFileUpstreamRequest{
		args: queueGetFileUpstreamRequestArgs{
			ServiceUri: req.ServiceUri,
		},
		result: rc,
	}

	result := <-rc

	return result.res, result.err
}

//

type queueGetFileUpstreamRequestArgs struct {
	ServiceUri string
}

type queueGetFileUpstreamRequest struct {
	args   queueGetFileUpstreamRequestArgs
	result chan<- queueGetFileUpstreamResult
}

type queueGetFileUpstreamResult struct {
	res *robotstxtdpb.File
	err error
}

type queueGetFileUpstreamExecution struct {
	res    queueGetFileUpstreamResult
	waiter chan struct{}
}

func (m *queueGetFileUpstreamExecution) wait(out chan<- queueGetFileUpstreamResult) {
	<-m.waiter
	out <- m.res
}

func (s *Service) queueGetFileUpstreamWorker() {
	memo := make(map[queueGetFileUpstreamRequestArgs]*queueGetFileUpstreamExecution)
	prune := time.NewTicker(time.Minute)

	for {
		select {
		case req, ok := <-s.queueGetFileUpstream:
			if !ok {
				return
			}

			e := memo[req.args]
			if e == nil {
				e = &queueGetFileUpstreamExecution{
					res:    queueGetFileUpstreamResult{},
					waiter: make(chan struct{}),
				}

				memo[req.args] = e

				go func() {
					e.res.res, e.res.err = s.queueGetFileUpstreamExecute(context.TODO(), req.args)
					close(e.waiter)
				}()
			}

			go e.wait(req.result)
		case <-prune.C:
			for k, e := range memo {
				if e.waiter == nil {
					delete(memo, k)
				}
			}
		}
	}
}

func (s *Service) queueGetFileUpstreamExecute(ctx context.Context, args queueGetFileUpstreamRequestArgs) (*robotstxtdpb.File, error) {
	res, err := s.upstream.GetFile(ctx, &robotstxtdpb.Crawler_GetFile_Request{
		ServiceUri: args.ServiceUri,
	})
	if err != nil {
		return nil, err
	}

	s.cacheGetFileMutex.Lock()
	s.cacheGetFileByService[args.ServiceUri] = res
	s.cacheGetFileMutex.Unlock()

	return res, nil
}
