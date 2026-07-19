package checker

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anishetty/kgraph/internal/graph"
)

func buildTestGraph() *graph.Graph {
	return graph.Build(graph.Resources{
		ConfigMaps: []corev1.ConfigMap{
			{ObjectMeta: metav1.ObjectMeta{UID: "cm1", Name: "app-config", Namespace: "prod"}},
		},
		Secrets: []corev1.Secret{
			{ObjectMeta: metav1.ObjectMeta{UID: "sec1", Name: "tls-cert", Namespace: "prod"}},
		},
		PVCs: []corev1.PersistentVolumeClaim{
			{ObjectMeta: metav1.ObjectMeta{UID: "pvc1", Name: "data-vol", Namespace: "prod"},
				Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}},
		},
		Services: []corev1.Service{
			{ObjectMeta: metav1.ObjectMeta{UID: "svc1", Name: "api-svc", Namespace: "prod"}},
		},
	})
}

const deploymentYAML = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  template:
    spec:
      serviceAccountName: default
      volumes:
        - name: config
          configMap:
            name: app-config
        - name: tls
          secret:
            secretName: missing-tls
        - name: data
          persistentVolumeClaim:
            claimName: data-vol
      containers:
        - name: api
          image: api:v1
          envFrom:
            - configMapRef:
                name: app-config
            - secretRef:
                name: tls-cert
          env:
            - name: DB_PASS
              valueFrom:
                secretKeyRef:
                  name: missing-db-secret
                  key: password
`

func TestCheckDetectsMissingAndFound(t *testing.T) {
	g := buildTestGraph()
	result, err := CheckReader(g, strings.NewReader(deploymentYAML), "test.yaml", "prod")
	if err != nil {
		t.Fatalf("CheckReader() error = %v", err)
	}
	if len(result.Findings) == 0 {
		t.Fatal("expected findings")
	}

	found := map[string]bool{}
	notFound := map[string]bool{}
	for _, f := range result.Findings {
		key := f.RefKind + "/" + f.RefName
		if f.Found {
			found[key] = true
		} else {
			notFound[key] = true
		}
	}

	// app-config and tls-cert and data-vol exist
	for _, ref := range []string{"ConfigMap/app-config", "Secret/tls-cert", "PersistentVolumeClaim/data-vol"} {
		if !found[ref] {
			t.Errorf("expected %s to be found", ref)
		}
	}
	// missing-tls and missing-db-secret do not exist
	for _, ref := range []string{"Secret/missing-tls", "Secret/missing-db-secret"} {
		if !notFound[ref] {
			t.Errorf("expected %s to be missing", ref)
		}
	}
}

const ingressYAML = `
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: main-ingress
  namespace: prod
spec:
  rules:
    - host: api.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: api-svc
                port:
                  number: 80
          - path: /v2
            pathType: Prefix
            backend:
              service:
                name: missing-svc
                port:
                  number: 80
`

func TestCheckIngressBackends(t *testing.T) {
	g := buildTestGraph()
	result, err := CheckReader(g, strings.NewReader(ingressYAML), "ingress.yaml", "prod")
	if err != nil {
		t.Fatalf("CheckReader() error = %v", err)
	}

	found := map[string]bool{}
	notFound := map[string]bool{}
	for _, f := range result.Findings {
		key := f.RefKind + "/" + f.RefName
		if f.Found {
			found[key] = true
		} else {
			notFound[key] = true
		}
	}

	if !found["Service/api-svc"] {
		t.Error("expected Service/api-svc to be found")
	}
	if !notFound["Service/missing-svc"] {
		t.Error("expected Service/missing-svc to be missing")
	}
}

const multiDocYAML = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: prod
spec:
  template:
    spec:
      volumes:
        - name: cfg
          configMap:
            name: missing-cm
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  namespace: prod
spec:
  template:
    spec:
      volumes:
        - name: cfg
          configMap:
            name: app-config
`

func TestCheckMultiDocument(t *testing.T) {
	g := buildTestGraph()
	result, err := CheckReader(g, strings.NewReader(multiDocYAML), "multi.yaml", "prod")
	if err != nil {
		t.Fatalf("CheckReader() error = %v", err)
	}
	blocking := result.Blocking()
	if len(blocking) != 1 || blocking[0].RefName != "missing-cm" {
		t.Errorf("blocking findings = %+v", blocking)
	}
}
