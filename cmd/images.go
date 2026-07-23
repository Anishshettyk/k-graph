package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/anishetty/kgraph/internal/collector"
)

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "List all container images in the cluster with risk flags",
	Long: `Enumerates every container image across running Pods. Flags ':latest'
tags, untagged images, and shows which workloads use each image.`,
	RunE: runImages,
}

func init() {
	imagesCmd.Flags().StringVarP(&imagesNamespace, "namespace", "n", "", "Namespace to scope")
	rootCmd.AddCommand(imagesCmd)
}

var imagesNamespace string

var (
	imgRiskStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	imgOKStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	imgNameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	imgDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

func runImages(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	c, err := collector.New(kubeconfig, kubeContext)
	if err != nil {
		return err
	}
	res, err := c.Collect(ctx)
	if err != nil {
		return err
	}

	type workloadRef struct{ kind, ns, name string }
	imageMap := map[string][]workloadRef{}

	for _, pod := range res.Pods {
		if imagesNamespace != "" && pod.Namespace != imagesNamespace {
			continue
		}
		owner := "Pod"
		ownerName := pod.Name
		for _, ref := range pod.OwnerReferences {
			owner = ref.Kind
			ownerName = ref.Name
			break
		}
		for _, c := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
			if c.Image == "" {
				continue
			}
			imageMap[c.Image] = append(imageMap[c.Image], workloadRef{owner, pod.Namespace, ownerName})
		}
	}

	total, risky := 0, 0
	for img, refs := range imageMap {
		total++
		tag := extractTag(img)
		isLatest := tag == "latest" || tag == ""

		if isLatest {
			risky++
		}

		var risk string
		if isLatest {
			if tag == "" {
				risk = imgRiskStyle.Render(" ⚠ no tag")
			} else {
				risk = imgRiskStyle.Render(" ⚠ :latest")
			}
		} else {
			risk = imgOKStyle.Render(" ✓")
		}

		// Deduplicate refs
		seen := map[string]bool{}
		var workloads []string
		for _, ref := range refs {
			key := ref.kind + "/" + ref.ns + "/" + ref.name
			if !seen[key] {
				seen[key] = true
				workloads = append(workloads, ref.kind+"/"+ref.name)
			}
		}

		fmt.Printf("  %s %s  %s\n",
			risk,
			imgNameStyle.Render(img),
			imgDimStyle.Render("← "+strings.Join(workloads, ", ")),
		)
	}

	fmt.Printf("\n  %s unique images  %s with risks\n",
		imgDimStyle.Render(fmt.Sprintf("%d", total)),
		imgRiskStyle.Render(fmt.Sprintf("%d", risky)),
	)
	return nil
}

func extractTag(image string) string {
	// strip digest
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		return image[lastColon+1:]
	}
	return ""
}
