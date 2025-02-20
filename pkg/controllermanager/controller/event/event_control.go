// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package event

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/controllermanager/apis/config"
)

var nowFunc = time.Now

func (c *Controller) enqueueEvent(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		c.log.Error(err, "Couldn't get key for object", "object", obj)
		return
	}
	c.eventQueue.Add(key)
}

// NewEventReconciler creates a new instance of a reconciler which reconciles Events.
func NewEventReconciler(gardenClient client.Client, cfg *config.EventControllerConfiguration) reconcile.Reconciler {
	return &eventReconciler{
		gardenClient: gardenClient,
		cfg:          cfg,
	}
}

type eventReconciler struct {
	gardenClient client.Client
	cfg          *config.EventControllerConfiguration
}

func (r *eventReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	event := &corev1.Event{}
	if err := r.gardenClient.Get(ctx, req.NamespacedName, event); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	if isShootEvent(event) {
		return reconcile.Result{}, nil
	}

	deleteAt := event.LastTimestamp.Add(r.cfg.TTLNonShootEvents.Duration)
	timeUntilDeletion := deleteAt.Sub(nowFunc())
	if timeUntilDeletion > 0 {
		return reconcile.Result{RequeueAfter: timeUntilDeletion}, nil
	}

	return reconcile.Result{}, r.gardenClient.Delete(ctx, event)
}

func isShootEvent(event *corev1.Event) bool {
	if gv, err := schema.ParseGroupVersion(event.InvolvedObject.APIVersion); err != nil || gv.Group != gardencorev1beta1.GroupName {
		return false
	}
	if event.InvolvedObject.Kind == "Shoot" {
		return true
	}
	return false
}
