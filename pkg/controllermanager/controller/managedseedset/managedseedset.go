// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedseedset

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	runtimecache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardencorev1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/client/kubernetes/clientmap"
	"github.com/gardener/gardener/pkg/client/kubernetes/clientmap/keys"
	"github.com/gardener/gardener/pkg/controllermanager/apis/config"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/logger"
	kutils "github.com/gardener/gardener/pkg/utils/kubernetes"
)

const (
	// ControllerName is the name of this controller.
	ControllerName = "managedseedset"
)

// Controller controls ManagedSeedSets.
type Controller struct {
	gardenClient kubernetes.Interface
	log          logr.Logger

	reconciler reconcile.Reconciler

	managedSeedSetInformer runtimecache.Informer
	shootInformer          runtimecache.Informer
	managedSeedInformer    runtimecache.Informer
	seedInformer           runtimecache.Informer

	managedSeedSetQueue workqueue.RateLimitingInterface

	numberOfRunningWorkers int
	workerCh               chan int
}

// NewManagedSeedSetController creates a new Gardener controller for ManagedSeedSets.
func NewManagedSeedSetController(
	ctx context.Context,
	log logr.Logger,
	clientMap clientmap.ClientMap,
	config *config.ControllerManagerConfiguration,
	recorder record.EventRecorder,
) (*Controller, error) {
	log = log.WithName(ControllerName)

	gardenClient, err := clientMap.GetClient(ctx, keys.ForGarden())
	if err != nil {
		return nil, fmt.Errorf("could not get garden client: %w", err)
	}

	managedSeedSetInformer, err := gardenClient.Cache().GetInformer(ctx, &seedmanagementv1alpha1.ManagedSeedSet{})
	if err != nil {
		return nil, fmt.Errorf("could not get ManagedSeedSet informer: %w", err)
	}

	shootInformer, err := gardenClient.Cache().GetInformer(ctx, &gardencorev1beta1.Shoot{})
	if err != nil {
		return nil, fmt.Errorf("could not get Shoot informer: %w", err)
	}

	managedSeedInformer, err := gardenClient.Cache().GetInformer(ctx, &seedmanagementv1alpha1.ManagedSeed{})
	if err != nil {
		return nil, fmt.Errorf("could not get ManagedSeed informer: %w", err)
	}

	seedInformer, err := gardenClient.Cache().GetInformer(ctx, &gardencorev1beta1.Seed{})
	if err != nil {
		return nil, fmt.Errorf("could not get Seed informer: %w", err)
	}

	replicaFactory := ReplicaFactoryFunc(NewReplica)
	replicaGetter := NewReplicaGetter(gardenClient, replicaFactory)
	actuator := NewActuator(gardenClient, replicaGetter, replicaFactory, config.Controllers.ManagedSeedSet, recorder)
	reconciler := NewReconciler(gardenClient, actuator, config.Controllers.ManagedSeedSet)

	return &Controller{
		gardenClient:           gardenClient,
		log:                    log,
		reconciler:             reconciler,
		managedSeedSetInformer: managedSeedSetInformer,
		shootInformer:          shootInformer,
		managedSeedInformer:    managedSeedInformer,
		seedInformer:           seedInformer,
		managedSeedSetQueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ManagedSeedSet"),
		workerCh:               make(chan int),
	}, nil
}

// Run runs the Controller until the given context is cancelled.
func (c *Controller) Run(ctx context.Context, workers int) {
	var waitGroup sync.WaitGroup

	c.managedSeedSetInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.managedSeedSetAdd,
		UpdateFunc: c.managedSeedSetUpdate,
		DeleteFunc: c.managedSeedSetDelete,
	})

	// TODO: switch to logr once kutils package is migrated
	logrusLogger := logger.Logger.WithField("logger", "controller."+ControllerName)

	// Add event handler for controlled shoots
	c.shootInformer.AddEventHandler(&kutils.ControlledResourceEventHandler{
		ControllerTypes: []kutils.ControllerType{
			{Type: &seedmanagementv1alpha1.ManagedSeedSet{}},
		},
		Ctx:                        ctx,
		Reader:                     c.gardenClient.Cache(),
		ControllerPredicateFactory: kutils.ControllerPredicateFactoryFunc(c.filterShoot),
		Enqueuer:                   kutils.EnqueuerFunc(func(obj client.Object) { c.managedSeedSetAdd(obj) }),
		Scheme:                     kubernetes.GardenScheme,
		Logger:                     logrusLogger,
	})

	// Add event handler for controlled managed seeds
	c.managedSeedInformer.AddEventHandler(&kutils.ControlledResourceEventHandler{
		ControllerTypes: []kutils.ControllerType{
			{Type: &seedmanagementv1alpha1.ManagedSeedSet{}},
		},
		Ctx:                        ctx,
		Reader:                     c.gardenClient.Cache(),
		ControllerPredicateFactory: kutils.ControllerPredicateFactoryFunc(c.filterManagedSeed),
		Enqueuer:                   kutils.EnqueuerFunc(func(obj client.Object) { c.managedSeedSetAdd(obj) }),
		Scheme:                     kubernetes.GardenScheme,
		Logger:                     logrusLogger,
	})

	// Add event handler for controlled seeds
	c.seedInformer.AddEventHandler(&kutils.ControlledResourceEventHandler{
		ControllerTypes: []kutils.ControllerType{
			{
				Type:      &seedmanagementv1alpha1.ManagedSeed{},
				Namespace: pointer.String(gardencorev1beta1constants.GardenNamespace),
				NameFunc:  func(obj client.Object) string { return obj.GetName() },
			},
			{Type: &seedmanagementv1alpha1.ManagedSeedSet{}},
		},
		Ctx:                        ctx,
		Reader:                     c.gardenClient.Cache(),
		ControllerPredicateFactory: kutils.ControllerPredicateFactoryFunc(c.filterSeed),
		Enqueuer:                   kutils.EnqueuerFunc(func(obj client.Object) { c.managedSeedSetAdd(obj) }),
		Scheme:                     kubernetes.GardenScheme,
		Logger:                     logrusLogger,
	})

	if !cache.WaitForCacheSync(ctx.Done(), c.managedSeedSetInformer.HasSynced, c.shootInformer.HasSynced, c.managedSeedInformer.HasSynced, c.seedInformer.HasSynced) {
		c.log.Error(wait.ErrWaitTimeout, "Timed out waiting for caches to sync")
		return
	}

	// Count number of running workers
	go func() {
		for res := range c.workerCh {
			c.numberOfRunningWorkers += res
		}
	}()

	c.log.Info("ManagedSeedSet controller initialized")

	for i := 0; i < workers; i++ {
		controllerutils.CreateWorker(ctx, c.managedSeedSetQueue, "ManagedSeedSet", c.reconciler, &waitGroup, c.workerCh, controllerutils.WithLogger(c.log))
	}

	// Shutdown handling
	<-ctx.Done()
	c.managedSeedSetQueue.ShutDown()

	for {
		if c.managedSeedSetQueue.Len() == 0 && c.numberOfRunningWorkers == 0 {
			c.log.V(1).Info("No running ManagedSeedSet worker and no items left in the queues. Terminating ManagedSeedSet controller")
			break
		}
		c.log.V(1).Info("Waiting for ManagedSeedSet workers to finish", "numberOfRunningWorkers", c.numberOfRunningWorkers, "queueLength", c.managedSeedSetQueue.Len())
		time.Sleep(5 * time.Second)
	}

	waitGroup.Wait()
}
