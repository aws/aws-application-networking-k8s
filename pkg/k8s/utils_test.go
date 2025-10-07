package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-sdk-go/aws"
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

func TestParseTagsFromAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotation  string
		expected    Tags
		description string
	}{
		{
			name:        "empty annotation",
			annotation:  "",
			expected:    Tags{},
			description: "should return empty map for empty annotation",
		},
		{
			name:        "multiple tags",
			annotation:  "Environment=Dev,Project=MyApp,Team=Platform",
			expected:    Tags{"Environment": aws.String("Dev"), "Project": aws.String("MyApp"), "Team": aws.String("Platform")},
			description: "should parse multiple tags correctly",
		},
		{
			name:        "tags with spaces",
			annotation:  "Environment = Dev , Project = MyApp",
			expected:    Tags{"Environment": aws.String("Dev"), "Project": aws.String("MyApp")},
			description: "should handle spaces around keys and values",
		},
		{
			name:        "invalid tag format",
			annotation:  "Environment,Project=MyApp",
			expected:    Tags{"Project": aws.String("MyApp")},
			description: "should skip invalid tag format and parse valid ones",
		},
		{
			name:        "empty key or value",
			annotation:  "=Dev,Project=,Team=Platform",
			expected:    Tags{"Team": aws.String("Platform")},
			description: "should skip tags with empty keys or values",
		},
		{
			name:        "trailing comma",
			annotation:  "Environment=Dev,Project=MyApp,",
			expected:    Tags{"Environment": aws.String("Dev"), "Project": aws.String("MyApp")},
			description: "should handle trailing comma gracefully",
		},
		{
			name:        "whitespace only pairs",
			annotation:  "Environment=Dev,   ,Project=MyApp",
			expected:    Tags{"Environment": aws.String("Dev"), "Project": aws.String("MyApp")},
			description: "should skip whitespace-only pairs",
		},
		{
			name:        "tag key too long",
			annotation:  strings.Repeat("a", 129) + "=value,Project=MyApp",
			expected:    Tags{"Project": aws.String("MyApp")},
			description: "should skip tags with keys longer than 128 characters",
		},
		{
			name:        "tag value too long",
			annotation:  "Environment=Dev,key=" + strings.Repeat("v", 257),
			expected:    Tags{"Environment": aws.String("Dev")},
			description: "should skip tags with values longer than 256 characters",
		},
		{
			name:        "duplicate keys",
			annotation:  "Environment=Dev,Project=App1,Environment=Prod,Team=Platform",
			expected:    Tags{"Environment": aws.String("Dev"), "Project": aws.String("App1"), "Team": aws.String("Platform")},
			description: "should keep first occurrence of duplicate keys",
		},
		{
			name:        "invalid characters in key",
			annotation:  "Env#ironment=Dev,Project=MyApp",
			expected:    Tags{"Project": aws.String("MyApp")},
			description: "should skip tags with invalid characters in key",
		},
		{
			name:        "invalid characters in value",
			annotation:  "Environment=Dev,Project=My$App",
			expected:    Tags{"Environment": aws.String("Dev")},
			description: "should skip tags with invalid characters in value",
		},
		{
			name: "more than 50 tags should keep 50 tags",
			annotation: func() string {
				var pairs []string
				for i := 1; i <= 60; i++ {
					pairs = append(pairs, "key"+string(rune(i/10+48))+string(rune(i%10+48))+"=value"+string(rune(i/10+48))+string(rune(i%10+48)))
				}
				return strings.Join(pairs, ",")
			}(),
			expected: func() Tags {
				tags := make(Tags)
				for i := 1; i <= 50; i++ {
					key := "key" + string(rune(i/10+48)) + string(rune(i%10+48))
					value := "value" + string(rune(i/10+48)) + string(rune(i%10+48))
					tags[key] = aws.String(value)
				}
				return tags
			}(),
			description: "should limit to 50 tags maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseTagsFromAnnotation(tt.annotation)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestGetNonAWSManagedTags(t *testing.T) {
	tests := []struct {
		name        string
		tags        Tags
		expected    Tags
		description string
	}{
		{
			name:        "nil tags",
			tags:        nil,
			expected:    Tags{},
			description: "should return empty map for nil input",
		},
		{
			name:        "empty tags",
			tags:        Tags{},
			expected:    Tags{},
			description: "should return empty map for empty input",
		},
		{
			name: "only additional tags",
			tags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
			},
			expected: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
			},
			description: "should return all tags when no AWS managed tags present",
		},
		{
			name: "only AWS managed tags",
			tags: Tags{
				"application-networking.k8s.aws/ManagedBy": aws.String("123456789/cluster/vpc-123"),
				"application-networking.k8s.aws/RouteType": aws.String("http"),
				"application-networking.k8s.aws/RouteName": aws.String("test-route"),
			},
			expected:    Tags{},
			description: "should return empty map when only AWS managed tags present",
		},
		{
			name: "mixed tags",
			tags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
				"application-networking.k8s.aws/ManagedBy": aws.String("123456789/cluster/vpc-123"),
				"application-networking.k8s.aws/RouteType": aws.String("http"),
				"Team": aws.String("Platform"),
			},
			expected: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
				"Team":        aws.String("Platform"),
			},
			description: "should filter out AWS managed tags and keep additional tags",
		},
		{
			name: "AWS reserved tags lowercase",
			tags: Tags{
				"aws:cloudformation:stack-name": aws.String("my-stack"),
				"aws:region":                    aws.String("us-west-2"),
				"Environment":                   aws.String("Dev"),
			},
			expected: Tags{
				"Environment": aws.String("Dev"),
			},
			description: "should filter out lowercase aws: prefixed tags",
		},
		{
			name: "AWS reserved tags uppercase",
			tags: Tags{
				"AWS:CloudFormation:StackName": aws.String("my-stack"),
				"AWS:Region":                   aws.String("us-west-2"),
				"Environment":                  aws.String("Dev"),
			},
			expected: Tags{
				"Environment": aws.String("Dev"),
			},
			description: "should filter out uppercase AWS: prefixed tags",
		},
		{
			name: "AWS reserved tags mixed case",
			tags: Tags{
				"Aws:Service":  aws.String("ec2"),
				"aWs:Resource": aws.String("instance"),
				"Environment":  aws.String("Dev"),
			},
			expected: Tags{
				"Environment": aws.String("Dev"),
			},
			description: "should filter out mixed case aws: prefixed tags",
		},
		{
			name: "tags that start with aws but not aws:",
			tags: Tags{
				"awesome":     aws.String("value"),
				"aws-region":  aws.String("us-west-2"),
				"Environment": aws.String("Dev"),
			},
			expected: Tags{
				"awesome":     aws.String("value"),
				"aws-region":  aws.String("us-west-2"),
				"Environment": aws.String("Dev"),
			},
			description: "should keep tags that start with 'aws' but not 'aws:'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNonAWSManagedTags(tt.tags)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestCalculateTagDifference(t *testing.T) {
	tests := []struct {
		name             string
		currentTags      Tags
		desiredTags      Tags
		expectedToAdd    Tags
		expectedToRemove []string
		description      string
	}{
		{
			name:             "both nil",
			currentTags:      nil,
			desiredTags:      nil,
			expectedToAdd:    Tags{},
			expectedToRemove: []string{},
			description:      "should handle nil inputs gracefully",
		},
		{
			name:        "current nil, desired has tags",
			currentTags: nil,
			desiredTags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
			},
			expectedToAdd: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
			},
			expectedToRemove: []string{},
			description:      "should add all desired tags when current is nil",
		},
		{
			name: "current has tags, desired nil",
			currentTags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
			},
			desiredTags:      nil,
			expectedToAdd:    Tags{},
			expectedToRemove: []string{"Environment", "Project"},
			description:      "should remove all current tags when desired is nil",
		},
		{
			name: "no changes needed",
			currentTags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
			},
			desiredTags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
			},
			expectedToAdd:    Tags{},
			expectedToRemove: []string{},
			description:      "should return empty when tags are identical",
		},
		{
			name: "add new tags",
			currentTags: Tags{
				"Environment": aws.String("Dev"),
			},
			desiredTags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
				"Team":        aws.String("Platform"),
			},
			expectedToAdd: Tags{
				"Project": aws.String("MyApp"),
				"Team":    aws.String("Platform"),
			},
			expectedToRemove: []string{},
			description:      "should add new tags",
		},
		{
			name: "remove tags",
			currentTags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
				"Team":        aws.String("Platform"),
			},
			desiredTags: Tags{
				"Environment": aws.String("Dev"),
			},
			expectedToAdd:    Tags{},
			expectedToRemove: []string{"Project", "Team"},
			description:      "should remove unwanted tags",
		},
		{
			name: "update tag values",
			currentTags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("OldApp"),
			},
			desiredTags: Tags{
				"Environment": aws.String("Prod"),
				"Project":     aws.String("NewApp"),
			},
			expectedToAdd: Tags{
				"Environment": aws.String("Prod"),
				"Project":     aws.String("NewApp"),
			},
			expectedToRemove: []string{},
			description:      "should update changed tag values",
		},
		{
			name: "mixed operations",
			currentTags: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("OldApp"),
				"OldTag":      aws.String("OldValue"),
			},
			desiredTags: Tags{
				"Environment": aws.String("Prod"),
				"Project":     aws.String("OldApp"),
				"NewTag":      aws.String("NewValue"),
			},
			expectedToAdd: Tags{
				"Environment": aws.String("Prod"),
				"NewTag":      aws.String("NewValue"),
			},
			expectedToRemove: []string{"OldTag"},
			description:      "should handle add, update, and remove operations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagsToAdd, tagsToRemove := CalculateTagDifference(tt.currentTags, tt.desiredTags)

			assert.Equal(t, tt.expectedToAdd, tagsToAdd, tt.description)

			removeStrings := make([]string, len(tagsToRemove))
			for i, tag := range tagsToRemove {
				if tag != nil {
					removeStrings[i] = *tag
				}
			}
			assert.ElementsMatch(t, tt.expectedToRemove, removeStrings, tt.description)
		})
	}
}

func TestGetAdditionalTagsFromAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		obj         client.Object
		expected    Tags
		description string
	}{
		{
			name:        "nil object",
			obj:         nil,
			expected:    nil,
			description: "should return nil for nil object",
		},
		{
			name: "object with no annotations",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
			},
			expected:    nil,
			description: "should return nil when annotations map is nil",
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
			expected:    nil,
			description: "should return nil when annotations map is empty",
		},
		{
			name: "object without tags annotation",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected:    nil,
			description: "should return nil when tags annotation is not present",
		},
		{
			name: "object with empty tags annotation",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						TagsAnnotationKey: "",
					},
				},
			},
			expected:    nil,
			description: "should return nil when tags annotation is empty",
		},
		{
			name: "object with valid tags annotation",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						TagsAnnotationKey: "Environment=Dev,Project=MyApp",
					},
				},
			},
			expected: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
			},
			description: "should parse and return valid tags",
		},
		{
			name: "object with tags containing AWS managed tags",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						TagsAnnotationKey: "Environment=Dev,application-networking.k8s.aws/ManagedBy=test-override,Project=MyApp",
					},
				},
			},
			expected: Tags{
				"Environment": aws.String("Dev"),
				"Project":     aws.String("MyApp"),
			},
			description: "should filter out AWS managed tags and return only additional tags",
		},
		{
			name: "object with only AWS managed tags",
			obj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						TagsAnnotationKey: "application-networking.k8s.aws/ManagedBy=test-override,application-networking.k8s.aws/RouteType=http",
					},
				},
			},
			expected:    nil,
			description: "should return nil when only AWS managed tags are present",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAdditionalTagsFromAnnotations(context.Background(), tt.obj)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}
