package controllers

import (
	"context"
	"fmt"
	"strings"
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

// ---------- Mock types ----------

// fakeClock implements Clock for deterministic testing.
type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time { return f.now }

// fakeCFClient implements cloudflare.Client for testing.
type fakeCFClient struct {
	ensureSessionResult bool
	ensureSessionErr    error
	ensureRouteErr      error
	deleteRouteErr      error

	ensureSessionCalls int
	ensureRouteCalls   int
	deleteRouteCalls   int
	lastRouteEndpoint  string
	lastRouteSessionID string
}

func (f *fakeCFClient) EnsureSession(_ context.Context, sessionID string) (bool, error) {
	f.ensureSessionCalls++
	return f.ensureSessionResult, f.ensureSessionErr
}

func (f *fakeCFClient) EnsureRoute(_ context.Context, sessionID, endpoint string) error {
	f.ensureRouteCalls++
	f.lastRouteSessionID = sessionID
	f.lastRouteEndpoint = endpoint
	return f.ensureRouteErr
}

func (f *fakeCFClient) DeleteRoute(_ context.Context, sessionID string) error {
	f.deleteRouteCalls++
	return f.deleteRouteErr
}

// fakeRecorder implements recordEventRecorder for testing.
type fakeRecorder struct {
	events []string
}

func (f *fakeRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	f.events = append(f.events, fmt.Sprintf("%s %s %s", eventtype, reason, message))
}

// ---------- Helpers ----------

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = v1alpha1.AddToScheme(s)
	return s
}

func newBinding(name, namespace, sessionID, targetDeployment string, ttl *int64) *v1alpha1.SessionBinding {
	return &v1alpha1.SessionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.Now(),
		},
		Spec: v1alpha1.SessionBindingSpec{
			SessionID:        sessionID,
			TargetDeployment: targetDeployment,
			TTLSeconds:       ttl,
		},
	}
}

func newDeployment(name, namespace string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8080},
							},
						},
					},
				},
			},
		},
	}
}

// ---------- sanitizePodName tests ----------

func TestSanitizePodName_ValidInput(t *testing.T) {
	result := sanitizePodName("session-abc123")
	if result != "session-abc123" {
		t.Errorf("expected session-abc123, got %s", result)
	}
}

func TestSanitizePodName_UpperCase(t *testing.T) {
	result := sanitizePodName("Session-ABC123")
	if result != "session-abc123" {
		t.Errorf("expected session-abc123, got %s", result)
	}
}

func TestSanitizePodName_Underscores(t *testing.T) {
	result := sanitizePodName("session_my_id")
	if result != "session-my-id" {
		t.Errorf("expected session-my-id, got %s", result)
	}
}

func TestSanitizePodName_SpecialCharacters(t *testing.T) {
	result := sanitizePodName("session-@#$%^&*()!")
	if result != "session" {
		t.Errorf("expected session, got %s", result)
	}
}

func TestSanitizePodName_LeadingTrailingHyphens(t *testing.T) {
	result := sanitizePodName("--session-abc--")
	if result != "session-abc" {
		t.Errorf("expected session-abc, got %s", result)
	}
}

func TestSanitizePodName_ConsecutiveHyphens(t *testing.T) {
	result := sanitizePodName("session---abc---def")
	if result != "session-abc-def" {
		t.Errorf("expected session-abc-def, got %s", result)
	}
}

func TestSanitizePodName_TooLong(t *testing.T) {
	long := "session-" + strings.Repeat("a", 100)
	result := sanitizePodName(long)
	if len(result) > 63 {
		t.Errorf("expected max 63 chars, got %d", len(result))
	}
}

func TestSanitizePodName_EmptyAfterSanitization(t *testing.T) {
	result := sanitizePodName("@#$%^&*()")
	if result != "session-unknown" {
		t.Errorf("expected session-unknown fallback, got %s", result)
	}
}

func TestSanitizePodName_Dots(t *testing.T) {
	result := sanitizePodName("session.user.123")
	if result != "session-user-123" {
		t.Errorf("expected session-user-123, got %s", result)
	}
}

func TestSanitizePodName_TruncationDoesNotEndWithHyphen(t *testing.T) {
	// Create a string that when truncated to 63 chars would end with a hyphen.
	name := strings.Repeat("a", 62) + "-b"
	result := sanitizePodName(name)
	if len(result) > 63 {
		t.Errorf("expected max 63 chars, got %d", len(result))
	}
	if strings.HasSuffix(result, "-") {
		t.Errorf("result should not end with hyphen: %s", result)
	}
}

// ---------- Reconciler tests ----------

func TestReconcile_EmptySessionID(t *testing.T) {
	scheme := testScheme()
	binding := newBinding("test-binding", "default", "", "my-deploy", nil)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding).
		WithStatusSubresource(binding).Build()

	cfClient := &fakeCFClient{}
	recorder := &fakeRecorder{}
	clock := &fakeClock{now: time.Now()}

	r := &SessionBindingReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		CFClient: cfClient,
		Recorder: recorder,
		Clock:    clock,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", result.RequeueAfter)
	}

	// Verify the binding was updated with Error phase.
	updated := &v1alpha1.SessionBinding{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-binding", Namespace: "default"}, updated); err != nil {
		t.Fatalf("failed to get binding: %v", err)
	}
	if updated.Status.Phase != v1alpha1.SessionBindingPhaseError {
		t.Errorf("expected Error phase, got %s", updated.Status.Phase)
	}
}

func TestReconcile_SessionNotFound(t *testing.T) {
	scheme := testScheme()
	binding := newBinding("test-binding", "default", "sess-123", "my-deploy", nil)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding).
		WithStatusSubresource(binding).Build()

	cfClient := &fakeCFClient{ensureSessionResult: false}
	recorder := &fakeRecorder{}
	clock := &fakeClock{now: time.Now()}

	r := &SessionBindingReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		CFClient: cfClient,
		Recorder: recorder,
		Clock:    clock,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &v1alpha1.SessionBinding{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-binding", Namespace: "default"}, updated)
	if updated.Status.Phase != v1alpha1.SessionBindingPhaseExpired {
		t.Errorf("expected Expired phase, got %s", updated.Status.Phase)
	}
}

func TestReconcile_SessionError(t *testing.T) {
	scheme := testScheme()
	binding := newBinding("test-binding", "default", "sess-123", "my-deploy", nil)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding).
		WithStatusSubresource(binding).Build()

	cfClient := &fakeCFClient{
		ensureSessionResult: false,
		ensureSessionErr:    fmt.Errorf("connection timeout"),
	}
	recorder := &fakeRecorder{}
	clock := &fakeClock{now: time.Now()}

	r := &SessionBindingReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		CFClient: cfClient,
		Recorder: recorder,
		Clock:    clock,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != time.Minute {
		t.Errorf("expected 1 minute requeue, got %v", result.RequeueAfter)
	}

	updated := &v1alpha1.SessionBinding{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-binding", Namespace: "default"}, updated)
	if updated.Status.Phase != v1alpha1.SessionBindingPhaseError {
		t.Errorf("expected Error phase, got %s", updated.Status.Phase)
	}
}

func TestReconcile_TTLExpired(t *testing.T) {
	scheme := testScheme()
	ttl := int64(300) // 5 minutes
	binding := newBinding("test-binding", "default", "sess-ttl", "my-deploy", &ttl)
	// Set creation time to 10 minutes ago.
	binding.CreationTimestamp = metav1.Time{Time: time.Now().Add(-10 * time.Minute)}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding).
		WithStatusSubresource(binding).Build()

	cfClient := &fakeCFClient{ensureSessionResult: true}
	recorder := &fakeRecorder{}
	clock := &fakeClock{now: time.Now()} // Now is after creation + TTL

	r := &SessionBindingReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		CFClient: cfClient,
		Recorder: recorder,
		Clock:    clock,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &v1alpha1.SessionBinding{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-binding", Namespace: "default"}, updated)
	if updated.Status.Phase != v1alpha1.SessionBindingPhaseExpired {
		t.Errorf("expected Expired phase, got %s", updated.Status.Phase)
	}

	// Verify EnsureSession was NOT called (TTL check happens before session check).
	if cfClient.ensureSessionCalls != 0 {
		t.Errorf("expected 0 EnsureSession calls (TTL expired early), got %d", cfClient.ensureSessionCalls)
	}

	// Verify TTLExpired event was recorded.
	foundTTLEvent := false
	for _, e := range recorder.events {
		if strings.Contains(e, "TTLExpired") {
			foundTTLEvent = true
			break
		}
	}
	if !foundTTLEvent {
		t.Error("expected TTLExpired event to be recorded")
	}
}

func TestReconcile_TTLNotExpired(t *testing.T) {
	scheme := testScheme()
	ttl := int64(3600) // 1 hour
	binding := newBinding("test-binding", "default", "sess-alive", "my-deploy", &ttl)
	// Created just now.
	binding.CreationTimestamp = metav1.Time{Time: time.Now()}

	deploy := newDeployment("my-deploy", "default")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(binding, deploy).
		WithStatusSubresource(binding).Build()

	cfClient := &fakeCFClient{ensureSessionResult: true}
	recorder := &fakeRecorder{}
	clock := &fakeClock{now: time.Now()}

	r := &SessionBindingReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		CFClient: cfClient,
		Recorder: recorder,
		Clock:    clock,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Session should proceed normally (EnsureSession called).
	if cfClient.ensureSessionCalls != 1 {
		t.Errorf("expected 1 EnsureSession call, got %d", cfClient.ensureSessionCalls)
	}
}

func TestReconcile_BindingNotFound(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &SessionBindingReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		CFClient: &fakeCFClient{},
		Recorder: &fakeRecorder{},
		Clock:    &fakeClock{now: time.Now()},
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Error("expected no requeue for not found binding")
	}
}

func TestReconcile_DeploymentNotFound(t *testing.T) {
	scheme := testScheme()
	binding := newBinding("test-binding", "default", "sess-123", "missing-deploy", nil)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(binding).
		WithStatusSubresource(binding).Build()

	cfClient := &fakeCFClient{ensureSessionResult: true}
	recorder := &fakeRecorder{}
	clock := &fakeClock{now: time.Now()}

	r := &SessionBindingReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		CFClient: cfClient,
		Recorder: recorder,
		Clock:    clock,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-binding", Namespace: "default"},
	})
	// Should error because the deployment does not exist.
	if err == nil {
		t.Fatal("expected error when deployment is missing")
	}
}

// ---------- isPodReady tests ----------

func TestIsPodReady_Running(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	if !isPodReady(pod) {
		t.Error("expected pod to be ready")
	}
}

func TestIsPodReady_NotRunning(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	if isPodReady(pod) {
		t.Error("expected pod to not be ready")
	}
}

func TestIsPodReady_RunningButNotReady(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
		},
	}
	if isPodReady(pod) {
		t.Error("expected pod to not be ready")
	}
}

// ---------- podEndpoint tests ----------

func TestPodEndpoint_WithIP(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
				},
			},
		},
		Status: corev1.PodStatus{PodIP: "10.0.0.5"},
	}
	result := podEndpoint(pod)
	if result != "10.0.0.5:8080" {
		t.Errorf("expected 10.0.0.5:8080, got %s", result)
	}
}

func TestPodEndpoint_NoIP(t *testing.T) {
	pod := &corev1.Pod{
		Spec:   corev1.PodSpec{},
		Status: corev1.PodStatus{PodIP: ""},
	}
	result := podEndpoint(pod)
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

func TestPodEndpoint_DefaultPort(t *testing.T) {
	pod := &corev1.Pod{
		Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
		Status: corev1.PodStatus{PodIP: "10.0.0.1"},
	}
	result := podEndpoint(pod)
	if result != "10.0.0.1:80" {
		t.Errorf("expected 10.0.0.1:80, got %s", result)
	}
}
