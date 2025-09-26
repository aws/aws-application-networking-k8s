package predicates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

func TestRouteChangedPredicate_Update(t *testing.T) {
	predicate := NewRouteChangedPredicate()

	tests := []struct {
		name     string
		oldRoute *gwv1.HTTPRoute
		newRoute *gwv1.HTTPRoute
		expected bool
	}{
		{
			name: "generation changed should trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
			},
			expected: true,
		},
		{
			name: "standalone annotation added should trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "standalone annotation removed should trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			expected: true,
		},
		{
			name: "standalone annotation value changed should trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "false",
					},
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "other annotation changes should not trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						"other-annotation": "value1",
					},
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						"other-annotation": "value2",
					},
				},
			},
			expected: false,
		},
		{
			name: "no changes should not trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateEvent := event.UpdateEvent{
				ObjectOld: tt.oldRoute,
				ObjectNew: tt.newRoute,
			}

			result := predicate.Update(updateEvent)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouteChangedPredicate_Create(t *testing.T) {
	predicate := NewRouteChangedPredicate()

	createEvent := event.CreateEvent{
		Object: &gwv1.HTTPRoute{},
	}

	result := predicate.Create(createEvent)
	assert.True(t, result, "Create events should always trigger reconcile")
}

func TestRouteChangedPredicate_Delete(t *testing.T) {
	predicate := NewRouteChangedPredicate()

	deleteEvent := event.DeleteEvent{
		Object: &gwv1.HTTPRoute{},
	}

	result := predicate.Delete(deleteEvent)
	assert.True(t, result, "Delete events should always trigger reconcile")
}

func TestRouteChangedPredicate_Generic(t *testing.T) {
	predicate := NewRouteChangedPredicate()

	genericEvent := event.GenericEvent{
		Object: &gwv1.HTTPRoute{},
	}

	result := predicate.Generic(genericEvent)
	assert.True(t, result, "Generic events should always trigger reconcile")
}

func TestGetStandaloneAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    string
	}{
		{
			name:        "nil annotations should return empty string",
			annotations: nil,
			expected:    "",
		},
		{
			name:        "empty annotations should return empty string",
			annotations: map[string]string{},
			expected:    "",
		},
		{
			name: "standalone annotation present should return value",
			annotations: map[string]string{
				k8s.StandaloneAnnotation: "true",
			},
			expected: "true",
		},
		{
			name: "other annotations present should return empty string",
			annotations: map[string]string{
				"other-annotation": "value",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStandaloneAnnotation(tt.annotations)
			assert.Equal(t, tt.expected, result)
		})
	}
}
