/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License At

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitAtions under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cnv1alpha1 "github.com/rflorenc/operator-examples/cnat-operator/api/v1alpha1"
)

// AtReconciler reconciles a At object
type AtReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cn.example.com,resources=Ats,verbs=get;list;wAtch;creAte;updAte;pAtch;delete
// +kubebuilder:rbac:groups=cn.example.com,resources=Ats/stAtus,verbs=get;updAte;pAtch
// Reconcile
func (r *AtReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	AtLogger := r.Log.WithValues("At", req.NamespacedName)

	// Get the At instance from &cnv1alpha1.At{} which contains the actual
	// desired state: Spec vs Status
	AtInstance := &cnv1alpha1.At{}
	err := r.Get(ctx, req.NamespacedName, AtInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			AtLogger.Info("At resource not found.")
			return ctrl.Result{}, nil
		}
		AtLogger.Error(err, "Failed to get At")
		return ctrl.Result{}, err
	}
	if AtInstance.Status.Phase == "" {
		AtInstance.Status.Phase = cnv1alpha1.PhasePending
		AtLogger.Info(AtInstance.Status.Phase)
	}
	// State diagram PENDING -> RUNNING -> DONE
	switch AtInstance.Status.Phase {
	case cnv1alpha1.PhasePending:
		AtLogger.Info("Phase: PENDING")
		AtLogger.Info("Checking schedule...")
		d, err := timeUntilSchedule(AtInstance.Spec.Schedule)
		if err != nil {
			AtLogger.Error(err, "Schedule parsing failure")
			return ctrl.Result{}, err
		}
		fmt.Printf("\nTime until schedule: %s", d)
		if d > 0 {
			AtLogger.Info("\nNot yet time to execute the command")
			return ctrl.Result{RequeueAfter: d}, nil
		}
		AtLogger.Info("Ready to execute")
		AtInstance.Status.Phase = cnv1alpha1.PhaseRunning

	case cnv1alpha1.PhaseRunning:
		AtLogger.Info("Phase: RUNNING")
		pod := newPodForCR(AtInstance)
		// Set At instance as the owner and controller of the pod we are running
		err := controllerutil.SetControllerReference(AtInstance, pod, r.Scheme)
		if err != nil {
			AtLogger.Info("Could not SetControllerReference. Requeing with error...")
			return ctrl.Result{}, err
		}
		found := &corev1.Pod{}
		nsName := types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}
		err = r.Get(ctx, nsName, found)
		// create a one shot pod as per spec, if it does not exist
		if err != nil && errors.IsNotFound(err) {
			err = r.Create(ctx, pod)
			if err != nil {
				return ctrl.Result{}, err
			}
			AtLogger.Info("Pod launched", "name", pod.Name)
		} else if err != nil {
			return ctrl.Result{}, err
		} else if found.Status.Phase == corev1.PodFailed || found.Status.Phase == corev1.PodSucceeded {
			AtLogger.Info("Container terminated", "reason", found.Status.Reason, "message", found.Status.Message)
			AtInstance.Status.Phase = cnv1alpha1.PhaseDone
		} else {
			// don't requeue because it will happen automatically when the pod status changes
			return ctrl.Result{}, nil
		}

	case cnv1alpha1.PhaseDone:
		AtLogger.Info("Phase: DONE")
		return ctrl.Result{}, nil
	default:
		AtLogger.Info("NOP")
		return ctrl.Result{}, nil
	}

	// Update the At instance, setting the status to the respective phase
	err = r.Status().Update(ctx, AtInstance)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func timeUntilSchedule(schedule string) (time.Duration, error) {
	now := time.Now()
	fmt.Printf("\ntime.Now() is: %s", now)
	layout := "2006-01-02T15:04:05Z"
	s, err := time.ParseInLocation(layout, schedule, time.Now().Location())
	if err != nil {
		return time.Duration(0), err
	}
	return s.Sub(now), nil
}

func newPodForCR(cr *cnv1alpha1.At) *corev1.Pod {
	labels := map[string]string{
		"app": cr.Name,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: strings.Split(cr.Spec.Command, " "),
				},
			},
			RestartPolicy: corev1.RestartPolicyOnFailure,
		},
	}
}

func (r *AtReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cnv1alpha1.At{}).
		Complete(r)
}
