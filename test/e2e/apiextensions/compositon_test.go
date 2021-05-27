// +build e2e

/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apiextensions

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"

	corev1 "k8s.io/api/core/v1"
	kextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	extv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	v1 "github.com/crossplane/crossplane/apis/pkg/v1"
)

func TestSomething(t *testing.T) {
	cases := map[string]struct {
		reason string
		body   func() error
	}{
		"TestSomething": {
			reason: "Should successfully test composition engine",
			body: func() error {
				ctx := context.Background()
				s := runtime.NewScheme()
				if err := v1.AddToScheme(s); err != nil {
					return err
				}
				if err := extv1.AddToScheme(s); err != nil {
					return err
				}

				cfg := ctrl.GetConfigOrDie()
				c, err := client.New(cfg, client.Options{
					Scheme: s,
				})
				if err != nil {
					return err
				}

				// dc, err := dynamic.NewForConfig(cfg)
				// if err != nil {
				// 	return err
				// }

				prv := &v1.Provider{
					ObjectMeta: metav1.ObjectMeta{Name: "provider-nop"},
					Spec: v1.ProviderSpec{
						PackageSpec: v1.PackageSpec{
							Package:                     "crossplane/provider-nop:main",
							IgnoreCrossplaneConstraints: pointer.BoolPtr(true),
						},
					},
				}

				if err := c.Create(ctx, prv); err != nil {
					t.Fatalf("Create provider %q: %v", prv.GetName(), err)
				}

				t.Logf("Created provider %q", prv.GetName())

				t.Cleanup(func() {
					t.Logf("Cleaning up provider %q.", prv.GetName())
					if err := c.Get(ctx, types.NamespacedName{Name: prv.GetName()}, prv); resource.IgnoreNotFound(err) != nil {
						t.Fatalf("Get provider %q: %v", prv.GetName(), err)
					}
					if err := c.Delete(ctx, prv); resource.IgnoreNotFound(err) != nil {
						t.Fatalf("Delete provider %q: %v", prv.GetName(), err)
					}
					t.Logf("Deleted provider %q", prv.GetName())
				})

				xrd := &extv1.CompositeResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "clusternopresources.nop.example.org"},
					Spec: extv1.CompositeResourceDefinitionSpec{
						Group: "nop.example.org",
						Names: kextv1.CustomResourceDefinitionNames{
							Kind:     "ClusterNopResource",
							ListKind: "ClusterNopResourceList",
							Plural:   "clusternopresources",
							Singular: "clusternopresource",
						},
						ClaimNames: &kextv1.CustomResourceDefinitionNames{
							Kind:     "NopResource",
							ListKind: "NopResourceList",
							Plural:   "nopresources",
							Singular: "nopresource",
						},
						ConnectionSecretKeys: []string{"test"},
						Versions: []extv1.CompositeResourceDefinitionVersion{{
							Name:          "v1alpha1",
							Served:        true,
							Referenceable: true,
							Schema: &extv1.CompositeResourceValidation{
								OpenAPIV3Schema: runtime.RawExtension{Raw: []byte(`{
									"type": "object",
									"properties": {
										"spec": {
											"type": "object",
											"properties": {
												"coolField": {
													"type": "string"
												}
											},
											"required": ["coolField"]
										},
										"status": {
											"type": "object",
											"properties": {
												"a": {"type": "string"},
												"b": {"type": "integer"},
												"c": {"type": "boolean"}
											}
										}
									}
								}`)},
							},
						}},
					},
				}
				// XRDs take a while to delete, so we try a few times in case creates are
				// failing due to an old XRD hanging around.
				if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
					if err := c.Create(ctx, xrd); err != nil {
						t.Logf("Create XRD %q: %v", xrd.GetName(), err)
						return false, nil
					}
					return true, nil
				}); err != nil {
					t.Fatalf("Create XRD %q: %v", xrd.GetName(), err)
				}
				t.Logf("Created XRD %q", xrd.GetName())

				t.Cleanup(func() {
					t.Logf("Cleaning up XRD %q.", xrd.GetName())
					if err := c.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, xrd); resource.IgnoreNotFound(err) != nil {
						t.Fatalf("Get XRD %q: %v", xrd.GetName(), err)
					}
					if err := c.Delete(ctx, xrd); resource.IgnoreNotFound(err) != nil {
						t.Fatalf("Delete XRD %q: %v", xrd.GetName(), err)
					}
					t.Logf("Deleted XRD %q", xrd.GetName())
				})

				t.Log("Waiting for the XRD's Established and Offered status conditions to become 'True'.")
				if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
					if err := c.Get(ctx, types.NamespacedName{Name: xrd.GetName()}, xrd); err != nil {
						return false, err
					}

					if xrd.Status.GetCondition(extv1.TypeEstablished).Status != corev1.ConditionTrue {
						t.Logf("XRD %q is not yet Established", xrd.GetName())
						return false, nil
					}

					if xrd.Status.GetCondition(extv1.TypeOffered).Status != corev1.ConditionTrue {
						t.Logf("XRD %q is not yet Offered", xrd.GetName())
						return false, nil
					}

					t.Logf("XRD %q is Established and Offered", xrd.GetName())
					return true, nil
				}); err != nil {
					t.Errorf("XRD %q never became Established and Offered: %v", xrd.GetName(), err)
				}

				comp := &extv1.Composition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "clusternopresources.sample.nop.example.org",
						Labels: map[string]string{
							"provider": "nop",
						},
					},
					Spec: extv1.CompositionSpec{
						CompositeTypeRef: extv1.TypeReference{
							APIVersion: "nop.example.org/v1alpha1",
							Kind:       "ClusterNopResource",
						},

						Resources: []extv1.ComposedTemplate{
							{
								Name: pointer.StringPtr("nopinstance1"),
								Base: runtime.RawExtension{Raw: []byte(`{
								"apiVersion": "nop.crossplane.io/v1alpha1",
								"kind": "NopResource",
								"spec": {
									"forProvider": {
									   "conditionAfter": [
										  {
											 "conditionType": "Ready",
											 "conditionStatus": "False",
											 "time": "0s"
										  },
										  {
											 "conditionType": "Ready",
											 "conditionStatus": "True",
											 "time": "10s"
										  },
										  {
											 "conditionType": "Synced",
											 "conditionStatus": "False",
											 "time": "0s"
										  },
										  {
											 "conditionType": "Synced",
											 "conditionStatus": "True",
											 "time": "10s"
										  }
									   ]
									},
									"writeConnectionSecretToRef": {
									   "namespace": "crossplane-system",
									   "name": "nop-example-resource"
									}
								}
							}`)},
							},
							{
								Name: pointer.StringPtr("nopinstance2"),
								Base: runtime.RawExtension{Raw: []byte(`{
									"apiVersion": "nop.crossplane.io/v1alpha1",
									"kind": "NopResource",
									"spec": {
										"forProvider": {
										   "conditionAfter": [
											  {
												 "conditionType": "Ready",
												 "conditionStatus": "False",
												 "time": "0s"
											  },
											  {
												 "conditionType": "Ready",
												 "conditionStatus": "True",
												 "time": "20s"
											  },
											  {
												 "conditionType": "Synced",
												 "conditionStatus": "False",
												 "time": "0s"
											  },
											  {
												 "conditionType": "Synced",
												 "conditionStatus": "True",
												 "time": "20s"
											  }
										   ]
										},
										"writeConnectionSecretToRef": {
										   "namespace": "crossplane-system",
										   "name": "nop-example-resource"
										}
									}
								}`)},
							},
						},
					},
				}

				if err := c.Create(ctx, comp); err != nil {
					t.Fatalf("Create composition %q: %v", comp.GetName(), err)
				}
				t.Logf("Created composition %q", comp.GetName())

				t.Cleanup(func() {
					t.Logf("Cleaning up composition %q.", comp.GetName())
					if err := c.Get(ctx, types.NamespacedName{Name: comp.GetName()}, comp); resource.IgnoreNotFound(err) != nil {
						t.Fatalf("Get composition %q: %v", comp.GetName(), err)
					}
					if err := c.Delete(ctx, comp); resource.IgnoreNotFound(err) != nil {
						t.Fatalf("Delete composition %q: %v", comp.GetName(), err)
					}
					t.Logf("Deleted composition %q", comp.GetName())
				})

				// nopRes := schema.GroupVersionResource{Group: "nop.example.org", Version: "v1alpha1", Resource: "nopresources"}

				nopresource := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "nop.example.org/v1alpha1",
						"kind":       "NopResource",
						"metadata": map[string]interface{}{
							"name": "nop-example",
						},
						"spec": map[string]interface{}{
							"coolField": "example",
						},
					},
				}

				if err := c.Create(ctx, nopresource); err != nil {
					t.Fatalf("Create nop-example %q: %v", nopresource.GetName(), err)
				}
				// res, err := dc.Resource(nopRes).Namespace("default").Create(context.TODO(), nopresource, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Create nop-example %q: %v", nopresource.GetName(), err)
				}

				t.Logf("Created nop-example %q", nopresource.GetName())

				if err := wait.PollImmediate(10*time.Second, 90*time.Second, func() (done bool, err error) {
					// if result, err := dc.Resource(nopRes).Namespace(res.GetNamespace()).Get(context.TODO(), res.GetName(), metav1.GetOptions{}); err != nil {
					// 	return false, err
					// }

					// c.Get(context.TODO(), types.NamespacedName{Name: res.GetName(), Namespace: res.GetNamespace()}, res)

					// TODO(rahulgrover99)
					// Fetch condition of the resource and check if its ready or not

					return true, nil
				}); err != nil {
					t.Errorf("nop-example %q never became Ready: %v", nopresource.GetName(), err)
				}

				t.Cleanup(func() {
					t.Logf("Cleaning up nop-example %q.", nopresource.GetName())
					if err := c.Get(ctx, types.NamespacedName{Name: nopresource.GetName()}, nopresource); resource.IgnoreNotFound(err) != nil {
						t.Fatalf("Get nop-example %q: %v", nopresource.GetName(), err)
					}
					if err := c.Delete(ctx, nopresource); resource.IgnoreNotFound(err) != nil {
						t.Fatalf("Delete nop-example %q: %v", nopresource.GetName(), err)
					}
					t.Logf("Deleted nop-example %q", nopresource.GetName())
				})

				return nil
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := tc.body(); err != nil {
				t.Fatal(err)
			}
		})
	}

}
