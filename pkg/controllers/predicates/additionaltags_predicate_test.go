package predicates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

func TestAdditionalTagsAnnotationChangedPredicate_Update(t *testing.T) {
	predicate := AdditionalTagsAnnotationChangedPredicate

	tests := []struct {
		name     string
		oldRoute *gwv1.HTTPRoute
		newRoute *gwv1.HTTPRoute
		expected bool
	}{
		{
			name: "additional tags annotation added should trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Dev,Project=MyApp",
					},
				},
			},
			expected: true,
		},
		{
			name: "additional tags annotation removed should trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Dev,Project=MyApp",
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
			name: "additional tags annotation value changed should trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Dev,Project=MyApp",
					},
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Prod,Project=MyApp",
					},
				},
			},
			expected: true,
		},
		{
			name: "additional tags annotation unchanged should not trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Dev,Project=MyApp",
					},
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Dev,Project=MyApp",
					},
				},
			},
			expected: false,
		},
		{
			name: "other annotation changes should not trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						"other-annotation":    "value1",
						k8s.TagsAnnotationKey: "Environment=Dev",
					},
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						"other-annotation":    "value2",
						k8s.TagsAnnotationKey: "Environment=Dev",
					},
				},
			},
			expected: false,
		},
		{
			name: "no annotations to no annotations should not trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			expected: false,
		},
		{
			name: "empty additional tags to empty additional tags should not trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "",
					},
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "",
					},
				},
			},
			expected: false,
		},
		{
			name: "empty additional tags to non-empty should trigger reconcile",
			oldRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "",
					},
				},
			},
			newRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Dev",
					},
				},
			},
			expected: true,
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

func TestAdditionalTagsAnnotationChangedPredicate_Create(t *testing.T) {
	predicate := AdditionalTagsAnnotationChangedPredicate

	tests := []struct {
		name     string
		route    *gwv1.HTTPRoute
		expected bool
	}{
		{
			name: "create with additional tags annotation should trigger reconcile",
			route: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Dev,Project=MyApp",
					},
				},
			},
			expected: true,
		},
		{
			name: "create without additional tags annotation should not trigger reconcile",
			route: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expected: false,
		},
		{
			name: "create with empty additional tags annotation should not trigger reconcile",
			route: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "",
					},
				},
			},
			expected: false,
		},
		{
			name: "create with other annotations but no additional tags should not trigger reconcile",
			route: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createEvent := event.CreateEvent{
				Object: tt.route,
			}

			result := predicate.Create(createEvent)
			assert.Equal(t, tt.expected, result)
		})
	}
}
