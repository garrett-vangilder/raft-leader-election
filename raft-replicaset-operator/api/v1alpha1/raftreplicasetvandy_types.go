/*
Copyright 2023.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RaftReplicaSetVandySpec defines the desired state of RaftReplicaSetVandy
type RaftReplicaSetVandySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of RaftReplicaSetVandy. Edit raftreplicasetvandy_types.go to remove/update
	Size int32 `json:"size,omitempty"`

	// Port defines the port that will be used to init the container with the image
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ContainerPort int32 `json:"containerPort,omitempty"`
}

// RaftReplicaSetVandyStatus defines the observed state of RaftReplicaSetVandy
type RaftReplicaSetVandyStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RaftReplicaSetVandy is the Schema for the raftreplicasetvandies API
type RaftReplicaSetVandy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RaftReplicaSetVandySpec   `json:"spec,omitempty"`
	Status RaftReplicaSetVandyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RaftReplicaSetVandyList contains a list of RaftReplicaSetVandy
type RaftReplicaSetVandyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RaftReplicaSetVandy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RaftReplicaSetVandy{}, &RaftReplicaSetVandyList{})
}
