// Package queueprocessor implements the BullMQ worker for omnivore-backend-queue.
package queueprocessor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/omnivore-app/omnivore/internal/bullmq"
	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/jobs"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

const (
	workerConcurrency  = 2
	workerPollInterval = 500 * time.Millisecond
)

// Worker processes jobs from the omnivore-backend-queue.
type Worker struct {
	ctx     context.Context
	cfg     *config.Config
	redisDS *redisutil.RedisDataSource
	db      *db.Pool
	wg      sync.WaitGroup
	sem     chan struct{}
}

// NewWorker creates a new queue processor worker.
func NewWorker(
	ctx context.Context,
	cfg *config.Config,
	redisDS *redisutil.RedisDataSource,
	dbPool *db.Pool,
) *Worker {
	return &Worker{
		ctx:     ctx,
		cfg:     cfg,
		redisDS: redisDS,
		db:      dbPool,
		sem:     make(chan struct{}, workerConcurrency),
	}
}

// Start launches the worker loop in the background.
func (w *Worker) Start() {
	w.wg.Add(1)
	go w.run()
}

// Wait blocks until all in-flight jobs complete and the worker exits.
func (w *Worker) Wait() {
	w.wg.Wait()
}

func (w *Worker) run() {
	defer w.wg.Done()

	log.Println("[queue-processor] worker started")

	_ = bullmq.EnsureQueueMeta(w.ctx, w.redisDS.MQClient, bullmq.BackendQueue)

	for {
		select {
		case <-w.ctx.Done():
			log.Println("[queue-processor] stopping, draining active slots...")
			for i := 0; i < workerConcurrency; i++ {
				w.sem <- struct{}{}
			}
			log.Println("[queue-processor] stopped")
			return
		default:
		}

		job, err := bullmq.PopJob(w.ctx, w.redisDS.MQClient, bullmq.BackendQueue)
		if err != nil {
			log.Printf("[queue-processor] error popping job: %v", err)
			time.Sleep(workerPollInterval)
			continue
		}
		if job == nil {
			time.Sleep(workerPollInterval)
			continue
		}

		w.sem <- struct{}{}
		w.wg.Add(1)
		go func(j *bullmq.RawJob) {
			defer func() { <-w.sem }()
			defer w.wg.Done()
			w.processJob(j)
		}(job)
	}
}

func (w *Worker) processJob(job *bullmq.RawJob) {
	log.Printf("[queue-processor] processing job id=%s name=%s", job.ID, job.Name)

	var err error
	switch job.Name {
	case "save-page":
		err = jobs.HandleSavePage(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "refresh-feed":
		err = jobs.HandleRefreshFeed(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "refresh-all-feeds":
		err = jobs.HandleRefreshAllFeeds(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "trigger-rule":
		err = jobs.HandleTriggerRule(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "find-thumbnail":
		err = jobs.HandleFindThumbnail(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "call-webhook":
		err = jobs.HandleCallWebhook(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "prune-trash":
		err = jobs.HandlePruneTrash(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "expire-folders":
		err = jobs.HandleExpireFolders(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "upload-content":
		err = jobs.HandleUploadContent(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "sync-read-positions":
		err = jobs.HandleSyncReadPositions(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "update-labels":
		err = jobs.HandleUpdateLabels(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "update-highlight":
		err = jobs.HandleUpdateHighlight(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "bulk-action":
		err = jobs.HandleBulkAction(w.ctx, w.cfg, w.redisDS, w.db, job.Data)
	case "ai-summarize", "send-email", "export", "update-home", "create-digest":
		log.Printf("[queue-processor] job %q not yet implemented, skipping", job.Name)
	default:
		log.Printf("[queue-processor] unknown job %q, skipping", job.Name)
	}

	if err != nil {
		log.Printf("[queue-processor] job id=%s failed: %v", job.ID, err)
		_ = bullmq.FailJob(w.ctx, w.redisDS.MQClient, bullmq.BackendQueue, job.ID, err.Error(), job.Opts)
		return
	}

	_ = bullmq.CompleteJob(w.ctx, w.redisDS.MQClient, bullmq.BackendQueue, job.ID)
	log.Printf("[queue-processor] job id=%s completed", job.ID)
}
