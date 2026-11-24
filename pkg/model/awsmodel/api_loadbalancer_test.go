/*
Copyright 2026 The Kubernetes Authors.

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

package awsmodel

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/model"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awstasks"
)

func TestChooseBestSubnetForELBWhenUpdatingOnlyNodes(t *testing.T) {
	subnets := []*kops.ClusterSubnetSpec{
		{Name: "a-nodes", Zone: "us-test-1a", Type: kops.SubnetTypePrivate},
		{Name: "b-control-plane", Zone: "us-test-1a", Type: kops.SubnetTypePrivate},
	}
	controlPlane := &kops.InstanceGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "control-plane"},
		Spec:       kops.InstanceGroupSpec{Role: kops.InstanceGroupRoleControlPlane, Subnets: []string{"b-control-plane"}},
	}
	nodes := &kops.InstanceGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "nodes"},
		Spec:       kops.InstanceGroupSpec{Role: kops.InstanceGroupRoleNode, Subnets: []string{"a-nodes"}},
	}

	b := &APILoadBalancerBuilder{
		AWSModelContext: &AWSModelContext{
			KopsModelContext: &model.KopsModelContext{
				AllInstanceGroups: []*kops.InstanceGroup{controlPlane, nodes},
				InstanceGroups:    []*kops.InstanceGroup{nodes},
			},
		},
	}
	if subnet := b.chooseBestSubnetForELB("us-test-1a", subnets); subnet.Name != "b-control-plane" {
		t.Errorf("expected the subnet of the control plane %q, but got %q", "b-control-plane", subnet.Name)
	}
}

func TestAPILoadBalancerPreserveClientIP(t *testing.T) {
	tests := []struct {
		name               string
		preserveClientIP   *bool
		wantPreserveClient string
	}{
		{
			name:               "unset retains existing behavior",
			preserveClientIP:   nil,
			wantPreserveClient: "",
		},
		{
			name:               "preserveClientIP=false sets attribute to false",
			preserveClientIP:   new(false),
			wantPreserveClient: "false",
		},
		{
			name:               "preserveClientIP=true sets attribute to true",
			preserveClientIP:   new(true),
			wantPreserveClient: "true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &kops.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test.example.com"},
				Spec: kops.ClusterSpec{
					API: kops.APISpec{
						LoadBalancer: &kops.LoadBalancerAccessSpec{
							Class:            kops.LoadBalancerClassNetwork,
							Type:             kops.LoadBalancerTypePublic,
							PreserveClientIP: tc.preserveClientIP,
						},
					},
					Networking: kops.NetworkingSpec{
						Topology: &kops.TopologySpec{
							DNS: kops.DNSTypeNone,
						},
						Subnets: []kops.ClusterSubnetSpec{
							{Name: "us-test-1a", Zone: "us-test-1a", Type: kops.SubnetTypePublic},
							{Name: "us-test-1b", Zone: "us-test-1b", Type: kops.SubnetTypePublic},
						},
					},
				},
			}
			controlPlane := &kops.InstanceGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "control-plane-us-test-1a"},
				Spec:       kops.InstanceGroupSpec{Role: kops.InstanceGroupRoleControlPlane, Subnets: []string{"us-test-1a"}},
			}
			nodes := &kops.InstanceGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "nodes"},
				Spec:       kops.InstanceGroupSpec{Role: kops.InstanceGroupRoleNode, Subnets: []string{"us-test-1a", "us-test-1b"}},
			}

			b := &APILoadBalancerBuilder{
				AWSModelContext: &AWSModelContext{
					KopsModelContext: &model.KopsModelContext{
						Cluster:           cluster,
						AllInstanceGroups: []*kops.InstanceGroup{controlPlane, nodes},
						InstanceGroups:    []*kops.InstanceGroup{controlPlane, nodes},
					},
				},
				Lifecycle:         fi.LifecycleSync,
				SecurityLifecycle: fi.LifecycleSync,
			}

			ctx := &fi.CloudupModelBuilderContext{
				Tasks: make(map[string]fi.CloudupTask),
			}
			if err := b.Build(ctx); err != nil {
				t.Fatalf("Build failed: %v", err)
			}

			tgCount := 0
			for _, task := range ctx.Tasks {
				tg, ok := task.(*awstasks.TargetGroup)
				if !ok {
					continue
				}
				tgCount++
				got := tg.Attributes[awstasks.TargetGroupAttributePreserveClientIPEnabled]
				if got != tc.wantPreserveClient {
					t.Errorf("TargetGroup %s %s = %q, want %q", fi.ValueOf(tg.Name), awstasks.TargetGroupAttributePreserveClientIPEnabled, got, tc.wantPreserveClient)
				}
			}
			if tgCount == 0 {
				t.Fatalf("expected at least one TargetGroup task to be built")
			}
		})
	}
}
