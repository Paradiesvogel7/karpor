/*
Copyright The Karbour Authors.

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

package relationship

import (
	"context"
	"fmt"
	"os"
	"reflect"

	topologyutil "github.com/KusionStack/karbour/pkg/util/topology"
	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"

	yaml "gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

func (rgn ResourceGraphNode) GetHash() string {
	return rgn.Group + "/" + rgn.Version + "." + rgn.Kind + ":" + rgn.Namespace + "." + rgn.Name
}

func (rgn RelationshipGraphNode) GetHash() string {
	return rgn.Group + "." + rgn.Version + "." + rgn.Kind
}

func (r Relationship) GetHash() string {
	return r.Group + "." + r.Version + "." + r.Kind
}

func (rgn RelationshipGraphNode) ConvertToMap() map[string]string {
	m := make(map[string]string, 0)
	for _, p := range rgn.Parent {
		parentHash := p.GetHash()
		if _, ok := m[parentHash]; !ok {
			m[parentHash] = "parent"
		}
	}
	for _, c := range rgn.Children {
		childHash := c.GetHash()
		if _, ok := m[childHash]; !ok {
			m[childHash] = "child"
		}
	}
	return m
}

// FindNodeByGVK locates the Node by GVK on a RelationshipGraph. Used to locate parent and child nodes when building the relationship graph
func (rg RelationshipGraph) FindNodeByGVK(group, version, kind string) (*RelationshipGraphNode, error) {
	for _, item := range rg.RelationshipNodes {
		if item.Group == group && item.Version == version && item.Kind == kind {
			return item, nil
		}
	}
	// If not found, return a new one so it can be inserted properly
	r := RelationshipGraphNode{
		Group:    group,
		Version:  version,
		Kind:     kind,
		Parent:   make([]*Relationship, 0),
		Children: make([]*Relationship, 0),
	}
	nodeNotFoundErr := fmt.Errorf("node not found by GVK: Group: %s, Version: %s, Kind: %s", group, version, kind)
	return &r, nodeNotFoundErr
}

// FindNodeOnGraph locates the Node on a built relationship graph. Used to locate parent and child nodes when traversing the relationship graph
func FindNodeOnGraph(g graph.Graph[string, RelationshipGraphNode], group, version, kind string) (*RelationshipGraphNode, error) {
	klog.Infof("Locating node on relationship graph: Group: %s, Version: %s, Kind: %s\n", group, version, kind)
	vertexHash := group + "." + version + "." + kind
	vertex, err := g.Vertex(vertexHash)
	if err != nil {
		return nil, err
	}
	return &vertex, nil
}

// BuildBuiltinRelationshipGraph returns the relationship graph built from the YAML describing resource relationships
func BuildBuiltinRelationshipGraph(ctx context.Context, client *dynamic.DynamicClient) (graph.Graph[string, RelationshipGraphNode], *RelationshipGraph, error) {
	r := RelationshipGraph{}
	yamlFile, err := os.ReadFile("relationship.yaml")
	if err != nil {
		klog.Infof("yamlFile.Get err #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, &r)
	if err != nil {
		klog.Fatalf("Unmarshal: %v", err)
	}

	// Process relationships between parent and child
	// TODO: Think about whether two-way relationship need to be enforced and explicitly declared.
	// Right now they are automatically derived
	for _, ri := range r.RelationshipNodes {
		for _, c := range ri.Children {
			c.ParentNode = ri
			c.ChildNode, err = r.FindNodeByGVK(c.Group, c.Version, c.Kind)
			if err != nil {
				r.RelationshipNodes = append(r.RelationshipNodes, c.ChildNode)
			}
			// Append the same parent-child relationship to child's parent node if it does not exist already
			c.ChildNode.Parent, err = InsertIfNotExist(c.ChildNode.Parent, *c, "parent")
			if err != nil {
				return nil, nil, err
			}
		}
		for _, p := range ri.Parent {
			p.ChildNode = ri
			p.ParentNode, err = r.FindNodeByGVK(p.Group, p.Version, p.Kind)
			if err != nil {
				r.RelationshipNodes = append(r.RelationshipNodes, p.ParentNode)
			}
			// Append the same parent-child relationship to parent's child node if it does not exist already
			p.ParentNode.Children, err = InsertIfNotExist(p.ParentNode.Children, *p, "child")
			if err != nil {
				return nil, nil, err
			}
		}
	}

	// Initialize the relationship graph based on GVK
	relationshipGraphNodeHash := func(rgn RelationshipGraphNode) string {
		return rgn.Group + "." + rgn.Version + "." + rgn.Kind
	}
	g := graph.New(relationshipGraphNodeHash, graph.Directed(), graph.PreventCycles())
	// Add Vertices
	for _, node := range r.RelationshipNodes {
		klog.Infof("Adding Vertex: %s\n", node.GetHash())
		_ = g.AddVertex(*node)
	}
	// Add Edges, requires all vertices to be present
	for _, node := range r.RelationshipNodes {
		for _, childRelation := range node.Children {
			klog.Infof("Adding or updating Edge from %s to %s with type %s\n", node.GetHash(), childRelation.ChildNode.GetHash(), childRelation.Type)
			if err := g.AddEdge(node.GetHash(), childRelation.ChildNode.GetHash(), graph.EdgeAttribute("type", childRelation.Type)); err != graph.ErrEdgeAlreadyExists && err != nil {
				panic(err)
			}
		}
		// Prevent duplicate edge
		for _, parentRelation := range node.Parent {
			klog.Infof("Adding or updating Edge from %s to %s with type %s\n", parentRelation.ParentNode.GetHash(), node.GetHash(), parentRelation.Type)
			if err := g.AddEdge(parentRelation.ParentNode.GetHash(), node.GetHash(), graph.EdgeAttribute("type", parentRelation.Type)); err != graph.ErrEdgeAlreadyExists && err != nil {
				panic(err)
			}
		}
	}

	klog.Infof("Built-in graph completed.")

	// Draw graph
	// TODO: This is drawn on the server side, not needed eventually
	file, _ := os.Create("./relationship.gv")
	_ = draw.DOT(g, file)

	return g, &r, nil
}

// BuildRelationshipGraph builds the complete relationship graph including the built-in one and customer-specified one
func BuildRelationshipGraph(ctx context.Context, client *dynamic.DynamicClient) (graph.Graph[string, RelationshipGraphNode], *RelationshipGraph, error) {
	res, rg, _ := BuildBuiltinRelationshipGraph(ctx, client)
	// TODO: Also include customized relationship graph
	return res, rg, nil
}

// InsertIfNotExist inserts relation into relationList only if it does not exist already
// This is used to auto-generate two-way relationships when they are not declared explicitly
func InsertIfNotExist(relationList []*Relationship, relation Relationship, relationshipDirection string) ([]*Relationship, error) {
	relationToInsert := &relation
	if relationshipDirection == "parent" {
		// Append parent-child relationship to child's parent also
		relationToInsert.Group = relation.ParentNode.Group
		relationToInsert.Version = relation.ParentNode.Version
		relationToInsert.Kind = relation.ParentNode.Kind
		relationToInsert.ParentNode = relation.ParentNode
		relationToInsert.ChildNode = relation.ChildNode
		relationToInsert.AutoGenerated = true
	} else if relationshipDirection == "child" {
		// Append parent-child relationship to parent's children also
		relationToInsert.Group = relation.ChildNode.Group
		relationToInsert.Version = relation.ChildNode.Version
		relationToInsert.Kind = relation.ChildNode.Kind
		relationToInsert.ParentNode = relation.ParentNode
		relationToInsert.ChildNode = relation.ChildNode
		relationToInsert.AutoGenerated = true
	}
	// Append only if the relationship does not exist already
	for _, r := range relationList {
		if RelationshipEquals(r, &relation) {
			klog.Infof("Relationship between %s and %s already exists. Skipping...", r.Kind, relation.Kind)
			return relationList, nil
		}
	}
	relationList = append(relationList, relationToInsert)
	return relationList, nil
}

// RelationshipEquals returns true if two relationships are equal
func RelationshipEquals(r, relation *Relationship) bool {
	return r.Group == relation.Group && r.Version == relation.Version && r.Kind == relation.Kind && r.Type == relation.Type && reflect.DeepEqual(r.JSONPath, relation.JSONPath)
}

// CountRelationshipGraph returns the same RelationshipGraph with the count for each resource
func (rg *RelationshipGraph) CountRelationshipGraph(ctx context.Context, dynamicClient *dynamic.DynamicClient, discoveryClient *discovery.DiscoveryInterface, countNamespace string) (*RelationshipGraph, error) {
	for _, node := range rg.RelationshipNodes {
		var resList *unstructured.UnstructuredList
		resGVR, err := topologyutil.GetGVRFromGVK(schema.GroupVersion{Group: node.Group, Version: node.Version}.String(), node.Kind)
		if err != nil {
			return rg, err
		}
		if countNamespace == "" {
			resList, err = dynamicClient.Resource(resGVR).List(ctx, metav1.ListOptions{})
		} else if countNamespace != "" && GVRNamespaced(resGVR, *discoveryClient) {
			resList, err = dynamicClient.Resource(resGVR).Namespace(countNamespace).List(ctx, metav1.ListOptions{})
		} else {
			continue
		}
		if err != nil {
			return rg, err
		}
		resCount := len(resList.Items)
		klog.Infof("Counted resources for Vertex %s: %d\n", node.GetHash(), resCount)
		node.ResourceCount = resCount
	}
	return rg, nil
}

// GVRNamespaced returns true if a given GVR is namespaced based on the result of discovery client
func GVRNamespaced(gvr schema.GroupVersionResource, discoveryClient discovery.DiscoveryInterface) bool {
	apiResourceList, err := discoveryClient.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
	if err != nil {
		return false
	}
	// Iterate over the APIResources to find the one that matches the Resource and determine if it is namespaced
	for _, apiResource := range apiResourceList.APIResources {
		if apiResource.Name == gvr.Resource {
			if apiResource.Namespaced {
				return true
			} else {
				return false
			}
		}
	}
	return false
}