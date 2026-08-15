// Command controlled-operation-operator runs the ControlledOperation
// controller for the aiops-platform. It watches ControlledOperation CRs via
// a dynamic informer and carries each one out against its target
// Deployment/CronJob with dry-run support. Pure client-go; no
// controller-runtime dependency.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"k8s-aiops.local/backend/internal/operator"
)

func main() {
	var (
		kubeconfig = flag.String("kubeconfig", "", "path to kubeconfig (defaults to in-cluster config)")
		workers    = flag.Int("workers", 1, "number of reconcile workers")
		resync     = flag.Duration("resync", 30*time.Minute, "informer resync period")
	)
	flag.Parse()

	cfg, err := buildConfig(*kubeconfig)
	if err != nil {
		log.Fatalf("build kubeconfig: %v", err)
	}

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("dynamic client: %v", err)
	}

	client := operator.NewClient(dyn)
	executor := operator.NewDynamicExecutor(dyn)
	reconciler := operator.NewReconciler(client, executor)
	ctrl := operator.NewController(reconciler)

	// Event-driven reconciliation: a dynamic shared informer watches the CRD
	// and enqueues keys on add/update; a delete tombstone handler enqueues
	// too so a finalizer racing a deletion is reconciled.
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dyn, *resync, metav1.NamespaceAll, nil,
	)
	informer := factory.ForResource(operator.Resource()).Informer()
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if err := ctrl.Enqueue(obj); err != nil {
				log.Printf("enqueue add: %v", err)
			}
		},
		UpdateFunc: func(_, obj any) {
			if err := ctrl.Enqueue(obj); err != nil {
				log.Printf("enqueue update: %v", err)
			}
		},
		DeleteFunc: func(obj any) {
			if err := ctrl.Enqueue(obj); err != nil {
				log.Printf("enqueue delete: %v", err)
			}
		},
	}); err != nil {
		log.Fatalf("register event handlers: %v", err)
	}
	// The lister is built from the same informer; it stays unused by the
	// queue path but documents the way to read the local cache.
	_ = informer.GetIndexer()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("starting controlled-operation-operator (pure client-go, dynamic informer)")
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		log.Fatal("timed out waiting for ControlledOperation informer cache sync")
	}
	log.Println("ControlledOperation informer cache synced")

	runWorkers(ctx, ctrl, *workers)

	log.Println("shutting down")
}

// runWorkers drains the controller queue until the context is cancelled.
func runWorkers(ctx context.Context, ctrl *operator.Controller, workers int) {
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctrl.ProcessOne(ctx) {
			}
		}()
	}
	<-ctx.Done()
	ctrl.ShutDown()
	wg.Wait()
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{}).ClientConfig()
}
