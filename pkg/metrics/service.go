package metrics

import (
	"context"
	"time"

	"pmanage/pkg/accelerator"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Service struct {
	app      *application.App
	cancel   context.CancelFunc
	registry *accelerator.Registry
	rate     time.Duration
	paused   bool
	sinks    []func(accelerator.Sample)
}

func NewService(registry *accelerator.Registry, rate time.Duration) *Service {
	return &Service{registry: registry, rate: rate}
}

// AddSink registers an in-process consumer (e.g. HistoryService, AlertEngine)
// that receives every sample; used instead of round-tripping through the
// frontend event loop.
func (s *Service) AddSink(fn func(accelerator.Sample)) {
	s.sinks = append(s.sinks, fn)
}

func (s *Service) SetApp(app *application.App) { s.app = app }

// App returns the Wails application handle so sinks (history/alerts) can emit
// frontend events from the polling goroutine.
func (s *Service) App() *application.App { return s.app }

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	appCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go s.pollLoop(appCtx)
	return nil
}

func (s *Service) ServiceShutdown() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *Service) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(s.rate)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.paused {
				continue
			}
			sample := s.registry.SampleAll(ctx)
			sample.Timestamp = time.Now().UnixMilli()
			s.app.Event.Emit("telemetry", sample)
			for _, sink := range s.sinks {
				sink(sample)
			}
		}
	}
}

func (s *Service) PauseSampling()  { s.paused = true }
func (s *Service) ResumeSampling() { s.paused = false }
