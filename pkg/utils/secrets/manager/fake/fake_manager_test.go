// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package fake_test

import (
	"context"

	secretutils "github.com/gardener/gardener/pkg/utils/secrets"
	secretsmanager "github.com/gardener/gardener/pkg/utils/secrets/manager"
	. "github.com/gardener/gardener/pkg/utils/secrets/manager/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("FakeManager", func() {
	var (
		ctx       = context.TODO()
		name      = "config"
		namespace = "shoot--foo--bar"

		fakeClient client.Client
		m          secretsmanager.Interface
	)

	BeforeEach(func() {
		fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetesscheme.Scheme).Build()
		m = New(fakeClient, namespace)
	})

	DescribeTable("#Get",
		func(expectedSecretName string, opts ...secretsmanager.GetOption) {
			secret, found := m.Get(name, opts...)
			Expect(found).To(BeTrue())
			Expect(secret).To(Equal(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      expectedSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{"data-for": []byte(name)},
			}))
		},

		Entry("no class option", name),
		Entry("current", name+"-current", secretsmanager.Current),
		Entry("old", name+"-old", secretsmanager.Old),
		Entry("bundle", name+"-bundle", secretsmanager.Bundle),
	)

	Describe("#Generate", func() {
		var (
			config         = &secretutils.BasicAuthSecretConfig{Name: name, Format: secretutils.BasicAuthFormatNormal}
			configChecksum = "17492942871593004096"
			secretName     = name + "-fa646dad"
		)

		It("should create a secret for the config", func() {
			secret, err := m.Generate(ctx, config, secretsmanager.Persist(), secretsmanager.Rotate(secretsmanager.InPlace))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.ObjectMeta).To(Equal(metav1.ObjectMeta{
				Name:            secretName,
				Namespace:       namespace,
				ResourceVersion: "1",
				Labels: map[string]string{
					"name":                          name,
					"managed-by":                    "secrets-manager",
					"manager-identity":              "fake",
					"checksum-of-config":            configChecksum,
					"last-rotation-initiation-time": "",
					"rotation-strategy":             "inplace",
					"persist":                       "true",
				},
			}))
			Expect(secret.Type).To(Equal(corev1.SecretTypeOpaque))
			Expect(secret.Immutable).To(PointTo(BeTrue()))
			Expect(secret.Data).To(And(HaveKey("username"), HaveKey("password")))

			obj := &corev1.Secret{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(secret), obj)).To(Succeed())
			secret.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}
			Expect(obj).To(Equal(secret))
		})

		It("should reconcile an existing secret for the config", func() {
			existingSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{"existing": []byte("data")},
			}
			Expect(fakeClient.Create(ctx, existingSecret)).To(Succeed())

			secret, err := m.Generate(ctx, config, secretsmanager.Persist(), secretsmanager.Rotate(secretsmanager.KeepOld))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).To(Equal(&corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            existingSecret.Name,
					Namespace:       existingSecret.Namespace,
					ResourceVersion: "2",
					Labels: map[string]string{
						"name":                          name,
						"managed-by":                    "secrets-manager",
						"manager-identity":              "fake",
						"checksum-of-config":            configChecksum,
						"last-rotation-initiation-time": "",
						"rotation-strategy":             "keepold",
						"persist":                       "true",
					},
				},
				Immutable: pointer.Bool(true),
				Type:      existingSecret.Type,
				Data:      existingSecret.Data,
			}))

			obj := &corev1.Secret{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(secret), obj)).To(Succeed())
			Expect(obj).To(Equal(secret))
		})
	})

	Describe("#Cleanup", func() {
		It("should return nil (not implemented)", func() {
			Expect(m.Cleanup(ctx)).To(BeNil())
		})
	})
})
