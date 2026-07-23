package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/anishetty/kgraph/internal/collector"
	"github.com/anishetty/kgraph/internal/graph"
)

var historyCmd = &cobra.Command{
	Use:   "history <kind>/<name>",
	Short: "Show rollout history for a Deployment or StatefulSet",
	Long: `Lists all ReplicaSets owned by a Deployment (newest first) with replica
counts and container images. For StatefulSets shows the current revision info.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runHistory,
}

func init() {
	rootCmd.AddCommand(historyCmd)
}

var (
	histCurrStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
	histOldStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	histImgStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	histTimeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

func runHistory(cmd *cobra.Command, args []string) error {
	ref := args[0]
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("expected <kind>/<name>, e.g. deployment/frontend")
	}
	kindRaw, name := parts[0], parts[1]
	kind, ok := kindAliases[strings.ToLower(kindRaw)]
	if !ok {
		kind = kindRaw // try as-is
	}

	ctx := context.Background()
	c, err := collector.New(kubeconfig, kubeContext)
	if err != nil {
		return err
	}
	res, err := c.Collect(ctx)
	if err != nil {
		return err
	}
	_ = graph.Build(res)

	// pod ready counts by owner UID
	podReady := map[types.UID]int32{}
	for _, pod := range res.Pods {
		for _, ref2 := range pod.OwnerReferences {
			ready := pod.Status.Phase == corev1.PodRunning
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					ready = false
				}
			}
			if ready {
				podReady[ref2.UID]++
			}
		}
	}

	switch kind {
	case "Deployment":
		return printHistoryDeploy(name, res.Deployments, res.ReplicaSets, podReady)
	case "StatefulSet":
		return printHistorySTS(name, res.StatefulSets)
	default:
		return fmt.Errorf("history supports Deployment and StatefulSet")
	}
}

func printHistoryDeploy(name string, deploys []appsv1.Deployment, rsets []appsv1.ReplicaSet, podReady map[types.UID]int32) error {
	var deploy *appsv1.Deployment
	for i := range deploys {
		if deploys[i].Name == name {
			deploy = &deploys[i]
			break
		}
	}
	if deploy == nil {
		return fmt.Errorf("Deployment %q not found", name)
	}

	fmt.Printf("\n  Rollout history  Deployment/%s  (%s)\n\n", name, deploy.Namespace)

	type rsRow struct {
		rs      appsv1.ReplicaSet
		ready   int32
		current bool
	}
	var rows []rsRow
	for _, rs := range rsets {
		ownedByDeploy := false
		for _, ref := range rs.OwnerReferences {
			if ref.UID == deploy.UID {
				ownedByDeploy = true
			}
		}
		if !ownedByDeploy || rs.Namespace != deploy.Namespace {
			continue
		}
		rows = append(rows, rsRow{rs: rs, ready: podReady[rs.UID]})
	}

	// Sort newest first
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[i].rs.CreationTimestamp.Before(&rows[j].rs.CreationTimestamp) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	// Mark current
	for i := range rows {
		if rows[i].rs.Status.Replicas > 0 {
			rows[i].current = true
			break
		}
	}
	if len(rows) > 0 {
		hasCurrent := false
		for _, r := range rows {
			if r.current {
				hasCurrent = true
			}
		}
		if !hasCurrent {
			rows[0].current = true
		}
	}

	for i, row := range rows {
		marker := "  "
		style := histOldStyle
		if row.current {
			marker = "▶ "
			style = histCurrStyle
		}
		des := int32(0)
		if row.rs.Spec.Replicas != nil {
			des = *row.rs.Spec.Replicas
		}
		fmt.Printf("  %s%s  rev %-3d  %s  desired=%-2d  ready=%-2d\n",
			marker,
			style.Render(row.rs.Name),
			len(rows)-i,
			histTimeStyle.Render(row.rs.CreationTimestamp.Format("2006-01-02 15:04")),
			des, row.ready,
		)
		for _, c := range row.rs.Spec.Template.Spec.Containers {
			fmt.Printf("     └─ %s\n", histImgStyle.Render(c.Image))
		}
	}
	if len(rows) == 0 {
		fmt.Println("  No ReplicaSets found")
	}
	return nil
}

func printHistorySTS(name string, stsets []appsv1.StatefulSet) error {
	for _, sts := range stsets {
		if sts.Name != name {
			continue
		}
		des := int32(1)
		if sts.Spec.Replicas != nil {
			des = *sts.Spec.Replicas
		}
		fmt.Printf("\n  StatefulSet/%s  (%s)  created %s\n",
			name, sts.Namespace,
			sts.CreationTimestamp.Format("2006-01-02 15:04"),
		)
		fmt.Printf("  Desired: %d  Ready: %d  Updated: %d  CurrentRevision: %s\n",
			des, sts.Status.ReadyReplicas,
			sts.Status.UpdatedReplicas, sts.Status.CurrentRevision,
		)
		for _, c := range sts.Spec.Template.Spec.Containers {
			fmt.Printf("  └─ %s\n", histImgStyle.Render(c.Image))
		}
		return nil
	}
	return fmt.Errorf("StatefulSet %q not found", name)
}
