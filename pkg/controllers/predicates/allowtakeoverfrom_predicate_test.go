package predicates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

func TestAllowTakeoverFromAnnotationChangedPredicate_UpdateFunc(t *testing.T) {
	tests := []struct {
		name           string
		oldAnnotations map[string]string
		newAnnotations map[string]string
		expected       bool
	}{
		{
			name:           "annotation added",
			oldAnnotations: nil,
			newAnnotations: map[string]string{
				k8s.AllowTakeoverFromAnnotation: "123456789012/old-cluster/vpc-123",
			},
			expected: true,
		},
		{
			name: "annotation removed",
			oldAnnotations: map[string]string{
				k8s.AllowTakeoverFromAnnotation: "123456789012/old-cluster/vpc-123",
			},
			newAnnotations: nil,
			expected:       true,
		},
		{
			name: "annotation unchanged",
			oldAnnotations: map[string]string{
				k8s.AllowTakeoverFromAnnotation: "123456789012/cluster/vpc-123",
			},
			newAnnotations: map[string]string{
				k8s.AllowTakeoverFromAnnotation: "123456789012/cluster/vpc-123",
			},
			expected: false,
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
			oldRoute := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.oldAnnotations,
				},
			}

			newRoute := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.newAnnotations,
				},
			}

			updateEvent := event.UpdateEvent{
				ObjectOld: oldRoute,
				ObjectNew: newRoute,
			}

			result := AllowTakeoverFromAnnotationChangedPredicate.Update(updateEvent)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAllowTakeoverFromAnnotationChangedPredicate_CreateFunc(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name: "create with takeover annotation",
			annotations: map[string]string{
				k8s.AllowTakeoverFromAnnotation: "123456789012/cluster/vpc-123",
			},
			expected: true,
		},
		{
			name:        "create without takeover annotation",
			annotations: nil,
			expected:    false,
		},
		{
			name: "create with empty takeover annotation",
			annotations: map[string]string{
				k8s.AllowTakeoverFromAnnotation: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}

			createEvent := event.CreateEvent{
				Object: route,
			}

			result := AllowTakeoverFromAnnotationChangedPredicate.Create(createEvent)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetAllowTakeoverFromAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    string
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    "",
		},
		{
			name: "annotation present",
			annotations: map[string]string{
				k8s.AllowTakeoverFromAnnotation: "123456789012/cluster/vpc-123",
			},
			expected: "123456789012/cluster/vpc-123",
		},
		{
			name: "annotation empty",
			annotations: map[string]string{
				k8s.AllowTakeoverFromAnnotation: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAllowTakeoverFromAnnotation(tt.annotations)
			assert.Equal(t, tt.expected, result)
		})
	}
}
