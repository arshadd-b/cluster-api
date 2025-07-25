/*
Copyright 2023 The Kubernetes Authors.

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

package ssa

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestPatch(t *testing.T) {
	g := NewWithT(t)

	// Create a namespace for running the test
	ns, err := env.CreateNamespace(ctx, "ssa")
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("Test patch with unstructured", func(*testing.T) {
		// Build the test object to work with.
		initialObject := builder.TestInfrastructureCluster(ns.Name, "obj1").WithSpecFields(map[string]interface{}{
			"spec.controlPlaneEndpoint.host": "1.2.3.4",
			"spec.controlPlaneEndpoint.port": int64(1234),
			"spec.foo":                       "bar",
		}).Build()

		fieldManager := "test-manager"
		ssaCache := NewCache("test-controller")

		// 1. Create the object
		createObject := initialObject.DeepCopy()
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, createObject)).To(Succeed())

		// 2. Update the object and verify that the request was not cached as the object was changed.
		// Get the original object.
		originalObject := initialObject.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject)).To(Succeed())
		// Modify the object
		modifiedObject := initialObject.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedObject.Object, "baz", "spec", "foo")).To(Succeed())
		// Compute request identifier, so we can later verify that the update call was not cached.
		modifiedUnstructured, err := prepareModified(env.Scheme(), modifiedObject)
		g.Expect(err).ToNot(HaveOccurred())
		requestIdentifier, err := ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedUnstructured)
		g.Expect(err).ToNot(HaveOccurred())
		// Update the object
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})).To(Succeed())
		// Verify that request was not cached (as it changed the object)
		g.Expect(ssaCache.Has(requestIdentifier, initialObject.GetKind())).To(BeFalse())

		// 3. Repeat the same update and verify that the request was cached as the object was not changed.
		// Get the original object.
		originalObject = initialObject.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject)).To(Succeed())
		// Modify the object
		modifiedObject = initialObject.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedObject.Object, "baz", "spec", "foo")).To(Succeed())
		// Compute request identifier, so we can later verify that the update call was cached.
		modifiedUnstructured, err = prepareModified(env.Scheme(), modifiedObject)
		g.Expect(err).ToNot(HaveOccurred())
		requestIdentifier, err = ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedUnstructured)
		g.Expect(err).ToNot(HaveOccurred())
		// Update the object
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})).To(Succeed())
		// Verify that request was cached (as it did not change the object)
		g.Expect(ssaCache.Has(requestIdentifier, initialObject.GetKind())).To(BeTrue())
	})

	t.Run("Test patch with Machine", func(*testing.T) {
		// Build the test object to work with.
		initialObject := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Machine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-1",
				Namespace: ns.Name,
				Labels: map[string]string{
					"label": "labelValue",
				},
				Annotations: map[string]string{
					"annotation": "annotationValue",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "cluster-1",
				Version:     "v1.25.0",
				Deletion: clusterv1.MachineDeletionSpec{
					NodeDrainTimeoutSeconds: ptr.To(int32(10)),
				},
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: ptr.To("data-secret"),
				},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: builder.InfrastructureGroupVersion.Group,
					Kind:     builder.TestInfrastructureMachineKind,
					Name:     "inframachine",
				},
			},
		}
		fieldManager := "test-manager"
		ssaCache := NewCache("test-controller")

		// 1. Create the object
		createObject := initialObject.DeepCopy()
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, createObject)).To(Succeed())
		// Verify that gvk is still set
		g.Expect(createObject.GroupVersionKind()).To(Equal(initialObject.GroupVersionKind()))

		// 2. Update the object and verify that the request was not cached as the object was changed.
		// Get the original object.
		originalObject := initialObject.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject)).To(Succeed())
		// Modify the object
		modifiedObject := initialObject.DeepCopy()
		modifiedObject.Spec.Deletion.NodeDrainTimeoutSeconds = ptr.To(int32(5))
		// Compute request identifier, so we can later verify that the update call was not cached.
		modifiedUnstructured, err := prepareModified(env.Scheme(), modifiedObject)
		g.Expect(err).ToNot(HaveOccurred())
		requestIdentifier, err := ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedUnstructured)
		g.Expect(err).ToNot(HaveOccurred())
		// Update the object
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})).To(Succeed())
		// Verify that gvk is still set
		g.Expect(modifiedObject.GroupVersionKind()).To(Equal(initialObject.GroupVersionKind()))
		// Verify that request was not cached (as it changed the object)
		g.Expect(ssaCache.Has(requestIdentifier, initialObject.GetObjectKind().GroupVersionKind().Kind)).To(BeFalse())

		// Wait for 1 second. We are also trying to verify in this test that the resourceVersion of the Machine
		// is not increased. Under some circumstances this would only happen if the timestamp in managedFields would
		// be increased by 1 second.
		// Please see the following issues for more context:
		// * https://github.com/kubernetes-sigs/cluster-api/issues/10533
		// * https://github.com/kubernetes/kubernetes/issues/124605
		time.Sleep(1 * time.Second)

		// 3. Repeat the same update and verify that the request was cached as the object was not changed.
		// Get the original object.
		originalObject = initialObject.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject)).To(Succeed())
		// Modify the object
		modifiedObject = initialObject.DeepCopy()
		modifiedObject.Spec.Deletion.NodeDrainTimeoutSeconds = ptr.To(int32(5))
		// Compute request identifier, so we can later verify that the update call was cached.
		modifiedUnstructured, err = prepareModified(env.Scheme(), modifiedObject)
		g.Expect(err).ToNot(HaveOccurred())
		requestIdentifier, err = ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedUnstructured)
		g.Expect(err).ToNot(HaveOccurred())
		// Update the object
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})).To(Succeed())
		// Verify that request was cached (as it did not change the object)
		g.Expect(ssaCache.Has(requestIdentifier, initialObject.GetObjectKind().GroupVersionKind().Kind)).To(BeTrue())
		// Verify that gvk is still set
		g.Expect(modifiedObject.GroupVersionKind()).To(Equal(initialObject.GroupVersionKind()))
	})
}
