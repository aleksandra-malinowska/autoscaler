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

package builder

import (
	"os"

	"github.com/golang/glog"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/aws"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/azure"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/gce"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/kubemark"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kube_client "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
)

// CloudProviderBuilder builds a cloud provider from all the necessary parameters including the name of a cloud provider e.g. aws, gce
// and the path to a config file
type CloudProviderBuilder struct {
	cloudProviderFlag string
	cloudConfig       string
}

// NewCloudProviderBuilder builds a new builder from static settings
func NewCloudProviderBuilder(cloudProviderFlag string, cloudConfig string) CloudProviderBuilder {
	return CloudProviderBuilder{
		cloudProviderFlag: cloudProviderFlag,
		cloudConfig:       cloudConfig,
	}
}

// Build a cloud provider from static settings contained in the builder and dynamic settings passed via args
func (b CloudProviderBuilder) Build(discoveryOpts cloudprovider.NodeGroupDiscoveryOptions) cloudprovider.CloudProvider {
	var err error
	var cloudProvider cloudprovider.CloudProvider

	nodeGroupsFlag := discoveryOpts.NodeGroupSpecs

	if b.cloudProviderFlag == "gce" {
		// GCE Manager
		var gceManager *gce.GceManager
		var gceError error
		if b.cloudConfig != "" {
			config, fileErr := os.Open(b.cloudConfig)
			if fileErr != nil {
				glog.Fatalf("Couldn't open cloud provider configuration %s: %#v", b.cloudConfig, err)
			}
			defer config.Close()
			gceManager, gceError = gce.CreateGceManager(config)
		} else {
			gceManager, gceError = gce.CreateGceManager(nil)
		}
		if gceError != nil {
			glog.Fatalf("Failed to create GCE Manager: %v", err)
		}
		cloudProvider, err = gce.BuildGceCloudProvider(gceManager, nodeGroupsFlag)
		if err != nil {
			glog.Fatalf("Failed to create GCE cloud provider: %v", err)
		}

		return cloudProvider
	}

	if b.cloudProviderFlag == "aws" {
		var awsManager *aws.AwsManager
		var awsError error
		if b.cloudConfig != "" {
			config, fileErr := os.Open(b.cloudConfig)
			if fileErr != nil {
				glog.Fatalf("Couldn't open cloud provider configuration %s: %#v", b.cloudConfig, err)
			}
			defer config.Close()
			awsManager, awsError = aws.CreateAwsManager(config)
		} else {
			awsManager, awsError = aws.CreateAwsManager(nil)
		}
		if awsError != nil {
			glog.Fatalf("Failed to create AWS Manager: %v", err)
		}
		cloudProvider, err = aws.BuildAwsCloudProvider(awsManager, discoveryOpts)
		if err != nil {
			glog.Fatalf("Failed to create AWS cloud provider: %v", err)
		}

		return cloudProvider
	}

	if b.cloudProviderFlag == "azure" {
		var azureManager *azure.AzureManager
		var azureError error
		if b.cloudConfig != "" {
			glog.Info("Creating Azure Manager using cloud-config file: %v", b.cloudConfig)
			config, fileErr := os.Open(b.cloudConfig)
			if fileErr != nil {
				glog.Fatalf("Couldn't open cloud provider configuration %s: %#v", b.cloudConfig, err)
			}
			defer config.Close()
			azureManager, azureError = azure.CreateAzureManager(config)
		} else {
			glog.Info("Creating Azure Manager with default configuration.")
			azureManager, azureError = azure.CreateAzureManager(nil)
		}
		if azureError != nil {
			glog.Fatalf("Failed to create Azure Manager: %v", err)
		}
		cloudProvider, err = azure.BuildAzureCloudProvider(azureManager, nodeGroupsFlag)
		if err != nil {
			glog.Fatalf("Failed to create Azure cloud provider: %v", err)
		}

		return cloudProvider
	}

	if b.cloudProviderFlag == kubemark.ProviderName {
		glog.Infof("Building kubemark cloud provider.")
		var kubemarkManager *kubemark.KubemarkManager
		externalConfig, err := rest.InClusterConfig()
		if err != nil {
			glog.Fatalf("Failed to get kubeclient config for external cluster: %v", err)
		}

		kubemarkConfig, err := clientcmd.BuildConfigFromFlags("", "/kubeconfig/cluster_autoscaler.kubeconfig")
		if err != nil {
			glog.Fatalf("Failed to get kubeclient config for kubemark cluster: %v", err)
		}

		externalClient := kube_client.NewForConfigOrDie(externalConfig)
		kubemarkClient := kube_client.NewForConfigOrDie(kubemarkConfig)

		stop := make(chan struct{})
		kubemarkManager, err = kubemark.CreateKubemarkManager(externalClient, kubemarkClient, stop)
		if err != nil {
			glog.Fatalf("Failed to create Kubemark cloud provider: %v", err)
		}

		cloudProvider, err = kubemark.BuildKubemarkCloudProvider(kubemarkManager, nodeGroupsFlag)
		if err != nil {
			glog.Fatalf("Failed to create Kubemark cloud provider: %v", err)
		}
		return cloudProvider
	}
	glog.Fatalf("Unexpected cloud provider name %s, bye!", b.cloudProviderFlag)
	return nil
}
