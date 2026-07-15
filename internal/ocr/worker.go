package ocr

import (
	"context"
	"log/slog"
	"sync"

	"document-tools/internal/store"
)

// Worker consumes document IDs from an in-process queue and runs OCR on them.
// Documents left in pending or processing state (for example after a restart)
// are re-enqueued by Recover.
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

// Enqueue schedules a document for OCR. It never blocks; if the queue is full
// the document stays pending and is picked up by the next Recover.
func (w *Worker) Enqueue(id string) {
	select {
	case w.queue <- id:
	default:
		w.logger.Warn("ocr queue full, leaving document pending", "id", id)
	}
}

// Recover re-enqueues documents that were interrupted mid-pipeline.
func (w *Worker) Recover() error {
	docs, err := w.store.List()
	if err != nil {
		return err
	}
	for _, doc := range docs {
		if doc.Status == store.StatusPending || doc.Status == store.StatusProcessing {
			w.Enqueue(doc.ID)
		}
	}
	return nil
}

func (w *Worker) process(ctx context.Context, id string) {
	if _, err := w.store.UpdateStatus(id, store.StatusProcessing, ""); err != nil {
		w.logger.Error("mark document processing", "id", id, "error", err)
		return
	}

	path, err := w.store.FilePath(id)
	if err != nil {
		w.fail(id, "stored file is missing")
		return
	}

	text, err := w.engine.Extract(ctx, path)
	if err != nil {
		w.logger.Error("ocr failed", "id", id, "error", err)
		w.fail(id, err.Error())
		return
	}

	if err := w.store.SetOCRText(id, text); err != nil {
		w.logger.Error("store ocr text", "id", id, "error", err)
		w.fail(id, "failed to store extracted text")
		return
	}
	if _, err := w.store.UpdateStatus(id, store.StatusCompleted, ""); err != nil {
		w.logger.Error("mark document completed", "id", id, "error", err)
		return
	}
	w.logger.Info("ocr completed", "id", id, "chars", len(text))
}

func (w *Worker) fail(id, msg string) {
	if _, err := w.store.UpdateStatus(id, store.StatusFailed, msg); err != nil {
		w.logger.Error("mark document failed", "id", id, "error", err)
	}
}
