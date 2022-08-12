/*
Copyright 2022 Andrew Melnick.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"k8s.io/client-go/rest"

	secretsv1alpha1 "github.com/meln5674/secrets-operator/api/v1alpha1"
	"github.com/meln5674/secrets-operator/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	referencedSecretsKey    = ".metadata.references.secrets"
	referencedConfigMapsKey = ".metadata.references.configMaps"
	secretDerivedFromKey    = ".metadata.derivedFrom.derivedSecret"
)

// DerivedSecretReconciler reconciles a DerivedSecret object
type DerivedSecretReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	RestConfig *rest.Config
	Manager    ctrl.Manager
}

type DerivedSecretReconcilerRunStage1 struct {
	*DerivedSecretReconciler
	logger logr.Logger
	ctx    context.Context
	src    *secretsv1alpha1.DerivedSecret
}

type DerivedSecretReconcilerRunStage2 struct {
	*DerivedSecretReconcilerRunStage1
	cmRefs map[string]corev1.ConfigMap
	sRefs  map[string]corev1.Secret
}

type DerivedSecretReconcilerRunStage3 struct {
	*DerivedSecretReconcilerRunStage2
	secret *corev1.Secret
}

func (r *DerivedSecretReconcilerRunStage1) FetchReferences() (nextR *DerivedSecretReconcilerRunStage2, err error) {
	cmRefs := make(map[string]corev1.ConfigMap)
	sRefs := make(map[string]corev1.Secret)

	refKeys := make(map[string]struct{})

	for _, refInfo := range r.src.Spec.References {
		refName := refInfo.Name
		if _, collided := refKeys[refName]; collided {
			return nil, fmt.Errorf("Reference names must be unique, but %s appeared multiple times", refName)
		}
		if refInfo.ConfigMapRef != nil {
			cm := corev1.ConfigMap{}
			err := r.Get(r.ctx, client.ObjectKey{Namespace: r.src.Namespace, Name: refInfo.ConfigMapRef.Name}, &cm)
			if client.IgnoreNotFound(err) != nil && refInfo.ConfigMapRef.Optional != nil && *refInfo.ConfigMapRef.Optional {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("While fetching reference %s: %s", refName, err)
			}
			cmRefs[refName] = cm
			continue
		}
		if refInfo.SecretRef != nil {
			s := corev1.Secret{}
			err := r.Get(r.ctx, client.ObjectKey{Namespace: r.src.Namespace, Name: refInfo.SecretRef.Name}, &s)
			if client.IgnoreNotFound(err) != nil && refInfo.SecretRef.Optional != nil && *refInfo.SecretRef.Optional {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("While fetching reference %s: %s", refName, err)
			}
			sRefs[refName] = s
			continue
		}
		return nil, fmt.Errorf("Reference %s does not specify a source", refName)
	}
	return &DerivedSecretReconcilerRunStage2{DerivedSecretReconcilerRunStage1: r, cmRefs: cmRefs, sRefs: sRefs}, nil
}

func (r *DerivedSecretReconcilerRunStage1) GetClientForSecret() (client.Client, error) {
	targetNamespace := r.src.Spec.TargetNamespace
	if targetNamespace == "" {
		targetNamespace = r.src.Namespace
	}

	if targetNamespace == r.src.Namespace {
		return r.Client, nil
	}

	r.logger.Info("Target namespace is different than source, using impersonation", "targetNamespace", targetNamespace)
	if r.src.Spec.ServiceAccountName == "" {
		return nil, fmt.Errorf("spec.serviceAccountName is required when creating a secret in another namespace")
	}

	clusterOpts := cluster.Options{
		NewClient: cluster.DefaultNewClient,
	}
	impConfig := *r.RestConfig
	impConfig.Impersonate = rest.ImpersonationConfig{
		UserName: fmt.Sprintf("system:serviceaccount:%s:%s", r.src.Namespace, r.src.Spec.ServiceAccountName),
	}
	mapper := r.Manager.GetRESTMapper()
	cache := r.Manager.GetCache()
	clientOptions := client.Options{Scheme: clusterOpts.Scheme, Mapper: mapper}

	impClient, err := clusterOpts.NewClient(cache, &impConfig, clientOptions, clusterOpts.ClientDisableCacheFor...)
	if err != nil {
		return nil, err
	}
	return impClient, nil
}

func (r *DerivedSecretReconcilerRunStage2) CreateSecret(secretClient client.Client) (nextR *DerivedSecretReconcilerRunStage3, err error) {
	secretCopy, noOverwrite, err := model.GenerateSecret(r.cmRefs, r.sRefs, r.src)
	if err != nil {
		return nil, err
	}
	r.logger.Info("Secret generated")

	if secretCopy.Namespace == r.src.Namespace {
		err = ctrl.SetControllerReference(r.src, &secretCopy, r.Scheme)
		if err != nil {
			return nil, err
		}
		r.logger.Info("Secret controller set")
	}

	secret := secretCopy.DeepCopy()

	_, err = ctrl.CreateOrUpdate(r.ctx, secretClient, secret, func() error {
		secret.Type = secretCopy.Type
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		for key, val := range secretCopy.Data {
			if _, skip := noOverwrite[key]; skip {
				continue
			}
			secret.Data[key] = val
		}
		if secret.StringData == nil {
			secret.StringData = make(map[string]string)
		}
		for key, val := range secretCopy.StringData {
			if _, skip := noOverwrite[key]; skip {
				continue
			}
			secret.StringData[key] = val
		}
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		for key, value := range secretCopy.Labels {
			secret.Labels[key] = value
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.src.Status.SecretName = secret.Name
	r.src.Status.SecretNamespace = secret.Namespace
	return &DerivedSecretReconcilerRunStage3{DerivedSecretReconcilerRunStage2: r, secret: secret}, nil
}

func (r *DerivedSecretReconcilerRunStage3) CleanOtherOwnedSecrets(secretClient client.Client) error {
	otherSecrets := corev1.SecretList{}
	err := secretClient.List(
		r.ctx,
		&otherSecrets,
		client.MatchingFields(map[string]string{
			secretDerivedFromKey: DerivedFromFieldValueFromObject(r.src),
		}),
	)
	if err != nil {
		// Technically not stopping us from continuing
		r.logger.Info("Failed to list other secrets owned by this DerivedSecret, previous secrets may still exist", "error", err)
	} else {
		for _, secret := range otherSecrets.Items {
			if secret.Name == r.secret.Name && secret.Namespace == r.secret.Namespace {
				continue
			}
			err = secretClient.Delete(r.ctx, &secret)
			if err != nil {
				// Also technically not stopping us
				r.logger.Info("Failed to delete previously derived secrets", "error", err)
			}
		}
	}
	return nil
}

func (r *DerivedSecretReconcilerRunStage1) SyncStatus(err error) error {
	if err == nil {
		now := metav1.Now()
		r.src.Status.Error = ""
		r.src.Status.LastSync = &now
	} else {
		r.src.Status.Error = err.Error()
	}
	uperr := r.Status().Update(r.ctx, r.src)
	if uperr != nil {
		return uperr
	}
	return err
}

func DerivedFromFieldValueFromObject(obj client.Object) string {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return fmt.Sprintf(
		"%s/%s/%s/%s/%s",
		gvk.Group,
		gvk.Version,
		gvk.Kind,
		obj.GetNamespace(),
		obj.GetName(),
	)
}

func DerivedFromFieldValueFromLabels(labels map[string]string) (string, bool) {
	ok := true
	name, subok := labels[secretsv1alpha1.DerivedFromNameLabel]
	ok = ok && subok
	namespace, subok := labels[secretsv1alpha1.DerivedFromNamespaceLabel]
	ok = ok && subok
	group, subok := labels[secretsv1alpha1.DerivedFromGroupLabel]
	ok = ok && subok
	version, subok := labels[secretsv1alpha1.DerivedFromVersionLabel]
	ok = ok && subok
	kind, subok := labels[secretsv1alpha1.DerivedFromKindLabel]
	ok = ok && subok
	return fmt.Sprintf(
		"%s/%s/%s/%s/%s",
		group,
		version,
		kind,
		namespace,
		name,
	), ok
}

//+kubebuilder:rbac:groups=secrets.meln5674.github.com,resources=derivedsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=secrets.meln5674.github.com,resources=derivedsecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=secrets.meln5674.github.com,resources=derivedsecrets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=*
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DerivedSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx).WithValues("request", req)

	logger.Info("Got request")

	src := secretsv1alpha1.DerivedSecret{}

	err = r.Get(ctx, req.NamespacedName, &src)

	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if err != nil {
		// TODO: Finalizers?
		logger.Info("Got not found, assuming deleted", "error", err)
		return ctrl.Result{}, nil
	}
	now := metav1.Now()
	src.Status.LastSyncAttempt = &now

	r1 := DerivedSecretReconcilerRunStage1{DerivedSecretReconciler: r, ctx: ctx, src: &src, logger: logger}

	// After this, any error will be captured in the .status.error field (assuming the Update() call succeeds)
	// or it will be cleared if no error is returned
	// As well, we retry any error after 5 seconds (Configurable?),
	// and any successful reconcilation does not requeue, as we will be triggered by updates
	defer func() {
		uperr := r1.SyncStatus(err)
		if err != nil || uperr == nil {
			result = ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}
		} else {
			result = ctrl.Result{}
		}
		err = uperr
	}()

	secretClient, err := r1.GetClientForSecret()
	if err != nil {
		return
	}

	r2, err := r1.FetchReferences()
	if err != nil {
		return
	}
	logger.Info("All references fetched")

	r3, err := r2.CreateSecret(secretClient)
	if err != nil {
		return
	}
	logger.Info("Secret created/updated", "result", result)

	err = r3.CleanOtherOwnedSecrets(secretClient)
	if err != nil {
		return
	}

	logger.Info("Status updated, reconciliation complete")
	return
}

type DerivedSecretWatcher struct {
	Reconciler *DerivedSecretReconciler
}

func (w *DerivedSecretWatcher) QueueReferencingDerivedSecrets(kind string, obj client.Object, q workqueue.RateLimitingInterface) {
	logger := log.FromContext(context.TODO()).WithValues("namespace", obj.GetNamespace(), kind, obj.GetName())
	var key string
	switch kind {
	case "Secret":
		key = referencedSecretsKey
	case "ConfigMap":
		key = referencedConfigMapsKey
	default:
		return
	}

	derivedSecrets := secretsv1alpha1.DerivedSecretList{}
	err := w.Reconciler.List(
		context.TODO(),
		&derivedSecrets,
		client.MatchingFields(map[string]string{key: obj.GetName()}),
	)
	if err != nil {
		logger.Info("Failed to find DerivedSecretes which have reference", "error", err)
		return
	}

	if len(derivedSecrets.Items) == 0 {
		return
	}
	logger.Info("Queuing referees", "referees", derivedSecrets.Items)
	for _, item := range derivedSecrets.Items {
		q.AddRateLimited(ctrl.Request{NamespacedName: types.NamespacedName{
			Namespace: item.Namespace,
			Name:      item.Name,
		}})
	}
}

type DerivedSecretSecretWatcher struct {
	DerivedSecretWatcher
}

var (
	_ = handler.EventHandler(&DerivedSecretSecretWatcher{})
)

func (w *DerivedSecretSecretWatcher) QueueSecretReferencingDerivedSecrets(secret client.Object, q workqueue.RateLimitingInterface) {
	w.QueueReferencingDerivedSecrets("Secret", secret, q)
}

func (w *DerivedSecretSecretWatcher) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	w.QueueSecretReferencingDerivedSecrets(e.Object, q)
}

func (w *DerivedSecretSecretWatcher) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	w.QueueSecretReferencingDerivedSecrets(e.ObjectOld, q)
}

func (w *DerivedSecretSecretWatcher) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	w.QueueSecretReferencingDerivedSecrets(e.Object, q)
}

func (w *DerivedSecretSecretWatcher) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	w.QueueSecretReferencingDerivedSecrets(e.Object, q)
}

type DerivedSecretConfigMapWatcher struct {
	DerivedSecretWatcher
}

var (
	_ = handler.EventHandler(&DerivedSecretConfigMapWatcher{})
)

func (w *DerivedSecretConfigMapWatcher) QueueConfigMapReferencingDerivedSecrets(configMap client.Object, q workqueue.RateLimitingInterface) {
	w.DerivedSecretWatcher.QueueReferencingDerivedSecrets("ConfigMap", configMap, q)
}

func (w *DerivedSecretConfigMapWatcher) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	w.QueueConfigMapReferencingDerivedSecrets(e.Object, q)
}

func (w *DerivedSecretConfigMapWatcher) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	w.QueueConfigMapReferencingDerivedSecrets(e.ObjectOld, q)
}

func (w *DerivedSecretConfigMapWatcher) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	w.QueueConfigMapReferencingDerivedSecrets(e.Object, q)
}

func (w *DerivedSecretConfigMapWatcher) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	w.QueueConfigMapReferencingDerivedSecrets(e.Object, q)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DerivedSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Secret{}, secretDerivedFromKey, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		secret := rawObj.(*corev1.Secret)
		owner := metav1.GetControllerOf(secret)
		var ownerStr string
		if owner == nil {
			var ok bool

			ownerStr, ok = DerivedFromFieldValueFromLabels(secret.Labels)
			if !ok {
				return nil
			}
		} else {
			// ...make sure it's a CronJob...
			if owner.APIVersion != secretsv1alpha1.GroupVersion.String() || owner.Kind != "DerivedSecret" {
				return nil
			}
			ownerStr = fmt.Sprintf("%s/%s/%s", owner.Kind, secret.Namespace, owner.Name)
		}
		// ...and if so, return it
		return []string{ownerStr}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &secretsv1alpha1.DerivedSecret{}, referencedSecretsKey, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		derivedSecret := rawObj.(*secretsv1alpha1.DerivedSecret)
		references := make([]string, 0, len(derivedSecret.Spec.References))
		for _, ref := range derivedSecret.Spec.References {
			if ref.SecretRef != nil {
				references = append(references, ref.SecretRef.Name)
			}
		}

		return references
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &secretsv1alpha1.DerivedSecret{}, referencedConfigMapsKey, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		derivedSecret := rawObj.(*secretsv1alpha1.DerivedSecret)
		references := make([]string, 0, len(derivedSecret.Spec.References))
		for _, ref := range derivedSecret.Spec.References {
			if ref.ConfigMapRef != nil {
				references = append(references, ref.ConfigMapRef.Name)
			}
		}

		return references
	}); err != nil {
		return err
	}

	watcher := DerivedSecretWatcher{Reconciler: r}
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1alpha1.DerivedSecret{}).
		Owns(&corev1.Secret{}).
		Watches(&source.Kind{Type: &corev1.Secret{}}, &DerivedSecretSecretWatcher{DerivedSecretWatcher: watcher}).
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, &DerivedSecretConfigMapWatcher{DerivedSecretWatcher: watcher}).
		Complete(r)
}
