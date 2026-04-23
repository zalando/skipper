package definitions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIngressV1Definitions(t *testing.T) {
	svc := Service{
		Name: "my-service",
		Port: BackendPortV1{
			Number: 8080,
		},
	}

	host := "my.host.example"
	item := &IngressV1Item{
		Metadata: &Metadata{
			Namespace: "ns",
			Name:      "my-ing",
		},
		Spec: &IngressV1Spec{
			Rules: []*RuleV1{
				{
					Host: host,
					Http: &HTTPRuleV1{
						Paths: []*PathRuleV1{
							{
								Path:     "/foo",
								PathType: "Prefix",
								Backend: &BackendV1{
									Service: svc,
								},
							},
						},
					},
				},
			},
		},
		Status: &IngressV1Status{},
	}

	assert.Equal(t, []string{host}, GetHostsFromIngressRulesV1(item))

	list := IngressV1List{
		Items: []*IngressV1Item{
			item,
		},
	}

	assert.NotEmpty(t, list)
	assert.Equal(t, "8080", svc.Port.String())

	port := BackendPortV1{
		Name: "alt-http",
	}
	assert.Equal(t, "alt-http", port.String())
}

func TestBackendPort(t *testing.T) {
	bp1 := BackendPort{
		Value: "8080",
	}
	bp2 := BackendPort{
		Value: 8080,
	}
	bp3 := BackendPort{
		Value: "alt-http",
	}

	// String()
	assert.Equal(t, "8080", bp1.String())
	assert.Equal(t, "8080", bp2.String())
	assert.Equal(t, "alt-http", bp3.String())

	// Number
	if n, ok := bp1.Number(); ok || n != 0 {
		t.Fatalf("Failed to get 0, got: %d,%v", n, ok)
	}
	if n, ok := bp2.Number(); !ok || n != 8080 {
		t.Fatalf("Failed to get 8080, got: %d,%v", n, ok)
	}
	if n, ok := bp3.Number(); ok || n != 0 {
		t.Fatalf("Failed to get 0, got: %d,%v", n, ok)
	}

	// Marshal/Unmarshal happy path
	bp4 := BackendPort{
		Value: 8080,
	}
	if buf, err := bp4.MarshalJSON(); err != nil {
		t.Fatalf("Failed to serialize to json BackendPort: %v", err)
	} else {
		bp5Ptr := &BackendPort{}
		if err := bp5Ptr.UnmarshalJSON(buf); err != nil {
			t.Fatalf("Failed to deserialize from json to BackendPort: %v", err)
		} else {
			bp5 := *bp5Ptr
			assert.Equal(t, bp4, bp5)
		}
	}
	bpString := `"8080"`
	bp6Ptr := &BackendPort{}
	if err := bp6Ptr.UnmarshalJSON([]byte(bpString)); err != nil {
		t.Fatalf("Failed to unmarshal string to BackendPort: %v", err)
	}

	// Marshal/Unmarshal errors
	bpStringFail := `"Value": 8080`
	bp7Ptr := &BackendPort{}
	if err := bp7Ptr.UnmarshalJSON([]byte(bpStringFail)); err == nil {
		t.Fatalf("Failed to get error to unmarshal string to BackendPort: %q", bpStringFail)
	}
	bpStringFail2 := []byte(`10.25`)
	bp8Ptr := &BackendPort{}
	if err := bp8Ptr.UnmarshalJSON(bpStringFail2); err == nil {
		t.Fatalf("Failed to get error to unmarshal string to BackendPort: %v", bpStringFail2)
	}

	bp9Ptr := &BackendPort{Value: 10.25}
	if _, err := bp9Ptr.MarshalJSON(); err != errInvalidPortType {
		t.Fatalf("Failed to get error to marshal BackendPort to byte slice: %q", bp9Ptr.String())
	}
	if s := bp9Ptr.String(); s != "" {
		t.Fatalf("Failed to get empty string, got: %q", s)
	}

}

func TestJsonToIngressList(t *testing.T) {
	data := `
{
    "apiVersion": "v1",
    "items": [
        {
            "apiVersion": "networking.k8s.io/v1",
            "kind": "Ingress",
            "metadata": {
                "name": "my-app",
                "namespace": "default",
                "resourceVersion": "74980712",
                "uid": "3a0fb729-7d86-45f1-963e-100fbdb83387"
            },
            "spec": {
                "rules": [
                    {
                        "host": "app.example",
                        "http": {
                            "paths": [
                                {
                                    "backend": {
                                        "service": {
                                            "name": "my-app",
                                            "port": {
                                                "number": 80
                                            }
                                        }
                                    },
                                    "pathType": "ImplementationSpecific"
                                }
                            ]
                        }
                    }
                ]
            },
            "status": {
                "loadBalancer": {
                    "ingress": [
                        {
                            "hostname": "kube-ingr-lb-example.com"
                        }
                    ]
                }
            }
        }
    ],
    "kind": "List",
    "metadata": {
        "resourceVersion": ""
    }
}
`
	ingList, err := ParseIngressV1JSON([]byte(data))
	assert.NoError(t, err)
	assert.NotEmpty(t, ingList.Items)
	assert.Equal(t, 1, len(ingList.Items))
	ing := ingList.Items[0]
	port := ing.Spec.Rules[0].Http.Paths[0].Backend.Service.Port
	assert.Equal(t, port.String(), "80")
	assert.Equal(t, []string{"app.example"}, GetHostsFromIngressRulesV1(ing))
}
