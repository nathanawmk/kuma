package controllers

import (
	"context"

	"github.com/pkg/errors"
	kube_core "k8s.io/api/core/v1"
	kube_apierrs "k8s.io/apimachinery/pkg/api/errors"
	kube_ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kumahq/kuma/pkg/core"
	core_mesh "github.com/kumahq/kuma/pkg/core/resources/apis/mesh"
	"github.com/kumahq/kuma/pkg/core/resources/manager"
	core_model "github.com/kumahq/kuma/pkg/core/resources/model"
	"github.com/kumahq/kuma/pkg/core/resources/store"
	defaults_mesh "github.com/kumahq/kuma/pkg/defaults/mesh"
	common_k8s "github.com/kumahq/kuma/pkg/plugins/common/k8s"
	"github.com/kumahq/kuma/pkg/plugins/resources/k8s"
	mesh_k8s "github.com/kumahq/kuma/pkg/plugins/resources/k8s/native/api/v1alpha1"
)

// MeshDefaultsReconciler creates default resources for created Mesh
type MeshDefaultsReconciler struct {
	ResourceManager manager.ResourceManager
}

func (r *MeshDefaultsReconciler) Reconcile(req kube_ctrl.Request) (kube_ctrl.Result, error) {
	mesh := core_mesh.NewMeshResource()
	if err := r.ResourceManager.Get(context.Background(), mesh, store.GetByKey(req.Name, core_model.NoMesh)); err != nil {
		if kube_apierrs.IsNotFound(err) {
			return kube_ctrl.Result{}, nil
		}
		return kube_ctrl.Result{}, errors.Wrap(err, "could not get default mesh resources")
	}

	// Before creating default policies for the mesh we want to ensure that this mesh wasn't processed before.
	// We can't rely on filtering by CreateFunc, because apparently it sends the create event every time resource
	// is added to the underlying Informer. That's why on the Kuma CP restart Mesh will be processed the second time
	if processed := mesh.GetMeta().(*k8s.KubernetesMetaAdapter).GetAnnotations()[common_k8s.K8sMeshDefaultsGenerated]; processed == "true" {
		return kube_ctrl.Result{}, nil
	}

	if err := defaults_mesh.EnsureDefaultMeshResources(r.ResourceManager, req.Name); err != nil {
		return kube_ctrl.Result{}, errors.Wrap(err, "could not create default mesh resources")
	}

	if mesh.GetMeta().(*k8s.KubernetesMetaAdapter).GetAnnotations() == nil {
		mesh.GetMeta().(*k8s.KubernetesMetaAdapter).Annotations = map[string]string{}
	}
	mesh.GetMeta().(*k8s.KubernetesMetaAdapter).GetAnnotations()[common_k8s.K8sMeshDefaultsGenerated] = "true"
	if err := r.ResourceManager.Update(context.Background(), mesh, store.ModifiedAt(core.Now())); err != nil {
		return kube_ctrl.Result{}, errors.Wrap(err, "could not update default mesh resources")
	}
	return kube_ctrl.Result{}, nil
}

func (r *MeshDefaultsReconciler) SetupWithManager(mgr kube_ctrl.Manager) error {
	if err := kube_core.AddToScheme(mgr.GetScheme()); err != nil {
		return errors.Wrapf(err, "could not add %q to scheme", kube_core.SchemeGroupVersion)
	}
	if err := mesh_k8s.AddToScheme(mgr.GetScheme()); err != nil {
		return errors.Wrapf(err, "could not add %q to scheme", mesh_k8s.GroupVersion)
	}
	return kube_ctrl.NewControllerManagedBy(mgr).
		For(&mesh_k8s.Mesh{}, builder.WithPredicates(onlyCreate)).
		Complete(r)
}

// we only want to react on Create events. User may want to delete default resources, we don't want to add them again when they update the Mesh
var onlyCreate = predicate.Funcs{
	CreateFunc: func(event event.CreateEvent) bool {
		return true
	},
	DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
		return false
	},
	UpdateFunc: func(updateEvent event.UpdateEvent) bool {
		return false
	},
	GenericFunc: func(genericEvent event.GenericEvent) bool {
		return false
	},
}
