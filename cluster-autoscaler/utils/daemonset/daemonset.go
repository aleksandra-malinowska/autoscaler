/*
Copyright 2017 The Kubernetes Authors.

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

package daemonset

import (
	"fmt"
	"math/rand"

	"k8s.io/autoscaler/cluster-autoscaler/simulator"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
)

// GetDaemonSetPodsForNode returns daemonset nodes for the given pod.
func GetDaemonSetPodsForNode(nodeInfo *schedulernodeinfo.NodeInfo, daemonsets []*appsv1.DaemonSet, predicateChecker simulator.PredicateChecker) ([]*apiv1.Pod, error) {
	result := make([]*apiv1.Pod, 0)

	// here we can use empty snapshot
	clusterSnapshot := simulator.NewBasicClusterSnapshot()

	// add a node with pods
	// TODO(scheduler framework migration) are we expecting any pods on passed nodeInfo?
	if err := clusterSnapshot.AddNodeWithPods(nodeInfo.Node(), nodeInfo.Pods()); err != nil {
		return nil, err
	}

	for _, ds := range daemonsets {
		pod := newPod(ds, nodeInfo.Node().Name)
		if err := predicateChecker.CheckPredicates(clusterSnapshot, pod, simulator.FakeNodeInfoForNodeName(nodeInfo.Node().Name)); err == nil {
			result = append(result, pod)
		}
	}
	return result, nil
}

func newPod(ds *appsv1.DaemonSet, nodeName string) *apiv1.Pod {
	newPod := &apiv1.Pod{Spec: ds.Spec.Template.Spec, ObjectMeta: ds.Spec.Template.ObjectMeta}
	newPod.Namespace = ds.Namespace
	newPod.Name = fmt.Sprintf("%s-pod-%d", ds.Name, rand.Int63())
	newPod.Spec.NodeName = nodeName
	return newPod
}
