// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package validation_test

import (
	"time"

	gardencore "github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/gardenlet/apis/config"
	. "github.com/gardener/gardener/pkg/gardenlet/apis/config/validation"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
)

var _ = Describe("GardenletConfiguration", func() {
	var (
		cfg *config.GardenletConfiguration

		deletionGracePeriodHours = 1
		concurrentSyncs          = 20
	)

	BeforeEach(func() {
		cfg = &config.GardenletConfiguration{
			Controllers: &config.GardenletControllerConfiguration{
				BackupEntry: &config.BackupEntryControllerConfiguration{
					DeletionGracePeriodHours:         &deletionGracePeriodHours,
					DeletionGracePeriodShootPurposes: []gardencore.ShootPurpose{gardencore.ShootPurposeDevelopment},
				},
				Bastion: &config.BastionControllerConfiguration{
					ConcurrentSyncs: &concurrentSyncs,
				},
				Shoot: &config.ShootControllerConfiguration{
					ConcurrentSyncs:      &concurrentSyncs,
					ProgressReportPeriod: &metav1.Duration{Duration: time.Hour},
					SyncPeriod:           &metav1.Duration{Duration: time.Hour},
					RetryDuration:        &metav1.Duration{Duration: time.Hour},
					DNSEntryTTLSeconds:   pointer.Int64(120),
				},
				ManagedSeed: &config.ManagedSeedControllerConfiguration{
					ConcurrentSyncs:  &concurrentSyncs,
					SyncPeriod:       &metav1.Duration{Duration: 1 * time.Hour},
					WaitSyncPeriod:   &metav1.Duration{Duration: 15 * time.Second},
					SyncJitterPeriod: &metav1.Duration{Duration: 5 * time.Minute},
				},
			},
			SeedConfig: &config.SeedConfig{
				SeedTemplate: gardencore.SeedTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"foo": "bar",
						},
					},
					Spec: gardencore.SeedSpec{
						DNS: gardencore.SeedDNS{
							IngressDomain: pointer.String("ingress.test.example.com"),
						},
						Networks: gardencore.SeedNetworks{
							Pods:     "100.96.0.0/11",
							Services: "100.64.0.0/13",
						},
						Provider: gardencore.SeedProvider{
							Type:   "foo",
							Region: "some-region",
						},
					},
				},
			},
			Resources: &config.ResourcesConfiguration{
				Capacity: corev1.ResourceList{
					"foo": resource.MustParse("42"),
					"bar": resource.MustParse("13"),
				},
				Reserved: corev1.ResourceList{
					"foo": resource.MustParse("7"),
				},
			},
		}
	})

	Describe("#ValidateGardenletConfiguration", func() {
		It("should allow valid configurations", func() {
			errorList := ValidateGardenletConfiguration(cfg, nil, false)

			Expect(errorList).To(BeEmpty())
		})

		Context("shoot controller", func() {
			It("should forbid invalid configuration", func() {
				invalidConcurrentSyncs := -1

				cfg.Controllers.Shoot.ConcurrentSyncs = &invalidConcurrentSyncs
				cfg.Controllers.Shoot.ProgressReportPeriod = &metav1.Duration{Duration: -1}
				cfg.Controllers.Shoot.SyncPeriod = &metav1.Duration{Duration: -1}
				cfg.Controllers.Shoot.RetryDuration = &metav1.Duration{Duration: -1}

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("controllers.shoot.concurrentSyncs"),
					})),
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("controllers.shoot.progressReporterPeriod"),
					})),
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("controllers.shoot.syncPeriod"),
					})),
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("controllers.shoot.retryDuration"),
					})),
				))
			})

			It("should forbid too low values for the DNS TTL", func() {
				cfg.Controllers.Shoot.DNSEntryTTLSeconds = pointer.Int64(-1)

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("controllers.shoot.dnsEntryTTLSeconds"),
				}))))
			})

			It("should forbid too high values for the DNS TTL", func() {
				cfg.Controllers.Shoot.DNSEntryTTLSeconds = pointer.Int64(601)

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("controllers.shoot.dnsEntryTTLSeconds"),
				}))))
			})
		})

		Context("managed seed controller", func() {
			It("should forbid invalid configuration", func() {
				invalidConcurrentSyncs := -1

				cfg.Controllers.ManagedSeed.ConcurrentSyncs = &invalidConcurrentSyncs
				cfg.Controllers.ManagedSeed.SyncPeriod = &metav1.Duration{Duration: -1}
				cfg.Controllers.ManagedSeed.WaitSyncPeriod = &metav1.Duration{Duration: -1}
				cfg.Controllers.ManagedSeed.SyncJitterPeriod = &metav1.Duration{Duration: -1}

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("controllers.managedSeed.concurrentSyncs"),
					})),
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("controllers.managedSeed.syncPeriod"),
					})),
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("controllers.managedSeed.waitSyncPeriod"),
					})),
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("controllers.managedSeed.syncJitterPeriod"),
					})),
				))
			})
		})

		Context("backup entry controller", func() {
			It("should forbid specifying purposes when not specifying hours", func() {
				cfg.Controllers.BackupEntry.DeletionGracePeriodHours = nil

				Expect(ValidateGardenletConfiguration(cfg, nil, false)).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeForbidden),
						"Field": Equal("controllers.backupEntry.deletionGracePeriodShootPurposes"),
					})),
				))
			})

			It("should allow valid purposes", func() {
				cfg.Controllers.BackupEntry.DeletionGracePeriodShootPurposes = []gardencore.ShootPurpose{
					gardencore.ShootPurposeEvaluation,
					gardencore.ShootPurposeTesting,
					gardencore.ShootPurposeDevelopment,
					gardencore.ShootPurposeInfrastructure,
					gardencore.ShootPurposeProduction,
				}

				Expect(ValidateGardenletConfiguration(cfg, nil, false)).To(BeEmpty())
			})

			It("should forbid invalid purposes", func() {
				cfg.Controllers.BackupEntry.DeletionGracePeriodShootPurposes = []gardencore.ShootPurpose{"does-not-exist"}

				Expect(ValidateGardenletConfiguration(cfg, nil, false)).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeNotSupported),
						"Field": Equal("controllers.backupEntry.deletionGracePeriodShootPurposes[0]"),
					})),
				))
			})
		})

		Context("bastion controller", func() {
			It("should forbid invalid configuration", func() {
				invalidConcurrentSyncs := -1
				cfg.Controllers.Bastion.ConcurrentSyncs = &invalidConcurrentSyncs

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("controllers.bastion.concurrentSyncs"),
					})),
				))
			})
		})

		Context("seed config", func() {
			It("should require a seedConfig", func() {
				cfg.SeedConfig = nil

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("seedConfig"),
				}))))
			})
		})

		Context("seed template", func() {
			It("should forbid invalid fields in seed template", func() {
				cfg.SeedConfig.Spec.Networks.Nodes = pointer.String("")

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeInvalid),
						"Field": Equal("seedConfig.spec.networks.nodes"),
					})),
				))
			})
		})

		Context("server", func() {
			It("should forbid invalid server configuration", func() {
				cfg.Server = &config.ServerConfiguration{}

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeRequired),
						"Field": Equal("server.https.bindAddress"),
					})),
					PointTo(MatchFields(IgnoreExtras, Fields{
						"Type":  Equal(field.ErrorTypeRequired),
						"Field": Equal("server.https.port"),
					})),
				))
			})
		})

		Context("resources", func() {
			It("should forbid reserved greater than capacity", func() {
				cfg.Resources = &config.ResourcesConfiguration{
					Capacity: corev1.ResourceList{
						"foo": resource.MustParse("42"),
					},
					Reserved: corev1.ResourceList{
						"foo": resource.MustParse("43"),
					},
				}

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("resources.reserved.foo"),
				}))))
			})

			It("should forbid reserved without capacity", func() {
				cfg.Resources = &config.ResourcesConfiguration{
					Reserved: corev1.ResourceList{
						"foo": resource.MustParse("42"),
					},
				}

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("resources.reserved.foo"),
				}))))
			})
		})

		Context("exposureClassHandlers", func() {
			BeforeEach(func() {
				cfg.ExposureClassHandlers = []config.ExposureClassHandler{
					{
						Name: "test",
						LoadBalancerService: config.LoadBalancerServiceConfig{
							Annotations: map[string]string{"test": "foo"},
						},
					},
				}
			})

			It("should pass valid exposureClassHandler", func() {
				errorList := ValidateGardenletConfiguration(cfg, nil, false)
				Expect(errorList).To(BeEmpty())
			})

			It("should fail as exposureClassHandler name is no DNS1123 label with zero length", func() {
				cfg.ExposureClassHandlers[0].Name = ""

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("exposureClassHandlers[0].name"),
				}))))
			})

			It("should fail as exposureClassHandler name is no DNS1123 label", func() {
				cfg.ExposureClassHandlers[0].Name = "TE:ST"

				errorList := ValidateGardenletConfiguration(cfg, nil, false)

				Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("exposureClassHandlers[0].name"),
				}))))
			})

		})
	})

	Describe("#ValidateGardenletConfigurationUpdate", func() {
		It("should allow valid configuration updates", func() {
			errorList := ValidateGardenletConfigurationUpdate(cfg, cfg, nil)

			Expect(errorList).To(BeEmpty())
		})

		It("should forbid changes to immutable fields in seed template", func() {
			newCfg := cfg.DeepCopy()
			newCfg.SeedConfig.Spec.Networks.Pods = "100.97.0.0/11"

			errorList := ValidateGardenletConfigurationUpdate(newCfg, cfg, nil)

			Expect(errorList).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal(field.ErrorTypeInvalid),
					"Field":  Equal("seedConfig.spec.networks.pods"),
					"Detail": Equal("field is immutable"),
				})),
			))
		})
	})
})
