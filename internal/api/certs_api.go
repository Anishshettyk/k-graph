package api

// certs_api.go — /api/certs: TLS certificate expiry scanner.
//
// Scans all Secrets of type kubernetes.io/tls, decodes the PEM certificate,
// and returns expiry info. Flags certs expiring within 7, 30, or 90 days.

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
)

type CertSeverity string

const (
	CertCritical CertSeverity = "critical" // ≤7 days or expired
	CertWarning  CertSeverity = "warning"  // ≤30 days
	CertWatch    CertSeverity = "watch"    // ≤90 days
	CertOK       CertSeverity = "ok"
)

type CertInfo struct {
	SecretName  string       `json:"secretName"`
	Namespace   string       `json:"namespace"`
	CommonName  string       `json:"commonName"`
	DNSNames    []string     `json:"dnsNames"`
	Issuer      string       `json:"issuer"`
	NotBefore   time.Time    `json:"notBefore"`
	NotAfter    time.Time    `json:"notAfter"`
	DaysLeft    int          `json:"daysLeft"`
	Severity    CertSeverity `json:"severity"`
	Expired     bool         `json:"expired"`
	ParseError  string       `json:"parseError,omitempty"`
}

type CertsResponse struct {
	Context   string     `json:"context"`
	Namespace string     `json:"namespace"`
	Certs     []CertInfo `json:"certs"`
	Total     int        `json:"total"`
	Expired   int        `json:"expired"`
	Critical  int        `json:"critical"`
	Warning   int        `json:"warning"`
}

func (h *Handler) handleCerts(w http.ResponseWriter, r *http.Request) {
	_, ctxName, ns, res, err := h.collect(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now()
	var certs []CertInfo
	if certs == nil {
		certs = []CertInfo{}
	}

	for _, secret := range res.Secrets {
		if ns != "" && secret.Namespace != ns {
			continue
		}
		if secret.Type != corev1.SecretTypeTLS {
			continue
		}
		certPEM := secret.Data["tls.crt"]
		if len(certPEM) == 0 {
			certs = append(certs, CertInfo{
				SecretName: secret.Name,
				Namespace:  secret.Namespace,
				ParseError: "tls.crt key missing or empty",
				Severity:   CertCritical,
			})
			continue
		}

		block, _ := pem.Decode(certPEM)
		if block == nil {
			certs = append(certs, CertInfo{
				SecretName: secret.Name,
				Namespace:  secret.Namespace,
				ParseError: "failed to decode PEM block",
				Severity:   CertCritical,
			})
			continue
		}

		cert, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr != nil {
			certs = append(certs, CertInfo{
				SecretName: secret.Name,
				Namespace:  secret.Namespace,
				ParseError: parseErr.Error(),
				Severity:   CertCritical,
			})
			continue
		}

		daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)
		expired := now.After(cert.NotAfter)
		sev := certSeverity(daysLeft, expired)

		certs = append(certs, CertInfo{
			SecretName: secret.Name,
			Namespace:  secret.Namespace,
			CommonName: cert.Subject.CommonName,
			DNSNames:   func() []string { if cert.DNSNames == nil { return []string{} }; return cert.DNSNames }(),
			Issuer:     cert.Issuer.CommonName,
			NotBefore:  cert.NotBefore,
			NotAfter:   cert.NotAfter,
			DaysLeft:   daysLeft,
			Severity:   sev,
			Expired:    expired,
		})
	}

	// Sort: expired first, then by days left asc, then by name
	sort.Slice(certs, func(i, j int) bool {
		si, sj := certSevRank(certs[i].Severity), certSevRank(certs[j].Severity)
		if si != sj {
			return si > sj
		}
		return certs[i].DaysLeft < certs[j].DaysLeft
	})

	expired, critical, warning := 0, 0, 0
	for _, c := range certs {
		if c.Expired {
			expired++
		}
		switch c.Severity {
		case CertCritical:
			critical++
		case CertWarning:
			warning++
		}
	}

	writeJSON(w, CertsResponse{
		Context:   ctxName,
		Namespace: ns,
		Certs:     certs,
		Total:     len(certs),
		Expired:   expired,
		Critical:  critical,
		Warning:   warning,
	})
}

func certSeverity(daysLeft int, expired bool) CertSeverity {
	if expired || daysLeft <= 7 {
		return CertCritical
	}
	if daysLeft <= 30 {
		return CertWarning
	}
	if daysLeft <= 90 {
		return CertWatch
	}
	return CertOK
}

func certSevRank(s CertSeverity) int {
	switch s {
	case CertCritical:
		return 3
	case CertWarning:
		return 2
	case CertWatch:
		return 1
	default:
		return 0
	}
}
