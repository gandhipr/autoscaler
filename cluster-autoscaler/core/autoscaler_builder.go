/*
Copyright 2020 The Kubernetes Authors.

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

package core

import (
	"flag"
	"strconv"
	"strings"
	"time"

	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	cloudBuilder "k8s.io/autoscaler/cluster-autoscaler/cloudprovider/builder"
	"k8s.io/autoscaler/cluster-autoscaler/config"
	"k8s.io/autoscaler/cluster-autoscaler/config/dynamic"
	"k8s.io/autoscaler/cluster-autoscaler/context"
	"k8s.io/autoscaler/cluster-autoscaler/core/scaledown/pdb"
	"k8s.io/autoscaler/cluster-autoscaler/core/scaleup"
	"k8s.io/autoscaler/cluster-autoscaler/debuggingsnapshot"
	"k8s.io/autoscaler/cluster-autoscaler/estimator"
	"k8s.io/autoscaler/cluster-autoscaler/expander"
	"k8s.io/autoscaler/cluster-autoscaler/expander/factory"
	ca_processors "k8s.io/autoscaler/cluster-autoscaler/processors"
	"k8s.io/autoscaler/cluster-autoscaler/simulator/clustersnapshot"
	"k8s.io/autoscaler/cluster-autoscaler/simulator/drainability/rules"
	"k8s.io/autoscaler/cluster-autoscaler/simulator/options"
	"k8s.io/autoscaler/cluster-autoscaler/simulator/predicatechecker"
	"k8s.io/autoscaler/cluster-autoscaler/utils/backoff"
	"k8s.io/autoscaler/cluster-autoscaler/utils/errors"
	"k8s.io/autoscaler/cluster-autoscaler/utils/units"
	"k8s.io/client-go/informers"
	kube_client "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// AutoscalerBuilder builds an instance of Autoscaler
type AutoscalerBuilder interface {
	SetDynamicConfig(config dynamic.Config) AutoscalerBuilder
	Build() (Autoscaler, errors.AutoscalerError)
}

// AutoscalerBuilderImpl wraps `AutoscalingOptions` given at startup and
// `dynamic.Config` read on demand from the configmap and
// builds a new autoscaler type
type AutoscalerBuilderImpl struct {
	autoscalingOptions     config.AutoscalingOptions
	dynamicConfig          *dynamic.Config
	AutoscalingKubeClients *context.AutoscalingKubeClients
	KubeClient             kube_client.Interface
	EventsKubeClient       kube_client.Interface
	InformerFactory        informers.SharedInformerFactory
	PredicateChecker       predicatechecker.PredicateChecker
	ClusterSnapshot        clustersnapshot.ClusterSnapshot
	Processors             *ca_processors.AutoscalingProcessors
	CloudProvider          cloudprovider.CloudProvider
	ExpanderStrategy       expander.Strategy
	EstimatorBuilder       estimator.EstimatorBuilder
	Backoff                backoff.Backoff
	DebuggingSnapshotter   debuggingsnapshot.DebuggingSnapshotter
	RemainingPdbTracker    pdb.RemainingPdbTracker
	ScaleUpOrchestrator    scaleup.Orchestrator
	DeleteOptions          options.NodeDeleteOptions
	DrainabilityRules      rules.Rules
}

// NewAutoscalerBuilder builds an AutoscalerBuilder from required parameters
func NewAutoscalerBuilder(opts config.AutoscalingOptions, predicateChecker predicatechecker.PredicateChecker,
	clusterSnapshot clustersnapshot.ClusterSnapshot, autoscalingKubeClients *context.AutoscalingKubeClients,
	kubeClient kube_client.Interface, processors *ca_processors.AutoscalingProcessors,
	cloudProvider cloudprovider.CloudProvider, expanderStrategy expander.Strategy,
	estimatorBuilder estimator.EstimatorBuilder, backoff backoff.Backoff,
	snapshotter debuggingsnapshot.DebuggingSnapshotter, remainingPdbTracker pdb.RemainingPdbTracker,
	scaleUpOrchestrator scaleup.Orchestrator, deleteOptions options.NodeDeleteOptions,
	drainabilityRules rules.Rules) *AutoscalerBuilderImpl {

	return &AutoscalerBuilderImpl{
		autoscalingOptions:     opts,
		AutoscalingKubeClients: autoscalingKubeClients,
		KubeClient:             kubeClient,
		PredicateChecker:       predicateChecker,
		ClusterSnapshot:        clusterSnapshot,
		Processors:             processors,
		CloudProvider:          cloudProvider,
		ExpanderStrategy:       expanderStrategy,
		EstimatorBuilder:       estimatorBuilder,
		Backoff:                backoff,
		DebuggingSnapshotter:   snapshotter,
		RemainingPdbTracker:    remainingPdbTracker,
		ScaleUpOrchestrator:    scaleUpOrchestrator,
		DeleteOptions:          deleteOptions,
	}
}

// SetDynamicConfig sets the fetched dynamic config
func (b *AutoscalerBuilderImpl) SetDynamicConfig(config dynamic.Config) AutoscalerBuilder {
	b.dynamicConfig = &config
	return b
}

// Build returns an autoscaler instance
func (b *AutoscalerBuilderImpl) Build() (Autoscaler, errors.AutoscalerError) {
	options := b.autoscalingOptions
	if b.dynamicConfig != nil {
		c := *(b.dynamicConfig)
		options.NodeGroups = c.NodeGroupSpecStrings()
		klog.V(3).Infof("Updating nodegroups to: %s", c.NodeGroupSpecStrings())
		b.CloudProvider = cloudBuilder.NewCloudProvider(options, b.InformerFactory)
		options = b.updateAutoScalerProfile(options)
		klog.V(3).Infof("Updating autoscaling options to: %v", options)
		b.CloudProvider = cloudBuilder.NewCloudProvider(options, b.InformerFactory)
		expanderFactory := factory.NewFactory()
		expanderFactory.RegisterDefaultExpanders(b.CloudProvider, b.AutoscalingKubeClients, b.KubeClient, options.ConfigNamespace, options.GRPCExpanderCert, options.GRPCExpanderURL)
		expanderStrategy, err := expanderFactory.Build(strings.Split(options.ExpanderNames, ","))
		if err == nil {
			b.ExpanderStrategy = expanderStrategy
		}

	}

	return NewStaticAutoscaler(options, b.PredicateChecker, b.ClusterSnapshot, b.AutoscalingKubeClients,
		b.Processors,
		b.CloudProvider,
		b.ExpanderStrategy,
		b.EstimatorBuilder,
		b.Backoff,
		b.DebuggingSnapshotter,
		b.RemainingPdbTracker,
		b.ScaleUpOrchestrator,
		b.DeleteOptions,
		b.DrainabilityRules), nil
}

// updateAutoScalerProfile updated config.AutoscalingOptions based on the provided autoscalerProfile
func (b *AutoscalerBuilderImpl) updateAutoScalerProfile(autoscalingOptions config.AutoscalingOptions) config.AutoscalingOptions {
	c := *(b.dynamicConfig)
	autoScalerProfile := c.AutoScalerProfile

	if autoScalerProfile.ScaleDownDelayAfterAdd != "" {
		scaleDownDelayAfterAdd, _ := time.ParseDuration(autoScalerProfile.ScaleDownDelayAfterAdd)
		autoscalingOptions.ScaleDownDelayAfterAdd = scaleDownDelayAfterAdd
	}

	if autoScalerProfile.ScaleDownDelayAfterDelete != "" {
		scaleDownDelayAfterDelete, _ := time.ParseDuration(autoScalerProfile.ScaleDownDelayAfterDelete)
		autoscalingOptions.ScaleDownDelayAfterDelete = scaleDownDelayAfterDelete
	}

	if autoScalerProfile.ScaleDownDelayAfterFailure != "" {
		scaleDownDelayAfterFailure, _ := time.ParseDuration(autoScalerProfile.ScaleDownDelayAfterFailure)
		autoscalingOptions.ScaleDownDelayAfterFailure = scaleDownDelayAfterFailure
	}

	if autoScalerProfile.ScaleDownUnneededTime != "" {
		scaleDownUnneededTime, _ := time.ParseDuration(autoScalerProfile.ScaleDownUnneededTime)
		autoscalingOptions.NodeGroupDefaults.ScaleDownUnneededTime = scaleDownUnneededTime
	}

	if autoScalerProfile.ScaleDownUnreadyTime != "" {
		scaleDownUnreadyTime, _ := time.ParseDuration(autoScalerProfile.ScaleDownUnreadyTime)
		autoscalingOptions.NodeGroupDefaults.ScaleDownUnreadyTime = scaleDownUnreadyTime
	}

	if autoScalerProfile.MaxCloudProviderNodeDeletionTime != "" {
		maxCloudProviderNodeDeletionTime, _ := time.ParseDuration(autoScalerProfile.MaxCloudProviderNodeDeletionTime)
		autoscalingOptions.MaxCloudProviderNodeDeletionTime = maxCloudProviderNodeDeletionTime
	}

	if autoScalerProfile.MaxNodeProvisionTime != "" {
		maxNodeProvisionTime, _ := time.ParseDuration(autoScalerProfile.MaxNodeProvisionTime)
		autoscalingOptions.MaxNodeProvisionTime = maxNodeProvisionTime
	}

	if autoScalerProfile.EnableGetVmss != "" {
		enableGetVmss, _ := strconv.ParseBool(autoScalerProfile.EnableGetVmss)
		autoscalingOptions.EnableGetVmss = enableGetVmss
	}

	if autoScalerProfile.GetVmssSizeRefreshPeriod != "" {
		getVmssSizeRefreshPeriod, _ := time.ParseDuration(autoScalerProfile.GetVmssSizeRefreshPeriod)
		autoscalingOptions.GetVmssSizeRefreshPeriod = getVmssSizeRefreshPeriod
	}
	
	if autoScalerProfile.EnableForceDelete != "" {
		enableForceDelete, _ := strconv.ParseBool(autoScalerProfile.EnableForceDelete)
		autoscalingOptions.EnableForceDelete = enableForceDelete
	}

	if autoScalerProfile.EnableDynamicInstanceList != "" {
		enableDynamicInstanceList, _ := strconv.ParseBool(autoScalerProfile.EnableDynamicInstanceList)
		autoscalingOptions.EnableDynamicInstanceList = enableDynamicInstanceList
	}

	if autoScalerProfile.MinCpu != "" {
		minCores, _ := strconv.ParseInt(autoScalerProfile.MinCpu, 10, 64)
		autoscalingOptions.MinCoresTotal = minCores
	}
	if autoScalerProfile.MaxCpu != "" {
		maxCores, _ := strconv.ParseInt(autoScalerProfile.MaxCpu, 10, 64)
		autoscalingOptions.MaxCoresTotal = maxCores
	}
	if autoScalerProfile.MinMemory != "" {
		minMemory, _ := strconv.ParseInt(autoScalerProfile.MinMemory, 10, 64)
		autoscalingOptions.MinMemoryTotal = minMemory * units.GiB
	}
	if autoScalerProfile.MaxMemory != "" {
		maxMemory, _ := strconv.ParseInt(autoScalerProfile.MaxMemory, 10, 64)
		autoscalingOptions.MaxMemoryTotal = maxMemory * units.GiB
	}

	if autoScalerProfile.ScaleDownUtilizationThreshold != "" {
		scaleDownUtilizationThreshold, _ := strconv.ParseFloat(autoScalerProfile.ScaleDownUtilizationThreshold, 64)
		autoscalingOptions.NodeGroupDefaults.ScaleDownUtilizationThreshold = scaleDownUtilizationThreshold
	}

	if autoScalerProfile.MaxGracefulTerminationSec != "" {
		maxGracefulTerminationSec, _ := strconv.Atoi(autoScalerProfile.MaxGracefulTerminationSec)
		autoscalingOptions.MaxGracefulTerminationSec = maxGracefulTerminationSec
	}

	if autoScalerProfile.BalanceSimilarNodeGroups != "" {
		balanceSimilarNodeGroups, _ := strconv.ParseBool(autoScalerProfile.BalanceSimilarNodeGroups)
		autoscalingOptions.BalanceSimilarNodeGroups = balanceSimilarNodeGroups
	}

	if autoScalerProfile.Expander != "" {
		autoscalingOptions.ExpanderNames = autoScalerProfile.Expander
	}

	if autoScalerProfile.NewPodScaleUpDelay != "" {
		newPodScaleUpDelay, _ := time.ParseDuration(autoScalerProfile.NewPodScaleUpDelay)
		autoscalingOptions.NewPodScaleUpDelay = newPodScaleUpDelay
	}

	if autoScalerProfile.MaxEmptyBulkDelete != "" {
		maxEmptyBulkDelete, _ := strconv.Atoi(autoScalerProfile.MaxEmptyBulkDelete)
		autoscalingOptions.MaxEmptyBulkDelete = maxEmptyBulkDelete
	}

	if autoScalerProfile.OkTotalUnreadyCount != "" {
		okTotalUnreadyCount, _ := strconv.Atoi(autoScalerProfile.OkTotalUnreadyCount)
		autoscalingOptions.OkTotalUnreadyCount = okTotalUnreadyCount
	}

	if autoScalerProfile.MaxTotalUnreadyPercentage != "" {
		maxTotalUnreadyPercentage, _ := strconv.ParseFloat(autoScalerProfile.MaxTotalUnreadyPercentage, 64)
		autoscalingOptions.MaxTotalUnreadyPercentage = maxTotalUnreadyPercentage
	}

	if autoScalerProfile.DaemonSetEvictionForEmptyNodes != "" {
		daemonSetEvictionForEmptyNodes, _ := strconv.ParseBool(autoScalerProfile.DaemonSetEvictionForEmptyNodes)
		autoscalingOptions.DaemonSetEvictionForEmptyNodes = daemonSetEvictionForEmptyNodes
	}

	if autoScalerProfile.DaemonSetEvictionForOccupiedNodes != "" {
		daemonSetEvictionForOccupiedNodes, _ := strconv.ParseBool(autoScalerProfile.DaemonSetEvictionForOccupiedNodes)
		autoscalingOptions.DaemonSetEvictionForOccupiedNodes = daemonSetEvictionForOccupiedNodes
	}

	if autoScalerProfile.SkipNodesWithLocalStorage != "" {
		flag.Set("skip-nodes-with-local-storage", autoScalerProfile.SkipNodesWithLocalStorage)
	}

	if autoScalerProfile.SkipNodesWithSystemPods != "" {
		flag.Set("skip-nodes-with-system-pods", autoScalerProfile.SkipNodesWithSystemPods)
	}

	if autoScalerProfile.ScanInterval != "" {
		flag.Set("scan-interval", autoScalerProfile.ScanInterval)
	}

	return autoscalingOptions
}
