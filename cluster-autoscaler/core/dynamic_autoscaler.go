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
	"fmt"
	"time"

	"k8s.io/autoscaler/cluster-autoscaler/config/dynamic"
	"k8s.io/autoscaler/cluster-autoscaler/metrics"
	"k8s.io/autoscaler/cluster-autoscaler/utils/errors"
	"k8s.io/klog/v2"
)

// DynamicAutoscaler is a wrapper around AutoScaler type
type DynamicAutoscaler struct {
	autoscaler        Autoscaler
	autoscalerBuilder AutoscalerBuilder
	configFetcher     dynamic.ConfigFetcher
}

// NewDynamicAutoscaler builds a DynamicAutoscaler
func NewDynamicAutoscaler(autoscalerBuilder AutoscalerBuilder, configFetcher dynamic.ConfigFetcher) (*DynamicAutoscaler, errors.AutoscalerError) {
	autoscaler, err := autoscalerBuilder.Build()
	if err != nil {
		return nil, err
	}
	return &DynamicAutoscaler{
		autoscaler:        autoscaler,
		autoscalerBuilder: autoscalerBuilder,
		configFetcher:     configFetcher,
	}, nil
}

// Start starts the components running in background.
func (a *DynamicAutoscaler) Start() error {
	a.autoscaler.Start()
	return nil
}

// ExitCleanUp cleans-up after this instance of autoscaler
func (a *DynamicAutoscaler) ExitCleanUp() {
	a.autoscaler.ExitCleanUp()
}

// RunOnce represents a single iteration of a dynamic autoscaler inside the CA infinite loop
func (a *DynamicAutoscaler) RunOnce(currentTime time.Time) errors.AutoscalerError {
	reconfigureStart := time.Now()
	metrics.UpdateLastTime(metrics.Reconfigure, reconfigureStart)
	if err := a.Reconfigure(); err != nil {
		klog.Errorf("Failed to reconfigure : %v", err)
	}
	metrics.UpdateDurationFromStart(metrics.Reconfigure, reconfigureStart)
	return a.autoscaler.RunOnce(currentTime)
}

// Reconfigure the dynamic autoscaler if the configmap is updated
func (a *DynamicAutoscaler) Reconfigure() error {
	var updatedConfig *dynamic.Config
	var err error

	if updatedConfig, err = a.configFetcher.FetchConfigIfUpdated(); err != nil {
		return fmt.Errorf("Failed to fetch updated config: %v", err)
	}

	if updatedConfig != nil {
		klog.V(3).Info("Config has changed - cleaning up and updating autoscaler configuration")
		// cleanup old autoscaler
		a.ExitCleanUp()

		newAutoScaler, err := a.autoscalerBuilder.SetDynamicConfig(*updatedConfig).Build()
		if err != nil {
			return err
		}

		a.autoscaler = newAutoScaler
		// start csr components running in background
		if err := a.autoscaler.Start(); err != nil {
			klog.Fatalf("Failed to start autoscaler background components: %v", err)
		}

		klog.V(3).Infof("Dynamic reconfiguration completed: updatedConfig=%v", updatedConfig)
	}
	return nil
}
