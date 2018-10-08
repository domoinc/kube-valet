package queues

import (
	"errors"
	"time"

	"github.com/op/go-logging"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type RetryingWorkQueue struct {
	queue             workqueue.RateLimitingInterface
	log               *logging.Logger
	indexer           cache.Indexer
	queueType         string
	businessLogicFunc ItemProcessFunc
	threadiness       int
	stopChan          chan struct{}
}

type ItemProcessFunc func(obj interface{}) error

func NewRetryingWorkQueue(queueType string, indexer cache.Indexer, threadiness int, stopCh chan struct{}) *RetryingWorkQueue {
	return &RetryingWorkQueue{
		queue:       workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		log:         logging.MustGetLogger(queueType + "RetryingWorkQueue"),
		queueType:   queueType,
		indexer:     indexer,
		threadiness: threadiness,
		stopChan:    stopCh,
	}
}

func (rwq *RetryingWorkQueue) AddItem(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err == nil {
		rwq.queue.Add(key)
	} else {
		rwq.log.Errorf("error adding add %s to queue %v", rwq.queueType, err)
	}
}

func (rwq *RetryingWorkQueue) Run(businessLogicFunc ItemProcessFunc) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer rwq.queue.ShutDown()

	rwq.businessLogicFunc = businessLogicFunc

	rwq.log.Infof("Starting %s Queue", rwq.queueType)

	for i := 0; i < rwq.threadiness; i++ {
		go wait.Until(rwq.runWorker, time.Second, rwq.stopChan)
	}

	<-rwq.stopChan
	rwq.log.Infof("Stopping %s Queue", rwq.queueType)
}

func (rwq *RetryingWorkQueue) runWorker() {
	for rwq.processNextItem() {
	}
}

func (rwq *RetryingWorkQueue) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := rwq.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer rwq.queue.Done(key)

	// if the indexer has not been set lets retry it
	if rwq.indexer == nil {
		rwq.handleErr(errors.New("indexer has not been set yet deferring to retry logic"), key)
		return true
	}

	// get the item from the indexer
	obj, exists, err := rwq.indexer.GetByKey(key.(string))

	if err != nil {
		rwq.handleErr(err, key)
		return true
	}

	// if it doesn't exist no reason to retry it
	if !exists {
		rwq.log.Warningf("%s %s does not exist anymore", rwq.queueType, key)
		rwq.handleErr(nil, key)
		return true
	} else {
		err := rwq.businessLogicFunc(obj)
		rwq.handleErr(err, key)
		return true
	}
}

// handleErr checks if an error happened and makes sure we will retry later.
func (rwq *RetryingWorkQueue) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		rwq.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if rwq.queue.NumRequeues(key) < 5 {
		rwq.log.Infof("Error syncing %s %v: %v", rwq.queueType, key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		rwq.queue.AddRateLimited(key)
		return
	}

	rwq.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	rwq.log.Infof("Dropping %s %q out of the queue: %v", rwq.queueType, key, err)
}
