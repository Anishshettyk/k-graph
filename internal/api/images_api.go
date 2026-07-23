package api

// images_api.go — /api/images handler.
//
// Enumerates every container image running in the cluster, groups by image,
// flags risk signals (latest tag, no digest, unknown registry), and lists
// which workloads use each image.

import (
	"net/http"
	"sort"
	"strings"
)

// ─── types ──────────────────────────────────────────────────────────────────

type ImageRisk string

const (
	RiskLatestTag   ImageRisk = "latest-tag"   // image:latest — unpinned
	RiskNoTag       ImageRisk = "no-tag"        // image with no tag at all
	RiskUnknownReg  ImageRisk = "unknown-reg"   // not a known public/private registry
)

type ImageWorkload struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type ImageInfo struct {
	Image       string          `json:"image"`
	Registry    string          `json:"registry"`
	Repository  string          `json:"repository"`
	Tag         string          `json:"tag"`
	Risks       []ImageRisk     `json:"risks"`
	Workloads   []ImageWorkload `json:"workloads"`
	PodCount    int             `json:"podCount"`
}

type ImagesResponse struct {
	Context   string      `json:"context"`
	Namespace string      `json:"namespace"`
	Images    []ImageInfo `json:"images"`
	// summary
	Total       int `json:"total"`
	WithRisks   int `json:"withRisks"`
	LatestTags  int `json:"latestTags"`
}

// Known registries — anything else is "unknown"
var knownRegistries = []string{
	"docker.io", "registry.k8s.io", "gcr.io", "ghcr.io",
	"quay.io", "mcr.microsoft.com", "public.ecr.aws",
	"registry.hub.docker.com",
}

func parseImage(raw string) (registry, repository, tag string) {
	// strip digest
	if i := strings.Index(raw, "@"); i >= 0 {
		raw = raw[:i]
	}
	// tag
	lastSlash := strings.LastIndex(raw, "/")
	lastColon := strings.LastIndex(raw, ":")
	if lastColon > lastSlash {
		tag = raw[lastColon+1:]
		raw = raw[:lastColon]
	}
	// registry vs repository
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		registry = parts[0]
		repository = parts[1]
	} else {
		registry = "docker.io"
		repository = raw
	}
	return
}

func imageRisks(registry, tag string) []ImageRisk {
	risks := []ImageRisk{}
	if tag == "" {
		risks = append(risks, RiskNoTag)
	} else if tag == "latest" {
		risks = append(risks, RiskLatestTag)
	}
	known := false
	for _, r := range knownRegistries {
		if registry == r || strings.HasSuffix(registry, "."+r) {
			known = true
			break
		}
	}
	if !known && !strings.Contains(registry, ".") {
		// bare name like "myimage" — not a registry
	} else if !known {
		risks = append(risks, RiskUnknownReg)
	}
	return risks
}

func (h *Handler) handleImages(w http.ResponseWriter, r *http.Request) {
	_, ctxName, ns, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// image → workloads that use it
	type workloadRef struct {
		kind, namespace, name string
	}
	imageWorkloads := map[string][]workloadRef{}
	imageSeen := map[string]struct{}{}

	addImages := func(kind, namespace, name string, images []string) {
		for _, img := range images {
			if img == "" {
				continue
			}
			if _, ok := imageSeen[img]; !ok {
				imageSeen[img] = struct{}{}
			}
			imageWorkloads[img] = append(imageWorkloads[img], workloadRef{kind, namespace, name})
		}
	}

	// Collect images from pod specs (running pods = ground truth)
	for _, pod := range res.Pods {
		if ns != "" && pod.Namespace != ns {
			continue
		}
		// derive workload name from owner or pod name
		workloadKind, workloadName := "Pod", pod.Name
		for _, ref := range pod.OwnerReferences {
			workloadKind = ref.Kind
			workloadName = ref.Name
			break
		}
		var imgs []string
		for _, c := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
			if c.Image != "" {
				imgs = append(imgs, c.Image)
			}
		}
		addImages(workloadKind, pod.Namespace, workloadName, imgs)
	}

	// Build ImageInfo list
	infos := make([]ImageInfo, 0, len(imageWorkloads))
	for img, refs := range imageWorkloads {
		registry, repository, tag := parseImage(img)
		risks := imageRisks(registry, tag)

		// Deduplicate workloads
		seen := map[string]bool{}
		var workloads []ImageWorkload
		for _, ref := range refs {
			key := ref.kind + "/" + ref.namespace + "/" + ref.name
			if !seen[key] {
				seen[key] = true
				workloads = append(workloads, ImageWorkload{ref.kind, ref.namespace, ref.name})
			}
		}
		sort.Slice(workloads, func(i, j int) bool {
			return workloads[i].Name < workloads[j].Name
		})

		if workloads == nil {
			workloads = []ImageWorkload{}
		}

		infos = append(infos, ImageInfo{
			Image:      img,
			Registry:   registry,
			Repository: repository,
			Tag:        tag,
			Risks:      risks,
			Workloads:  workloads,
			PodCount:   len(refs),
		})
	}

	// Sort: risky first, then by pod count desc, then alphabetical
	sort.Slice(infos, func(i, j int) bool {
		ri, rj := len(infos[i].Risks), len(infos[j].Risks)
		if ri != rj {
			return ri > rj
		}
		if infos[i].PodCount != infos[j].PodCount {
			return infos[i].PodCount > infos[j].PodCount
		}
		return infos[i].Image < infos[j].Image
	})

	withRisks, latestTags := 0, 0
	for _, inf := range infos {
		if len(inf.Risks) > 0 {
			withRisks++
		}
		for _, r := range inf.Risks {
			if r == RiskLatestTag {
				latestTags++
			}
		}
	}

	writeJSON(w, ImagesResponse{
		Context:    ctxName,
		Namespace:  ns,
		Images:     infos,
		Total:      len(infos),
		WithRisks:  withRisks,
		LatestTags: latestTags,
	})
}
