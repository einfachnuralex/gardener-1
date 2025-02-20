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

package project

import (
	"context"
	"strings"

	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gutil "github.com/gardener/gardener/pkg/utils/gardener"
)

func (c *Controller) roleBindingUpdate(ctx context.Context, _, new interface{}) {
	c.roleBindingDelete(ctx, new)
}

func (c *Controller) roleBindingDelete(ctx context.Context, obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		c.log.Error(err, "Couldn't get key for object", "object", obj)
		return
	}

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.log.Error(err, "Couldn't get key for object", "object", obj)
		return
	}

	if name == "gardener.cloud:system:project-member" ||
		name == "gardener.cloud:system:project-viewer" ||
		strings.HasPrefix(name, "gardener.cloud:extension:project:") {

		c.log.V(1).Info("RoleBinding modified", "roleBinding", client.ObjectKey{Namespace: namespace, Name: name})

		project, err := gutil.ProjectForNamespaceFromReader(ctx, c.gardenClient, namespace)
		if err != nil {
			c.log.Error(err, "Couldn't get Project for Namespace", "namespaceName", namespace)
			return
		}

		if project.DeletionTimestamp == nil {
			c.projectQueue.Add(project.Name)
		}
	}
}
