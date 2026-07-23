package cmd

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/anishetty/kgraph/internal/collector"
)

var certsCmd = &cobra.Command{
	Use:   "certs",
	Short: "Scan TLS certificate expiry for all kubernetes.io/tls Secrets",
	Long: `Decodes every Secret of type kubernetes.io/tls and reports days until
expiry. Flags certs expiring in ≤7 days as CRITICAL, ≤30 days as WARNING,
≤90 days as WATCH.`,
	RunE: runCerts,
}

func init() {
	certsCmd.Flags().StringVarP(&certsNamespace, "namespace", "n", "", "Namespace to scope (default: all)")
	rootCmd.AddCommand(certsCmd)
}

var certsNamespace string

var (
	certCritStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	certWarnStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	certWatchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	certOKStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	certLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

func runCerts(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	c, err := collector.New(kubeconfig, kubeContext)
	if err != nil {
		return err
	}
	res, err := c.Collect(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	found := 0

	for _, secret := range res.Secrets {
		if certsNamespace != "" && secret.Namespace != certsNamespace {
			continue
		}
		if secret.Type != "kubernetes.io/tls" {
			continue
		}
		certPEM := secret.Data["tls.crt"]
		if len(certPEM) == 0 {
			fmt.Printf("  %s  %s/%s  — tls.crt missing\n",
				certCritStyle.Render("CRITICAL"),
				secret.Namespace, secret.Name)
			found++
			continue
		}
		block, _ := pem.Decode(certPEM)
		if block == nil {
			fmt.Printf("  %s  %s/%s  — failed to decode PEM\n",
				certCritStyle.Render("CRITICAL"),
				secret.Namespace, secret.Name)
			found++
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			fmt.Printf("  %s  %s/%s  — parse error: %v\n",
				certCritStyle.Render("CRITICAL"),
				secret.Namespace, secret.Name, err)
			found++
			continue
		}

		daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)
		expired := now.After(cert.NotAfter)
		found++

		var sevLabel string
		switch {
		case expired:
			sevLabel = certCritStyle.Render("EXPIRED ")
		case daysLeft <= 7:
			sevLabel = certCritStyle.Render("CRITICAL")
		case daysLeft <= 30:
			sevLabel = certWarnStyle.Render("WARNING ")
		case daysLeft <= 90:
			sevLabel = certWatchStyle.Render("WATCH   ")
		default:
			sevLabel = certOKStyle.Render("OK      ")
		}

		daysStr := fmt.Sprintf("%4d days", daysLeft)
		if expired {
			daysStr = fmt.Sprintf("%4d days ago", -daysLeft)
		}

		fmt.Printf("  %s  %-40s  %s  expires %s  CN=%s\n",
			sevLabel,
			certLabelStyle.Render(secret.Namespace+"/"+secret.Name),
			daysStr,
			cert.NotAfter.Format("2006-01-02"),
			cert.Subject.CommonName,
		)
	}

	if found == 0 {
		fmt.Println("  ✓  No kubernetes.io/tls Secrets found")
	}
	return nil
}
