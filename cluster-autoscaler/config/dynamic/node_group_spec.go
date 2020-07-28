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

package dynamic

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
)

// NodeGroupSpec represents a specification of a node group to be auto-scaled
type NodeGroupSpec struct {
	// The name of the autoscaling target
	Name string `json:"name"`
	// Min size of the autoscaling target
	MinSize int `json:"minSize"`
	// Max size of the autoscaling target
	MaxSize int               `json:"maxSize"`
	Taints  string            `json:"taints"`
	Labels  map[string]string `json:"labels"`
	// Specifies whether this node group can scale to zero nodes.
	SupportScaleToZero bool
}

// SpecFromStringWithLabelsAndTaints parses a node group spec represented in the form of `<minSize>:<maxSize>:<name>:<labels>|<string>`
// and produces a node group spec object
// It falls-back to the the default
func SpecFromStringWithLabelsAndTaints(value string, SupportScaleToZero bool) (*NodeGroupSpec, error) {
	tokens := strings.SplitN(value, ":", 4)

	if len(tokens) < 3 {
		return nil, fmt.Errorf("error while parsing NodeGroupSpec: %s", value)
	}

	// first parse the min, max and name
	spec, err := SpecFromString(strings.Join(tokens[0:3], ":"), SupportScaleToZero)
	if err != nil {
		return nil, fmt.Errorf("error while parsing NodeGroupSpec: %s, %s", value, err)
	}

	if len(tokens) > 3 {
		labelsTaints := strings.Split(tokens[3], "|")
		// attempt to parse labels
		var labels map[string]string
		err = json.Unmarshal([]byte(labelsTaints[0]), &labels)
		if err != nil {
			return nil, fmt.Errorf("error while parsing NodeGroupSpec: %s for labels, %s", value, err)
		}

		spec.Labels = labels
		if len(labelsTaints) > 1 {
			spec.Taints = labelsTaints[1]
		}
	}
	klog.V(4).Infof("Parsed spec is: nodeName: %s, minSize: %d, maxSize: %d,"+
		"labels: %s, taints:%s, supportScaleToZero: %t", spec.Name, spec.MinSize, spec.MaxSize, spec.Labels, spec.Taints, spec.SupportScaleToZero)
	return spec, err
}

// SpecFromString parses a node group spec represented in the form of `<minSize>:<maxSize>:<name>` and produces a node group spec object
func SpecFromString(value string, SupportScaleToZero bool) (*NodeGroupSpec, error) {
	tokens := strings.SplitN(value, ":", 3)
	if len(tokens) != 3 {
		return nil, fmt.Errorf("wrong nodes configuration: %s", value)
	}

	spec := NodeGroupSpec{SupportScaleToZero: SupportScaleToZero}
	if size, err := strconv.Atoi(tokens[0]); err == nil {

		spec.MinSize = size
	} else {
		return nil, fmt.Errorf("failed to set min size: %s, expected integer", tokens[0])
	}

	if size, err := strconv.Atoi(tokens[1]); err == nil {
		spec.MaxSize = size
	} else {
		return nil, fmt.Errorf("failed to set max size: %s, expected integer", tokens[1])
	}

	spec.Name = tokens[2]

	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("invalid node group spec: %v", err)
	}

	return &spec, nil
}

// Validate produces an error if there's an invalid field in the node group spec
func (s NodeGroupSpec) Validate() error {
	if s.SupportScaleToZero {
		if s.MinSize < 0 {
			return fmt.Errorf("min size must be >= 0")
		}
	} else {
		if s.MinSize <= 0 {
			return fmt.Errorf("min size must be >= 1")
		}
	}
	if s.MaxSize < s.MinSize {
		return fmt.Errorf("max size must be greater or equal to min size")
	}
	if s.Name == "" {
		return fmt.Errorf("name must not be blank")
	}
	return nil
}

// Represents the node group spec in the form of `<minSize>:<maxSize>:<name>`
func (s NodeGroupSpec) String() string {
	return fmt.Sprintf("%d:%d:%s", s.MinSize, s.MaxSize, s.Name)
}

// StringWithLabelsAndTaints the node group spec in the form of `<minSize>:<maxSize>:<name>:<labels>|<taints>`
func (s NodeGroupSpec) StringWithLabelsAndTaints() string {
	if len(s.Labels) == 0 {
		s.Labels = map[string]string{}
	}
	labels, _ := json.Marshal(s.Labels)
	return fmt.Sprintf("%d:%d:%s:%s|%s", s.MinSize, s.MaxSize, s.Name, labels, s.Taints)
}
