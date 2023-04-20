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

package controllers

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	replicasetv1alpha1 "github.com/vanderbilt/raft-replicaset-operator/api/v1alpha1"
)

const raftreplicasetFinalizer = "raftreplicaset.vanderbilt.edu/finalizer"

// RaftReplicaSetVandyReconciler reconciles a RaftReplicaSetVandy object
type RaftReplicaSetVandyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Definitions to manage status conditions
const (
	// typeAvailableRaftReplicaSet represents the status of the Deployment reconciliation
	typeAvailableRaftReplicaset = "Available"
	// typeDegradedRaftReplicaSet represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	typeDegradedRaftReplicaset = "Degraded"
	raftPodName                = "raft-replicaset"
)

//+kubebuilder:rbac:groups=replicaset.vanderbilt.edu,resources=raftreplicasetvandies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=replicaset.vanderbilt.edu,resources=raftreplicasetvandies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=replicaset.vanderbilt.edu,resources=raftreplicasetvandies/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RaftReplicaSetVandy object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *RaftReplicaSetVandyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// _ = log.FromContext(ctx)
	log := log.FromContext(ctx)
	log.Info("RaftReplicaSetVandyReconciler Starts ...")

	raftreplicaset := &replicasetv1alpha1.RaftReplicaSetVandy{}
	err := r.Get(ctx, req.NamespacedName, raftreplicaset)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("raftreplicaset resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get raftreplicaset")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if raftreplicaset.Status.Conditions == nil || len(raftreplicaset.Status.Conditions) == 0 {
		meta.SetStatusCondition(&raftreplicaset.Status.Conditions, metav1.Condition{Type: typeAvailableRaftReplicaset, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, raftreplicaset); err != nil {
			log.Error(err, "Failed to update RaftReplicaSet status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the RaftReplicaSet Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, raftreplicaset); err != nil {
			log.Error(err, "Failed to re-fetch RaftReplicaSet")
			return ctrl.Result{}, err
		}
	}

	if !controllerutil.ContainsFinalizer(raftreplicaset, raftreplicasetFinalizer) {
		log.Info("Adding Finalizer for RaftReplicaSet")
		if ok := controllerutil.AddFinalizer(raftreplicaset, raftreplicasetFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, raftreplicaset); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	isRaftReplicaSetMarkedToBeDeleted := raftreplicaset.GetDeletionTimestamp() != nil
	if isRaftReplicaSetMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(raftreplicaset, raftreplicasetFinalizer) {
			log.Info("Performing Finalizer Operations for RaftReplicaSet before delete CR")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(&raftreplicaset.Status.Conditions, metav1.Condition{Type: typeDegradedRaftReplicaset,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", raftreplicaset.Name)})

			if err := r.Status().Update(ctx, raftreplicaset); err != nil {
				log.Error(err, "Failed to update RaftReplicaSet status")
				return ctrl.Result{}, err
			}

			// Perform all operations required before remove the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			r.doFinalizerOperationsForRaftReplicaSet(raftreplicaset)

			// TODO(user): If you add operations to the doFinalizerOperationsForRaftReplicaSet method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the RaftReplicaSet Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, raftreplicaset); err != nil {
				log.Error(err, "Failed to re-fetch RaftReplicaSet")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&raftreplicaset.Status.Conditions, metav1.Condition{Type: typeDegradedRaftReplicaset,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", raftreplicaset.Name)})

			if err := r.Status().Update(ctx, raftreplicaset); err != nil {
				log.Error(err, "Failed to update RaftReplicaSet status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for RaftReplicaSet after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(raftreplicaset, raftreplicasetFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for RaftReplicaSet")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, raftreplicaset); err != nil {
				log.Error(err, "Failed to remove finalizer for RaftReplicaSet")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	found := &corev1.Pod{}
	size := int(raftreplicaset.Spec.Size)

	cm := r.createConfigMap(size)
	namespaceId := types.NamespacedName{
		Name:      "raft-replicaset",
		Namespace: "default",
	}
	raftresource := r.Get(ctx, namespaceId, cm)

	if raftresource != nil {
		if err = r.Create(ctx, cm); err != nil {
			log.Error(err, "Failed to create new CM")
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: raftreplicaset.Name, Namespace: raftreplicaset.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		for i := 0; i < size; i++ {
			dep, err := r.deployRaftReplicaSet(raftreplicaset, i)

			namespaceId := types.NamespacedName{
				Name:      "raft-replicaset-" + strconv.Itoa(i),
				Namespace: "default",
			}
			raftresource := r.Get(ctx, namespaceId, dep)
			if raftresource != nil {
				if err = r.Create(ctx, dep); err != nil {
					log.Error(err, "Failed to create new Pod")
				}
			}

			svc, err := r.deployRaftReplicaSetServices(raftreplicaset, i)
			raftresource = r.Get(ctx, namespaceId, svc)
			if raftresource != nil {
				if err = r.Create(ctx, svc); err != nil {
					log.Error(err, "Failed to create new Service")
				}
			}
		}

		// Replicaset created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Replicaset")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RaftReplicaSetVandyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&replicasetv1alpha1.RaftReplicaSetVandy{}).
		Complete(r)
}

// finalizeRaftReplicaSet will perform the required operations before delete the CR.
func (r *RaftReplicaSetVandyReconciler) doFinalizerOperationsForRaftReplicaSet(cr *replicasetv1alpha1.RaftReplicaSetVandy) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete resources which are
	// created and managed in the reconciliation. These ones, such as the Deployment created on this reconcile,
	// are defined as depended of the custom resource. See that we use the method ctrl.SetControllerReference.
	// to set the ownerRef which means that the Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

func (r *RaftReplicaSetVandyReconciler) deployRaftReplicaSet(
	RaftReplicaSet *replicasetv1alpha1.RaftReplicaSetVandy, podIndex int) (*corev1.Pod, error) {
	// ls := labelsForRaftReplicaSet(RaftReplicaSet.Name)
	// replicas := RaftReplicaSet.Spec.Size
	image, err := getImage()

	podName := raftPodName + "-" + strconv.Itoa(podIndex)

	lables := labelsForRaftReplicaset(podName)

	if err != nil {
		return nil, err
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: "default",
			Labels:    lables,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "raft-noder",
					Image: image,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "config-volume",
							MountPath: "/etc/raftconfig",
						},
					},
					Command: []string{"tail", "-f", "/dev/null"},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "config-volume",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "raft-replicaset",
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(RaftReplicaSet, pod, r.Scheme); err != nil {
		return nil, err
	}
	return pod, nil
}

func (r *RaftReplicaSetVandyReconciler) deployRaftReplicaSetServices(
	RaftReplicaSet *replicasetv1alpha1.RaftReplicaSetVandy, podIndex int) (*corev1.Service, error) {

	podName := raftPodName + "-" + strconv.Itoa(podIndex)

	lables := labelsForRaftReplicaset(podName)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Labels:    lables,
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "raft-service-port",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Protocol:   "TCP",
				},
			},
			Selector:  serviceSelectorForRaftReplicaset(podName),
			ClusterIP: "",
		},
	}

	return service, nil
}

func getImage() (string, error) {
	var imageEnvVar = "RAFT_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("Unable to find %s environment variable with the image", imageEnvVar)
	}
	return image, nil
}

func labelsForRaftReplicaset(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

func serviceSelectorForRaftReplicaset(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/instance": name,
	}
}

func (r *RaftReplicaSetVandyReconciler) createConfigMap(size int) *corev1.ConfigMap {
	configMapList := []map[string]string{}

	// Loop 3 times and create a new map for each iteration
	for i := 0; i < size; i++ {
		// Create a new map with a unique key-value pair
		newMap := map[string]string{
			fmt.Sprintf("name"):    fmt.Sprintf("node%d", i),
			fmt.Sprintf("address"): fmt.Sprintf("%s-%d.default.svc.cluster.local:8080", raftPodName, i),
		}
		// Append the new map to the list
		configMapList = append(configMapList, newMap)
	}

	jsonString, err := json.Marshal(configMapList)
	if err != nil {
		fmt.Println("Error marshaling list to JSON:", err)
	}
	configMapData := make(map[string]string, 0)
	configMapData["config.json"] = string(jsonString)

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "raft-replicaset",
			Namespace: "default",
		},
		Data: configMapData,
	}
	return configMap
}

// func (r *RaftReplicaSetVandyReconciler) createResource(*corev1.Pod) error {

// 	return nil
// }
