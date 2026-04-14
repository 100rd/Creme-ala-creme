package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Creme-ala-creme/cloudflare-session-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeClock is a controllable clock for testing.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

// fakeRecorder captures events for testing.
type fakeRecorder struct {
	events []string
}

func (r *fakeRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.events = append(r.events, fmt.Sprintf("%s %s %s", eventtype, reason, message))
}

// fakeCFClient is a mock Cloudflare client.
type fakeCFClient struct {
	sessionExists bool
	sessionErr    error
	routeErr      error
	deleteErr     error
}

func (c *fakeCFClient) EnsureSession(_ context.Context, _ string) (bool, error) {
	return c.sessionExists, c.sessionErr
}

func (c *fakeCFClient) EnsureRoute(_ context.Context, _, _ string) error {
	return c.routeErr
}

func (c *fakeCFClient) DeleteRoute(_ context.Context, _ string) error {
	return c.deleteErr
}

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = v1alpha1.AddToScheme(s)
	return s
}

func int64Ptr(v int64) *int64 { return &v }

func TestReconcileActive_ValidSession_PodCreated(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	binding := &v1alpha1.SessionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-binding",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(now),
		},
		Spec: v1alpha1.SessionBindingSpec{
			SessionID:        "valid-session-1",
			TargetDeployment: "my-app",
		},
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "my-app"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "my-app"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "my-app:latest",
						Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
					}},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(binding, deployment).
		WithStatusSubresource(binding).
		Build()

	rec := &fakeRecorder{}
	r := &SessionBindingReconciler{
		Client:   client,
		Scheme:   scheme,
		CFClient: &fakeCFClient{sessionExists: true},
		Recorder: rec,
		Clock:    &fakeClock{now: now},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify pod was created
	pod := &corev1.Pod{}
	err = client.Get(context.Background(), types.NamespacedName{
		Name: "session-valid-session-1", Namespace: "default",
	}, pod)
	if err != nil {
		t.Fatalf("expected session pod to be created, got error: %v", err)
	}

	if pod.Labels[podSessionLabelKey] != "valid-session-1" {
		t.Errorf("pod label %q = %q, want %q", podSessionLabelKey, pod.Labels[podSessionLabelKey], "valid-session-1")
	}

	// Verify event was emitted
	if len(rec.events) == 0 {
		t.Error("expected at least one event to be recorded")
	}
}

func TestReconcileActive_SessionNotFound_Expired(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	binding := &v1alpha1.SessionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-binding",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(now),
		},
		Spec: v1alpha1.SessionBindingSpec{
			SessionID:        "missing-session",
			TargetDeployment: "my-app",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(binding).
		WithStatusSubresource(binding).
		Build()

	r := &SessionBindingReconciler{
		Client:   client,
		Scheme:   scheme,
		CFClient: &fakeCFClient{sessionExists: false},
		Recorder: &fakeRecorder{},
		Clock:    &fakeClock{now: now},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify status is Expired
	updated := &v1alpha1.SessionBinding{}
	_ = client.Get(context.Background(), types.NamespacedName{Name: "test-binding", Namespace: "default"}, updated)
	if updated.Status.Phase != v1alpha1.SessionBindingPhaseExpired {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.SessionBindingPhaseExpired)
	}
}

func TestReconcileActive_TTLExpired(t *testing.T) {
	scheme := newTestScheme()
	creationTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// Set current time to 2 hours after creation, TTL is 1 hour
	now := creationTime.Add(2 * time.Hour)

	binding := &v1alpha1.SessionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-binding",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(creationTime),
		},
		Spec: v1alpha1.SessionBindingSpec{
			SessionID:        "ttl-session",
			TargetDeployment: "my-app",
			TTLSeconds:       int64Ptr(3600), // 1 hour
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(binding).
		WithStatusSubresource(binding).
		Build()

	rec := &fakeRecorder{}
	r := &SessionBindingReconciler{
		Client:   client,
		Scheme:   scheme,
		CFClient: &fakeCFClient{sessionExists: true},
		Recorder: rec,
		Clock:    &fakeClock{now: now},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify status is Expired due to TTL
	updated := &v1alpha1.SessionBinding{}
	_ = client.Get(context.Background(), types.NamespacedName{Name: "test-binding", Namespace: "default"}, updated)
	if updated.Status.Phase != v1alpha1.SessionBindingPhaseExpired {
		t.Errorf("phase = %q, want %q", updated.Status.Phase, v1alpha1.SessionBindingPhaseExpired)
	}

	// Verify TTL event was emitted
	found := false
	for _, e := range rec.events {
		if e == "Normal TTLExpired Session binding expired after 1h0m0s" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected TTLExpired event, got events: %v", rec.events)
	}
}

func TestReconcileActive_TTLNotExpired(t *testing.T) {
	scheme := newTestScheme()
	creationTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// Set current time to 30 minutes after creation, TTL is 1 hour
	now := creationTime.Add(30 * time.Minute)

	binding := &v1alpha1.SessionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-binding",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(creationTime),
		},
		Spec: v1alpha1.SessionBindingSpec{
			SessionID:        "active-session",
			TargetDeployment: "my-app",
			TTLSeconds:       int64Ptr(3600), // 1 hour
		},
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "my-app"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "my-app"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "my-app:latest",
					}},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(binding, deployment).
		WithStatusSubresource(binding).
		Build()

	r := &SessionBindingReconciler{
		Client:   client,
		Scheme:   scheme,
		CFClient: &fakeCFClient{sessionExists: true},
		Recorder: &fakeRecorder{},
		Clock:    &fakeClock{now: now},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify status is NOT Expired (should be Pending since pod just created)
	updated := &v1alpha1.SessionBinding{}
	_ = client.Get(context.Background(), types.NamespacedName{Name: "test-binding", Namespace: "default"}, updated)
	if updated.Status.Phase == v1alpha1.SessionBindingPhaseExpired {
		t.Error("phase should not be Expired when TTL has not elapsed")
	}
}

func TestReconcileActive_InvalidSessionID(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		sessionID string
	}{
		{"empty", ""},
		{"with slashes", "session/with/slashes"},
		{"with spaces", "session with spaces"},
		{"with special chars", "session@#$%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binding := &v1alpha1.SessionBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-binding",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(now),
				},
				Spec: v1alpha1.SessionBindingSpec{
					SessionID:        tt.sessionID,
					TargetDeployment: "my-app",
				},
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(binding).
				WithStatusSubresource(binding).
				Build()

			r := &SessionBindingReconciler{
				Client:   client,
				Scheme:   scheme,
				CFClient: &fakeCFClient{sessionExists: true},
				Recorder: &fakeRecorder{},
				Clock:    &fakeClock{now: now},
			}

			_, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
			})
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}

			updated := &v1alpha1.SessionBinding{}
			_ = client.Get(context.Background(), types.NamespacedName{Name: "test-binding", Namespace: "default"}, updated)
			if updated.Status.Phase != v1alpha1.SessionBindingPhaseError {
				t.Errorf("phase = %q, want %q for invalid sessionID", updated.Status.Phase, v1alpha1.SessionBindingPhaseError)
			}
		})
	}
}

func TestReconcileActive_CloudflareError(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	binding := &v1alpha1.SessionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-binding",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(now),
		},
		Spec: v1alpha1.SessionBindingSpec{
			SessionID:        "error-session",
			TargetDeployment: "my-app",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(binding).
		WithStatusSubresource(binding).
		Build()

	r := &SessionBindingReconciler{
		Client:   client,
		Scheme:   scheme,
		CFClient: &fakeCFClient{sessionErr: fmt.Errorf("cloudflare API timeout")},
		Recorder: &fakeRecorder{},
		Clock:    &fakeClock{now: now},
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Should requeue after error
	if result.RequeueAfter != time.Minute {
		t.Errorf("RequeueAfter = %v, want %v", result.RequeueAfter, time.Minute)
	}
}

func TestHandleDeletion_CleansUpResources(t *testing.T) {
	scheme := newTestScheme()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	deletionTime := metav1.NewTime(now)

	binding := &v1alpha1.SessionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-binding",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
			Finalizers:        []string{sessionBindingFinalizer},
			CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Hour)),
		},
		Spec: v1alpha1.SessionBindingSpec{
			SessionID:        "cleanup-session",
			TargetDeployment: "my-app",
		},
		Status: v1alpha1.SessionBindingStatus{
			BoundPod: "session-cleanup-session",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "session-cleanup-session",
			Namespace: "default",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(binding, pod).
		WithStatusSubresource(binding).
		Build()

	rec := &fakeRecorder{}
	r := &SessionBindingReconciler{
		Client:   client,
		Scheme:   scheme,
		CFClient: &fakeCFClient{},
		Recorder: rec,
		Clock:    &fakeClock{now: now},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify cleanup event was emitted
	found := false
	for _, e := range rec.events {
		if e == "Normal CleanedUp Removed Cloudflare route and session pod" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected CleanedUp event, got events: %v", rec.events)
	}
}

func TestIsPodReady(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "running and ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			want: true,
		},
		{
			name: "running but not ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
					}},
				},
			},
			want: false,
		},
		{
			name: "pending",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPodReady(tt.pod); got != tt.want {
				t.Errorf("isPodReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodEndpoint(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "pod with IP and port",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
					}},
				},
				Status: corev1.PodStatus{PodIP: "10.0.0.1"},
			},
			want: "10.0.0.1:8080",
		},
		{
			name: "pod with IP, no ports (defaults to 80)",
			pod: &corev1.Pod{
				Spec:   corev1.PodSpec{Containers: []corev1.Container{{}}},
				Status: corev1.PodStatus{PodIP: "10.0.0.2"},
			},
			want: "10.0.0.2:80",
		},
		{
			name: "pod without IP",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{PodIP: ""},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := podEndpoint(tt.pod); got != tt.want {
				t.Errorf("podEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}
