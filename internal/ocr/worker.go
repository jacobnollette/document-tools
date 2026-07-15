package ocr

import (
	"context"
	"log/slog"
	"sync"

	"document-tools/internal/store"
)

// Worker consumes document IDs from an in-process queue and runs them through
// the processing Engine. Documents left in pending or processing state (for
// example after a restart) are re-enqueued by Recover.
type Worker struct {
	store   *store.Store
	engine  Engine
	logger  *slog.Logger
	queue   chan string
	workers int
	wg      sync.WaitGroup
}

// NewWorker creates a worker pool. concurrency defaults to 2 when <= 0.
func NewWorker(s *store.Store, engine Engine, logger *slog.Logger, concurrency int) *Worker {
	if concurrency <= 0 {
		concurrency = 2
	}
	return &Worker{
		store:   s,
		engine:  engine,
		logger:  logger,
		queue:   make(chan string, 256),
		workers: concurrency,
	}
}

// Start launches the worker goroutines. They exit when ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case id := <-w.queue:
					w.process(ctx, id)
				}
			}
		}()
	}
}

// Wait blocks until all workers have exited.
func (w *Worker) Wait() {
	w.wg.Wait()
}

// Enqueue schedules a document for processing. It never blocks; if the queue
// is full the document stays pending and is picked up by the next Recover.
func (w *Worker) Enqueue(id string) {
	select {
	case w.queue <- id:
	default:
		w.logger.Warn("ocr queue full, leaving document pending", "id", id)
	}
}

// Recover re-enqueues documents that were interrupted mid-pipeline.
func (w *Worker) Recover(ctx context.Context) error {
	ids, err := w.store.Recoverable(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		w.Enqueue(id)
	}
	return nil
}

func (w *Worker) process(ctx context.Context, id string) {
	if _, err := w.store.UpdateStatus(ctx, id, store.StatusProcessing, ""); err != nil {
		w.logger.Error("mark document processing", "id", id, "error", err)
		return
	}

	doc, err := w.store.Get(ctx, id)
	if err != nil {
		w.logger.Error("load document", "id", id, "error", err)
		return
	}
	path, err := w.store.FilePath(ctx, id)
	if err != nil {
		w.fail(ctx, id, "stored file is missing")
		return
	}

	result, err := w.engine.Process(ctx, path, doc.ContentType, w.store.PreviewDir(id))
	if err != nil {
		w.logger.Error("processing failed", "id", id, "error", err)
		w.fail(ctx, id, err.Error())
		return
	}

	if err := w.store.SetOCRResult(ctx, id, result.Text, result.PageCount); err != nil {
		w.logger.Error("store ocr result", "id", id, "error", err)
		w.fail(ctx, id, "failed to store extracted text")
		return
	}
	if _, err := w.store.UpdateStatus(ctx, id, store.StatusCompleted, ""); err != nil {
		w.logger.Error("mark document completed", "id", id, "error", err)
		return
	}
	w.logger.Info("processing completed", "id", id, "chars", len(result.Text), "pages", result.PageCount)
}

func (w *Worker) fail(ctx context.Context, id, msg string) {
	if _, err := w.store.UpdateStatus(ctx, id, store.StatusFailed, msg); err != nil {
		w.logger.Error("mark document failed", "id", id, "error", err)
	}
}
