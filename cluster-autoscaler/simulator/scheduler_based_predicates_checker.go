/*
Copyright 2016 The Kubernetes Authors.

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

package simulator

import (
	"context"
	"fmt"
	"time"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	kube_client "k8s.io/client-go/kubernetes"
	v1listers "k8s.io/client-go/listers/core/v1"

	scheduler_apis_config "k8s.io/kubernetes/pkg/scheduler/apis/config"
	scheduler_plugins "k8s.io/kubernetes/pkg/scheduler/framework/plugins"
	scheduler_framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	scheduler_listers "k8s.io/kubernetes/pkg/scheduler/listers"
	scheduler_nodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	scheduler_volumebinder "k8s.io/kubernetes/pkg/scheduler/volumebinder"
	// We need to import provider to initialize default scheduler.
	"k8s.io/kubernetes/pkg/scheduler/algorithmprovider"
)

// SchedulerBasedPredicateChecker checks whether all required predicates pass for given Pod and Node.
// The verification is done by calling out to scheduler code.
type SchedulerBasedPredicateChecker struct {
	framework              scheduler_framework.Framework
	delegatingSharedLister *DelegatingSchedulerSharedLister
	nodeLister             v1listers.NodeLister
	podLister              v1listers.PodLister
}

// DelegatingSchedulerSharedLister is an implementation of scheduler.SharedLister which
// passes logic to delegate. Delegate can be updated.
type DelegatingSchedulerSharedLister struct {
	delegate scheduler_listers.SharedLister
}

// NewDelegatingSchedulerSharedLister creates new NewDelegatingSchedulerSharedLister
func NewDelegatingSchedulerSharedLister(delegate scheduler_listers.SharedLister) *DelegatingSchedulerSharedLister {
	return &DelegatingSchedulerSharedLister{
		delegate: delegate,
	}
}

// Pods returns a PodLister
func (lister *DelegatingSchedulerSharedLister) Pods() scheduler_listers.PodLister {
	return lister.delegate.Pods()
}

// NodeInfos returns a NodeInfoLister.
func (lister *DelegatingSchedulerSharedLister) NodeInfos() scheduler_listers.NodeInfoLister {
	return lister.delegate.NodeInfos()
}

// UpdateDelegate updates the delegate
func (lister *DelegatingSchedulerSharedLister) UpdateDelegate(delegate scheduler_listers.SharedLister) {
	lister.delegate = delegate
}

// NewSchedulerBasedPredicateChecker builds scheduler based PredicateChecker.
func NewSchedulerBasedPredicateChecker(kubeClient kube_client.Interface, stop <-chan struct{}) (*SchedulerBasedPredicateChecker, error) {
	informerFactory := informers.NewSharedInformerFactory(kubeClient, 0)
	providerRegistry := algorithmprovider.NewRegistry(1) // 1 here is hardPodAffinityWeight not relevant for CA
	config := providerRegistry[scheduler_apis_config.SchedulerDefaultProviderName]
	sharedLister := NewDelegatingSchedulerSharedLister(NewEmptySnapshot())

	volumeBinder := scheduler_volumebinder.NewVolumeBinder(
		kubeClient,
		informerFactory.Core().V1().Nodes(),
		informerFactory.Storage().V1().CSINodes(),
		informerFactory.Core().V1().PersistentVolumeClaims(),
		informerFactory.Core().V1().PersistentVolumes(),
		informerFactory.Storage().V1().StorageClasses(),
		time.Duration(10)*time.Second,
	)

	framework, err := scheduler_framework.NewFramework(
		scheduler_plugins.NewInTreeRegistry(),
		config.FrameworkPlugins,
		config.FrameworkPluginConfig,
		scheduler_framework.WithInformerFactory(informerFactory),
		scheduler_framework.WithSnapshotSharedLister(sharedLister),
		scheduler_framework.WithVolumeBinder(volumeBinder),
	)

	if err != nil {
		return nil, fmt.Errorf("couldn't create scheduler framework; %v", err)
	}

	checker := &SchedulerBasedPredicateChecker{
		framework:              framework,
		delegatingSharedLister: sharedLister,
		nodeLister:             informerFactory.Core().V1().Nodes().Lister(),
		podLister:              informerFactory.Core().V1().Pods().Lister(),
	}

	// this MUST be called after all the informers/listers are acquired via the
	// informerFactory....Lister()/informerFactory....Informer() methods
	informerFactory.Start(stop)

	return checker, nil
}

// SnapshotClusterState updates cluster snapshot used by the predicate checker.
// It should be called every CA loop iteration.
func (p *SchedulerBasedPredicateChecker) SnapshotClusterState() error {
	nodes, err := p.nodeLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("could not list Nodes; %v", err)
	}
	pods, err := p.podLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("could not list Pods; %v", err)
	}
	nodeInfoMap := CreateNodeInfoMap(pods, nodes)
	newSnapshot := NewSnapshot(nodeInfoMap)
	p.delegatingSharedLister.UpdateDelegate(newSnapshot)
	return nil
}

// FitsAny checks if the given pod can be place on any of the given nodes.
func (p *SchedulerBasedPredicateChecker) FitsAny(pod *apiv1.Pod, nodeInfos map[string]*scheduler_nodeinfo.NodeInfo) (string, error) {
	state := scheduler_framework.NewCycleState()
	preFilterStatus := p.framework.RunPreFilterPlugins(context.TODO(), state, pod)
	if !preFilterStatus.IsSuccess() {
		return "", fmt.Errorf("error running pre filter plugins for pod %s; %s", pod.Name, preFilterStatus.Message())
	}

	for name, nodeInfo := range nodeInfos {
		// Be sure that the node is schedulable.
		if nodeInfo.Node().Spec.Unschedulable {
			continue
		}

		filterStatuses := p.framework.RunFilterPlugins(context.TODO(), state, pod, nodeInfo)
		ok := true
		for _, filterStatus := range filterStatuses {
			if !filterStatus.IsSuccess() {
				ok = false
				break
			}
		}
		if ok {
			return name, nil
		}
	}
	return "", fmt.Errorf("cannot put pod %s on any node", pod.Name)
}

// CheckPredicates checks if the given pod can be placed on the given node.
func (p *SchedulerBasedPredicateChecker) CheckPredicates(pod *apiv1.Pod, nodeInfo *scheduler_nodeinfo.NodeInfo) PredicateError {
	state := scheduler_framework.NewCycleState()
	preFilterStatus := p.framework.RunPreFilterPlugins(context.TODO(), state, pod)
	if !preFilterStatus.IsSuccess() {
		return NewPredicateError(
			InternalPredicateError,
			"",
			preFilterStatus.Message(),
			preFilterStatus.Reasons(),
			emptyString)
	}

	filterStatuses := p.framework.RunFilterPlugins(context.TODO(), state, pod, nodeInfo)
	for filterName, filterStatus := range filterStatuses {
		if !filterStatus.IsSuccess() {
			if filterStatus.IsUnschedulable() {
				return NewPredicateError(
					NotSchedulablePredicateError,
					filterName,
					filterStatus.Message(),
					filterStatus.Reasons(),
					p.buildDebugInfo(filterName, nodeInfo))
			}
			return NewPredicateError(
				InternalPredicateError,
				filterName,
				filterStatus.Message(),
				filterStatus.Reasons(),
				p.buildDebugInfo(filterName, nodeInfo))
		}
	}
	return nil
}

func (p *SchedulerBasedPredicateChecker) buildDebugInfo(filterName string, nodeInfo *scheduler_nodeinfo.NodeInfo) func() string {
	switch filterName {
	case "TaintToleration":
		taints := nodeInfo.Node().Spec.Taints
		return func() string {
			return fmt.Sprintf("taints on node: %#v", taints)
		}
	default:
		return emptyString
	}
}
