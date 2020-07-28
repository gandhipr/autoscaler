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

package dynamic

import (
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
)

// AutoScalerProfile holds the configurable autoscaler parameters via managed cluster API
// Empty or non-existent indicates letting CA handle the default
type AutoScalerProfile struct {
	ScanInterval                  string `json:"scan-interval,omitempty" yaml:"scan-interval,omitempty"`
	ScaleDownDelayAfterAdd        string `json:"scale-down-delay-after-add,omitempty" yaml:"scale-down-delay-after-add,omitempty"`
	ScaleDownDelayAfterDelete     string `json:"scale-down-delay-after-delete,omitempty" yaml:"scale-down-delay-after-delete,omitempty"`
	ScaleDownDelayAfterFailure    string `json:"scale-down-delay-after-failure,omitempty" yaml:"scale-down-delay-after-failure,omitempty"`
	ScaleDownUnneededTime         string `json:"scale-down-unneeded-time,omitempty" yaml:"scale-down-unneeded-time,omitempty"`
	ScaleDownUnreadyTime          string `json:"scale-down-unready-time,omitempty" yaml:"scale-down-unready-time,omitempty"`
	ScaleDownUtilizationThreshold string `json:"scale-down-utilization-threshold,omitempty" yaml:"scale-down-utilization-threshold,omitempty"`
	MaxGracefulTerminationSec     string `json:"max-graceful-termination-sec,omitempty" yaml:"max-graceful-termination-sec,omitempty"`
	BalanceSimilarNodeGroups      string `json:"balance-similar-node-groups,omitempty" yaml:"balance-similar-node-groups,omitempty"`
	Expander                      string `json:"expander,omitempty" yaml:"expander,omitempty"`
	NewPodScaleUpDelay            string `json:"new-pod-scale-up-delay,omitempty" yaml:"new-pod-scale-up-delay,omitempty"`
	MaxEmptyBulkDelete            string `json:"max-empty-bulk-delete,omitempty" yaml:"max-empty-bulk-delete,omitempty"`
	SkipNodesWithLocalStorage     string `json:"skip-nodes-with-local-storage,omitempty" yaml:"skip-nodes-with-local-storage,omitempty"`
	SkipNodesWithSystemPods       string `json:"skip-nodes-with-system-pods,omitempty" yaml:"skip-nodes-with-system-pods,omitempty"`
}

// Config holds the dynamic configuration of autoscaler which can be refreshed at runtime
type Config struct {
	NodeGroups        []NodeGroupSpec   `json:"nodeGroups" yaml:"nodeGroups"`
	AutoScalerProfile AutoScalerProfile `json:"autoScalerProfile" yaml:"autoScalerProfile"`
}

// NewDefaultConfig returns a default empty config
func NewDefaultConfig() Config {
	return Config{
		NodeGroups:        []NodeGroupSpec{},
		AutoScalerProfile: AutoScalerProfile{},
	}
}

// BuildConfig builds a Config object from the mounted path
func BuildConfig(reader io.Reader) (*Config, error) {
	config, err := umarshalConfig(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode autoscaler config: %v", err)
	}

	klog.V(4).Infof("nodeGroups=%v, autoScalerProfile=%v", config.NodeGroups, config.AutoScalerProfile)

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("error while validating config: %v", err)
	}

	return &config, nil
}

// umarshalConfig decodes the yaml or json reader into a struct
func umarshalConfig(reader io.Reader) (Config, error) {
	config := Config{}
	if err := yaml.NewYAMLOrJSONDecoder(reader, 4096).Decode(&config); err != nil {
		return Config{}, err
	}
	return config, nil
}

// NodeGroupSpecStrings returns node group specs represented in the form of `<minSize>:<maxSize>:<name>:<labels>|<taints>` to be passed to
// the cloudprovider autoscaling options
func (c Config) NodeGroupSpecStrings() []string {
	result := []string{}
	for _, spec := range c.NodeGroups {
		result = append(result, spec.StringWithLabelsAndTaints())
	}
	return result
}

func (c Config) validate() error {
	for _, g := range c.NodeGroups {
		if g.Name == "" {
			return fmt.Errorf("invalid nodeGroup: name must not be blank")
		}
		if g.MaxSize < g.MinSize {
			return fmt.Errorf("invalid nodeGroup: %s, max size must be greater or equal to min size", g.Name)
		}
	}
	return nil
}
