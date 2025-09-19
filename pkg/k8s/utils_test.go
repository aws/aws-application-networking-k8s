package k8s

import (
	"context"
	"testing"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestParseBoolAnnotation(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{
			name:     "true lowercase",
			value:    "true",
			expected: true,
		},
		{
			name:     "True capitalized",
			value:    "True",
			expected: true,
		},
		{
			name:     "TRUE uppercase",
			value:    "TRUE",
			expected: true,
		},
		{
			name:     "false lowercase",
			value:    "false",
			expected: false,
		},
		{
			name:     "False capitalized",
			value:    "False",
			expected: false,
		},
		{
			name:     "FALSE uppercase",
			value:    "FALSE",
			expected: false,
		},
		{
			name:     "empty string",
			value:    "",
			expected: false,
		},
		{
			name:     "invalid value",
			value:    "invalid",
			expected: false,
		},
		{
			name:     "numeric value",
			value:    "1",
			expected: false,
		},
		{
			name:     "whitespace only",
			value:    "   ",
			expected: false,
		},
		{
			name:     "true with leading whitespace",
			value:    "  true",
			expected: true,
		},
		{
			name:     "true with trailing whitespace",
			value:    "true  ",
			expected: true,
		},
		{
			name:     "true with surrounding whitespace",
			value:    "  true  ",
			expected: true,
		},
		{
			name:     "false with whitespace",
			value:    "  false  ",
			expected: false,
		},
		{
			name:     "mixed case with whitespace",
			value:    "  True  ",
			expected: true,
		},
		{
			name:     "yes value",
			value:    "yes",
			expected: false,
		},
		{
			name:     "1 numeric",
			value:    "1",
			expected: false,
		},
		{
			name:     "0 numeric",
			value:    "0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseBoolAnnotation(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsStandaloneAnnotationEnabled(t *testing.T) {
	tests := []struct {
		name        string
		obj         client.Object
		expected    bool
		description string
	}{
		{
			name:        "nil object",
			obj:         nil,
			expected:    false,
			description: "should return false for nil object",
		},
		{
			name: "object with no annotations",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
			},
			expected:    false,
			description: "should return false when annotations map is nil",
		},
		{
			name: "object with empty annotations",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-route",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expected:    false,
			description: "should return false when annotations map is empty",
		},
		{
			name: "object with standalone annotation set to true",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "true",
					},
				},
			},
			expected:    true,
			description: "should return true when standalone annotation is 'true'",
		},
		{
			name: "object with standalone annotation set to True",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "True",
					},
				},
			},
			expected:    true,
			description: "should return true when standalone annotation is 'True'",
		},
		{
			name: "object with standalone annotation set to false",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "false",
					},
				},
			},
			expected:    false,
			description: "should return false when standalone annotation is 'false'",
		},
		{
			name: "object with standalone annotation set to invalid value",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "invalid",
					},
				},
			},
			expected:    false,
			description: "should return false when standalone annotation has invalid value",
		},
		{
			name: "object with other annotations but no standalone",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected:    false,
			description: "should return false when standalone annotation is not present",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsStandaloneAnnotationEnabled(tt.obj)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestGetStandaloneModeForRoute(t *testing.T) {
	// Create a scheme for the fake client
	scheme := runtime.NewScheme()
	require.NoError(t, gwv1.Install(scheme))

	tests := []struct {
		name        string
		route       core.Route
		gateways    []client.Object
		expected    bool
		expectError bool
		description string
	}{
		{
			name: "route with standalone annotation true",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "true",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gateway",
						Namespace: "default",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:    true,
			expectError: false,
			description: "should return true when route has standalone annotation set to true",
		},
		{
			name: "route with standalone annotation false, gateway with standalone true",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "false",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gateway",
						Namespace: "default",
						Annotations: map[string]string{
							StandaloneAnnotation: "true",
						},
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:    false,
			expectError: false,
			description: "should return false when route annotation overrides gateway annotation",
		},
		{
			name: "route without annotation, gateway with standalone true",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gateway",
						Namespace: "default",
						Annotations: map[string]string{
							StandaloneAnnotation: "true",
						},
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:    true,
			expectError: false,
			description: "should return true when gateway has standalone annotation and route doesn't",
		},
		{
			name: "route without annotation, gateway without annotation",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gateway",
						Namespace: "default",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:    false,
			expectError: false,
			description: "should return false when neither route nor gateway has standalone annotation",
		},
		{
			name: "route with missing gateway",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "missing-gateway",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:    false,
			expectError: true,
			description: "should return error when gateway is missing",
		},
		{
			name: "route with multiple gateways, one with standalone true",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "gateway-1",
							},
							{
								Name: "gateway-2",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gateway-1",
						Namespace: "default",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gateway-2",
						Namespace: "default",
						Annotations: map[string]string{
							StandaloneAnnotation: "true",
						},
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:    true,
			expectError: false,
			description: "should return true when any parent gateway has standalone annotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with test objects
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.gateways...).
				Build()

			result, err := GetStandaloneModeForRoute(context.Background(), client, tt.route)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, tt.expected, result, tt.description)
			}
		})
	}
}

func TestValidateStandaloneAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		obj         client.Object
		expected    bool
		expectError bool
		description string
	}{
		{
			name:        "nil object",
			obj:         nil,
			expected:    false,
			expectError: true,
			description: "should return error for nil object",
		},
		{
			name: "object with no annotations",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
			},
			expected:    false,
			expectError: false,
			description: "should return false with no error when annotations map is nil",
		},
		{
			name: "object with empty annotations",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-route",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expected:    false,
			expectError: false,
			description: "should return false with no error when annotations map is empty",
		},
		{
			name: "object with valid true annotation",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "true",
					},
				},
			},
			expected:    true,
			expectError: false,
			description: "should return true with no error for valid 'true' annotation",
		},
		{
			name: "object with valid false annotation",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "false",
					},
				},
			},
			expected:    false,
			expectError: false,
			description: "should return false with no error for valid 'false' annotation",
		},
		{
			name: "object with empty annotation value",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "",
					},
				},
			},
			expected:    false,
			expectError: true,
			description: "should return error for empty annotation value",
		},
		{
			name: "object with whitespace-only annotation value",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "   ",
					},
				},
			},
			expected:    false,
			expectError: true,
			description: "should return error for whitespace-only annotation value",
		},
		{
			name: "object with invalid annotation value",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "invalid",
					},
				},
			},
			expected:    false,
			expectError: true,
			description: "should return error for invalid annotation value",
		},
		{
			name: "object with numeric annotation value",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "1",
					},
				},
			},
			expected:    false,
			expectError: true,
			description: "should return error for numeric annotation value",
		},
		{
			name: "object with true annotation with whitespace",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "  true  ",
					},
				},
			},
			expected:    true,
			expectError: false,
			description: "should handle whitespace around valid values",
		},
		{
			name: "object with mixed case true annotation",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "True",
					},
				},
			},
			expected:    true,
			expectError: false,
			description: "should handle mixed case valid values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateStandaloneAnnotation(tt.obj)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestGetStandaloneModeForRouteWithValidation(t *testing.T) {
	// Create a scheme for the fake client
	scheme := runtime.NewScheme()
	require.NoError(t, gwv1.Install(scheme))

	tests := []struct {
		name             string
		route            core.Route
		gateways         []client.Object
		expected         bool
		expectedWarnings int
		expectError      bool
		description      string
	}{
		{
			name:             "nil route",
			route:            nil,
			expected:         false,
			expectedWarnings: 0,
			expectError:      true,
			description:      "should return error for nil route",
		},
		{
			name: "route with invalid annotation value",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "invalid",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gateway",
						Namespace: "default",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:         false,
			expectedWarnings: 1,
			expectError:      false,
			description:      "should return false with warning for invalid route annotation",
		},
		{
			name: "route with empty annotation value",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						StandaloneAnnotation: "",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gateway",
						Namespace: "default",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:         false,
			expectedWarnings: 1,
			expectError:      false,
			description:      "should return false with warning for empty route annotation",
		},
		{
			name: "gateway with invalid annotation value",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gateway",
						Namespace: "default",
						Annotations: map[string]string{
							StandaloneAnnotation: "invalid",
						},
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:         false,
			expectedWarnings: 1,
			expectError:      false,
			description:      "should return false with warning for invalid gateway annotation",
		},
		{
			name: "route being deleted with missing gateway",
			route: func() core.Route {
				now := metav1.Now()
				route := core.NewHTTPRoute(gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-route",
						Namespace:         "default",
						DeletionTimestamp: &now,
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "missing-gateway",
								},
							},
						},
					},
				})
				return route
			}(),
			gateways: []client.Object{
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:         false,
			expectedWarnings: 1,
			expectError:      false,
			description:      "should handle missing gateway gracefully during deletion",
		},
		{
			name: "valid route with valid gateway annotation",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateways: []client.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gateway",
						Namespace: "default",
						Annotations: map[string]string{
							StandaloneAnnotation: "true",
						},
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "application-networking.k8s.aws/gateway-api-controller",
					},
					Spec: gwv1.GatewayClassSpec{
						ControllerName: "application-networking.k8s.aws/gateway-api-controller",
					},
				},
			},
			expected:         true,
			expectedWarnings: 0,
			expectError:      false,
			description:      "should return true with no warnings for valid gateway annotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with test objects
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.gateways...).
				Build()

			result, warnings, err := GetStandaloneModeForRouteWithValidation(context.Background(), client, tt.route)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}

			assert.Equal(t, tt.expected, result, tt.description)
			assert.Len(t, warnings, tt.expectedWarnings, "Expected %d warnings, got %d: %v", tt.expectedWarnings, len(warnings), warnings)
		})
	}
}
