// Copyright The Karbour Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package insight

import (
	"context"

	"github.com/KusionStack/karbour/pkg/core"
	"github.com/KusionStack/karbour/pkg/multicluster"
	topologyutil "github.com/KusionStack/karbour/pkg/util/topology"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "sigs.k8s.io/yaml"
)

// GetResource returns the unstructured cluster object for a given cluster
func (i *InsightManager) GetResource(ctx context.Context, client *multicluster.MultiClusterClient, loc *core.Locator) (*unstructured.Unstructured, error) {
	resourceGVR, err := topologyutil.GetGVRFromGVK(loc.APIVersion, loc.Kind)
	if err != nil {
		return nil, err
	}
	return client.DynamicClient.Resource(resourceGVR).Namespace(loc.Namespace).Get(ctx, loc.Name, metav1.GetOptions{})
}

// GetYAMLForResource returns the yaml byte array for a given cluster
func (i *InsightManager) GetYAMLForResource(ctx context.Context, client *multicluster.MultiClusterClient, loc *core.Locator) ([]byte, error) {
	obj, err := i.GetResource(ctx, client, loc)
	if err != nil {
		return nil, err
	}
	return k8syaml.Marshal(obj.Object)
}