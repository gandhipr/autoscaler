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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/autoscaler/cluster-autoscaler/config"
	"k8s.io/autoscaler/cluster-autoscaler/config/dynamic"
)

func TestUpdateAutoScalerProfile(t *testing.T) {
	b := &AutoscalerBuilderImpl{
		dynamicConfig: &dynamic.Config{
			AutoScalerProfile: dynamic.AutoScalerProfile{
				ScaleDownDelayAfterAdd:            "10s",
				ScaleDownDelayAfterDelete:         "20s",
				ScaleDownDelayAfterFailure:        "30s",
				ScaleDownUnneededTime:             "40s",
				ScaleDownUnreadyTime:              "50s",
				MaxCloudProviderNodeDeletionTime:  "60s",
				MaxNodeProvisionTime:              "70s",
				EnableGetVmss:                     "true",
				GetVmssSizeRefreshPeriod:          "80s",
				EnableForceDelete:                 "true",
				EnableDetailedCSEMessage:          "true",
				EnableDynamicInstanceList:         "true",
				MinCpu:                            "2",
				MaxCpu:                            "4",
				MinMemory:                         "8",
				MaxMemory:                         "16",
				ScaleDownUtilizationThreshold:     "0.5",
				MaxGracefulTerminationSec:         "90",
				BalanceSimilarNodeGroups:          "true",
				Expander:                          "test",
				NewPodScaleUpDelay:                "100s",
				MaxEmptyBulkDelete:                "100",
				OkTotalUnreadyCount:               "5",
				MaxTotalUnreadyPercentage:         "0.2",
				DaemonSetEvictionForEmptyNodes:    "true",
				DaemonSetEvictionForOccupiedNodes: "true",
				EnableQOSLogging:                  "true",
				SkipNodesWithLocalStorage:         "true",
				SkipNodesWithSystemPods:           "true",
				ScanInterval:                      "5m",
			},
		},
	}

	autoscalingOptions := config.AutoscalingOptions{}

	// Update the autoscalingOptions with the dynamicConfig's autoScalerProfile
	updatedAutoscalingOptions := b.updateAutoScalerProfile(autoscalingOptions)

	// Now we compare updatedAutoscalingOptions with our expected results

	// We want to check that autoscaler builder overrides MaxScaleDownParallelism
	assert.Equal(t, 100, updatedAutoscalingOptions.MaxEmptyBulkDelete)
	assert.Equal(t, 100, updatedAutoscalingOptions.MaxScaleDownParallelism)

	// Validate that time.Duration values are parsed correctly
	assert.Equal(t, 10*time.Second, updatedAutoscalingOptions.ScaleDownDelayAfterAdd)
	assert.Equal(t, 20*time.Second, updatedAutoscalingOptions.ScaleDownDelayAfterDelete)
	assert.Equal(t, 30*time.Second, updatedAutoscalingOptions.ScaleDownDelayAfterFailure)
	assert.Equal(t, 40*time.Second, updatedAutoscalingOptions.NodeGroupDefaults.ScaleDownUnneededTime)
	assert.Equal(t, 50*time.Second, updatedAutoscalingOptions.NodeGroupDefaults.ScaleDownUnreadyTime)
	assert.Equal(t, 60*time.Second, updatedAutoscalingOptions.MaxCloudProviderNodeDeletionTime)
	assert.Equal(t, 70*time.Second, updatedAutoscalingOptions.MaxNodeProvisionTime)
	assert.Equal(t, 80*time.Second, updatedAutoscalingOptions.GetVmssSizeRefreshPeriod)

	// Continue with the boolean values
	assert.True(t, updatedAutoscalingOptions.EnableGetVmss)
	assert.True(t, updatedAutoscalingOptions.EnableForceDelete)
	assert.True(t, updatedAutoscalingOptions.EnableDetailedCSEMessage)
	assert.True(t, updatedAutoscalingOptions.EnableDynamicInstanceList)
	assert.True(t, updatedAutoscalingOptions.BalanceSimilarNodeGroups)
	assert.True(t, updatedAutoscalingOptions.DaemonSetEvictionForEmptyNodes)
	assert.True(t, updatedAutoscalingOptions.DaemonSetEvictionForOccupiedNodes)

	// Check the string values
	assert.Equal(t, "test", updatedAutoscalingOptions.ExpanderNames)

	// Continue with the integer and float values
	assert.Equal(t, int64(2), updatedAutoscalingOptions.MinCoresTotal)
	assert.Equal(t, int64(4), updatedAutoscalingOptions.MaxCoresTotal)
	assert.Equal(t, 0.5, updatedAutoscalingOptions.NodeGroupDefaults.ScaleDownUtilizationThreshold)
	assert.Equal(t, 90, updatedAutoscalingOptions.MaxGracefulTerminationSec)
	assert.Equal(t, 5, updatedAutoscalingOptions.OkTotalUnreadyCount)
	assert.Equal(t, 0.2, updatedAutoscalingOptions.MaxTotalUnreadyPercentage)

}
