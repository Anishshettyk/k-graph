package api

// history_api.go — /api/history: rollout history from owned ReplicaSets.

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type PodSummary struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Phase   string `json:"phase"`
}

type RSRevision struct {
	Name            string       `json:"name"`
	Namespace       string       `json:"namespace"`
	CreatedAt       time.Time    `json:"createdAt"`
	DesiredReplicas int32        `json:"desiredReplicas"`
	ReadyReplicas   int32        `json:"readyReplicas"`
	Replicas        int32        `json:"replicas"`
	Images          []string     `json:"images"`
	IsCurrent       bool         `json:"isCurrent"`
	Pods            []PodSummary `json:"pods"`
}

type HistoryResponse struct {
	Context   string       `json:"context"`
	Kind      string       `json:"kind"`
	Name      string       `json:"name"`
	Namespace string       `json:"namespace"`
	Revisions []RSRevision `json:"revisions"`
}

func (h *Handler) handleHistory(w http.ResponseWriter, r *http.Request) {
	_, ctxName, _, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		writeError(w, http.StatusBadRequest, "resource parameter required")
		return
	}
	kindAlias, name := parseResourceRef(resource)
	kind := expandKind(strings.ToLower(kindAlias))

	resp := HistoryResponse{Context: ctxName, Kind: kind, Revisions: []RSRevision{}}

	// pod owner → pods
	podsByOwner := map[types.UID][]corev1.Pod{}
	for _, pod := range res.Pods {
		for _, ref := range pod.OwnerReferences {
			podsByOwner[ref.UID] = append(podsByOwner[ref.UID], pod)
		}
	}

	switch kind {
	case "Deployment":
		var deploy *appsv1.Deployment
		for i := range res.Deployments {
			if res.Deployments[i].Name == name {
				deploy = &res.Deployments[i]
				break
			}
		}
		if deploy == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("Deployment %q not found", name))
			return
		}
		resp.Name, resp.Namespace = deploy.Name, deploy.Namespace
		for _, rs := range res.ReplicaSets {
			if !ownedBy(rs.OwnerReferences, deploy.UID) {
				continue
			}
			resp.Revisions = append(resp.Revisions, buildRSRevision(rs, podsByOwner))
		}
		sort.Slice(resp.Revisions, func(i, j int) bool {
			return resp.Revisions[i].CreatedAt.After(resp.Revisions[j].CreatedAt)
		})
		// mark current = most recent with replicas > 0
		for i := range resp.Revisions {
			if resp.Revisions[i].Replicas > 0 {
				resp.Revisions[i].IsCurrent = true
				break
			}
		}
		if len(resp.Revisions) > 0 {
			allFalse := true
			for _, r := range resp.Revisions {
				if r.IsCurrent {
					allFalse = false
				}
			}
			if allFalse {
				resp.Revisions[0].IsCurrent = true
			}
		}

	case "StatefulSet":
		var sts *appsv1.StatefulSet
		for i := range res.StatefulSets {
			if res.StatefulSets[i].Name == name {
				sts = &res.StatefulSets[i]
				break
			}
		}
		if sts == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("StatefulSet %q not found", name))
			return
		}
		resp.Name, resp.Namespace = sts.Name, sts.Namespace
		desired := int32(1)
		if sts.Spec.Replicas != nil {
			desired = *sts.Spec.Replicas
		}
		rev := RSRevision{
			Name: sts.Name, Namespace: sts.Namespace,
			CreatedAt:       sts.CreationTimestamp.Time,
			DesiredReplicas: desired,
			ReadyReplicas:   sts.Status.ReadyReplicas,
			Replicas:        sts.Status.Replicas,
			Images:          containerImages(sts.Spec.Template.Spec.Containers),
			IsCurrent:       true,
		}
		for _, pod := range podsByOwner[sts.UID] {
			rev.Pods = append(rev.Pods, makePodSummary(pod))
		}
		resp.Revisions = []RSRevision{rev}

	default:
		writeError(w, http.StatusBadRequest, "history is supported for Deployment and StatefulSet")
		return
	}

	writeJSON(w, resp)
}

func ownedBy(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, r := range refs {
		if r.UID == uid {
			return true
		}
	}
	return false
}

func buildRSRevision(rs appsv1.ReplicaSet, podsByOwner map[types.UID][]corev1.Pod) RSRevision {
	desired := int32(0)
	if rs.Spec.Replicas != nil {
		desired = *rs.Spec.Replicas
	}
	rev := RSRevision{
		Name: rs.Name, Namespace: rs.Namespace,
		CreatedAt:       rs.CreationTimestamp.Time,
		DesiredReplicas: desired,
		ReadyReplicas:   rs.Status.ReadyReplicas,
		Replicas:        rs.Status.Replicas,
		Images:          containerImages(rs.Spec.Template.Spec.Containers),
	}
	for _, pod := range podsByOwner[rs.UID] {
		rev.Pods = append(rev.Pods, makePodSummary(pod))
	}
	return rev
}

func containerImages(containers []corev1.Container) []string {
	seen := map[string]bool{}
	var imgs []string
	for _, c := range containers {
		if c.Image != "" && !seen[c.Image] {
			seen[c.Image] = true
			imgs = append(imgs, c.Image)
		}
	}
	return imgs
}

func makePodSummary(pod corev1.Pod) PodSummary {
	healthy := pod.Status.Phase == corev1.PodRunning
	for _, c := range pod.Status.ContainerStatuses {
		if !c.Ready {
			healthy = false
		}
	}
	return PodSummary{Name: pod.Name, Healthy: healthy, Phase: string(pod.Status.Phase)}
}
