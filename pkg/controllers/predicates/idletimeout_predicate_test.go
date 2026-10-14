package predicates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

func TestIdleTimeoutSecondsAnnotationChangedPredicate_UpdateFunc(t *testing.T) {
	tests := []struct {
		name           string
		oldAnnotations map[string]string
		newAnnotations map[string]string
		expected       bool
	}{
		{
			name:           "annotation added",
			oldAnnotations: nil,
			newAnnotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "120"},
			expected:       true,
		},
		{
			name:           "annotation removed",
			oldAnnotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "120"},
			newAnnotations: nil,
			expected:       true,
		},
		{
			name:           "annotation changed",
			oldAnnotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "120"},
			newAnnotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "300"},
			expected:       true,
		},
		{
			name:           "annotation unchanged",
			oldAnnotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "120"},
			newAnnotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "120"},
			expected:       false,
		},
		{
			name:           "whitespace-only difference is not a change",
			oldAnnotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "120"},
			newAnnotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "120 "},
			expected:       false,
		},
		{
			name:           "unrelated annotation changed only",
			oldAnnotations: map[string]string{"other": "a"},
			newAnnotations: map[string]string{"other": "b"},
			expected:       false,
		},
		{
			name:           "no annotation in both",
			oldAnnotations: nil,
			newAnnotations: nil,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Annotations: tt.oldAnnotations}}
			newRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Annotations: tt.newAnnotations}}
			updateEvent := event.UpdateEvent{ObjectOld: oldRoute, ObjectNew: newRoute}

			result := IdleTimeoutSecondsAnnotationChangedPredicate.Update(updateEvent)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIdleTimeoutSecondsAnnotationChangedPredicate_CreateFunc(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "create with idle timeout annotation",
			annotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "120"},
			expected:    true,
		},
		{
			name:        "create without idle timeout annotation",
			annotations: nil,
			expected:    false,
		},
		{
			name:        "create with empty idle timeout annotation",
			annotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: ""},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			createEvent := event.CreateEvent{Object: route}

			result := IdleTimeoutSecondsAnnotationChangedPredicate.Create(createEvent)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetIdleTimeoutSecondsAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    string
	}{
		{name: "nil annotations", annotations: nil, expected: ""},
		{name: "annotation present", annotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: "300"}, expected: "300"},
		{name: "annotation empty", annotations: map[string]string{k8s.IdleTimeoutSecondsAnnotation: ""}, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIdleTimeoutSecondsAnnotation(tt.annotations)
			assert.Equal(t, tt.expected, result)
		})
	}
}
