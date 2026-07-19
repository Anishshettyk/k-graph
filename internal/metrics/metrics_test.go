package metrics

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestResourcesOf(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				},
				{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("200m"),
						},
					},
				},
			},
		},
	}

	r := ResourcesOf(pod)
	if r.CPUReqMilli != 50 {
		t.Errorf("CPUReqMilli = %d, want 50", r.CPUReqMilli)
	}
	if r.CPULimitMilli != 300 {
		t.Errorf("CPULimitMilli = %d, want 300 (100m + 200m)", r.CPULimitMilli)
	}
	if r.MemReqBytes != 64*1024*1024 {
		t.Errorf("MemReqBytes = %d, want %d", r.MemReqBytes, 64*1024*1024)
	}
	if r.MemLimitBytes != 128*1024*1024 {
		t.Errorf("MemLimitBytes = %d, want %d", r.MemLimitBytes, 128*1024*1024)
	}
}
