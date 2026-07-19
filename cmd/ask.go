package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anishetty/kgraph/internal/query"
	"github.com/anishetty/kgraph/internal/renderer"
)

// askCmd answers a plain-English question about the cluster by mapping it,
// deterministically, to a graph traversal.
var askCmd = &cobra.Command{
	Use:   "ask <question>",
	Short: "Ask a plain-English question (e.g. \"what depends on service/frontend\")",
	Long: `Ask a question in a natural sentence. KGraph maps it to a deterministic
graph traversal — no AI involved. Examples:

  kgraph ask "what does deployment/frontend depend on"
  kgraph ask "what depends on service/frontend"
  kgraph ask "what breaks if I change pod/frontend-abc"
  kgraph ask "why is deployment/frontend unhealthy"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		question := strings.Join(args, " ")
		in, err := interpret(question)
		if err != nil {
			return err
		}

		g, err := buildGraph(cmd.Context())
		if err != nil {
			return err
		}
		node, err := resolveNode(g, in.Ref)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		color := colorEnabled(out)

		switch in.Verb {
		case "deps":
			fmt.Fprintf(out, "What %s depends on:\n\n", describe(node))
			tree := query.DependencyTree(g, node)
			renderer.Tree(out, tree, renderer.Options{Color: color})
			if len(tree.Children) == 0 {
				fmt.Fprintln(out, "  (nothing to depend on)")
			}
		case "impact":
			fmt.Fprintf(out, "What depends on %s:\n\n", describe(node))
			tree := query.ImpactTree(g, node)
			renderer.Tree(out, tree, renderer.Options{Color: color})
			if len(tree.Children) == 0 {
				fmt.Fprintln(out, "  (nothing depends on this)")
			}
		case "why":
			fmt.Fprintf(out, "Health of %s and its dependencies:\n\n", describe(node))
			tree := query.DependencyTree(g, node)
			renderer.Tree(out, tree, renderer.Options{Color: color, Annotate: healthBadge(color)})
			unhealthy := collectUnhealthy(tree)
			fmt.Fprintln(out)
			if len(unhealthy) == 0 {
				fmt.Fprintln(out, "No unhealthy pods found in the dependency tree.")
			} else {
				fmt.Fprintln(out, "Likely root cause(s):")
				for _, u := range unhealthy {
					fmt.Fprintf(out, "  - %s\n", u)
				}
			}
		}
		return nil
	},
}
